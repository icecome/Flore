# Code Reuse Ladder 分析报告

> 分析时间：2026-08-01
> 分析范围：apps/web/src、server/go/internal、apps/routing-tool/src
> 分析方法：Code Reuse Ladder（7 步决策框架）

---

## 分析结果汇总

| # | 代码单元 | 位置 | 结论 | 优先级 |
|---|---------|------|------|--------|
| 1 | urlRe 正则重复 | handlers/reader.go:1403, 1591, 1712, 1735 | REUSE_EXISTING | HIGH |
| 2 | cheerio/AnyNode 重复导入 | sites/chinawriter.ts, sites/rss-feed.ts | REUSE_EXISTING | MEDIUM |
| 3 | escapeSQLitePath 分散定义 | backup.go, backup_restore.go | REUSE_EXISTING_DEPRECATE | MEDIUM |
| 4 | escapeSQLWildcards 分散定义 | reader.go (未导出) | IMPLEMENT_MINIMAL | LOW |
| 5 | ErrorBoundary 使用 | main.tsx | REUSE_EXISTING ✓ | — |
| 6 | cn() 工具函数 | lib/cn.ts | REUSE_EXISTING ✓ | — |
| 7 | showToast 工具函数 | utils/toast.ts | REUSE_EXISTING ✓ | — |
| 8 | date format 函数 | lib/cn.ts | REUSE_EXISTING ✓ | — |
| 9 | icons.tsx re-export | components/icons.tsx | REUSE_EXISTING ✓ | — |

---

## 详细分析

### 1. urlRe 正则重复定义 — HIGH

**位置**：`server/go/internal/handlers/reader.go`
- 行 1403：`urlRe := regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)`
- 行 1591：`urlRe := regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)`
- 行 1712：`urlRe := regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)`
- 行 1735：`urlRe := regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)`

**现状**：相同的正则表达式在 4 处局部定义，每次调用时重复编译。

**Ladder 结果**：Step 2 — REUSE_EXISTING

**复用证据**：包内已有 `linkRePre`、`hrefRePre`、`cspMetaRePre` 三个包级预编译正则（行 29-31），`urlRe` 应遵循相同模式。

**迁移步骤**：
1. 在包级变量区（行 29-31 附近）添加：
   ```go
   urlRePre = regexp.MustCompile(`url\(\s*["']?([^"'\s\)]+)["']?\s*\)`)
   ```
2. 将 4 处 `urlRe := regexp.MustCompile(...)` 替换为 `urlRe := urlRePre`

**迁移成本**：S（< 15 分钟）
**破坏性变更**：No
**测试要求**：现有测试应全部通过（行为完全不变）

---

### 2. cheerio/AnyNode 重复导入 — MEDIUM

**位置**：`apps/routing-tool/src/scrapers/sites/`
- `chinawriter.ts:3-4`：`import * as cheerio from 'cheerio'` + `import type { AnyNode } from 'domhandler'`
- `rss-feed.ts:4`：`import * as cheerio from 'cheerio'`（ofetch 和 cheerio 均重复）

**现状**：`base-fetcher.ts` 已导入这些依赖，但各 site 文件又各自导入了一次。

**Ladder 结果**：Step 2 — REUSE_EXISTING（通过 re-export）

**复用证据**：`base-fetcher.ts` 第 1-3 行已导入 `ofetch`、`cheerio`、`AnyNode`。

**迁移步骤**：
1. 在 `base-fetcher.ts` 末尾添加 re-export：
   ```typescript
   export { default as cheerio } from 'cheerio';
   export type { AnyNode } from 'domhandler';
   ```
2. `chinawriter.ts` 改为从 `../base-fetcher.js` 导入
3. `rss-feed.ts` 移除 `ofetch` 和 `cheerio` 的直接导入，从 `../base-fetcher.js` 导入

**迁移成本**：S（< 15 分钟）
**破坏性变更**：No（导入行为等价）
**测试要求**：TypeScript 编译验证即可

---

### 3. escapeSQLitePath 分散定义 — MEDIUM

**位置**：
- `server/go/internal/services/backup.go:22`
- 可能在 `backup_restore.go` 有类似逻辑（需确认）

**Ladder 结果**：Step 2 — REUSE_EXISTING_DEPRECATE

**复用证据**：`backup.go` 第 22 行定义了 `func escapeSQLitePath(path string) string`，在 `backup.go` 中被调用 2 次。

**迁移步骤**：
1. 创建 `server/go/internal/services/sqlutil.go`，存放 `escapeSQLitePath`
2. `backup.go` 移除本地定义，改为导入
3. 若 `backup_restore.go` 有类似逻辑，一并迁移

**迁移成本**：S（< 15 分钟）
**破坏性变更**：No
**测试要求**：go build + 现有测试通过

---

### 4. escapeSQLWildcards 未导出 — LOW

**位置**：`server/go/internal/services/reader.go:1037`

**Ladder 结果**：Step 7 — IMPLEMENT_MINIMAL

**说明**：该函数仅在 `reader.go` 内部使用（行 1104），目前无需提取。若后续其他服务需要类似功能，可参考本函数的实现提取到 `sqlutil.go`。

**迁移成本**：L（> 1 人天，非紧急）
**破坏性变更**：No

---

### 5. ErrorBoundary — 已正确复用 ✓

**位置**：`apps/web/src/main.tsx:5,26`
- 唯一导入点，包裹整个 App，符合 React 最佳实践

---

### 6. cn() 工具函数 — 已正确复用 ✓

**位置**：`apps/web/src/lib/cn.ts`
- 被 12+ 组件文件导入使用，分布合理

---

### 7. showToast 工具函数 — 已正确复用 ✓

**位置**：`apps/web/src/utils/toast.ts`
- 被 App.tsx 及多个组件导入使用，分布合理

---

### 8. 日期格式函数 — 已正确复用 ✓

**位置**：`apps/web/src/lib/cn.ts`
- `formatDate`、`formatTime`、`formatRelative`、`formatFull` 均已集中定义并按需导入

---

### 9. icons.tsx re-export — 已正确复用 ✓

**位置**：`apps/web/src/components/icons.tsx`
- 统一从 `lucide-react` re-export，所有组件从此导入，符合 AGENTS.md 规范

---

## 执行计划

### 立即执行（HIGH + MEDIUM）

| 任务 | 文件 | 预估耗时 | 状态 |
|------|------|---------|------|
| 提取 urlRePre 到包级变量 | handlers/reader.go | 5 min | ✓ 已完成 |
| re-export cheerio/AnyNode | base-fetcher.ts | 5 min | ✓ 已完成 |
| 更新 sites 导入 | chinawriter.ts, rss-feed.ts | 5 min | ✓ 已完成 |
| 提取 escapeSQLitePath | sqlutil.go（新建） | 10 min | ⏭ 跳过（仅在 backup.go 内部使用） |

### 后续考虑（LOW）

| 任务 | 说明 |
|------|------|
| escapeSQLWildcards 提取 | 非紧急，待有其他调用方时再提取 |
| 路由工具导入循环文档 | AGENTS.md 已记录，无需代码修改 |

---

## 执行记录（2026-08-01）

### 已完成

1. **urlRePre 提取**（`server/go/internal/handlers/reader.go`）
   - 在包级变量区新增 `urlRePre`，替代 4 处重复的 `regexp.MustCompile(...)` 调用
   - 验证：`go build ./...` ✓，`go vet ./...` ✓

2. **cheerio/AnyNode re-export**（`apps/routing-tool/src/scrapers/base-fetcher.ts`）
   - 添加 `export { ofetch } from 'ofetch'`、`export { cheerio }`、`export type { AnyNode } from 'domhandler'`
   - 更新 `chinawriter.ts`：合并为单行导入 `import { BaseFetcher, type ScrapeResult, cheerio, type AnyNode } from '../base-fetcher.js'`
   - 更新 `rss-feed.ts`：移除冗余的 `ofetch` 和 `cheerio` 直接导入，从 `base-fetcher.js` 导入

---

## 规范遵循情况

| 规范项 | 状态 | 说明 |
|--------|------|------|
| lucide-react 统一导入 | ✓ | 所有组件通过 icons.tsx 导入 |
| cn() 集中工具 | ✓ | 在 lib/cn.ts 定义并按需导入 |
| toast 集中管理 | ✓ | 在 utils/toast.ts 定义 |
| Go 正则预编译 | ⚠ | urlRe 未遵循包级预编译约定 |
| Go 错误包装 | ✓ | 所有 error 使用 %w 包装 |
| TypeScript 类型导入 | ✓ | 使用 import type |
