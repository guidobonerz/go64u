#!/usr/bin/env bash
# Per-arch macOS build orchestrator running inside the osxcross image.
#
# Invoked by build_macos.ps1 via `docker run`. Builds x264 + FFmpeg statically
# for each requested arch, then statically links them into the Go binary.
# libSystem and Apple frameworks remain dynamic (mandatory on macOS).

set -euo pipefail

ARCHS=""
STATIC=0
MODULE_NAME="go64u"
INSPECT_PATH=""

usage() {
    cat <<USAGE
Usage: build-go64u --arch <arm64|amd64|both|arm64,amd64> [--static] [--module-name <name>]
       build-go64u --inspect <path-to-binary>
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --arch)         ARCHS="$2"; shift 2 ;;
        --static)       STATIC=1; shift ;;
        --module-name)  MODULE_NAME="$2"; shift 2 ;;
        --inspect)      INSPECT_PATH="$2"; shift 2 ;;
        -h|--help)      usage; exit 0 ;;
        *)              echo "Unknown arg: $1" >&2; usage; exit 1 ;;
    esac
done

# Inspect mode: report Mach-O details for an existing binary using osxcross
# cctools, then exit. Useful for verification from the Windows orchestrator.
if [[ -n "$INSPECT_PATH" ]]; then
    if [[ ! -f "$INSPECT_PATH" ]]; then
        echo "inspect: file not found: $INSPECT_PATH" >&2
        exit 1
    fi
    # Pick whichever otool/lipo wrapper matches; both archs of cctools are
    # installed by osxcross and behave identically for read-only inspection.
    OTOOL="$(command -v x86_64-apple-darwin-otool 2>/dev/null \
             || command -v arm64-apple-darwin-otool 2>/dev/null \
             || command -v otool 2>/dev/null)"
    LIPO="$(command -v x86_64-apple-darwin-lipo 2>/dev/null \
            || command -v arm64-apple-darwin-lipo 2>/dev/null \
            || command -v lipo 2>/dev/null)"
    echo "=== file ==="
    file "$INSPECT_PATH"
    if [[ -n "$LIPO" ]]; then
        echo "=== lipo -info ==="
        "$LIPO" -info "$INSPECT_PATH" || true
    fi
    if [[ -n "$OTOOL" ]]; then
        echo "=== otool -L ==="
        "$OTOOL" -L "$INSPECT_PATH" || true
    fi
    exit 0
fi

if [[ -z "$ARCHS" ]]; then
    usage; exit 1
fi
if [[ "$ARCHS" == "both" ]]; then
    ARCHS="arm64,amd64"
fi

cd /work

# Resolve osxcross triplet for a given GOARCH. osxcross typically generates
# triplets like x86_64-apple-darwin23 / arm64-apple-darwin23. We discover the
# actual triplet at runtime so we don't pin a darwin version.
discover_triplet() {
    local clang_arch="$1"
    local cc
    cc=$(ls /opt/osxcross/target/bin/${clang_arch}-apple-darwin*-clang 2>/dev/null | head -1)
    if [[ -z "$cc" ]]; then
        echo "ERROR: no osxcross clang found for ${clang_arch}" >&2
        exit 1
    fi
    basename "$cc" | sed 's/-clang$//'
}

build_arch() {
    local goarch="$1"
    local clang_arch ff_arch triplet
    case "$goarch" in
        arm64) clang_arch="arm64";  ff_arch="aarch64" ;;
        amd64) clang_arch="x86_64"; ff_arch="x86_64"  ;;
        *) echo "Unsupported GOARCH: $goarch" >&2; return 1 ;;
    esac
    triplet="$(discover_triplet "$clang_arch")"

    local static_dir="/work/ffmpeg_static_${goarch}"
    local prefix="${static_dir}/prefix"
    mkdir -p "$static_dir" "$prefix"

    echo "==> [$goarch] Using triplet: $triplet"

    # ---- x264 ----
    if [[ ! -f "$prefix/lib/libx264.a" ]]; then
        echo "==> [$goarch] Building x264 (static)"
        if [[ ! -d "$static_dir/x264" ]]; then
            git clone --depth 1 https://code.videolan.org/videolan/x264.git \
                "$static_dir/x264"
        fi
        (
            cd "$static_dir/x264"
            make distclean >/dev/null 2>&1 || true
            # x264's configure defaults to ${cross-prefix}gcc, which osxcross
            # doesn't provide — set CC explicitly to the clang wrapper.
            CC="${triplet}-clang" \
            CXX="${triplet}-clang++" \
            AR="${triplet}-ar" \
            RANLIB="${triplet}-ranlib" \
            STRIP="${triplet}-strip" \
            ./configure --prefix="$prefix" \
                --enable-static --disable-cli --disable-opencl \
                --host="${clang_arch}-apple-darwin" \
                --extra-cflags="-arch ${clang_arch}" \
                --extra-ldflags="-arch ${clang_arch}"
            # NB: no --extra-asflags. NASM (used for x86_64) doesn't accept
            # clang's -arch flag; x264's configure auto-picks the right -f
            # output format (macho64 for darwin x86_64) from --host.
            make -j"$(nproc)"
            make install
        )
        if [[ ! -f "$prefix/lib/libx264.a" ]]; then
            echo "ERROR: x264 build failed for $goarch" >&2
            return 1
        fi
    else
        echo "==> [$goarch] x264 already built, skipping"
    fi

    # ---- FFmpeg ----
    if [[ ! -f "$prefix/lib/libavcodec.a" ]] || [[ ! -f "$prefix/lib/pkgconfig/libavcodec.pc" ]]; then
        echo "==> [$goarch] Building FFmpeg (static, minimal)"
        if [[ ! -d "$static_dir/ffmpeg" ]]; then
            git clone --depth 1 --branch master \
                https://github.com/FFmpeg/FFmpeg.git "$static_dir/ffmpeg"
        fi
        (
            cd "$static_dir/ffmpeg"
            make distclean >/dev/null 2>&1 || true
            export PKG_CONFIG_PATH="$prefix/lib/pkgconfig"
            ./configure --prefix="$prefix" \
                --enable-gpl --enable-libx264 \
                --enable-videotoolbox \
                --enable-static --disable-shared \
                --disable-programs --disable-doc \
                --enable-protocol=file,pipe,tcp,rtmp \
                --enable-encoder=libx264,h264_videotoolbox,aac \
                --enable-decoder=rawvideo,pcm_s16le \
                --enable-muxer=flv,mp4,matroska \
                --enable-demuxer=rawvideo \
                --enable-swscale --enable-swresample \
                --extra-cflags="-I$prefix/include -arch ${clang_arch}" \
                --extra-ldflags="-L$prefix/lib -arch ${clang_arch}" \
                --arch="${ff_arch}" --target-os=darwin \
                --enable-cross-compile \
                --cc="${triplet}-clang" \
                --cxx="${triplet}-clang++" \
                --ar="${triplet}-ar" \
                --ranlib="${triplet}-ranlib" \
                --strip="${triplet}-strip" \
                --nm="${triplet}-nm"
            make -j"$(nproc)"
            make install
        )
        if [[ ! -f "$prefix/lib/libavcodec.a" ]]; then
            echo "ERROR: FFmpeg build failed for $goarch" >&2
            return 1
        fi
    else
        echo "==> [$goarch] FFmpeg already built, skipping"
    fi

    # ---- Go binary ----
    local out="${MODULE_NAME}-darwin-${goarch}"
    echo "==> [$goarch] Building $out"

    export PKG_CONFIG_PATH="$prefix/lib/pkgconfig"
    local pkg_flags
    pkg_flags=$(pkg-config --static --libs \
        libavcodec libavformat libavutil libswscale libswresample)
    local frameworks="-framework VideoToolbox -framework CoreMedia \
-framework CoreVideo -framework CoreFoundation -framework AudioToolbox \
-framework Security -framework AVFoundation -framework CoreServices"

    export CC="${triplet}-clang"
    export CXX="${triplet}-clang++"
    export CGO_CFLAGS="-arch ${clang_arch}"
    # -rtlib=compiler-rt makes clang link in libclang_rt.osx.a, which provides
    # Darwin runtime helpers like __isPlatformVersionAtLeast (referenced by
    # FFmpeg's hwcontext_videotoolbox for @available checks).
    export CGO_LDFLAGS="$pkg_flags $frameworks -arch ${clang_arch} -rtlib=compiler-rt"
    export GOOS=darwin
    export GOARCH="${goarch}"
    export CGO_ENABLED=1

    go build -trimpath -ldflags '-w -s' -o "$out" main.go

    if [[ ! -f "$out" ]]; then
        echo "ERROR: go build did not produce $out" >&2
        return 1
    fi

    # Quick static-link verification
    local otool="${triplet}-otool"
    if command -v "$otool" >/dev/null 2>&1; then
        if "$otool" -L "$out" | grep -E 'libav|libsw|libx264' >/dev/null; then
            echo "WARNING: [$goarch] $out still references FFmpeg/x264 dylibs:"
            "$otool" -L "$out" | grep -E 'libav|libsw|libx264' || true
        else
            echo "==> [$goarch] $out is statically linked against FFmpeg/x264"
        fi
    fi

    local size
    size=$(stat -c%s "$out")
    printf "==> [%s] Built %s (%.1f MB)\n" "$goarch" "$out" "$(echo "$size/1048576" | bc -l)"
}

IFS=',' read -ra ARCH_ARRAY <<< "$ARCHS"
for arch in "${ARCH_ARRAY[@]}"; do
    arch="${arch// /}"
    if [[ "$STATIC" -ne 1 ]]; then
        echo "ERROR: dynamic cross-compile from container is not supported." >&2
        echo "       Use build.ps1 -Targets macos for the SSH-to-Mac dynamic path." >&2
        exit 1
    fi
    build_arch "$arch"
done

echo "=== All requested archs built successfully ==="
