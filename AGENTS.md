# AGENTS.md — Flore (RSS Reader) 项目 AI 行为总纲

> 本文档是 AI 编码工具的行为约束与项目上下文总纲。
> 所有 AI 在参与本项目开发时，优先读取本文档。

---

## 一、项目身份

- **项目名称**：Flore (RSS Reader)
- **项目定位**：本地优先的 RSS 阅读器桌面应用，附带 Web 版
- **技术栈摘要**：
  - 后端：Go 1.26 + Gin v1.10 + GORM v1.25 + SQLite（纯 Go 驱动）
  - 前端：React 19 + Vite 6 + TypeScript 5 + Tailwind CSS 3 + lucide-react
  - 桌面壳：Wails v2.13（Go + Web 前端）
  - 独立工具：`apps/routing-tool/`（Hono + Prisma，**不修改**）
- **仓库根目录**：`./`
- **版本号来源**：项目根目录 `package.json` 的 `version` 字段是唯一来源

---

## 二、目录规范（SDD — 结构驱动开发）

### 2.1 顶层目录结构

```
rss/                                   # 项目根目录
├── apps/                              # 应用项目
│   ├── web/                           #   Web 前端（React + Vite + Tailwind）
│   │   ├── src/                       #   源码
│   │   │   ├── components/            #   React 组件（每个组件一个文件）
│   │   │   │   ├── settings/          #   设置面板子组件
│   │   │   ├── hooks/                 #   React hooks
│   │   │   ├── lib/                   #   工具函数（cn.ts 等）
│   │   │   ├── utils/                 #   API 调用、设置、toast 等
│   │   │   ├── types.ts               #   TypeScript 类型定义
│   │   │   └── *.d.ts                 #   类型声明文件
│   │   ├── public/                    #   静态资源（manifest.json、sw.js）
│   │   ├── dist/                      #   构建产物（不提交）
│   │   ├── tailwind.config.js         #   Tailwind 配置
│   │   ├── vite.config.ts             #   Vite 配置
│   │   └── package.json
│   ├── desktop/                       #   Wails 桌面壳
│   │   ├── src/                       #   Go 壳代码
│   │   ├── frontend/                  #   前端构建后的资源
│   │   ├── build/                     #   桌面安装包构建产物（不提交）
│   │   ├── wails.json                 #   Wails 配置
│   │   ├── build-frontend.mjs         #   跨平台前端构建（Node，替代 build-frontend.ps1）
│   │   └── package.json
│   └── routing-tool/                  #   独立路由工具项目（只读，不修改）
│       ├── src/
│       ├── dist/                      #   构建产物（不提交）
│       └── prisma/                    #   Prisma schema（独立项目）
├── server/                            # 后端服务
│   └── go/                            #   Go 后端
│       ├── cmd/                       #   入口（main.go）
│       ├── internal/
│       │   ├── handlers/              #   HTTP 路由处理器
│       │   ├── services/              #   业务逻辑层
│       │   ├── models/                #   数据模型
│       │   └── database/              #   数据库连接和迁移
│       ├── go.mod / go.sum
│       └── bin/                       #   编译产物（不提交）
├── docs/                              # 项目文档中心
│   ├── architecture/                  #   架构设计文档
│   ├── design/                        #   品牌设计、UI 规范
│   ├── planning/                      #   功能规划、路线图
│   └── standards/                     #   编码规范
├── assets/                            # 静态资源
│   └── favicon/                       #   网站图标集
├── .trae/                             # IDE 配置（skills 等，不提交）
├── .design/                           # 设计预览文件（工具生成，不提交）
├── .workbuddy/                        # AI 助手工作区
├── graphify-out/                      # 代码图谱分析输出（不提交）
├── AGENTS.md                          # AI 行为总纲（本文件）
├── package.json                       # 项目级 npm 脚本编排
└── .gitignore                         # Git 忽略规则
```

### 2.2 各目录职责与约束

| 目录 | 用途 | 允许存放 | 禁止存放 |
|------|------|---------|---------|
| `apps/web/src/` | Web 前端源码 | 组件、hooks、工具、类型 | 构建产物、Go 代码 |
| `apps/desktop/` | Wails 桌面壳 | Go 壳代码、构建脚本、Wails 配置 | 业务组件代码 |
| `apps/routing-tool/` | 独立路由工具 | 路由配置、抓取器 | **本项目不修改此目录** |
| `server/go/cmd/` | Go 后端入口 | main.go、启动配置 | 业务逻辑 |
| `server/go/internal/handlers/` | HTTP 层 | 路由处理、请求/响应格式化 | 业务逻辑、数据库操作 |
| `server/go/internal/services/` | 业务逻辑层 | 核心业务实现 | HTTP 类型依赖（gin.Context） |
| `server/go/internal/models/` | 数据模型 | GORM 模型定义、自定义类型 | 业务逻辑 |
| `server/go/internal/database/` | 数据库层 | 连接池、迁移 | 业务逻辑 |
| `docs/architecture/` | 架构文档 | 系统架构、数据流说明 | 代码文件 |
| `docs/design/` | 设计文档 | 品牌、UI 规范、配色方案 | 功能规划文档 |
| `docs/planning/` | 规划文档 | 功能需求、迭代计划 | 架构设计文档 |
| `docs/standards/` | 编码规范 | 代码规范、命名约定 | 架构文档 |
| `assets/` | 静态资源 | 图标、图片、字体 | 代码文件 |
| `scripts/` | 脚本工具 | 跨项目自动化脚本 | 业务代码 |

### 2.3 文件放置规则

1. **文档统一归入 `docs/`**：架构文档放 `docs/architecture/`，设计文档放 `docs/design/`，规划文档放 `docs/planning/`，规范文档放 `docs/standards/`
2. **静态资源归入 `assets/`**：favicon、logo 等图标放 `assets/favicon/`
3. **构建产物不混入源码**：Go 编译产物输出到 `server/go/bin/`，前端构建产物在 `apps/web/dist/`，桌面安装包在 `apps/desktop/build/`
4. **工具类函数**：小工具放 `src/lib/`，业务工具放 `src/utils/`，React hooks 放 `src/hooks/`
5. **每个组件独立文件**：文件名与组件名一致，PascalCase
6. **根目录保持精简**：仅保留项目级配置文件（package.json、AGENTS.md、.gitignore、.editorconfig 等）

---

## 三、技术栈

### 3.1 后端（server/go/）

| 领域 | 版本 | 说明 |
|------|------|------|
| 语言 | Go 1.26+ | 单二进制部署优势 |
| HTTP 框架 | Gin v1.10+ | 高性能 Web 框架 |
| ORM | GORM v1.25+ | 优先使用 GORM；FTS5 用原生 SQL |
| 数据库 | SQLite | 纯 Go 驱动（github.com/glebarez/sqlite），无 CGO |
| 架构分层 | handler → service → model | 职责清晰，handler 不含业务逻辑 |

### 3.2 前端（apps/web/）

| 领域 | 版本 | 说明 |
|------|------|------|
| 框架 | React 19 | 当前版本 |
| 构建工具 | Vite 6 + TypeScript 5 | 快速构建，类型安全 |
| 样式方案 | Tailwind CSS 3 + CSS 变量 | Tailwind 用于布局/间距；CSS 变量用于主题色 |
| 图标库 | lucide-react | 统一从 `src/components/icons.tsx` 导入 |
| 工具库 | `cn()` | `src/lib/cn.ts`，条件类名拼接（clsx 兼容） |

### 3.3 桌面壳（apps/desktop/）

| 领域 | 说明 |
|------|------|
| 桌面框架 | Wails v2.13 |
| 后端进程管理 | Go 后端作为子进程启动，由 `app.go` 管理生命周期 |

---

## 四、命名规范

### 4.1 Go 命名

| 实体 | 规范 | 示例 |
|------|------|------|
| 文件/目录 | `snake_case.go` | `reader_service.go`, `base_fetcher.go` |
| 包名 | 小写，简洁 | `handlers`, `services`, `models` |
| 类型/接口 | `PascalCase` | `ReaderService`, `Source`, `FilterRule` |
| 导出函数 | `PascalCase` | `GetSources()`, `NewReaderService()` |
| 私有函数 | `camelCase` | `updateSourceHealth()` |
| 变量/常量 | `camelCase` / `UPPER_SNAKE_CASE` | `db`, `defaultPort`, `MaxKeepCount` |

### 4.2 TypeScript/React 命名

| 实体 | 规范 | 示例 |
|------|------|------|
| 组件文件 | `PascalCase.tsx` | `ArticleList.tsx`, `SettingsModal.tsx` |
| 工具文件 | `camelCase.ts` | `api.ts`, `cn.ts`, `toast.ts` |
| 类型声明文件 | `camelCase.d.ts` | `css.d.ts`, `fs-access.d.ts` |
| React 组件 | `PascalCase` | `ArticleList`, `TitleBar` |
| props/state | `camelCase` | `selectedSourceId`, `isOpen` |
| CSS 变量 | `--kebab-case` | `--bg-surface`, `--text-primary`, `--primary` |
| 接口类型 | `PascalCase` | `Source`, `FilterRule`, `Item` |
| 枚举替代 | 联合类型 | `'unread' \| 'read' \| 'starred'` |

---

## 五、代码规范

### 5.1 Go 代码规范

**导入分组**（标准库 → 第三方库 → 内部库，组间空行）：
```go
import (
    "fmt"
    "os"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    "github.com/rss/go-server/internal/models"
)
```

**错误处理**：
- 所有返回 `error` 的函数调用必须检查错误，禁止 `_, _ :=` 丢弃
- 使用 `%w` 包装错误上下文：`fmt.Errorf("failed to get sources: %w", err)`
- Service 层返回错误，Handler 层转化为 HTTP 响应
- 资源操作配对 `defer Close()`

**Service 层规约**：
- 不引入 `gin.Context` 或任何 HTTP 类型
- 参数和返回值使用纯 Go 类型
- 通过依赖注入或全局变量获取数据库实例
- 事务使用 `s.db.Transaction(func(tx *gorm.DB) error { ... })`

**Handler 层规约**：
- 只做：参数解析 → 调用 service → 返回 JSON
- 不做：业务逻辑、数据库操作
- 错误返回格式：`{ "error": "..." }`

**注释规范**：
- 导出函数必须写 `// 函数名 说明` 注释
- 禁止无意义注释（如 `// 获取数据`）
- TODO 格式：`// TODO: 说明`

### 5.2 前端代码规范

**组件职责**：
- `App.tsx`：状态管理中心，所有数据获取在此完成
- 各组件各自管理弹窗内逻辑，通过回调通知 App 刷新数据
- 禁止在组件内直接调用 API，统一通过 `src/utils/api.ts`

**状态管理**：
- 状态在 `App.tsx` 集中管理，通过 props 向下传递
- 禁止引入 Redux / Zustand 等状态管理库
- Hook 封装数据获取逻辑（如 `useSourcesData.ts`）

**样式规范**：
- 布局/间距/排版：优先 Tailwind 类名
- 主题色/暗色模式：CSS 变量（`--bg-*`, `--text-*`, `--primary` 等）
- 动态值：内联 `style={}`（如进度条宽度、右键菜单位置）
- 禁止使用 `!important`
- 禁止使用 emoji

**图标引用**：
- 统一从 `src/components/icons.tsx` 导入
- 禁止直接引用 `lucide-react`

**异步操作**：
- 所有 `async` 操作必须有 `try/catch` 或 `.catch()`
- `catch` 中应给用户反馈（toast），不能只 `console.error`

### 5.3 禁止事项

| 禁止项 | 原因 |
|--------|------|
| `server/go/bin/` 提交到 git | 编译产物，应忽略 |
| `apps/web/dist/` 提交到 git | 构建产物 |
| `apps/desktop/build/` 提交到 git | 安装包构建产物 |
| `graphify-out/` 提交到 git | 工具生成，运行时产生 |
| `.env` 文件提交到 git | 敏感信息，使用 `.env.example` 模板 |
| `node_modules/` 提交到 git | 依赖锁文件足够 |
| `apps/routing-tool/` 修改 | 独立项目，不耦合 |
| Go 中忽略错误（`_ = ...`） | 掩盖潜在 bug |
| 前端中未捕获的 Promise | 导致静默失败 |
| 硬编码端口/路径/密钥 | 使用环境变量 |

---

## 六、关键路径

### 6.1 后端

| 用途 | 路径 |
|------|------|
| Go 后端入口 | `server/go/cmd/main.go` |
| Go 后端路由 | `server/go/internal/handlers/reader.go` |
| Go 后端核心服务 | `server/go/internal/services/reader.go` |
| Go 后端 RSS 抓取 | `server/go/internal/services/fetcher.go` |
| Go 后端调度器 | `server/go/internal/services/scheduler.go` |
| Go 后端数据模型 | `server/go/internal/models/models.go` |
| Go 后端过滤规则 | `server/go/internal/services/filter.go` |
| Go 后端全文提取 | `server/go/internal/services/readability.go` |
| Go 后端导入/导出 | `server/go/internal/services/backup.go` / `export.go` |

### 6.2 前端

| 用途 | 路径 |
|------|------|
| 前端主应用 | `apps/web/src/App.tsx` |
| 前端侧边栏 | `apps/web/src/components/Sidebar.tsx` |
| 前端文章列表 | `apps/web/src/components/ArticleList.tsx` |
| 前端阅读器 | `apps/web/src/components/Reader.tsx` |
| 前端设置面板 | `apps/web/src/components/SettingsModal.tsx` |
| 前端 API 层 | `apps/web/src/utils/api.ts` |
| 前端类型定义 | `apps/web/src/types.ts` |
| 前端工具函数 | `apps/web/src/lib/cn.ts` |
| 前端 CSS 变量 | `apps/web/src/index.css` |

### 6.3 桌面壳 & 构建

| 用途 | 路径 |
|------|------|
| 桌面壳 App | `apps/desktop/app.go` |
| 桌面壳入口 | `apps/desktop/main.go` |
| 桌面构建/打包 | `apps/desktop/build-frontend.mjs` + `cmd/package-tool`（跨平台，替代原 .ps1） |
| Wails 配置 | `apps/desktop/wails.json` |

### 6.4 文档

| 用途 | 路径 |
|------|------|
| 架构文档 | `docs/architecture/ARCHITECTURE.md` |
| 产品文档 | `docs/architecture/PROJECT.md` |
| 设计文档 | `docs/design/` |
| 规划文档 | `docs/planning/` |
| 编码规范 | `docs/standards/` |

---

## 七、前端组件清单

| 组件 | 路径 | 用途 |
|------|------|------|
| `App.tsx` | `src/App.tsx` | 主应用，管理路由、布局、全局状态 |
| `Sidebar.tsx` | `src/components/Sidebar.tsx` | 侧边栏，订阅源/文件夹树 |
| `ArticleList.tsx` | `src/components/ArticleList.tsx` | 文章列表，无限滚动、多选、搜索 |
| `Reader.tsx` | `src/components/Reader.tsx` | 阅读器，RSS/Readability/iframe 三种模式 |
| `SettingsModal.tsx` | `src/components/SettingsModal.tsx` | 设置面板入口 |
| `AddSourceModal.tsx` | `src/components/AddSourceModal.tsx` | 添加订阅源弹窗 |
| `EditSourceModal.tsx` | `src/components/EditSourceModal.tsx` | 编辑订阅源弹窗 |
| `MoveSourceModal.tsx` | `src/components/MoveSourceModal.tsx` | 移动订阅源到文件夹 |
| `RenameModal.tsx` | `src/components/RenameModal.tsx` | 重命名弹窗 |
| `ConfirmDialog.tsx` | `src/components/ConfirmDialog.tsx` | 通用确认对话框 |
| `ModalLayout.tsx` | `src/components/ModalLayout.tsx` | 通用弹窗布局 |
| `ContextMenu.tsx` | `src/components/ContextMenu.tsx` | 右键菜单 |
| `SearchBox.tsx` | `src/components/SearchBox.tsx` | 搜索输入框 |
| `IconButton.tsx` | `src/components/IconButton.tsx` | 图标按钮 |
| `SourceAvatar.tsx` | `src/components/SourceAvatar.tsx` | 订阅源头像/图标 |
| `Loading.tsx` | `src/components/Loading.tsx` | 加载动画 |
| `EmptyState.tsx` | `src/components/EmptyState.tsx` | 空状态占位 |
| `ErrorBoundary.tsx` | `src/components/ErrorBoundary.tsx` | React 错误边界 |
| `TitleBar.tsx` | `src/components/TitleBar.tsx` | 桌面端标题栏 |
| `ShortcutsHelpModal.tsx` | `src/components/ShortcutsHelpModal.tsx` | 快捷键帮助弹窗 |
| `ExportArticlesModal.tsx` | `src/components/ExportArticlesModal.tsx` | 导出文章弹窗 |
| `ImportOPMLModal.tsx` | `src/components/ImportOPMLModal.tsx` | 导入 OPML 弹窗 |
| `icons.tsx` | `src/components/icons.tsx` | 图标 re-export（兼容 lucide-react） |
| `settings/` | `src/components/settings/` | 设置面板子标签组件 |

---

## 八、数据模型核心

```
Folder 1→N Source 1→N Item
Source 1→1 SourceHealth
FilterRule → scope: global|source|folder
```

- `Source` 包含 `active`、`isPrivate`、`hideInTimeline` 等状态字段
- `Item` 通过 `isRead`、`isStarred`、`isReadLater` 标记状态
- 全文搜索通过 FTS5 虚拟表 `ItemSearch` 实现
- 健康数据存储在独立的 `SourceHealth` 表
- `GetSources()` 列表接口做 JOIN 查询，`GetSource()` 单条查询不加载健康数据

---

## 九、版本规范

### 9.1 版本号格式

| 类型 | 格式 | 示例 |
|------|------|------|
| 开发版本 | `MAJOR.MINOR.PATCH.buildYYYYMMDD` | `0.0.1-20260801` |
| 正式版本 | `MAJOR.MINOR.PATCH` | `0.1.0` |
| 预发布版本 | `MAJOR.MINOR.PATCH-alpha/beta.N` | `1.0.0-alpha.1` |

### 9.2 版本变更规则

| 变更类型 | 版本变化 |
|---------|---------|
| Bug 修复（UI、抓取崩溃、安全漏洞等） | 递增 PATCH |
| 新功能新增（菜单、导入导出、设置项等） | 递增 MINOR |
| 样式改版（仅 UI 调整） | 递增 PATCH 或 MINOR |
| 底层重构（对外行为不变） | 递增 PATCH |
| 数据库表结构变更 | 递增 MAJOR |
| API 路由重写 | 递增 MAJOR |
| 配置格式变更 | 递增 MAJOR |

### 9.3 版本号注入流程

```
package.json (version: 0.0.1-20260801)
    │
    ├── 根 npm build:desktop 读取 version → wails build -ldflags 注入 Go 二进制
    │
    └── Go 后端 /api/version 返回 version
            └── 前端 SettingsAboutTab 调用此 API 动态显示
```

---

## 十、构建命令

| 命令 | 说明 |
|------|------|
| `npm run dev` | 同时启动 Go 后端 (`:3002`) + Web 前端 (`:5173`) |
| `npm run dev:web` | 仅启动 Web 前端 |
| `npm run dev:go` | 仅启动 Go 后端 |
| `npm run build:web` | 仅构建 Web 前端 |
| `npm run build:go` | 仅构建 Go 后端 |
| `npm run build:desktop` | 构建 Go 后端 → 打包桌面安装包 |
| `npm run build` | 构建全部（web + go + desktop） |
| `npm run wails:dev` | Wails 开发模式（桌面壳） |
| `npm run wails:build` | 同 `build:desktop` |

---

## 十一、安全约定

1. 请勿提交数据库文件（`*.db`、`*.db-journal`、`*.db-wal`）
2. 请勿提交 `.env` 文件（已有 `.env.example` 模板）
3. 请勿提交 `node_modules/`、`dist/`、`bin/` 等构建产物
4. 请勿修改 `apps/desktop/build/` 目录下的安装包配置
5. 所有用户输入拼接 SQL 时必须使用参数化查询
6. 数据库路径从环境变量读取，禁止硬编码

---

## 十二、AI 开发规范

### 12.1 文件存放规范

- **新增组件**：放入 `apps/web/src/components/`，文件名与组件名一致
- **新增 hooks**：放入 `apps/web/src/hooks/`
- **新增工具函数**：小工具放 `src/lib/`，业务工具放 `src/utils/`
- **新增后端服务**：放入 `server/go/internal/services/`
- **新增后端 handler**：放入 `server/go/internal/handlers/`
- **新增数据模型**：放入 `server/go/internal/models/`
- **新增文档**：根据类型放入 `docs/architecture/`、`docs/design/`、`docs/planning/`、`docs/standards/`
- **新增资源**：放入 `assets/` 对应子目录
- **新增脚本**：放入 `scripts/`

### 12.2 命名约定规范

- 所有新增文件严格遵守第四章命名规范
- 禁止使用下划线（`_`）或空格作为文件名的组成部分
- 禁止使用中文文件名（资产目录除外）
- 禁止使用缩写作为变量名（如 `tmp`、`res`、`cfg`），使用完整描述性名称

### 12.3 禁止事项

- 禁止修改 `apps/routing-tool/` 目录下的代码
- 禁止在 `server/go/` 外创建新的 Go 包
- 禁止在前端直接导入 Go 模块（反之亦然）
- 禁止在代码中硬编码路径、端口、密钥
- 禁止在 commit 中包含构建产物

### 12.4 代码审查清单（AI 自审）

每次代码修改后，必须检查：

- [ ] 符号是否闭合（括号、引号、模板标签、JSX 标签）
- [ ] 跨文件类型定义是否一致
- [ ] 是否有遗漏的错误处理
- [ ] 是否有遗漏的边界情况（空数组、null、0 值）
- [ ] 是否有不必要的依赖引入
- [ ] 是否符合命名规范
- [ ] 构建产物是否被正确忽略

---

## 十三、已识别问题

### 13.1 导入循环（静态分析误报，非真实问题）

graphify 工具报告 `apps/routing-tool/src/scrapers/` 存在文件级导入循环：

| 循环 | 文件 1 | 文件 2 |
|------|--------|--------|
| 循环 1 | `registry.ts` | `sites/generic-css.ts` |
| 循环 2 | `registry.ts` | `sites/chinawriter.ts` |
| 循环 3 | `registry.ts` | `sites/rss-feed.ts` |

**原因**：`registry.ts` 使用 `import type { ScrapeResult }` 导入类型，`sites/*.ts` 同时导入 `base-fetcher.ts` 和 `registry.ts`。TypeScript 的 `import type` 在编译时会被完全擦除，不产生运行时依赖，因此**不影响执行**。这是静态分析工具对类型导入的误报。

**无需修复**。如需消除误报，可将 `ScrapeResult` 类型提取到独立文件 `types.ts`，但这无实际收益。

### 13.2 缺失目录/文件

- `scripts/` 目录已创建（2026-08-01），用于存放跨项目自动化脚本
- `docs/planning/pending/` 子目录存在，用于存放待确认规划文档

---

## 十四、文件放置规则（速查）

```
新增组件  → apps/web/src/components/
新增 hook → apps/web/src/hooks/
新增工具  → apps/web/src/lib/（小工具）或 src/utils/（业务工具）
新增类型  → apps/web/src/types.ts（集中维护）
新增服务  → server/go/internal/services/
新增 handler → server/go/internal/handlers/
新增模型  → server/go/internal/models/
新增文档  → docs/{architecture,design,planning,standards}/
新增资源  → assets/
新增脚本  → scripts/
```
