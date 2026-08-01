# 书签收集器设计方案

## 背景

用户希望将**在其他渠道看到的文章**，通过分享/复制链接等动作，收集到本项目的稍后阅读和收藏中，统一在本项目查看。

---

## 1. 现网案例调研

### 主流方案对比

| 产品 | 收集方式 | 关键技术 |
|------|----------|----------|
| **Inoreader** | 浏览器扩展（右键菜单）、Bookmarklet、"Save Page" 按钮 | 扩展注入 + REST API |
| **Pocket** | 浏览器扩展、Bookmarklet、分享菜单、邮件转发 | 专用 API + 浏览器 SDK |
| **Instapaper** | 浏览器扩展、Bookmarklet、分享菜单 | 扩展 + 内容提取 |
| **Feedly** | 浏览器扩展、"Save for later" | 扩展 + 侧栏 |
| **Corvus RSS** | 本地 Safari 扩展，分享菜单 | 原生集成 |

### 通用模式

```
用户浏览网页 ──→ 点击"保存到..." ──→ 发送 URL 到服务端
                                              │
                                              ▼
                                    服务端抓取文章内容
                                              │
                                              ▼
                                    提取正文（Readability）
                                              │
                                              ▼
                                    存入稍后阅读/收藏列表
```

### 收集入口类型

1. **浏览器扩展**（最主流）— 右键菜单 + 工具栏按钮
2. **Bookmarklet**（最轻量）— 拖到书签栏，点击即可
3. **系统分享菜单**（移动端）— iOS/Android Share Sheet
4. **手动输入 URL**（最基础）— 应用内输入框

---

## 2. 本项目可行性分析

### ✅ 可利用的现有能力

| 能力 | 复用程度 | 说明 |
|------|----------|------|
| Item 数据模型 | **完全可用** | 已有 `isReadLater`、`isStarred` 字段，数据库有索引 |
| `isReadLater`/`isStarred` 筛选 | **完全可用** | `GET /api/items?readLater=true` 已实现 |
| 侧边栏筛选入口 | **完全可用** | 侧边栏已有"稍后阅读""收藏"按钮 |
| `FetchReadability()` 全文提取 | **完全可用** | 已实现，可接受任意 URL 抓取正文 |
| `upsertFeedItems()` 去重写入 | **可复用** | 可提取为通用 upsert 单篇文章 |
| 文章列表/阅读器展示 | **完全可用** | 现有 UI 可直接展示收集的文章 |
| 导出功能 | **完全可用** | 支持 Markdown/JSON 导出收藏和稍后阅读 |

### 需要新增的能力

| 能力 | 工作量 | 说明 |
|------|--------|------|
| **后端：添加单篇文章 API** | 小 | 新增 `POST /api/items/save`，接收 URL，调用 `FetchReadability()` 提取，创建 Item |
| **后端：专用"收集"来源** | 小 | 需要创建一个特殊的虚拟 Source 或允许 SourceID 为 0 的 Item |
| **前端：URL 输入弹窗** | 小 | 新增 URL 输入框 + 确认按钮的弹窗组件 |
| **前端：Bookmarklet 生成** | 极小 | 一个可拖拽到书签栏的链接 |
| **浏览器扩展** | 中 | 需要打包为浏览器扩展，可拆分到后续阶段 |

### 唯一需要决策的问题：收集的文章放在哪个"订阅源"下？

**方案 A：创建虚拟 Source**（推荐）
- 在 `sources` 表创建一个系统级虚拟 Source（如 `id=0` 或 `id=-1`，`title="手动收集"`）
- 所有通过 URL 手动添加的文章都关联到这个 Source
- 优点：不破坏现有数据模型，Item 关联 Source 的约束不变
- 缺点：需要处理 Source 的`active`、`fetch`等行为

**方案 B：允许 SourceID 为 NULL**
- 修改 Item 的 `source_id` 列为可空
- 优点：语义清晰，"无来源"的文章
- 缺点：需要修改 GORM 模型、查询和 JOIN 逻辑，影响面大

**方案 C：SourceID 设为 0**
- 在 Go 中 `int` 默认值为 0，不创建真实 Source 记录，所有查询中特殊处理
- 缺点：违反外键约束，GORM 关联查询会出问题

**结论：选方案 A，创建虚拟 Source。**

---

## 3. 设计方案

### 3.1 后端

#### 数据层

```go
// 在 sources 表预置一条系统记录（数据库初始化时创建）
// id: 0 (或 1), title: "手动收集", type: "manual", active: true
```

#### Service 层 — 新增 `SaveArticleService`

```go
// 新增方法
func (s *ItemService) SaveArticleByURL(url string, opts SaveOptions) (*Item, error)
```

流程：
1. 调用 `FetchReadability(url)` 提取标题和正文
2. 构造 Item 对象，`SourceID` 指向虚拟 Source
3. 调用 `upsertSingleItem()` 按 `link` 去重写入
4. 根据 opts 设置 `IsReadLater` / `IsStarred` 初始状态
5. 异步建立 FTS5 搜索索引

#### Handler 层 — 新增路由

```
POST /api/items/save
Body: {
  "url": "https://...",
  "saveTo": "readLater" | "starred" | "both"  // 可选，默认 readLater
}
Response: { "id": 123, "title": "...", ... }
```

#### 验证逻辑

- URL 格式校验
- 重复检测（已存在相同 link 的 Item 返回已有记录）
- 内容提取失败时返回清晰错误

### 3.2 前端

#### 新增"添加链接"入口

在侧边栏底部或工具栏新增"添加链接"按钮：

```
[App.tsx] 新增状态:
  showAddLinkModal: boolean

[Sidebar.tsx] 或工具栏:
  新增按钮 → 点击打开 AddLinkModal
```

#### AddLinkModal 组件

```
┌──────────────────────────────┐
│  添加链接到稍后阅读           │
│                              │
│  ┌────────────────────────┐  │
│  │ 粘贴文章链接...         │  │
│  └────────────────────────┘  │
│                              │
│  ○ 稍后阅读  ○ 收藏  ○ 两者  │
│                              │
│  [取消]        [确认添加]    │
└──────────────────────────────┘
```

#### 交互流程

1. 用户粘贴 URL → 点击"确认添加"
2. 前端调用 `POST /api/items/save`
3. 服务端抓取并提取内容
4. 返回成功 → 显示 Toast 提示
5. 列表自动刷新（如果当前在稍后阅读/收藏视图）

### 3.3 Bookmarklet（轻量级收集方案）

生成一段 JavaScript 代码，用户拖到浏览器书签栏：

```javascript
javascript:(function(){
  window.open(
    'https://reader.example.com/add?url=' + encodeURIComponent(location.href),
    '_blank',
    'width=500,height=300'
  );
})();
```

点击书签 → 打开 Reader 的添加页面，自动填入当前 URL。

### 3.4 浏览器扩展（进阶方案，后续阶段）

| 功能 | 说明 |
|------|------|
| 右键菜单 | 右键任意链接 → "保存到 Flore 稍后阅读" |
| 工具栏图标 | 点击保存当前页面 |
| 快捷键 | 如 Ctrl+Shift+S 保存当前页面 |

---

## 4. 实施路线图

### 阶段一：核心功能（MVP）

| 任务 | 文件 | 预估 |
|------|------|------|
| 数据库初始化添加虚拟 Source | `models/models.go` | 小 |
| 后端 `SaveArticleByURL` Service | `services/reader.go` | 中 |
| 后端 `POST /api/items/save` Handler | `handlers/reader.go` | 小 |
| 前端 AddLinkModal 组件 | `components/AddLinkModal.tsx` | 中 |
| 前端侧边栏/工具栏入口 | `App.tsx` + `Sidebar.tsx` | 小 |
| TypeScript + Go 验证 | 全局 | 小 |

### 阶段二：体验完善

| 任务 | 说明 |
|------|------|
| 去重优化 | 已有链接时显示"已存在"并跳转 |
| 批量添加 | 一次粘贴多个 URL |
| 添加后的即时反馈 | 列表自动刷新、计数更新 |

### 阶段三：外部收集

| 任务 | 说明 |
|------|------|
| Bookmarklet 生成器 | 在设置页生成可拖拽书签 |
| 浏览器扩展 | 右键菜单 + 工具栏按钮 |
| 系统分享集成 | 桌面端 Wails 分享菜单接收 |

---

## 5. 关键设计决策

### 5.1 为什么选虚拟 Source 方案

- 最小化数据模型改动，不破坏现有查询逻辑
- 侧边栏按 Source 分组时，"手动收集"作为一个独立分组展示
- 虚拟 Source 的 `fetch` 操作直接返回空（无需抓取 RSS）

### 5.2 内容提取策略

```go
// 优先顺序：
1. 调用 FetchReadability(url) 提取正文
2. 提取失败时，仅保存 title + link，desc 留空
3. 用户可在阅读器中切换到"阅读模式"重新提取
```

### 5.3 去重策略

```go
// 按 link 唯一索引去重：
//   - 已存在相同 link → 返回已有记录 + 更新状态（isReadLater/isStarred）
//   - 不存在 → 创建新记录
```

### 5.4 隐私考虑

- 所有收集的文章仅存储在本地数据库
- 无外部服务依赖
- 虚拟 Source 不触发 RSS 抓取

---

## 6. 项目现状评估

| 维度 | 评估 |
|------|------|
| 后端能力 | 除缺少"单篇添加 API"外，其余能力（内容提取、筛选、展示）均完备 |
| 前端能力 | 列表展示、筛选、阅读器均完备，仅缺少收集入口 UI |
| 数据模型 | 状态字段已就绪，只需处理 Source 归属问题 |
| 外部扩展 | Bookmarklet 极小成本，浏览器扩展需额外开发 |
| **总体评估** | **核心功能可在 1-2 天内完成，方案可行** |