# Build the Wails desktop app and bundle Go backend for standalone distribution.
param(
    [switch]$Dev
)

$ErrorActionPreference = 'Stop'
$desktopDir = $PSScriptRoot
$projectRoot = Join-Path (Join-Path $desktopDir '..') '..'
$binDir = Join-Path (Join-Path $desktopDir 'build') 'bin'
$goExe = Join-Path $binDir 'flore-backend.exe'
$wailsJsonPath = Join-Path $desktopDir 'wails.json'

# 0. 读取版本号（根 package.json 是唯一版本源，使用 UTF-8 编码避免中文乱码）
$pkgJson = Get-Content (Join-Path $projectRoot 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$appVersion = $pkgJson.version
Write-Host "Building version: $appVersion"

# 1. 优雅停止可能正在运行的旧实例，避免构建时文件被占用。
#    N9：先尝试 CloseMainWindow 让应用走正常退出流程（后端优雅关闭 + WAL checkpoint），
#    仅在超时后才 Kill，避免无条件强杀导致用户数据库进入非干净状态。
$processNames = @('Flore', 'flore-backend')
foreach ($name in $processNames) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        $proc = $_
        Write-Host "Stopping running process: $($proc.ProcessName) (PID $($proc.Id))"
        try {
            $closed = $false
            if (-not $proc.HasExited -and $proc.MainWindowHandle -ne 0) {
                $closed = $proc.CloseMainWindow()
            }
            if ($closed) {
                if (-not $proc.WaitForExit(8000)) {
                    Write-Warning "$($proc.ProcessName) did not exit gracefully within 8s, killing."
                    $proc.Kill()
                    $proc.WaitForExit(3000) | Out-Null
                }
            } elseif (-not $proc.HasExited) {
                $proc.Kill()
                $proc.WaitForExit(3000) | Out-Null
            }
        } catch {
            Write-Warning "Failed to stop $($proc.ProcessName): $_"
        }
    }
}

# 2. 把版本号注入 wails.json 的 info.productVersion，
#    N8：消除 wails.json 硬编码版本与 package.json 不一致的问题（单一版本源）。
$wailsJson = Get-Content $wailsJsonPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ($wailsJson.info.productVersion -ne $appVersion) {
    Write-Host "Updating wails.json productVersion: $($wailsJson.info.productVersion) -> $appVersion"
    $wailsJson.info.productVersion = $appVersion
    $json = $wailsJson | ConvertTo-Json -Depth 10
    [System.IO.File]::WriteAllText($wailsJsonPath, $json, [System.Text.UTF8Encoding]::new($false))
}

# 3. Build Go backend（每次重新构建，避免源码修改后仍使用旧二进制）
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

# 4. Build desktop app with Wails
#    项目硬规范：桌面壳正式构建必须走 `wails build`，不可用 `go build` 代替
#    （go build 不会生成 WebView2 资源清单 / 图标 / 版本信息，也不会构建前端）。
Push-Location $desktopDir
try {
    $goPath = (go env GOPATH).Trim()
    $wails = Join-Path (Join-Path $goPath 'bin') 'wails'
    if ($Dev) {
        & $wails dev
        if ($LASTEXITCODE -ne 0) {
            throw "wails dev failed with exit code $LASTEXITCODE"
        }
    } else {
        & $wails build
        if ($LASTEXITCODE -ne 0) {
            throw "wails build failed with exit code $LASTEXITCODE"
        }
    }
} finally {
    Pop-Location
}

# N11：-Dev 是长驻的开发模式，退出后不应继续执行打包步骤
if ($Dev) {
    return
}

# 5. 创建便携分发包（调用独立脚本）
#    N10：$LASTEXITCODE 只反映原生可执行文件的退出码，对 .ps1 调用无效；
#    package-portable.ps1 内部用 throw 报错，这里用 try/catch 捕获。
Write-Host "Creating portable package..."
try {
    & "$desktopDir\package-portable.ps1" -Version $appVersion
} catch {
    throw "package-portable.ps1 failed: $_"
}
Write-Host "Portable packaging complete."
