# 文章粒度本地备份/导出功能实现方案

## 上下文

本项目是一款本地优先、无云端账户的 RSS 阅读器。用户已明确要求支持“指定文章备份”，即不依赖全量数据库备份，而是可以按收藏、文件夹、订阅源或多选文章进行定向归档。该功能将显著提升本地阅读器的“内容主权”感知，并与删除订阅源前的留档场景形成闭环。

本方案在不上云、不增加账户体系的前提下，设计最小可行实现。

## 推荐方案概述

实现一套文章导出能力：

* **导出范围**：多选文章、当前筛选列表、当前文件夹、当前订阅源、全部收藏、阅读器中单篇文章。

* **导出格式**：

  * Markdown 压缩包（默认）：每篇文章一个 `.md` 文件，带 YAML frontmatter。

  * JSON 数据包：结构化导出，为未来重新导入预留格式。

  * 单篇文章：直接下载 `.md` 文件。

* **内容来源**：MVP 使用数据库中已有的 `desc` 字段（文章摘要/正文 HTML），不联网抓取 readability，不下载图片。

* **文件生成**：后端使用 Go 标准库 `archive/zip` 流式生成 ZIP，前端仅触发下载。

## 后端设计

### 新增路由

在 `go-server/internal/handlers/reader.go` 中注册：

```go
api.POST("/items/export", h.ExportItems)              // 批量导出（ZIP/JSON）
api.GET("/items/:id/export.md", h.ExportItemMarkdown) // 单篇 Markdown 下载
```

### 批量导出接口

**请求**：`POST /api/items/export`

```json
{
  "scope": {
    "ids": [1, 2, 3],
    "sourceId": 5,
    "folderId": 2,
    "starred": true,
    "unread": false,
    "hidePrivate": false
  },
  "format": "markdown"
}
```

优先级：

1. 若 `ids` 非空，仅导出指定文章。
2. 否则按 `sourceId` / `folderId` / `starred` / `unread` / `hidePrivate` 组合查询。

**响应**：

* `format=markdown`：返回 `application/zip`，文件名 `articles-YYYY-MM-DD.zip`。

* `format=json`：返回 `application/json`，文件名 `articles-YYYY-MM-DD.json`。

### 单篇导出接口

**请求**：`GET /api/items/:id/export.md`

**响应**：`text/markdown`，文件名 `YYYY-MM-DD-title-slug.md`。

### 新增服务方法

在 `go-server/internal/services/reader.go` 中新增：

```go
type ExportScope struct {
    IDs         []int `json:"ids"`
    SourceID    *int  `json:"sourceId"`
    FolderID    *int  `json:"folderId"`
    Starred     bool  `json:"starred"`
    Unread      bool  `json:"unread"`
    HidePrivate bool  `json:"hidePrivate"`
}

func (s *ReaderService) GetItemsForExport(scope ExportScope) ([]models.ItemWithSource, error)
func (s *ReaderService) GetItemWithSource(id int) (*models.ItemWithSource, error)
func (s *ReaderService) ExportItemsMarkdown(items []models.ItemWithSource, w io.Writer) error
func (s *ReaderService) ExportItemsJSON(items []models.ItemWithSource, w io.Writer) error
func (s *ReaderService) ItemToMarkdown(item models.ItemWithSource) string
func (s *ReaderService) GenerateSafeFilename(item models.ItemWithSource, used map[string]bool) string
```

### Markdown 文件格式

```markdown
---
title: "文章标题"
author: "作者"
link: "https://example.com/article"
source: "来源名称"
sourceUrl: "https://example.com/feed"
pubDate: "2026-07-26T10:00:00+08:00"
isRead: true
isStarred: false
---

<article desc HTML>
```

### JSON 文件格式

```json
{
  "version": "1.0",
  "exportedAt": "2026-07-26T12:00:00+08:00",
  "count": 3,
  "items": [
    {
      "id": 1,
      "sourceId": 5,
      "title": "...",
      "link": "...",
      "desc": "...",
      "author": "...",
      "pubDate": "...",
      "isRead": true,
      "isStarred": false,
      "sourceName": "...",
      "sourceUrl": "..."
    }
  ]
}
```

### 文件命名规则

* 单篇文章：`{YYYY-MM-DD}-{slug}.md`

  * `slug` 由标题转小写、特殊字符替换为 `-`、截断至 60 字符生成。

  * 空标题回退为 `article`。

  * 同一天同名文件追加序号：`2026-07-26-title.md`、`2026-07-26-title-1.md`。

* 批量 Markdown：`articles-YYYY-MM-DD.zip`

* 批量 JSON：`articles-YYYY-MM-DD.json`

## 前端设计

### 新增组件：`ExportArticlesModal.tsx`

位置：`node-app/client/src/components/ExportArticlesModal.tsx`

Props：

```ts
interface ExportScope {
  ids?: number[];
  sourceId?: number;
  folderId?: number;
  starred?: boolean;
  unread?: boolean;
  hidePrivate?: boolean;
}

interface Props {
  onClose: () => void;
  onExport: (scope: ExportScope, format: 'markdown' | 'json') => Promise<void>;
  currentSourceId: number | null;
  currentFolderId: number | null;
  currentFilter: 'all' | 'unread' | 'starred';
  hidePrivate: boolean;
  selectedIds: Set<number>;
}
```

界面：

* 导出范围（单选）：

  * 当前筛选结果

  * 当前文件夹（仅当选中文件夹时显示）

  * 当前订阅源（仅当选中订阅源时显示）

  * 全部收藏文章

  * 已选中的 N 篇文章（仅当 `selectedIds.size > 0` 时显示，且默认选中）

* 导出格式（单选）：Markdown 压缩包（默认）、JSON

* 底部提示：导出内容为文章摘要 HTML，不下载图片。

* 按钮：取消 / 导出。

### `ArticleList.tsx` 改动

新增状态与行为：

* `multiSelectMode: boolean`：是否处于多选模式。

* `selectedIds: Set<number>`：选中的文章 ID。

新增 Props（由 App 注入并回传）：

* `selectedIds: Set<number>`

* `onToggleSelectedId: (id: number) => void`

* `onClearSelection: () => void`

* `multiSelectMode: boolean`

* `onToggleMultiSelectMode: () => void`

头部操作栏：

* 新增“多选”按钮：进入/退出多选模式。

* 新增“导出”按钮：打开导出弹窗。

多选模式下：

* 每个 `ArticleItem` 左侧显示复选框。

* 点击条目切换选中状态，不再打开文章。

* 顶部显示“已选择 N 篇”，提供“导出选中”快捷入口。

* 提供“全选当前列表”/“取消全选”。

右键菜单：

* 新增“导出为 Markdown”项，导出当前右键对应单篇文章。

### `Reader.tsx` 改动

顶部操作栏新增“下载”图标按钮：

```tsx
<IconButton onClick={() => onExportItem?.(item)} title="导出为 Markdown">
  <Download size={16} />
</IconButton>
```

新增 Prop：`onExportItem?: (item: Item) => void`。

点击后由 App 调用 `GET /api/items/:id/export.md` 触发浏览器下载。

### `App.tsx` 改动

新增状态：

* `showExportModal: boolean`

* `articleSelectedIds: Set<number>`

* `articleMultiSelectMode: boolean`

新增处理函数：

* `handleExportArticles(scope, format)`：调用 `POST /api/items/export`，生成 Blob 并触发下载。

* `handleExportSingleItem(item)`：调用 `GET /api/items/:id/export.md` 触发下载。

将相关状态和回调透传给 `ArticleList`、`Reader`、`ExportArticlesModal`。

### `icons.tsx` 改动

新增导出图标：

```ts
import { Download, Square, SquareCheck, ... } from 'lucide-react';

export { Download, Square, SquareCheck, ... };
```

## 数据流与内容来源

| 内容来源        | 处理方式 | 说明                    |
| ----------- | ---- | --------------------- |
| `desc`      | 直接导出 | MVP 默认来源，数据库已有，秒出结果   |
| readability | 不采用  | 需逐篇联网抓取，慢且不稳定，后续可作为增强 |
| 图片          | 不下载  | 保留原链接，减小文件体积和实现复杂度    |

## 关键文件清单

### 后端

* `go-server/internal/handlers/reader.go`：注册路由、实现 handler。

* `go-server/internal/services/reader.go`：新增导出查询、格式化、ZIP 生成方法。

### 前端

* `node-app/client/src/components/ExportArticlesModal.tsx`（新建）：导出范围和格式选择弹窗。

* `node-app/client/src/components/ArticleList.tsx`：多选模式、复选框、导出入口、右键菜单新增导出项。

* `node-app/client/src/components/Reader.tsx`：顶部工具栏新增单篇导出按钮。

* `node-app/client/src/components/icons.tsx`：新增 Download、Square、SquareCheck 图标。

* `node-app/client/src/App.tsx`：管理导出弹窗状态、多选状态、调用后端下载。

## 实现顺序建议

1. 后端：实现单篇 Markdown 导出 `GET /items/:id/export.md`。
2. 后端：实现批量查询与 ZIP/JSON 导出 `POST /items/export`。
3. 前端：新增 `ExportArticlesModal` 组件。
4. 前端：`ArticleList` 增加多选模式与导出入口。
5. 前端：`Reader` 增加单篇导出按钮。
6. 前端：`App.tsx` 串联状态与下载逻辑。
7. 验证与边界处理。

## 验证步骤

### 构建验证

```bash
# 后端
cd go-server
go build ./...

# 前端
cd node-app/client
npm run build
```

### 功能验证

1. **单篇导出**

   * 在 ArticleList 右键点击文章，选择“导出为 Markdown”。

   * 在 Reader 顶部点击下载按钮。

   * 确认下载的 `.md` 文件包含完整 frontmatter 和 `desc` 内容。

2. **当前列表导出**

   * 选择某个订阅源或文件夹，点击 ArticleList 头部“导出”。

   * 选择 Markdown 压缩包，确认下载 ZIP。

   * 解压检查文件命名、frontmatter 字段、内容完整性。

3. **JSON 导出**

   * 同样流程选择 JSON。

   * 确认结构包含 `version`、`exportedAt`、`count`、`items`。

4. **多选导出**

   * 点击“多选”，勾选若干文章，点击“导出选中”。

   * 确认 ZIP 内仅包含选中文章。

5. **全部收藏导出**

   * 在导出弹窗选择“全部收藏文章”。

   * 确认结果包含所有 `isStarred=true` 的文章。

6. **边界情况**

   * 空列表导出：导出按钮禁用并提示。

   * 标题含特殊字符：文件名安全无非法字符。

   * 同名文章：追加序号后缀。

   * 大数量导出：测试 500 篇以上，观察响应时间和内存占用。

7. **回归验证**

   * OPML 导入/导出保持正常。

   * 文章列表选择、阅读、收藏、标已读未受影响。

