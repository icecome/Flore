# Build the Wails desktop app and bundle Go backend for standalone distribution.
param(
    [switch]$Dev
)

$ErrorActionPreference = 'Stop'
$desktopDir = $PSScriptRoot
$projectRoot = Join-Path (Join-Path $desktopDir '..') '..'
$binDir = Join-Path (Join-Path $desktopDir 'build') 'bin'
$goExe = Join-Path $binDir 'flore-backend.exe'

# 0. 读取版本号（从根 package.json 获取，使用 UTF-8 编码避免中文乱码）
$pkgJson = Get-Content (Join-Path $projectRoot 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$appVersion = $pkgJson.version
Write-Host "Building version: $appVersion"

# 1. 终止可能正在运行的旧实例，避免构建时文件被占用
$processNames = @('Flore', 'flore-backend')
foreach ($name in $processNames) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        Write-Host "Stopping running process: $($_.ProcessName) (PID $($_.Id))"
        try {
            $_.Kill()
            $_.WaitForExit(2000) | Out-Null
        } catch {
            Write-Warning "Failed to stop $($_.ProcessName): $_"
        }
    }
}

# 1. Build Go backend（每次重新构建，避免源码修改后仍使用旧二进制）
Write-Host "Building Go backend..."
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}
Push-Location (Join-Path (Join-Path $projectRoot 'server') 'go')
try {
    go build -ldflags="-X github.com/rss/go-server/internal/handlers.appVersion=$appVersion" -o $goExe ./cmd
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

# 2. Build desktop app with Wails
Push-Location $desktopDir
try {
    $goPath = (go env GOPATH).Trim()
    $wails = Join-Path (Join-Path $goPath 'bin') 'wails'
    if ($Dev) {
        & $wails dev
    } else {
        & $wails build
    }
    if ($LASTEXITCODE -ne 0) {
        throw "wails build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

# 3. 创建便携分发包（调用独立脚本）
Write-Host "Creating portable package..."
& "$desktopDir\package-portable.ps1"
if ($LASTEXITCODE -ne 0) {
    throw "package-portable.ps1 failed with exit code $LASTEXITCODE"
}
Write-Host "Portable packaging complete."