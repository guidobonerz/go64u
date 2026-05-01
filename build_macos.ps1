# Cross-compile macOS binaries from Windows via Docker + osxcross.
#
# Static-only — see tools/macos-cross/README.md for the rationale and
# one-time SDK setup.

param(
    [ValidateSet("arm64", "amd64", "both")]
    [string]$Arch = "arm64",
    [switch]$static = $true,
    [switch]$Rebuild,
    [string]$Image = "go64u-macos-cross:latest"
)

$ErrorActionPreference = "Stop"

function Write-Info($msg)  { Write-Host $msg -ForegroundColor Cyan }
function Write-Ok($msg)    { Write-Host $msg -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host $msg -ForegroundColor Yellow }
function Write-Skip($msg)  { Write-Host $msg -ForegroundColor DarkGray }
function Write-Err($msg)   { Write-Host $msg -ForegroundColor Red }

function Get-FileSizeMb($path) {
    if (-not (Test-Path $path)) { return "" }
    return "{0:N1} MB" -f ((Get-Item $path).Length / 1MB)
}

# --- Pre-flight: dynamic mode is unsupported via container ---
if (-not $static) {
    Write-Err "Dynamic cross-compile via container is not supported."
    Write-Host "  Use 'build.ps1 -Targets macos' for the SSH-to-Mac dynamic path." -ForegroundColor DarkGray
    exit 1
}

$moduleName = (Select-String -Path "go.mod" -Pattern "^module\s+(.+)$").Matches.Groups[1].Value.Split("/")[-1]

# --- Pre-flight: docker reachable ---
Write-Info "=== macOS cross-build (Docker + osxcross) ==="
try {
    & docker version --format "{{.Server.Version}}" 2>&1 | Out-Null
} catch {
    Write-Err "docker not available — install Docker Desktop (WSL2 backend) and ensure 'docker version' works."
    exit 1
}
if ($LASTEXITCODE -ne 0) {
    Write-Err "docker daemon not responding — start Docker Desktop and try again."
    exit 1
}

# --- Pre-flight: SDK tarball present ---
$contextDir = Join-Path $PSScriptRoot "tools\macos-cross"
$sdkDir = Join-Path $contextDir "sdk"
$sdkFiles = @()
if (Test-Path $sdkDir) {
    $sdkFiles = @(Get-ChildItem $sdkDir -Filter "MacOSX*.sdk.tar.*" -File -ErrorAction SilentlyContinue)
}
if ($sdkFiles.Count -eq 0) {
    Write-Err "No Apple SDK tarball found under $sdkDir."
    Write-Host "  See tools/macos-cross/README.md for how to extract MacOSX*.sdk.tar.xz from a Mac." -ForegroundColor DarkGray
    exit 1
}
Write-Skip ("SDK: " + ($sdkFiles | ForEach-Object Name) -join ", ")

# --- Image presence / build ---
$imageExists = $false
& docker image inspect $Image *>$null
if ($LASTEXITCODE -eq 0) { $imageExists = $true }

if (-not $imageExists -or $Rebuild) {
    if ($Rebuild) {
        Write-Info "--- Rebuilding Docker image $Image (force) ---"
    } else {
        Write-Info "--- Building Docker image $Image (first run, 15-25 min) ---"
    }
    & docker build -t $Image $contextDir
    if ($LASTEXITCODE -ne 0) {
        Write-Err "docker build failed."
        exit 1
    }
    Write-Ok "Image $Image ready"
} else {
    Write-Skip "Image $Image already present, skipping build (use -Rebuild to force)"
}

# --- Run the cross-build ---
$archArg = $Arch
if ($Arch -eq "both") { $archArg = "arm64,amd64" }

# Docker on Windows accepts forward-slash paths; convert backslashes for safety.
$projectMount = ($PSScriptRoot -replace '\\','/') + ":/work"
# Mount the container script at runtime so edits take effect without an
# image rebuild. The image still has a baked-in copy from the Dockerfile;
# the bind mount overrides it.
$scriptHost   = (Join-Path $PSScriptRoot "tools\macos-cross\build-in-container.sh") -replace '\\','/'
$scriptMount  = "${scriptHost}:/usr/local/bin/build-go64u:ro"

Write-Info "--- Building $moduleName for arch=$archArg (static) ---"
& docker run --rm `
    -v $projectMount `
    -v $scriptMount `
    $Image `
    --arch $archArg --static --module-name $moduleName

if ($LASTEXITCODE -ne 0) {
    Write-Err "Container build failed."
    exit 1
}

# --- Summary ---
$archs = if ($Arch -eq "both") { @("arm64","amd64") } else { @($Arch) }
$results = @()
foreach ($a in $archs) {
    $out = "$moduleName-darwin-$a"
    $path = Join-Path $PSScriptRoot $out
    if (Test-Path $path) {
        $results += [pscustomobject]@{
            Arch   = $a
            Mode   = "static"
            Status = "OK"
            Output = $out
            Size   = (Get-FileSizeMb $path)
        }
    } else {
        $results += [pscustomobject]@{
            Arch   = $a
            Mode   = "static"
            Status = "MISSING"
            Output = $out
            Size   = ""
        }
    }
}

Write-Host ""
Write-Info "=== Summary ==="
$results | Format-Table -AutoSize Arch, Mode, Status, Output, Size | Out-String | Write-Host

if ($results | Where-Object { $_.Status -ne "OK" }) {
    exit 1
}

Write-Host "Inspect a binary with:" -ForegroundColor DarkGray
Write-Host "  docker run --rm -v `"$($PSScriptRoot -replace '\\','/'):/work`" $Image --inspect /work/$moduleName-darwin-arm64" -ForegroundColor DarkGray
