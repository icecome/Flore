# 本地优先 RSS 阅读器功能补足增强方案

## 上下文

本项目是一款本地优先、无云端账户的 RSS 阅读器，采用 React + TypeScript 前端、Go + Node.js 后端、SQLite 数据库。经过产品审查，当前核心阅读闭环已通，但在信息检索、阅读沉浸感、效率操作、数据维护等方面存在明显短板。

本方案坚持以下约束：

* **不上云、不增加账户体系**

* **不处理新手引导和首次空状态**（按需求排除）

* 优先补足 RSS 阅读器核心体验，不做推荐算法、社交、AI 总结等增值模块

## 设计原则

| 原则    | 含义               | 对功能选择的影响                    |
| ----- | ---------------- | --------------------------- |
| 数据主权  | 用户数据完全在本地 SQLite | 用本地 FTS 替代云端搜索，用本地文件备份替代云同步 |
| 离线可用  | 已抓取内容可随时阅读       | 阅读模式缓存、图片本地化、PWA 离线包        |
| 效率至上  | 键盘驱动、信息降噪        | 快捷键闭环、规则过滤、批量操作             |
| 低维护成本 | 源失效可感知、可清理       | Feed 健康度检测、死源标记             |
| 沉浸式阅读 | 排版可控、干扰最小        | 字体/字号/行距/主题/专注模式            |

## 功能补足矩阵

### P0：必须补足（没有则产品不完整）

| 功能         | 解决的问题      | 用户感知价值 | 技术成本                       |
| ---------- | ---------- | ------ | -------------------------- |
| 全文搜索       | 找不到历史文章    | ⭐⭐⭐⭐⭐  | 中：SQLite FTS5 或本地索引        |
| 阅读模式缓存     | 二次打开慢、重复抓取 | ⭐⭐⭐⭐⭐  | 中：新增 `readability_cache` 表 |
| 快捷键真正可用    | 设置里展示但不可用  | ⭐⭐⭐⭐   | 低：全局键盘事件监听                 |
| Feed 健康度检测 | 死源堆积、用户不知道 | ⭐⭐⭐⭐   | 低：抓取失败计数 + 最后更新时间          |

### P1：显著提升体验

| 功能         | 解决的问题     | 用户感知价值 | 技术成本                    |
| ---------- | --------- | ------ | ----------------------- |
| 字体/排版调节    | 阅读疲劳、个性化弱 | ⭐⭐⭐⭐   | 低：CSS 变量 + localStorage |
| 稍后阅读列表     | 收藏与待读混淆   | ⭐⭐⭐⭐   | 低：新增 `isReadLater` 字段   |
| 规则过滤       | 信息过载、噪音多  | ⭐⭐⭐⭐   | 中：关键词包含/排除规则            |
| 响应式布局      | 移动端不可用    | ⭐⭐⭐⭐   | 中：三栏改可折叠/单栏             |
| 本地数据备份/恢复  | 数据安全感     | ⭐⭐⭐⭐   | 低：导出/导入 SQLite 文件或 ZIP  |
| **文章粒度备份** | 指定文章归档    | ⭐⭐⭐⭐   | 中：Markdown/JSON/ZIP 导出  |

### P2：差异化增强

| 功能        | 解决的问题      | 用户感知价值 | 技术成本                        |
| --------- | ---------- | ------ | --------------------------- |
| 阅读统计      | 阅读成就感、习惯洞察 | ⭐⭐⭐    | 中：聚合阅读时长/数量                 |
| 重复内容合并    | 多源转载重复     | ⭐⭐⭐    | 中：标题/链接相似度计算                |
| OPML 自动备份 | 防止误删源      | ⭐⭐⭐    | 低：定时导出 OPML 到指定目录           |
| PWA 离线安装  | 更像原生应用     | ⭐⭐⭐    | 中：Service Worker + manifest |

## 分阶段路线图

### 阶段一：基础体验闭环（2–3 周）

目标：让产品从“可用”变成“顺手”。

1. **快捷键闭环**

   * 在 `App.tsx` 或 `useKeyboardShortcuts` hook 中监听全局 `keydown`。

   * 实现 ↑/↓/Enter/M/S/R/?/Esc。

   * 输入框聚焦时禁用快捷键。

2. **阅读模式缓存**

   * 新增 `readability_cache` 表：`itemId`, `title`, `content`, `excerpt`, `byline`, `cachedAt`。

   * 首次进入阅读模式时缓存全文，后续直接读缓存。

   * 提供刷新缓存按钮，可配置缓存过期时间（如 7 天）。

3. **Feed 健康度检测**

   * `Source` 表新增：`lastFetchAt`, `lastSuccessAt`, `fetchFailCount`, `lastError`。

   * 健康状态：正常/警告/失效。

   * UI：侧边栏状态点，设置中“清理失效源”。

### 阶段二：信息检索与降噪（2–3 周）

目标：解决信息焦虑者的核心痛点。

1. **全文搜索**

   * SQLite 启用 FTS5 虚拟表，与 `Item` 表同步。

   * 接口：`GET /items/search?q=keyword&limit=50`。

   * 前端：文章列表顶部搜索框，结果高亮。

2. **稍后阅读**

   * `Item` 表新增 `isReadLater`。

   * 文章列表/阅读区增加“稍后阅读”按钮。

   * 侧边栏新增“稍后阅读”入口。

3. **字体/排版调节**

   * 阅读区增加 Aa 面板：字体、字号、行高、页面宽度。

   * 设置保存到 `localStorage` 或 `settings` 表。

### 阶段三：数据主权与归档（2–3 周）

目标：强化本地阅读器的“内容可控”卖点。

1. **文章粒度备份/导出**（详见下文详细方案）
2. **本地数据全量备份/恢复**

   * 设置中“导出数据库”“导入数据库”。

   * 可选 OPML 自动备份到固定目录。

### 阶段四：高级本地能力（3–4 周）

1. 规则过滤
2. 响应式布局 / PWA
3. 阅读统计
4. 重复内容合并

## P0 详细方案

### 1. 快捷键闭环

**关键文件**：`node-app/client/src/App.tsx`

新增 `useKeyboardShortcuts` hook：

```ts
useEffect(() => {
  const handler = (e: KeyboardEvent) => {
    if (['INPUT', 'TEXTAREA'].includes((e.target as HTMLElement).tagName)) return;
    switch (e.key) {
      case 'ArrowUp': selectPrevItem(); break;
      case 'ArrowDown': selectNextItem(); break;
      case 'Enter': openSelectedItem(); break;
      case 'm': case 'M': toggleReadSelected(); break;
      case 's': case 'S': toggleStarSelected(); break;
      case 'r': case 'R': refreshItems(); break;
      case '?': showShortcutsHelp(); break;
      case 'Escape': closeModal(); break;
    }
  };
  window.addEventListener('keydown', handler);
  return () => window.removeEventListener('keydown', handler);
}, [...]);
```

### 2. 阅读模式缓存

**关键文件**：

* `go-server/internal/models/models.go`：新增 `ReadabilityCache` 模型

* `go-server/internal/services/reader.go`：`GetReadability` 先查缓存再抓取

* `go-server/internal/database/database.go`：`AutoMigrate` 加入新模型

```go
type ReadabilityCache struct {
  ItemID    int       `json:"itemId" gorm:"primaryKey"`
  Title     string    `json:"title"`
  Content   string    `json:"content"`
  Excerpt   string    `json:"excerpt"`
  Byline    string    `json:"byline"`
  CachedAt  time.Time `json:"cachedAt"`
}
```

### 3. Feed 健康度检测

**关键文件**：

* `go-server/internal/models/models.go`：`Source` 新增健康字段

* `go-server/internal/services/reader.go`：抓取后更新 `lastFetchAt`/`lastSuccessAt`/`fetchFailCount`

* `node-app/client/src/components/Sidebar.tsx`：显示状态点

状态规则：

* 正常：`lastSuccessAt` 在 7 天内且 `fetchFailCount` < 3

* 警告：`fetchFailCount` ≥ 3 或 7–30 天无更新

* 失效：连续 30 天无成功更新

## P1 详细方案：全文搜索（SQLite FTS5）

**关键文件**：

* `go-server/internal/models/models.go`：新增 `ItemSearch` FTS5 虚拟表模型

* `go-server/internal/services/reader.go`：新增 `SearchItems` 服务方法

* `go-server/internal/handlers/reader.go`：新增 `SearchItems` handler

* `node-app/client/src/components/SearchBox.tsx`：新增搜索框组件

* `node-app/client/src/App.tsx`：集成搜索状态与结果展示

### 数据模型

在 Prisma schema 的 `Item` 表基础上，由 Go 后端直接通过原生 SQL 维护 FTS5 虚拟表（Prisma 不直接支持 FTS5）：

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS ItemSearch USING fts5(
  title, content, itemId UNINDEXED,
  tokenize = 'porter unicode61'
);
```

Go 模型（仅用于触发 GORM 不迁移，实际表通过原生 SQL 创建）：

```go
type ItemSearch struct {
  ItemID  int    `json:"itemId" gorm:"column:itemId"`
  Title   string `json:"title" gorm:"column:title"`
  Content string `json:"content" gorm:"column:content"`
}

func (ItemSearch) TableName() string { return "ItemSearch" }
```

### 同步策略

FTS5 虚拟表需要与 `Item` 表保持同步。采用**写入时同步**策略：

1. Node 服务在 `source.ts` 中 `upsert` 文章时，同时调用 Go 后端 `POST /api/items/:id/index-search`。
2. Go 后端 `IndexItemForSearch(id int)` 方法：

   * 查询 `Item` 标题与 `desc` 内容。

   * 先 `DELETE FROM ItemSearch WHERE itemId = ?`。

   * 再 `INSERT INTO ItemSearch(title, content, itemId) VALUES (?, ?, ?)`。
3. 已有历史数据在首次启用搜索时执行一次性批量重建：`INSERT INTO ItemSearch(title, content, itemId) SELECT title, desc, id FROM Item`。

### 搜索接口

**请求**：`GET /api/items/search?q={keyword}&limit=50&offset=0`

**实现**：

```go
func (s *ReaderService) SearchItems(query string, limit, offset int) ([]models.ItemWithSource, int64, error) {
  // 使用 FTS5 查询语法：title:keyword OR content:keyword
  var ids []int
  s.db.Raw(
    "SELECT itemId FROM ItemSearch WHERE ItemSearch MATCH ? ORDER BY rank LIMIT ? OFFSET ?",
    query, limit, offset,
  ).Scan(&ids)

  // 统计总数
  var total int64
  s.db.Raw("SELECT count(*) FROM ItemSearch WHERE ItemSearch MATCH ?", query).Scan(&total)

  // 根据 ids 查询完整 Item 与 Source 信息
  // ...
}
```

**响应**：与现有 `/api/items` 列表结构一致，前端可复用 `ArticleList`。

### 前端集成

1. 在 `Sidebar` 上方或文章列表顶部放置搜索框 `SearchBox`。
2. 搜索时 `App.tsx` 进入「搜索模式」：

   * 清空 `selectedSourceId`、`selectedFolderId`、`filter`。

   * 调用 `/api/items/search` 加载结果到 `items`。

   * `ArticleList` 正常渲染，标题/摘要中关键词高亮。
3. 清空搜索框后退出搜索模式，恢复原有列表。

### 高亮实现

`ArticleList` 接收可选 `highlightKeyword` prop，对标题和摘要进行分词高亮：

```tsx
function highlight(text: string, keyword: string) {
  if (!keyword) return text;
  const parts = text.split(new RegExp(`(${escapeRegExp(keyword)})`, 'gi'));
  return parts.map((part, i) =>
    part.toLowerCase() === keyword.toLowerCase() ? <mark key={i}>{part}</mark> : part
  );
}
```

***

## P1 详细方案：稍后阅读列表

**关键文件**：

* `node-app/server/prisma/schema.prisma`：`Item` 表新增 `isReadLater Boolean @default(false)`

* `node-app/server/src/services/item.ts`（或复用 `source.ts`）：新增标/取消稍后读方法

* `go-server/internal/models/models.go`：`Item` 结构体新增 `IsReadLater`

* `node-app/client/src/components/Sidebar.tsx`：新增「稍后阅读」入口

* `node-app/client/src/components/Reader.tsx`：新增「稍后阅读」按钮

### 数据模型变更

```prisma
model Item {
  // ... 现有字段
  isReadLater Boolean @default(false)
}
```

对应 Go 模型：

```go
type Item struct {
  // ... 现有字段
  IsReadLater bool `json:"isReadLater" gorm:"column:isReadLater;default:false;index"`
}
```

### API 设计

Node 服务新增路由：

```ts
app.put('/items/:id/read-later', async (c) => {
  const id = Number(c.req.param('id'));
  const { isReadLater } = await c.req.json();
  const item = await prisma.item.update({
    where: { id },
    data: { isReadLater },
  });
  return c.json(item);
});
```

### 前端交互

1. **文章列表项**：在现有「收藏」按钮旁新增「稍后阅读」图标按钮（时钟图标）。
2. **阅读区顶部工具栏**：新增「稍后阅读」切换按钮。
3. **侧边栏**：在「收藏」上方新增「稍后阅读」入口，显示未读稍后读数量。

   * 点击后 `filter = 'readLater'`，等同于一个虚拟筛选条件。
4. **快捷键**：`L` 切换当前聚焦文章的稍后读状态。

### 与收藏的区别

| 维度    | 收藏（Starred） | 稍后阅读（Read Later） |
| ----- | ----------- | ---------------- |
| 语义    | 长期保存、有价值    | 临时待读、会清理         |
| 默认显示  | 独立入口        | 独立入口             |
| 阅读后行为 | 不自动移除       | 可设置「阅读后自动移除稍后读」  |

***

## P1 详细方案：字体/排版调节

**关键文件**：

* `node-app/client/src/utils/readerSettings.ts`：新增阅读设置工具

* `node-app/client/src/components/Reader.tsx`：集成排版控制面板

* `node-app/client/src/components/icons.tsx`：新增 `Type` 图标

### 可调项

| 项      | 范围                | 默认值   | 存储           |
| ------ | ----------------- | ----- | ------------ |
| 字体族    | 系统无衬线 / 系统衬线 / 等宽 | 系统无衬线 | localStorage |
| 字号     | 14px–24px         | 16px  | localStorage |
| 行高     | 1.4–2.0           | 1.8   | localStorage |
| 内容最大宽度 | 600px–900px       | 720px | localStorage |
| 主题     | 浅色 / 深色 /  sepia  | 浅色    | localStorage |
| 字间距    | 默认 / 宽松           | 默认    | localStorage |

### 实现方式

阅读区容器使用 CSS 变量，根据设置动态生成 style：

```tsx
const readerStyle: React.CSSProperties = {
  '--reader-font-family': fontFamily,
  '--reader-font-size': `${fontSize}px`,
  '--reader-line-height': lineHeight,
  '--reader-max-width': `${maxWidth}px`,
  '--reader-letter-spacing': letterSpacing,
} as React.CSSProperties;
```

CSS：

```css
.reader-content {
  font-family: var(--reader-font-family);
  font-size: var(--reader-font-size);
  line-height: var(--reader-line-height);
  max-width: var(--reader-max-width);
  letter-spacing: var(--reader-letter-spacing);
}
```

### 控制面板

在阅读区标题栏新增 `Aa` 按钮，点击后弹出轻量下拉面板：

* 顶部一排字体预览按钮。

* 字号滑块。

* 行高滑块。

* 宽度滑块。

* 主题色块（白/黑/sepia）。

面板状态保存到 `localStorage` key：`rss-reader-settings`，与现有 `AppSettings` 合并管理。

***

## P1 详细方案：本地数据全量备份/恢复

**关键文件**：

* `go-server/internal/handlers/reader.go`：新增 `ExportDatabase` / `ImportDatabase`

* `node-app/client/src/components/SettingsModal.tsx`：新增备份/恢复入口

### 备份机制

1. **数据库文件导出**：

   * Go 后端读取当前连接的 SQLite 文件路径。

   * 使用 `VACUUM INTO '/tmp/rss-backup-xxx.db'` 生成一致快照（避免拷贝时 WAL 不一致）。

   * 接口返回 `application/octet-stream`，文件名 `rss-backup-YYYY-MM-DD-HHmmss.db`。

2. **OPML 自动备份（可选）**：

   * 设置中可开启「自动导出 OPML 到指定目录」。

   * 每次订阅源变更时，Node 服务将 OPML 写入用户指定路径。

### 恢复机制

**请求**：`POST /api/database/restore`，`Content-Type: multipart/form-data`，字段 `file`。

**流程**：

1. 前端选择 `.db` 文件上传。
2. Go 后端保存到临时文件。
3. 校验 SQLite 文件有效性（尝试 `PRAGMA schema_version`）。
4. 关闭当前数据库连接，替换原数据库文件。
5. 重新初始化数据库连接，返回 `{ ok: true }`。
6. 前端提示用户刷新页面以重新加载数据。

### 安全与回滚

* 恢复前自动备份当前数据库到 `dev.db.bak.YYYY-MM-DD-HHmmss`。

* 若校验失败，不替换原文件，返回错误。

* 恢复后清空 Go 后端缓存（如 `ReadabilityCache` 内存缓存，如果有的话）。

***

## P1 详细方案：文章粒度备份/导出

### 新增路由

在 `go-server/internal/handlers/reader.go` 中注册：

```go
api.POST("/items/export", h.ExportItems)
api.GET("/items/:id/export.md", h.ExportItemMarkdown)
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

优先级：`ids` > `sourceId` > `folderId` > `starred`/`unread` 组合。

**响应**：

* `format=markdown`：`application/zip`，文件名 `articles-YYYY-MM-DD.zip`

* `format=json`：`application/json`，文件名 `articles-YYYY-MM-DD.json`

### 单篇导出接口

**请求**：`GET /api/items/:id/export.md`

**响应**：`text/markdown`，文件名 `YYYY-MM-DD-title-slug.md`

### 后端服务方法

在 `go-server/internal/services/reader.go` 新增：

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

ZIP 生成使用 Go 标准库 `archive/zip`，直接流式写入响应。

### Markdown 文件格式

```markdown
---
title: "文章标题"
author: "作者"
link: "https://example.com/article"
source: "来源名称"
sourceUrl: "https://source.com"
pubDate: "2026-07-26T10:00:00+08:00"
isRead: true
isStarred: false
---

<article desc HTML>
```

### 前端组件：`ExportArticlesModal.tsx`

位置：`node-app/client/src/components/ExportArticlesModal.tsx`

界面：

* 导出范围：当前筛选结果 / 当前文件夹 / 当前订阅源 / 全部收藏 / 已选中的 N 篇

* 导出格式：Markdown 压缩包 / JSON

* 底部提示：导出内容为文章摘要 HTML，不下载图片

### `ArticleList.tsx` 改动

* 新增多选模式：`multiSelectMode`, `selectedIds`

* 头部增加“多选”“导出”按钮

* 多选模式下条目显示复选框，点击切换选中

* 右键菜单新增“导出为 Markdown”

### `Reader.tsx` 改动

顶部操作栏新增下载按钮，导出当前文章。

### `icons.tsx` 改动

新增 `Download`, `Square`, `SquareCheck` 图标导出。

### `App.tsx` 改动

* 管理导出弹窗状态、多选状态

* `handleExportArticles(scope, format)`：调用 `POST /api/items/export`

* `handleExportSingleItem(item)`：调用 `GET /api/items/:id/export.md`

## P2 详细方案：规则过滤

**关键文件**：

* `node-app/server/prisma/schema.prisma`：新增 `FilterRule` 模型

* `node-app/server/src/services/filter.ts`：过滤规则引擎

* `node-app/server/src/services/source.ts`：抓取后应用规则

* `node-app/client/src/components/SettingsModal.tsx`：规则管理 UI

### 数据模型

```prisma
model FilterRule {
  id        Int      @id @default(autoincrement())
  name      String
  type      String   // "include" | "exclude"
  field     String   // "title" | "desc" | "author" | "link"
  pattern   String   // 关键词或正则
  isRegex   Boolean  @default(false)
  sourceId  Int?     // null 表示全局规则
  folderId  Int?     // null 表示全局规则
  active    Boolean  @default(true)
  createdAt DateTime @default(now())
  updatedAt DateTime @updatedAt
}
```

### 规则引擎

```ts
function applyFilterRules(item: Item, rules: FilterRule[]): boolean {
  // 排除规则优先：任一排除规则命中 → 丢弃
  for (const rule of rules.filter((r) => r.type === 'exclude')) {
    if (matchRule(item, rule)) return false;
  }
  // 包含规则：如果存在包含规则，则必须命中至少一条
  const includeRules = rules.filter((r) => r.type === 'include');
  if (includeRules.length > 0) {
    return includeRules.some((rule) => matchRule(item, rule));
  }
  return true;
}
```

### 执行时机

1. **抓取时过滤**：`doFetchSource` 中拿到 items 后，先应用规则，再写入数据库。
2. **事后过滤**：用户新增/修改规则后，可触发「对历史文章重新应用规则」，将命中排除规则的文章标记为已读或隐藏。

### 前端交互

* 设置中新增「规则过滤」标签页。

* 表格展示规则列表：名称、类型、字段、匹配内容、作用范围。

* 支持新增/编辑/删除/启用/禁用。

* 提供「测试规则」功能：输入示例标题，显示命中结果。

***

## P2 详细方案：响应式布局 / PWA

**关键文件**：

* `node-app/client/src/App.tsx`：布局状态管理

* `node-app/client/src/components/Sidebar.tsx`、`ArticleList.tsx`、`Reader.tsx`：响应式样式

* `node-app/client/public/manifest.json`：PWA 配置

* `node-app/client/vite.config.ts`（或 `public/sw.js`）：Service Worker

### 三栏布局的移动端适配

当前桌面端为三栏：侧边栏（固定 260px）+ 文章列表（固定 360px）+ 阅读区（弹性）。移动端改为单视图栈：

```
视图状态：folder-list | article-list | reader
```

* 默认显示「订阅源/文件夹列表」。

* 点击文件夹/订阅源 → 进入文章列表。

* 点击文章 → 进入阅读区。

* 阅读区/文章列表左上角返回按钮返回上一级。

### 状态管理

在 `App.tsx` 中新增 `mobileView: 'folders' | 'articles' | 'reader'`：

```ts
const [mobileView, setMobileView] = useState<'folders' | 'articles' | 'reader'>(() =>
  window.innerWidth < 768 ? 'folders' : 'articles'
);
```

选择文章时，窄屏下自动切换到 `reader`，宽屏下保持三栏。

### CSS 媒体查询

```css
@media (max-width: 768px) {
  .app-layout {
    display: block;
  }
  .sidebar, .article-list, .reader {
    width: 100%;
    height: 100vh;
  }
  .sidebar.hidden, .article-list.hidden {
    display: none;
  }
}
```

### PWA 最小实现

1. `manifest.json`：名称、图标、主题色、`display: standalone`。
2. Service Worker：仅缓存静态资源（`index.html`、`js`、`css`、字体），不缓存文章内容（避免内容过期）。
3. 图标：提供 192x192 和 512x512 PNG。

***

## 取舍说明

### 不做

* 云端账户与同步

* 社交分享/评论

* AI 总结生成

* 推荐算法

* 新手引导 / 预置源

### 优先做

* 全文搜索 + 阅读模式缓存：本地阅读器的核心武器

* 快捷键 + Feed 健康检测：把展示型功能变成可用功能

* 文章粒度备份：强化“内容主权”卖点

## 验证策略

### 构建验证

```bash
cd go-server && go build ./...
cd node-app/client && npm run build
```

### 功能验证清单

1. 快捷键：↑/↓ 切换文章，Enter 打开，M 标已读，S 收藏，R 刷新，? 显示帮助。
2. 阅读模式缓存：同一文章二次打开秒开，刷新缓存后内容更新。
3. Feed 健康度：模拟抓取失败，侧边栏显示黄色/红色状态点。
4. 全文搜索：输入关键词，结果包含标题/正文匹配项。
5. 稍后阅读：将文章加入稍后读，侧边栏入口显示正确数量。
6. 文章粒度导出：

   * 单篇导出 `.md` 文件

   * 多选导出 ZIP

   * 全部收藏导出 ZIP

   * JSON 导出结构正确
7. 本地全量备份：导出 SQLite 文件，导入后数据恢复完整。
8. 回归验证：OPML 导入/导出、文章列表操作、阅读、收藏、标已读未受影响。

