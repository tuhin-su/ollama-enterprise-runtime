# install.ps1 — Windows installer for tuhin-su/loom-master
# Supports: Windows amd64, arm64
#
# Usage (run in PowerShell as Administrator):
#   irm https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.ps1 | iex
#   $env:VERSION="v1.2.3"; irm ... | iex    # pin a specific version

param(
    [string]$Version = "",
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\Loom"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "Continue"

$REPO     = "tuhin-su/loom-master"
$BINARY   = "loom.exe"
$API_BASE = "https://api.github.com/repos/$REPO"
$GH_BASE  = "https://github.com/$REPO/releases/download"

# ─── Helpers ─────────────────────────────────────────────────────────────────
function Write-Info    { param($msg) Write-Host "[info]  $msg"  -ForegroundColor Cyan }
function Write-Success { param($msg) Write-Host "[done]  $msg"  -ForegroundColor Green }
function Write-Warn    { param($msg) Write-Host "[warn]  $msg"  -ForegroundColor Yellow }
function Write-Err     { param($msg) Write-Host "[error] $msg"  -ForegroundColor Red; exit 1 }

function Get-Arch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64"   { return "amd64" }
        "Arm64" { return "arm64" }
        default { Write-Err "Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    try {
        $release = Invoke-RestMethod -Uri "$API_BASE/releases/latest" -UseBasicParsing
        return $release.tag_name.TrimStart('v')
    } catch {
        Write-Err "Failed to fetch latest version: $_"
    }
}

function Download-File {
    param($Url, $Dest)
    Write-Info "Downloading: $Url"
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
    } catch {
        Write-Err "Download failed: $_"
    }
}

function Verify-Checksum {
    param($File, $ChecksumUrl)
    $checksumFile = "$File.sha256"
    try {
        Invoke-WebRequest -Uri $ChecksumUrl -OutFile $checksumFile -UseBasicParsing -ErrorAction Stop
    } catch {
        Write-Warn "No checksum file found, skipping verification."
        return
    }

    Write-Info "Verifying checksum..."
    $expected = (Get-Content $checksumFile -Raw).Trim().Split()[0].ToLower()
    $actual   = (Get-FileHash -Algorithm SHA256 $File).Hash.ToLower()
    if ($expected -ne $actual) {
        Write-Err "Checksum mismatch!`n  Expected: $expected`n  Got:      $actual"
    }
    Write-Info "Checksum OK."
}

function Add-ToUserPath {
    param($Dir)
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$Dir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$Dir", "User")
        $env:Path += ";$Dir"
        Write-Info "Added $Dir to user PATH."
    }
}

function Install-LoomService {
    param($ExePath)
    # Only register Windows service if running as admin
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
                [Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Write-Warn "Not running as Administrator — skipping Windows service registration."
        return
    }

    $svcName = "LoomService"
    $existing = Get-Service -Name $svcName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Info "Stopping existing service..."
        Stop-Service -Name $svcName -Force -ErrorAction SilentlyContinue
        sc.exe delete $svcName | Out-Null
        Start-Sleep -Seconds 2
    }

    Write-Info "Registering Windows service: $svcName"
    sc.exe create $svcName binPath= "`"$ExePath`" serve" start= auto DisplayName= "Loom AI Runtime" | Out-Null
    sc.exe description $svcName "Native high-performance Loom AI server with long-term memory" | Out-Null
    sc.exe start $svcName | Out-Null
    Write-Info "Service started. Use 'sc.exe stop LoomService' to stop."
}

# ─── Banner ───────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ██████╗ ██╗     ██╗      █████╗ ███╗   ███╗ █████╗ " -ForegroundColor Blue
Write-Host " ██╔═══██╗██║     ██║     ██╔══██╗████╗ ████║██╔══██╗" -ForegroundColor Blue
Write-Host " ██║   ██║██║     ██║     ███████║██╔████╔██║███████║" -ForegroundColor Blue
Write-Host " ██║   ██║██║     ██║     ██╔══██║██║╚██╔╝██║██╔══██║" -ForegroundColor Blue
Write-Host " ╚██████╔╝███████╗███████╗██║  ██║██║ ╚═╝ ██║██║  ██║" -ForegroundColor Blue
Write-Host "  ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝╚═╝  ╚═╝" -ForegroundColor Blue
Write-Host ""
Write-Host "  github.com/tuhin-su/loom-master" -ForegroundColor Gray
Write-Host ""

# ─── Main ─────────────────────────────────────────────────────────────────────
$arch    = Get-Arch
$version = if ($Version -ne "") { $Version.TrimStart('v') } else { Get-LatestVersion }

Write-Info "Architecture: $arch"
Write-Info "Version:      v$version"
Write-Info "Install dir:  $InstallDir"

$assetName    = "loom-windows-$arch.zip"
$assetUrl     = "$GH_BASE/v$version/$assetName"
$checksumUrl  = "$GH_BASE/v$version/$assetName.sha256"

$tmpDir = Join-Path $env:TEMP "loom-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

try {
    $archivePath = Join-Path $tmpDir $assetName
    Download-File $assetUrl $archivePath
    Verify-Checksum $archivePath $checksumUrl

    Write-Info "Extracting archive..."
    Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

    # Find binary
    $binSrc = Get-ChildItem -Path $tmpDir -Filter $BINARY -Recurse | Select-Object -First 1
    if (-not $binSrc) { Write-Err "Binary '$BINARY' not found in archive." }

    # Install
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $destBin = Join-Path $InstallDir $BINARY
    Copy-Item -Path $binSrc.FullName -Destination $destBin -Force
    Write-Info "Installed to: $destBin"

    Add-ToUserPath $InstallDir
    Install-LoomService $destBin

    Write-Host ""
    Write-Success "Loom v$version installed successfully!"
    Write-Host ""
    Write-Host "  Quick start:" -ForegroundColor White
    Write-Host "    loom serve          # start the server" -ForegroundColor Gray
    Write-Host "    loom run gemma4     # run a model" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  Docs: https://github.com/tuhin-su/loom-master#readme" -ForegroundColor Gray
    Write-Host ""
} finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
