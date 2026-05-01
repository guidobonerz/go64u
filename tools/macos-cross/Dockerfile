# Cross-compile go64u from Linux/Windows-via-Docker to macOS using osxcross.
#
# Image build needs an Apple SDK tarball at sdk/MacOSX*.sdk.tar.xz. See
# README.md for how to extract one from a Mac you have access to.

# Ubuntu 24.04 ships clang 18, which can parse the libc++ headers from
# MacOSX15.x SDKs (Debian bookworm's clang 14 cannot — it chokes on
# _LIBCPP_EXPORTED_FROM_ABI attributes in <new>).
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# osxcross + cross-compile toolchain dependencies, FFmpeg/x264 build deps,
# plus Go's CGO needs (clang as the host compiler used by osxcross's bootstrap).
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential clang lld llvm libclang-dev \
        cmake make patch nasm pkg-config bc \
        git curl wget ca-certificates xz-utils cpio python3 \
        libssl-dev libxml2-dev libbz2-dev zlib1g-dev liblzma-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Go matching the project's go.mod toolchain. Update GO_VERSION as needed.
ARG GO_VERSION=1.25.5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xz

ENV PATH=/usr/local/go/bin:/opt/osxcross/target/bin:$PATH \
    CGO_ENABLED=1 \
    GOOS=darwin \
    GOPATH=/go

# Clone osxcross. The SDK tarball is staged into tarballs/ from the build
# context (./sdk/) and consumed by ./build.sh.
RUN git clone --depth 1 https://github.com/tpoechtrager/osxcross /opt/osxcross
COPY sdk/MacOSX*.sdk.tar.xz /opt/osxcross/tarballs/

# When more than one SDK is staged, osxcross requires SDK_VERSION to pick one.
ARG SDK_VERSION
ARG OSX_VERSION_MIN=11.0
RUN cd /opt/osxcross && UNATTENDED=1 \
    SDK_VERSION="${SDK_VERSION}" \
    OSX_VERSION_MIN="${OSX_VERSION_MIN}" \
    ./build.sh

# Build compiler-rt builtins so clang can resolve Darwin runtime helpers like
# __isPlatformVersionAtLeast (emitted for @available checks in FFmpeg's
# hwcontext_videotoolbox). Without this, the final link fails with
# "Undefined symbols ... ___isPlatformVersionAtLeast".
RUN cd /opt/osxcross && UNATTENDED=1 \
    SDK_VERSION="${SDK_VERSION}" \
    ./build_compiler_rt.sh

# Container entrypoint — the per-arch build orchestrator.
COPY build-in-container.sh /usr/local/bin/build-go64u
RUN chmod +x /usr/local/bin/build-go64u

WORKDIR /work
ENTRYPOINT ["/usr/local/bin/build-go64u"]
