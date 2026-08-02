# Portable distribution packaging script for Flore.
# Called by build-desktop.ps1 after the desktop build completes.
param(
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'

$desktopDir = $PSScriptRoot
$projectRoot = Join-Path (Join-Path $desktopDir '..') '..'
$buildDir = Join-Path $desktopDir 'build'
$binDir = Join-Path $buildDir 'bin'
$goExe = Join-Path $binDir 'flore-backend.exe'
$floreExe = Join-Path $binDir 'Flore.exe'
$portableDir = Join-Path $buildDir 'Flore-portable'

# N12：便携包文件名带版本号，避免多个版本的 zip 互相覆盖、无法追溯。
if (-not $Version) {
    $pkgJson = Get-Content (Join-Path $projectRoot 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $Version = $pkgJson.version
}
$zipPath = Join-Path $buildDir "Flore-portable-$Version.zip"

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

# 便携模式约定：exe 同级存在 data/ 目录时，所有数据（reader.db / 日志 / WebView2 缓存）
# 都落在该目录内，随包迁移。Compress-Archive 会丢弃空目录，
# 因此放一个占位文件确保 data/ 能被打进 zip 并在解压后存在。
$dataDir = Join-Path $portableDir 'data'
New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
Set-Content -Path (Join-Path $dataDir '.keep') -Value '' -NoNewline -Encoding UTF8

Copy-Item $floreExe -Destination $portableDir -Force
Copy-Item $goExe -Destination $portableDir -Force

if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path $portableDir -DestinationPath $zipPath -Force
Write-Host "Portable package created: $zipPath"
