#!/usr/bin/env bash
# macOS counterpart to build.ps1.
#
# Usage:
#   ./build_macos.sh                # normal dynamic build (uses Homebrew FFmpeg via pkg-config)
#   ./build_macos.sh --static       # static build of x264 + FFmpeg, then statically linked into the binary
#   ./build_macos.sh -p             # plus UPX compression
#   ./build_macos.sh --static -p    # static build + UPX
#
# Fully-static binaries are not really a thing on macOS (libSystem is always
# dynamically linked), so "static" here means: x264 + FFmpeg are linked
# statically; system frameworks remain dynamic.

set -euo pipefail

P=0
STATIC=0
for arg in "$@"; do
    case "$arg" in
        -p|--upx) P=1 ;;
        --static) STATIC=1 ;;
        *) echo "Unknown argument: $arg" >&2; exit 1 ;;
    esac
done

# Last segment of the module path in go.mod becomes the binary name.
module_name=$(awk '/^module /{split($2,a,"/"); print a[length(a)]}' go.mod)
script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Homebrew prefix differs between Apple Silicon (/opt/homebrew) and Intel
# (/usr/local). Fall back to whichever exists; macOS users almost always have
# brew, but handle the no-brew case gracefully too.
brew_prefix=""
if command -v brew >/dev/null 2>&1; then
    brew_prefix="$(brew --prefix)"
fi

# Color helpers (best-effort; no-op on dumb terminals).
if [[ -t 1 ]]; then
    C_CYAN=$'\033[36m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
    C_GRAY=$'\033[90m'; C_RED=$'\033[31m';   C_RESET=$'\033[0m'
else
    C_CYAN=""; C_GREEN=""; C_YELLOW=""; C_GRAY=""; C_RED=""; C_RESET=""
fi
log()  { printf "%s%s%s\n" "$C_CYAN"   "$*" "$C_RESET"; }
ok()   { printf "%s%s%s\n" "$C_GREEN"  "$*" "$C_RESET"; }
warn() { printf "%s%s%s\n" "$C_YELLOW" "$*" "$C_RESET"; }
skip() { printf "%s%s%s\n" "$C_GRAY"   "$*" "$C_RESET"; }
err()  { printf "%s%s%s\n" "$C_RED"    "$*" "$C_RESET" >&2; }

if (( STATIC )); then
    log "=== Static build with minimal FFmpeg ==="

    ffmpeg_build="$script_root/ffmpeg_static"
    ffmpeg_prefix="$ffmpeg_build/prefix"
    x264_prefix="$ffmpeg_build/prefix"

    # --- Tooling check ---
    missing_tools=()
    command -v make      >/dev/null 2>&1 || missing_tools+=("make (Xcode Command Line Tools: xcode-select --install)")
    command -v pkg-config>/dev/null 2>&1 || missing_tools+=("pkg-config (brew install pkg-config)")
    command -v nasm      >/dev/null 2>&1 || missing_tools+=("nasm (brew install nasm)")
    command -v git       >/dev/null 2>&1 || missing_tools+=("git (xcode-select --install)")

    if (( ${#missing_tools[@]} > 0 )); then
        err "Missing required tools:"
        for t in "${missing_tools[@]}"; do err "  - $t"; done
        exit 1
    fi

    mkdir -p "$ffmpeg_build"

    cpu_count=$(sysctl -n hw.ncpu 2>/dev/null || echo 4)

    # --- Build x264 (static) ---
    x264_dir="$ffmpeg_build/x264"
    if [[ ! -f "$x264_prefix/lib/libx264.a" ]]; then
        ok "--- Building x264 (static) ---"
        if [[ ! -d "$x264_dir" ]]; then
            git clone --depth 1 https://code.videolan.org/videolan/x264.git "$x264_dir"
        fi
        (
            cd "$x264_dir"
            ./configure \
                --prefix="$x264_prefix" \
                --enable-static --disable-cli --disable-opencl
            make -j"$cpu_count"
            make install
        )
        if [[ ! -f "$x264_prefix/lib/libx264.a" ]]; then
            err "x264 build failed"
            exit 1
        fi
        ok "x264 built successfully"
    else
        skip "x264 already built, skipping"
    fi

    # --- Build FFmpeg (static, minimal, with VideoToolbox H.264) ---
    ffmpeg_dir="$ffmpeg_build/ffmpeg"
    if [[ ! -f "$ffmpeg_prefix/lib/libavcodec.a" ]] || [[ ! -f "$ffmpeg_prefix/lib/pkgconfig/libavcodec.pc" ]]; then
        ok "--- Building FFmpeg (static, minimal) ---"
        if [[ ! -d "$ffmpeg_dir" ]]; then
            git clone --depth 1 --branch master https://github.com/FFmpeg/FFmpeg.git "$ffmpeg_dir"
        fi
        arch=$(uname -m)
        case "$arch" in
            arm64) ffmpeg_arch="aarch64" ;;
            x86_64) ffmpeg_arch="x86_64" ;;
            *) err "Unsupported arch: $arch"; exit 1 ;;
        esac
        (
            cd "$ffmpeg_dir"
            export PKG_CONFIG_PATH="$ffmpeg_prefix/lib/pkgconfig"
            ./configure --prefix="$ffmpeg_prefix" \
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
                --extra-cflags="-I$ffmpeg_prefix/include" \
                --extra-ldflags="-L$ffmpeg_prefix/lib" \
                --arch="$ffmpeg_arch" \
                --target-os=darwin
            make -j"$cpu_count"
            make install
        )
        if [[ ! -f "$ffmpeg_prefix/lib/libavcodec.a" ]]; then
            err "FFmpeg build failed"
            exit 1
        fi
        ok "FFmpeg built successfully"
    else
        skip "FFmpeg already built, skipping"
    fi

    # --- Build the Go binary linked against our static FFmpeg ---
    ok "--- Building $module_name (static) ---"
    export CGO_ENABLED=1
    export PKG_CONFIG_PATH="$ffmpeg_prefix/lib/pkgconfig"

    # Pull static link flags from our private FFmpeg, then append the macOS
    # frameworks h264_videotoolbox / FFmpeg need at link time.
    pkg_flags=$(pkg-config --static --libs libavcodec libavformat libavutil libswscale libswresample)
    frameworks="-framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation -framework AudioToolbox -framework Security -framework AVFoundation -framework CoreServices"
    export CGO_LDFLAGS="$pkg_flags $frameworks"

    # Note: no `-extldflags -static` on macOS — libSystem must remain dynamic.
    go build -trimpath -ldflags "-w -s" -o "$module_name" main.go

    if (( P )); then
        log "Compressing with UPX..."
        upx --best "$module_name"
    fi

    size_mb=$(awk -v b="$(stat -f%z "$module_name")" 'BEGIN{printf "%.1f", b/1048576}')
    ok "=== Build successful: $module_name ($size_mb MB, static FFmpeg) ==="

    # Verify the binary doesn't pull in a Homebrew FFmpeg dylib.
    log "Checking dylib dependencies..."
    deps=$(otool -L "$module_name" | awk 'NR>1 {print $1}' | grep -E 'libav(codec|format|util|filter|device)|libsw(scale|resample)|libx264' || true)
    if [[ -n "$deps" ]]; then
        warn "WARNING: still has FFmpeg/x264 dylib dependencies:"
        printf "  %s\n" $deps | sed "s/^/  /"
    else
        ok "No FFmpeg/x264 dylib dependencies — FFmpeg is statically linked!"
    fi
else
    # --- Normal dynamic build (Homebrew FFmpeg via pkg-config) ---
    export CGO_ENABLED=1
    if [[ -n "$brew_prefix" ]]; then
        export PKG_CONFIG_PATH="$brew_prefix/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
    fi
    go build -trimpath -ldflags "-w -s" -o "$module_name" main.go

    if (( P )); then
        upx --best "$module_name"
    fi
fi
