# Portable distribution packaging script for Flore.
# Called by build-desktop.ps1 after the desktop build completes.

$ErrorActionPreference = 'Stop'

$desktopDir = $PSScriptRoot
$binDir = Join-Path (Join-Path $desktopDir 'build') 'bin'
$goExe = Join-Path $binDir 'flore-backend.exe'
$floreExe = Join-Path $binDir 'Flore.exe'
$portableDir = Join-Path (Join-Path $desktopDir 'build') 'Flore-portable'
$zipPath = Join-Path (Join-Path $desktopDir 'build') 'Flore-portable.zip'

# 前置校验
if (-not (Test-Path $floreExe)) {
    throw "Flore.exe not found at: $floreExe. Run build-desktop.ps1 first."
}
if (-not (Test-Path $goExe)) {
    throw "flore-backend.exe not found at: $goExe. Run build-desktop.ps1 first."
}

if (Test-Path $portableDir) {
    Remove-Item -Recurse -Force $portableDir
}
New-Item -ItemType Directory -Path $portableDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $portableDir 'data') -Force | Out-Null
Copy-Item $floreExe -Destination $portableDir -Force
Copy-Item $goExe -Destination $portableDir -Force
Compress-Archive -Path $portableDir -DestinationPath $zipPath -Force
Write-Host "Portable package created: $zipPath"