# Flore 桌面壳（Wails v2）

Flore RSS 阅读器的 Windows 桌面外壳：内嵌 WebView2 承载 `apps/web` 前端，
并以子进程方式拉起 `server/go` 后端（`flore-backend.exe`）。

- 技术栈：Go 1.26 + Wails v2.13 + energye/systray
- 目标平台：Windows（x64）

## 目录结构

```
apps/desktop/
├── app.go               # 应用主体：后端进程生命周期、窗口行为、导出、通知
├── main.go              # Wails 启动入口、单实例锁、重启等待逻辑
├── systray.go           # 系统托盘（独立 OS 线程 + 消息泵）
├── job_windows.go       # Windows Job Object：主进程退出时连带回收后端子进程
├── job_other.go         # 非 Windows 平台的空实现
├── dialog_windows.go    # 启动失败时的原生 MessageBox
├── build/               # Wails 资源目录（图标 / manifest / installer 模板）
├── frontend/dist/       # 由 build-frontend.ps1 从 apps/web 复制而来（不入库）
├── build-desktop.ps1    # 【构建入口】
├── build-frontend.ps1   # 前端构建 + 拷贝
└── package-portable.ps1 # 便携包打包
```

## 构建

**必须使用 `wails build`，不能用 `go build` 代替。**
`go build` 不会生成 WebView2 应用清单、图标与版本信息资源，也不会触发前端构建，
产出的 exe 无法正常运行。`go build ./...` 仅用于本地快速校验 Go 代码是否编译通过。

推荐始终通过唯一入口脚本构建：

```powershell
# 完整构建：后端 exe + 前端 + wails build + 便携包
powershell -NoProfile -ExecutionPolicy Bypass -File .\build-desktop.ps1

# 开发模式（wails dev，热重载；不会执行打包步骤）
powershell -NoProfile -ExecutionPolicy Bypass -File .\build-desktop.ps1 -Dev
```

脚本会依次完成：

1. 从根 `package.json` 读取版本号（**唯一版本源**），并写入 `wails.json` 的 `info.productVersion`
2. 优雅停止正在运行的 `Flore` / `flore-backend`（先 `CloseMainWindow`，超时才 Kill）
3. `go build` 后端到 `build/bin/flore-backend.exe`
4. `wails build` 产出 `build/bin/Flore.exe`（内部调用 `build-frontend.ps1` 构建并拷贝前端）
5. `package-portable.ps1` 生成 `build/Flore-portable-<version>.zip`

前置依赖：Go 1.26+、Node.js（`apps/web` 构建）、`wails` CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0`，可执行文件位于 `$(go env GOPATH)/bin`）、WebView2 Runtime。

### build/ 目录中被 gitignore 的资源

仓库根 `.gitignore` 有一条 `build/` 规则会忽略所有 `build` 目录。
`apps/desktop/.gitignore` 已用 `!` 逐级反向豁免了下列**必须入库**的资源
（注意：Git 不递归进入被忽略的目录，所以必须先豁免目录本身，再豁免文件）：

| 文件 | 用途 |
|---|---|
| `build/favicon.png` | `systray.go` 通过 `go:embed` 内嵌的托盘图标，**缺失会导致编译失败** |
| `build/appicon.png` / `build/favicon.ico` | Wails 应用图标 |
| `build/windows/icon.ico` | exe 图标 |
| `build/windows/info.json` | 版本信息资源 |
| `build/windows/wails.exe.manifest` | DPI / WebView2 应用清单 |
| `build/windows/installer/*.nsi`、`*.nsh` | NSIS 安装包模板 |
| `build/darwin/Info*.plist` | macOS 打包模板 |

若在干净检出上 `wails build` 报缺文件，请先确认这些资源确实存在于工作区。

## 运行期约定

### 数据目录

按以下顺序解析（`app.go: appDataDir`）：

1. 环境变量 `FLORE_DATA_DIR`
2. **便携模式**：exe 同级存在 `data/` 目录时使用它（便携包中已预置 `data/.keep` 占位文件，
   确保解压后目录存在；空目录会被 `Compress-Archive` 丢弃）
3. 回退：`%LOCALAPPDATA%\Flore`（无则 `%APPDATA%\Flore`）

该目录下存放 `reader.db`、`flore-desktop.log`、`flore-backend.log`、`window-state.json`、`webview2/`。

### 后端二进制查找

只信任绝对路径（`app.go: findGoBackend`）：

1. 环境变量 `FLORE_BACKEND_PATH`（必须是绝对路径，供开发态显式指定）
2. exe 所在目录及其 `build/bin/` 子目录，向上最多 3 层

**不会**再从当前工作目录解析相对路径 —— 那等价于允许任意可写目录植入同名二进制。

### 本地 API 鉴权

桌面壳启动时用 `crypto/rand` 生成一次性 token，通过 `FLORE_API_TOKEN` 注入后端。
后端敏感接口（数据库导出/恢复、OPML 导入、删除订阅源、image/css 代理、`/api/settings/:key` 等）
需要 `Authorization: Bearer <token>`。

- 桌面壳自身的请求由 `App.authorize` 自动附加
- 前端需先调用绑定方法 `GetAPIToken()` 取得 token，再附加到 fetch 头

### 环境变量一览

| 变量 | 说明 |
|---|---|
| `FLORE_DATA_DIR` | 覆盖数据目录 |
| `FLORE_BACKEND_PATH` | 后端二进制绝对路径（开发态） |
| `FLORE_DEVTOOLS=1` | 启用 WebView2 DevTools |
| `FLORE_RESTART_WAIT_PID` | 内部使用：`RestartApp` 让新实例等待旧实例退出 |
| `DATABASE_URL` | 覆盖数据库文件路径 |

## 进程与实例模型

- **单实例锁**：Wails `SingleInstanceLock`（`UniqueId = flore-rss-reader-desktop`）。
  第二个实例会把参数发给已运行实例并自杀，已运行实例负责 `WindowUnminimise + WindowShow`。
- **Job Object**：后端子进程被加入带 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 的 Job，
  主进程崩溃或被强杀时由内核连带终止后端，杜绝孤儿 `flore-backend.exe` 持有 SQLite WAL。
- **重启**：`RestartApp` 用 `CREATE_BREAKAWAY_FROM_JOB` 启动新实例并传入旧实例 PID，
  新实例在 `wails.Run` 之前等待旧实例退出（互斥体释放）后再继续。

## 本地校验

```powershell
cd apps/desktop
go build ./...
go vet ./...
gofmt -l .
```

`gofmt -l .` 必须无输出。仓库根 `.gitattributes` 已声明 `*.go text eol=lf`，
避免 `core.autocrlf=true` 的 Windows 检出把 Go 源码变成 CRLF 而导致格式检查全量报错。
