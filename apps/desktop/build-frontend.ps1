# Build web frontend and copy dist into Wails frontend folder
param(
    [switch]$Dev,
    [switch]$Install
)

$ErrorActionPreference = 'Stop'
$webDir = Join-Path (Join-Path $PSScriptRoot '..') 'web'
$frontendDir = Join-Path $PSScriptRoot 'frontend'
$distSource = Join-Path $webDir 'dist'
$distTarget = Join-Path $frontendDir 'dist'

Push-Location $webDir
try {
    if ($Install) {
        npm install
        if ($LASTEXITCODE -ne 0) {
            throw "npm install failed with exit code $LASTEXITCODE"
        }
    } elseif ($Dev) {
        npm run dev
        if ($LASTEXITCODE -ne 0) {
            throw "npm run dev failed with exit code $LASTEXITCODE"
        }
    } else {
        npm run build
        if ($LASTEXITCODE -ne 0) {
            throw "npm run build failed with exit code $LASTEXITCODE"
        }
    }
} finally {
    Pop-Location
}

if (-not $Dev -and -not $Install) {
    if (Test-Path $distTarget) {
        Remove-Item $distTarget -Recurse -Force
    }
    Copy-Item $distSource -Destination $distTarget -Recurse -Force
    Write-Host "Copied web dist to $distTarget"
}
