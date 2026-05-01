# macOS Cross-Compile Container

Builds `go64u-darwin-arm64` and/or `go64u-darwin-amd64` from a Windows or
Linux host using [osxcross](https://github.com/tpoechtrager/osxcross) inside
Docker — no Mac hardware needed at build time.

## One-time setup

### 1. Extract the macOS SDK on a Mac

osxcross needs an Apple SDK tarball. You must extract it yourself from a Mac
you have access to (Apple's license forbids redistribution).

On a Mac with Xcode (or the Command Line Tools) installed:

```bash
git clone https://github.com/tpoechtrager/osxcross
cd osxcross/tools
./gen_sdk_package.sh
```

This produces a file like `MacOSX14.5.sdk.tar.xz` in the current directory.
Pick the SDK that matches the lowest macOS version you want to support — the
resulting binary will run on that version and all newer ones.

### 2. Drop the tarball into this repo

Copy the produced `MacOSX*.sdk.tar.xz` to:

```
tools/macos-cross/sdk/MacOSX14.5.sdk.tar.xz
```

The `sdk/` directory is gitignored so the licensed tarball never gets
committed.

### 3. Verify Docker is available

`docker version` on Windows (Docker Desktop with the WSL2 backend works
fine) or Linux. The first image build takes 15-25 minutes; subsequent
runs reuse the layer cache.

## Building

From the project root:

```powershell
# Default: arm64 (M-family), static
.\build_macos.ps1

# Intel cross-build
.\build_macos.ps1 -Arch amd64

# Both arches in one run
.\build_macos.ps1 -Arch both

# Force a fresh image build (e.g. after updating the SDK)
.\build_macos.ps1 -Rebuild
```

The Go binaries land directly in the project root: `go64u-darwin-arm64`
and/or `go64u-darwin-amd64`.

## Caches

- **Docker image** (`go64u-macos-cross:latest`): osxcross + Go + tooling.
  Rebuilt only on `-Rebuild` or when the SDK changes.
- **x264 / FFmpeg static prefixes**: per-arch under
  `ffmpeg_static_arm64/` and `ffmpeg_static_amd64/` in the project root.
  Persistent on disk; rebuilt only if a `.a` is missing.

## Why static only?

Cross-compiling a *dynamic* macOS binary from outside macOS would require
the actual Homebrew/system `.dylib`s (libavcodec.dylib etc.) at link time.
osxcross only provides Apple system framework stubs, not third-party
dylibs. For a dynamic build use `build.ps1 -Targets macos`, which runs the
build over SSH on a real Mac.
