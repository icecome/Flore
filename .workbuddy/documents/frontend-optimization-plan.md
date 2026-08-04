# RSS 阅读器前端优化与订阅导入导出计划

## Context

当前 RSS 阅读器已完成三栏基础布局（订阅源列表 / 文章列表 / 阅读区）和阅读模式改造。用户参考 Folo 界面，希望在前端样式、布局对齐、可阅读性以及基础管理工具上做适应性增强；同时根据 `follow.opml` 的层级结构，需要增加 OPML 订阅导入导出功能，并据此设计分类/文件夹机制。

`follow.opml` 中的典型结构：顶层 `outline` 可以是独立订阅源，也可以是文件夹（如“豆瓣”），文件夹下再挂若干子订阅源。因此分类/文件夹需要是独立数据实体，而不是 Source 上的一个字符串字段。

## Goals

1. **顶栏对齐**：Sidebar 顶栏、ArticleList 顶栏、Reader 顶栏（含 LOGO、加号、收藏标题、阅读区标题、工具栏）高度相等、视觉平齐。
2. **分类/文件夹**：
   - 订阅源按文件夹分组展示；
   - 文件夹右键：重命名、删除；
   - 订阅源右键：移动到文件夹、刷新、删除；
   - 文件夹与订阅源均支持展开/折叠。
3. **文章右键菜单**：标记已读/未读、收藏、新标签页打开。
4. **阅读模式可读性优化**：限制正文最大宽度并居中，增加两侧留白，避免文字顶满整个阅读区。
5. **OPML 导入导出**：
   - 导入：解析 OPML，按 outline 层级创建 Folder 与 Source；
   - 导出：按 Folder/Source 层级生成 OPML；
   - 与分类/文件夹数据模型天然对应。

## Recommended Approach

采用 **Folder 表（单层）+ OPML 映射** 方案：
- 数据库新增 `Folder` 表（`id`, `name`, `createdAt`, `updatedAt`），`Source` 增加 `folderId` 外键。
- 前端 Sidebar 按 Folder 分组渲染；Folder 可展开折叠，Folder 内 Source 可移动到其它 Folder。
- OPML 导入时：有子 outline 的节点创建为 Folder，叶子 outline（`type="rss"`）创建为 Source 并归属到父 Folder；顶层叶子 outline 作为无 Folder 的 Source。
- OPML 导出时：有 Folder 的 Source 放入对应 `<outline text="FolderName">` 下，无 Folder 的 Source 作为顶层 outline。
- 先不支持 Folder 嵌套（OPML 导入时若遇到二级以上文件夹，可扁平化到最近一层 Folder），保持基础功能简单可用，后续可扩展 `parentId`。

## Files to Modify

### 后端

- `node-app/server/prisma/schema.prisma`
  - 新增 `Folder` 模型；`Source` 新增 `folderId` 外键与关系。
- `node-app/server/src/services/source.ts`
  - `createSource` / `updateSource` 支持 `folderId`。
- `node-app/server/src/services/folder.ts`（新建）
  - Folder CRUD、获取 Folder 及其 Source 列表。
- `node-app/server/src/services/opml.ts`（新建）
  - `importOPML(xml: string)`：解析 OPML，创建 Folder/Source。
  - `exportOPML()`：遍历 Folder/Source 生成 OPML XML。
- `node-app/server/src/routes/folder.ts`（新建）
  - `GET /folders`、 `POST /folders`、 `PUT /folders/:id`、 `DELETE /folders/:id`。
- `node-app/server/src/routes/opml.ts`（新建）
  - `POST /opml/import`、 `GET /opml/export`。
- `node-app/server/src/index.ts`
  - 注册 `folderRouter` 与 `opmlRouter`。
- `go-server/internal/models/models.go`
  - 新增 `Folder` 模型；`Source` 增加 `FolderID`。
- `go-server/internal/services/reader.go`
  - 新增 `GetFolders`、`CreateFolder`、`UpdateFolder`、`DeleteFolder`、`MoveSourceFolder` 等方法。
  - 新增 `ImportOPMLViaNode`、`ExportOPMLViaNode` 透传到 Node 服务。
- `go-server/internal/handlers/reader.go`
  - 新增 Folder CRUD 路由。
  - 新增 OPML 导入导出路由。
  - 新增 `PUT /api/sources/:id` 更新 Source（含 folderId）。

### 前端

- `node-app/client/src/types.ts`
  - 新增 `Folder` 类型；`Source` 增加 `folderId`。
- `node-app/client/src/components/Sidebar.tsx`
  - 统一 header 高度与 ArticleList/Reader 顶栏对齐。
  - 按 Folder 分组渲染 Source，支持 Folder 与 Source 展开/折叠。
  - 集成右键菜单：Folder 右键（重命名、删除）、Source 右键（移动到、刷新、删除）。
- `node-app/client/src/components/ArticleList.tsx`
  - 统一 header 高度；文章项集成右键菜单。
- `node-app/client/src/components/Reader.tsx`
  - 统一 header 高度，工具栏与标题视觉平齐。
  - 阅读模式内容区限制最大宽度（`maxWidth: 720px`）并居中，增加两侧内边距。
- `node-app/client/src/components/ContextMenu.tsx`（新建）
  - 通用右键菜单组件。
- `node-app/client/src/components/MoveSourceModal.tsx`（新建）
  - 选择目标 Folder 的移动弹窗。
- `node-app/client/src/components/ImportOPMLModal.tsx`（新建）
  - OPML 文件上传与预览导入弹窗。
- `node-app/client/src/App.tsx`
  - 加载 Folder 列表；维护 source/folder 状态同步。
  - 新增 OPML 导入导出入口按钮与处理函数。

## Implementation Steps

1. **数据库与模型**
   - 修改 Prisma schema，新增 `Folder` 表与 `Source.folderId`。
   - 执行 `npx prisma db push`。
   - 同步修改 Go `models.Folder` 与 `models.Source`。

2. **后端 Folder & OPML API**
   - Node 端实现 Folder CRUD 与 OPML 导入导出。
   - Go 端暴露对应 API 并透传给 Node 服务。

3. **前端样式对齐**
   - 设定统一顶栏高度（建议 `44px`）。
   - 同步调整 Sidebar、ArticleList、Reader、AddSourceModal 的 header padding/字号/按钮尺寸。

4. **分类与右键菜单**
   - 新建 `ContextMenu`、`MoveSourceModal`。
   - Sidebar 按 Folder 分组，绑定右键菜单。
   - ArticleList 文章项绑定右键菜单。

5. **阅读模式可读性**
   - Reader 内容区 `maxWidth: 720px`、`margin: 0 auto`。
   - 增加两侧留白；正文字号/行高优化。

6. **OPML 导入导出**
   - 新建 `ImportOPMLModal`。
   - 在 Sidebar 顶栏增加导入/导出按钮。
   - 测试 `follow.opml` 导入后 Folder 与 Source 结构正确。

## Verification

1. 启动 `npm run dev` 一键启动服务。
2. 浏览器访问前端，确认三栏顶栏高度一致、对齐平齐。
3. 测试创建/重命名/删除 Folder，Source 移动到不同 Folder。
4. 右键测试：Folder、Source、文章项右键菜单功能正常。
5. 进入阅读模式，确认正文宽度受限、居中、留白合适。
6. 导入 `follow.opml`，验证“豆瓣”等文件夹及下属订阅源正确创建。
7. 导出 OPML，验证结构与导入前一致。
