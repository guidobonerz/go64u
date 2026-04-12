param(
    [switch]$p,
    [switch]$static
)

$ErrorActionPreference = "Stop"
$moduleName = (Select-String -Path "go.mod" -Pattern "^module\s+(.+)$").Matches.Groups[1].Value.Split("/")[-1]

$msys2 = "C:\Users\guido\applications\msys2"
$mingw = "$msys2\mingw64"
$env:PATH = "$mingw\bin;C:\Users\guido\applications\mingw64\bin;$env:PATH"

if ($static) {
    Write-Host "=== Static build with minimal FFmpeg ===" -ForegroundColor Cyan

    $ffmpegBuild = "$PSScriptRoot\ffmpeg_static"
    $ffmpegPrefix = "$ffmpegBuild\prefix"
    $x264Prefix = "$ffmpegBuild\prefix"

    # Check for required tools
    $requiredPacmanPkgs = @()
    if (-not (Test-Path "$msys2\usr\bin\make.exe")) { $requiredPacmanPkgs += "make" }
    if (-not (Test-Path "$msys2\usr\bin\patch.exe")) { $requiredPacmanPkgs += "patch" }
    if (-not (Test-Path "$mingw\bin\nasm.exe")) {
        if (-not (Get-Command nasm -ErrorAction SilentlyContinue)) {
            $requiredPacmanPkgs += "mingw-w64-x86_64-nasm"
        }
    }

    if ($requiredPacmanPkgs.Count -gt 0) {
        Write-Host "Installing missing tools: $($requiredPacmanPkgs -join ', ')" -ForegroundColor Yellow
        & "$msys2\usr\bin\pacman.exe" -S --noconfirm --needed $requiredPacmanPkgs
    }

    # Ensure make is in PATH
    $env:PATH = "$msys2\usr\bin;$env:PATH"

    if (-not (Test-Path $ffmpegBuild)) { New-Item -ItemType Directory -Path $ffmpegBuild | Out-Null }

    # --- Install nv-codec-headers (NVENC headers, no CUDA SDK needed) ---
    $nvHeadersDir = "$ffmpegBuild\nv-codec-headers"
    if (-not (Test-Path "$ffmpegPrefix\include\ffnvcodec\nvEncodeAPI.h")) {
        Write-Host "--- Installing nv-codec-headers ---" -ForegroundColor Green
        if (-not (Test-Path $nvHeadersDir)) {
            git clone --depth 1 https://github.com/FFmpeg/nv-codec-headers.git $nvHeadersDir
        }
        Push-Location $nvHeadersDir
        & "$msys2\usr\bin\bash.exe" -c @"
export PATH='/mingw64/bin:/c/Users/guido/applications/mingw64/bin:/usr/bin:`$PATH'
cd '$($nvHeadersDir -replace '\\','/')'
make PREFIX='$($ffmpegPrefix -replace '\\','/')' install
"@
        Pop-Location
        if (-not (Test-Path "$ffmpegPrefix\include\ffnvcodec\nvEncodeAPI.h")) {
            Write-Error "nv-codec-headers install failed"
            exit 1
        }
        Write-Host "nv-codec-headers installed" -ForegroundColor Green
    } else {
        Write-Host "nv-codec-headers already installed, skipping" -ForegroundColor DarkGray
    }

    # --- Build x264 static ---
    $x264Dir = "$ffmpegBuild\x264"
    if (-not (Test-Path "$x264Prefix\lib\libx264.a")) {
        Write-Host "--- Building x264 (static) ---" -ForegroundColor Green
        if (-not (Test-Path $x264Dir)) {
            git clone --depth 1 https://code.videolan.org/videolan/x264.git $x264Dir
        }
        Push-Location $x264Dir
        & "$msys2\usr\bin\bash.exe" -c @"
export PATH='/mingw64/bin:/c/Users/guido/applications/mingw64/bin:/usr/bin:`$PATH'
cd '$($x264Dir -replace '\\','/')'
./configure --prefix='$($x264Prefix -replace '\\','/')' \
    --enable-static --disable-cli --disable-opencl
make -j$env:NUMBER_OF_PROCESSORS
make install
"@
        Pop-Location
        if (-not (Test-Path "$x264Prefix\lib\libx264.a")) {
            Write-Error "x264 build failed"
            exit 1
        }
        Write-Host "x264 built successfully" -ForegroundColor Green
    } else {
        Write-Host "x264 already built, skipping" -ForegroundColor DarkGray
    }

    # --- Build FFmpeg static (minimal) ---
    $ffmpegDir = "$ffmpegBuild\ffmpeg"
    if (-not (Test-Path "$ffmpegPrefix\lib\libavcodec.a") -or -not (Test-Path "$ffmpegPrefix\lib\pkgconfig\libavcodec.pc")) {
        Write-Host "--- Building FFmpeg (static, minimal) ---" -ForegroundColor Green
        if (-not (Test-Path $ffmpegDir)) {
            git clone --depth 1 --branch master https://github.com/FFmpeg/FFmpeg.git $ffmpegDir
        }
        Push-Location $ffmpegDir
        & "$msys2\usr\bin\bash.exe" -c @"
export PATH='/mingw64/bin:/c/Users/guido/applications/mingw64/bin:/usr/bin:`$PATH'
export PKG_CONFIG_PATH='$($ffmpegPrefix -replace '\\','/')/lib/pkgconfig'
cd '$($ffmpegDir -replace '\\','/')'
./configure --prefix='$($ffmpegPrefix -replace '\\','/')' \
    --enable-gpl --enable-libx264 \
    --enable-nvenc --enable-ffnvcodec \
    --enable-static --disable-shared \
    --disable-programs --disable-doc \
    --enable-protocol=file,pipe,tcp,rtmp \
    --enable-encoder=libx264,h264_nvenc,aac \
    --enable-decoder=rawvideo,pcm_s16le \
    --enable-muxer=flv,mp4,matroska \
    --enable-demuxer=rawvideo \
    --enable-swscale --enable-swresample \
    --extra-cflags='-I$($ffmpegPrefix -replace '\\','/')/include' \
    --extra-ldflags='-L$($ffmpegPrefix -replace '\\','/')/lib' \
    --arch=x86_64 \
    --target-os=mingw64
make -j$env:NUMBER_OF_PROCESSORS
make install
"@
        Pop-Location
        if (-not (Test-Path "$ffmpegPrefix\lib\libavcodec.a")) {
            Write-Error "FFmpeg build failed"
            exit 1
        }
        Write-Host "FFmpeg built successfully" -ForegroundColor Green
    } else {
        Write-Host "FFmpeg already built, skipping" -ForegroundColor DarkGray
    }

    # --- Build go64u with static FFmpeg ---
    Write-Host "--- Building $moduleName (static) ---" -ForegroundColor Green
    $env:CGO_ENABLED = "1"
    $env:PKG_CONFIG_PATH = "$ffmpegPrefix\lib\pkgconfig"

    # Get static link flags from our minimal FFmpeg
    $pkgFlags = & pkg-config --static --libs libavcodec libavformat libavutil libswscale libswresample 2>&1
    # Add Windows system libs needed by FFmpeg
    $ldflags = "$pkgFlags -lws2_32 -lbcrypt -lole32 -lsecur32 -lstrmiids -luuid"

    $env:CGO_LDFLAGS = $ldflags
    go build -trimpath -ldflags "-w -s -extldflags '-static'" -o "$moduleName.exe" main.go

    if ($LASTEXITCODE -eq 0) {
        if ($p) {
            Write-Host "Compressing with UPX..." -ForegroundColor Cyan
            upx --best "$moduleName.exe"
        }
        $size = [math]::Round((Get-Item "$moduleName.exe").Length / 1MB, 1)
        Write-Host "=== Build successful: $moduleName.exe ($size MB, static) ===" -ForegroundColor Green
        # Verify no DLL dependencies from MSYS2
        Write-Host "Checking DLL dependencies..." -ForegroundColor Cyan
        $deps = & ldd "$moduleName.exe" 2>&1 | Select-String "mingw64"
        if ($deps) {
            Write-Host "WARNING: Still has MSYS2 DLL dependencies:" -ForegroundColor Yellow
            $deps | ForEach-Object { Write-Host "  $_" -ForegroundColor Yellow }
        } else {
            Write-Host "No MSYS2 DLL dependencies -- fully static!" -ForegroundColor Green
        }
    } else {
        Write-Error "Build failed"
    }
} else {
    # --- Normal dynamic build ---
    $env:CGO_ENABLED = "1"
    $env:PKG_CONFIG_PATH = "$mingw\lib\pkgconfig"
    go build -trimpath -ldflags "-w -s" -o "$moduleName.exe" main.go

    if ($p) {
        upx --best "$moduleName.exe"
    }
}
