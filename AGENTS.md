# AGENTS.md — RSS Reader 项目 AI 行为总纲

> 本文档是 AI 编码工具的行为约束与项目上下文总纲。
> 所有 AI 在参与本项目开发时，优先读取本文档。

## 项目身份

- **项目名称**：Flore (RSS Reader)
- **项目定位**：本地优先的 RSS 阅读器桌面应用，附带 Web 版
- **技术栈摘要**：Go (Gin+GORM+SQLite) 后端 + React (Vite + Tailwind) 前端 + Wails 桌面壳
- **仓库根目录**：`./`

## 目录规范

### 顶层目录结构

```
rss/                          # 项目根目录（仅保留项目级配置）
├── apps/                     # 应用项目
│   ├── web/                  #   Web 前端 (React + Vite + Tailwind)
│   ├── desktop/              #   Wails 桌面壳
│   └── routing-tool/         #   独立路由工具项目（不修改）
├── server/                   # 后端服务
│   └── go/                   #   Go 后端 (Gin + GORM + SQLite)
├── docs/                     # 项目文档中心
│   ├── architecture/         #   架构设计文档
│   ├── design/               #   设计分析、品牌文案
│   ├── planning/             #   规划文档、优化方案
│   └── standards/            #   编码规范
├── assets/                   # 静态资源
│   └── favicon/              #   网站图标集
├── scripts/                  # 脚本工具
├── .trae/                    # IDE 配置（skills、preferences 等）
│   └── skills/               #   TRAE 技能安装目录
├── .design/                  # 设计预览文件（工具生成，不手动修改）
├── AGENTS.md                 # AI 行为总纲（本文件）
├── package.json              # 项目级 npm 脚本（dev/build 编排）
├── package-lock.json         # 依赖锁文件
└── .gitignore                # Git 忽略规则
```

### 各目录职责与约束

| 目录         | 用途     | 允许存放                                  | 禁止存放                |
| ---------- | ------ | ------------------------------------- | ------------------- |
| `apps/`    | 应用项目   | 各子应用的源码、配置、构建脚本                       | 不存放跨项目共享代码          |
| `server/`  | 后端服务   | Go 源码、go.mod、.env.example             | 编译产物（.exe）、数据库文件    |
| `docs/`    | 项目文档   | Markdown 文档、图表、规范说明                   | 代码文件、配置文件           |
| `assets/`  | 静态资源   | 图标、图片、字体等资源文件                         | 源码文件、文档             |
| `scripts/` | 脚本工具   | 自动化脚本、用户脚本                            | 业务代码                |
| `.trae/`   | IDE 配置 | skills、preferences、documents 等 IDE 配置 | 项目业务文档（应放入 `docs/`） |

### 文件放置规则

1. **项目文档统一归入 `docs/`**：架构文档放 `docs/architecture/`，设计文档放 `docs/design/`，规划文档放 `docs/planning/`
2. **静态资源归入 `assets/`**：favicon、logo 等放 `assets/favicon/`，后续新增图片资源也放 `assets/` 下
3. **构建产物不混入源码**：Go 编译产物输出到 `apps/desktop/build/bin/`，前端构建产物在 `apps/web/dist/`，桌面安装包在 `apps/desktop/build/`
4. **IDE 相关配置归入 `.trae/`**：skills 安装、本地 preferences 等，不放入项目业务文档
5. **根目录保持精简**：仅保留 `AGENTS.md`、`package.json`、`.gitignore` 等项目级配置

## 技术栈推荐

### 后端（server/go/）

| 领域      | 推荐方案                           | 说明                                                |
| ------- | ------------------------------ | ------------------------------------------------- |
| 语言      | Go 1.26+                       | 利用 Go 的高性能和单二进制部署优势                               |
| HTTP 框架 | Gin v1.10+                     | 社区成熟、性能优秀的 Go Web 框架                              |
| ORM     | GORM v1.25+                    | 优先使用 GORM 操作数据库；FTS5 等 SQLite 特有功能可使用原生 SQL       |
| 数据库     | SQLite                         | 本地优先场景下的轻量选择，使用纯 Go 驱动 github.com/glebarez/sqlite |
| 数据库迁移   | GORM AutoMigrate + FTS5 原生 SQL | 保持迁移简单，避免引入额外迁移工具                                 |
| 架构分层    | handler → service → model      | 推荐清晰分层，handler 负责 HTTP，service 负责业务，model 负责数据    |

### 前端（apps/web/）

| 领域    | 推荐方案                     | 说明                                                                             |
| ----- | ------------------------ | ------------------------------------------------------------------------------ |
| 框架    | React 19                 | 项目当前采用 React 19                                                                |
| 构建工具  | Vite 6 + TypeScript 5    | 快速构建和类型安全                                                                      |
| 样式方案  | Tailwind CSS 3 + CSS 变量  | Tailwind 类用于布局/间距/排版；CSS 变量 (`--bg-*`, `--text-*`, `--primary` 等) 用于主题色和暗色模式切换 |
| 图标库   | lucide-react             | 项目当前采用 lucide-react                                                            |
| 工具库   | `cn()` (`src/lib/cn.ts`) | 条件类名拼接，用法同 `clsx`                                                              |
| 运行时依赖 | 保持精简                     | 优先复用已有依赖，新增依赖前评估体积和维护成本                                                        |

### 桌面壳（apps/desktop/）

| 领域     | 推荐方案         | 说明                                         |
| ------ | ------------ | ------------------------------------------ |
| 桌面框架   | Wails v2.13+ | 项目当前采用 Wails，利用 Go + Web 前端构建轻量桌面应用        |
| 构建脚本   | PowerShell   | `build-desktop.ps1` / `build-frontend.ps1` |
| 后端进程管理 | Go 后端作为子进程启动 | 通过 `app.go` 管理后端生命周期                       |

## 代码规范建议

1. **依赖管理**：优先复用现有依赖，新增依赖前评估体积、许可证和维护成本
2. **CGO 注意**：SQLite 使用纯 Go 驱动，避免引入 CGO 依赖以简化跨平台构建
3. **架构分层**：推荐 handler → service → model 分层，保持职责清晰
4. **端口与路径**：推荐通过环境变量或动态分配确定端口，数据库路径通过 `app.go` 的 `readerDatabasePath()` 确定
5. **前后端分离**：Go 后端作为独立 HTTP 服务，前端通过 `fetch()` 调用 API，不依赖 SSR
6. **项目边界**：`apps/routing-tool/` 是独立项目，Reader 项目开发时不修改其代码
7. **前端样式**：优先使用 Tailwind 类名；CSS 变量仅用于主题色（`--primary`, `--bg-*`, `--text-*` 等）；`style={}` 内联仅用于动态值（如进度条宽度、右键菜单位置）
8. **图标引用**：统一从 `src/components/icons.tsx` 导入，避免直接引用 `lucide-react`

## 关键路径

| 用途           | 路径                                                    |
| ------------ | ----------------------------------------------------- |
| Go 后端入口      | `server/go/cmd/main.go`                               |
| Go 后端路由      | `server/go/internal/handlers/reader.go`               |
| Go 后端核心服务    | `server/go/internal/services/reader.go`               |
| Go 后端 RSS 抓取 | `server/go/internal/services/fetcher.go`              |
| Go 后端调度器     | `server/go/internal/services/scheduler.go`            |
| Go 后端数据模型    | `server/go/internal/models/models.go`                 |
| Go 后端过滤规则    | `server/go/internal/services/filter.go`               |
| Go 后端全文提取    | `server/go/internal/services/readability.go`          |
| Go 后端导入/导出   | `server/go/internal/services/backup.go` / `export.go` |
| 前端主应用        | `apps/web/src/App.tsx`                                |
| 前端侧边栏        | `apps/web/src/components/Sidebar.tsx`                 |
| 前端文章列表       | `apps/web/src/components/ArticleList.tsx`             |
| 前端阅读器        | `apps/web/src/components/Reader.tsx`                  |
| 前端设置面板       | `apps/web/src/components/SettingsModal.tsx`           |
| 前端 API 层     | `apps/web/src/utils/api.ts`                           |
| 前端类型定义       | `apps/web/src/types.ts`                               |
| 前端工具函数       | `apps/web/src/lib/cn.ts`                              |
| 前端 CSS 变量    | `apps/web/src/index.css`                              |
| 桌面壳 App      | `apps/desktop/app.go`                                 |
| 桌面壳入口        | `apps/desktop/main.go`                                |
| 桌面构建脚本       | `apps/desktop/build-desktop.ps1`                      |
| 架构文档         | `docs/architecture/ARCHITECTURE.md`                   |
| 产品文档         | `docs/architecture/PROJECT.md`                        |
| 设计文档         | `docs/design/`                                        |
| 规划文档         | `docs/planning/`                                      |
| 编码规范         | `docs/standards/`                                     |

## 前端组件清单

| 组件                        | 用途                                   |
| ------------------------- | ------------------------------------ |
| `App.tsx`                 | 主应用，管理路由、布局、全局状态                     |
| `Sidebar.tsx`             | 侧边栏，订阅源/文件夹树                         |
| `ArticleList.tsx`         | 文章列表，无限滚动、多选、搜索                      |
| `Reader.tsx`              | 阅读器，RSS/Readability/iframe 三种模式      |
| `SettingsModal.tsx`       | 设置面板，通用/订阅源/过滤规则/关于                  |
| `AddSourceModal.tsx`      | 添加订阅源弹窗                              |
| `EditSourceModal.tsx`     | 编辑订阅源弹窗                              |
| `MoveSourceModal.tsx`     | 移动订阅源到文件夹                            |
| `RenameModal.tsx`         | 重命名弹窗                                |
| `ConfirmDialog.tsx`       | 通用确认对话框                              |
| `ModalLayout.tsx`         | 通用弹窗布局                               |
| `ContextMenu.tsx`         | 右键菜单                                 |
| `SearchBox.tsx`           | 搜索输入框                                |
| `IconButton.tsx`          | 图标按钮（工具栏用）                           |
| `SourceAvatar.tsx`        | 订阅源头像/图标                             |
| `Loading.tsx`             | 加载动画                                 |
| `EmptyState.tsx`          | 空状态占位                                |
| `ErrorBoundary.tsx`       | React 错误边界                           |
| `TitleBar.tsx`            | 桌面端标题栏                               |
| `ShortcutsHelpModal.tsx`  | 快捷键帮助弹窗                              |
| `ExportArticlesModal.tsx` | 导出文章弹窗                               |
| `ImportOPMLModal.tsx`     | 导入 OPML 弹窗                           |
| `icons.tsx`               | 自定义图标导出（兼容 lucide-react 的 re-export） |

## 数据模型核心

```
Folder 1→N Source 1→N Item
Source 1→1 SourceHealth
FilterRule → scope: global|source|folder
```

- `Source` 包含 `active`、`isPrivate`、`hideInTimeline` 等状态字段
- `Item` 通过 `isRead`、`isStarred`、`isReadLater` 标记状态
- 全文搜索通过 FTS5 虚拟表 `ItemSearch` 实现
- 健康数据存储在独立的 `SourceHealth` 表，`GetSources()` 列表接口做 JOIN 查询，`GetSource()` 单条查询不加载健康数据

## 版本规范

### 版本号格式

采用 SemVer 扩展格式，区分开发版本和发布版本：

| 类型 | 格式 | 示例 | 说明 |
|------|------|------|------|
| 开发版本 | `MAJOR.MINOR.PATCH.buildYYYYMMDD` | `0.0.1-20260730` | 第四段为日期构建号，仅用于日常开发构建 |
| 正式版本 | `MAJOR.MINOR.PATCH` | `0.1.0` | 标准 SemVer，用于发布标签（Git tag） |
| 预发布版本 | `MAJOR.MINOR.PATCH-alpha/beta.N` | `1.0.0-alpha.1` | 功能冻结后、正式发布前的测试版本 |

### 版本变更规则

遵循 SemVer 语义化版本控制，核心原则：**判断代码变更是否向下兼容**。

| 变更类型 | 版本变化 | 说明 |
|---------|---------|------|
| Bug 修复（UI 渲染、抓取崩溃、安全漏洞等） | 递增 PATCH | 向下兼容的修复，无论严重程度 |
| 新功能新增（添加菜单、导入导出、设置项等） | 递增 MINOR | 向下兼容的新功能，无论功能大小 |
| 样式改版（仅 UI 调整，不涉及 API 变更） | 递增 PATCH 或 MINOR | 按是否属于新功能定位判断 |
| 底层重构（内部重构，对外行为不变） | 递增 PATCH | 无 API 变化的内部重构 |
| 数据库表结构变更 | 递增 MAJOR | 破坏性变更（数据不兼容） |
| API 路由重写 | 递增 MAJOR | 破坏性变更（接口不兼容） |
| 配置格式变更 | 递增 MAJOR | 用户配置不兼容 |

**0.x 开发阶段特殊规则**：当前处于 `0.x` 初始开发阶段，API 不稳定。`0.x` 内 MAJOR 段位不递增，MINOR 递增视为"功能里程碑"，PATCH 递增视为"修复"。首次稳定发布时 `0.x.0` → `1.0.0`。

### 版本号唯一来源

版本号以项目根目录 `package.json` 的 `version` 字段为 **唯一来源**。所有子系统的版本号通过构建流程统一注入，不各自硬编码。

### 版本号注入流程

```
package.json (version: 0.0.1-20260730)
    │
    ├── build-desktop.ps1 读取 version → -ldflags 注入 Go 二进制
    │
    └── Go 后端 /api/version 返回 version
            │
            └── 前端 SettingsAboutTab 调用此 API 动态显示
```

### 版本号更新时机

每次有意义的代码变更（修复、添功能、重构）后，由开发者手动更新 `package.json` 中的 `version` 字段，按上述变更规则递增对应段位。

## 构建命令

| 命令                      | 说明                                      |
| ----------------------- | --------------------------------------- |
| `npm run dev`           | 同时启动 Go 后端 (`:3002`) + Web 前端 (`:5173`) |
| `npm run dev:web`       | 仅启动 Web 前端                              |
| `npm run dev:go`        | 仅启动 Go 后端                               |
| `npm run build:web`     | 仅构建 Web 前端                              |
| `npm run build:go`      | 仅构建 Go 后端                               |
| `npm run build:desktop` | 构建 Go 后端 → 打包桌面安装包                      |
| `npm run build`         | 构建全部（web + go + desktop）                |
| `npm run wails:dev`     | Wails 开发模式（桌面壳）                         |
| `npm run wails:build`   | 同 `build:desktop`                       |

## 安全约定

1. 请勿提交数据库文件（`*.db`、`*.db-journal`、`*.db-wal`）
2. 请勿提交 `.env` 文件（已有 `.env.example` 模板）
3. 请勿提交 `node_modules/`、`dist/` 等构建产物
4. 请勿修改 `apps/desktop/build/` 目录下的安装包配置
