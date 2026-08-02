# 订阅源页面 — 结构分析与整改方案

> 参照数据管理页的整改思路，对订阅源设置页进行统一布局重构。

---

## 一、现状分析

### 1.1 当前布局

```
┌─ 顶部工具栏 ──────────────────────────────────────────────┐
│ [检测可用性] [▼全部 ▼] | [＋] [↑] [↓]                    │
└───────────────────────────────────────────────────────────┘

┌─ 表格 ────────────────────────────────────────────────────┐
│ ☐  名称            订阅日期      状态                      │
│ ☐  Source Name     2026-01-01   正常                      │
│     https://...                                           │
│ ☐  Source Name2    2026-01-02   超时 (失败3次)            │
│     https://...                                           │
└───────────────────────────────────────────────────────────┘

┌─ 底部工具栏 ──────────────────────────────────────────────┐
│ [隐藏] [重命名] [删除]           3 项已选中                │
└───────────────────────────────────────────────────────────┘
```

### 1.2 现有问题

| 问题 | 说明 |
|------|------|
| **布局不统一** | 与其他设置页（数据管理/通用/外观）使用不同的 Section+Row 布局范式 |
| **全高撑满** | 使用 `flex h-full` 撑满弹窗，导致列表内容过多时底部工具栏溢出 |
| **样式不一致** | 使用原生 `<input type="checkbox">` 而非项目的 Toggle/Row 组件 |
| **操作路径分散** | 顶部工具栏 + 表格行内操作 + 底部工具栏，操作入口分散 |
| **行内编辑突兀** | 在表格行内直接展开编辑表单，表格行高突变，视觉效果差 |
| **缺少文件夹信息** | 表格列缺少 `folderId` 对应的文件夹名展示 |
| **状态指示冗余** | 底部「隐藏」按钮 + 表格行内无直接操作，需通过选择+点击组合操作 |

### 1.3 数据管理页的整改模式（参考模版）

```
┌─ 标题 ───────────────────────────────────────────────────┐
│ h3 标题 + border-b                                        │
├───────────────────────────────────────────────────────────┤
│ 顶部操作栏：primary 按钮 + secondary 按钮 + 刷新按钮      │
├───────────────────────────────────────────────────────────┤
│ 表格：grid-cols-12 表头 + 数据行，hover 高亮              │
│ 行内操作：icon buttons（全部恢复/仅设置/仅订阅/导出/删除）│
├───────────────────────────────────────────────────────────┤
│ 策略配置：Row 布局（左标题+描述，右 Toggle/Select/Input） │
│ 清理按钮：独立于行列表之外，mt-4                          │
└───────────────────────────────────────────────────────────┘
```

---

## 二、整体项目设计规范

### 2.1 设置页布局规范

```
弹窗：920px 宽，740px 高
左侧导航：200px，浅灰背景 (bg-canvas)
右侧内容区：flex-1，白色背景 (bg-surface)
内容间距：px-6 py-5
```

### 2.2 组件规范

| 组件 | 规范 |
|------|------|
| **Section** | h3 标题 14px font-semibold + border-b border-border-subtle + mb-3 |
| **Row** | flex items-center justify-between, py-3, border-b border-border-subtle, gap-1 |
| 行左侧 | flex flex-col: 标题 13px text-primary + 描述 mt-0.5 text-[12px] text-muted leading-snug |
| 行右侧 | ml-4 shrink-0, 放置 Toggle/Select/NumberInput |
| **Toggle** | 40x22px, rounded-full, bg-primary/bg-border-strong, 白色滑块 16x16px |
| **Table** | border border-border rounded-lg overflow-hidden, 表头 bg-elevated |
| 表头行 | grid grid-cols-12, 11.5px font-semibold text-muted uppercase, px-3 py-2.5 |
| 数据行 | grid grid-cols-12, 13px text-primary, px-3 py-2.5, hover:bg-hover, border-b border-border-subtle |
| 行内按钮 | p-1 rounded, hover:bg-primary/10 hover:text-primary, danger: hover:bg-danger/10 hover:text-danger |
| **SmallBtn** | px-3 py-1.5, 12.5px, border border-border rounded-md, bg-surface text-secondary, hover:bg-hover |
| **IconBtn** | h-7 w-7, rounded-md, text-secondary hover:bg-hover hover:text-primary |

---

## 三、订阅源页面整改方案

### 3.1 整体布局

```
┌─ 订阅源 ──────────────────────────────────────────────────┐
│ h3 标题 + border-b                                         │
├───────────────────────────────────────────────────────────┤
│ 顶部操作栏：                                               │
│ [检测可用性] [▼ 全部 ▼] [＋] [↑导入] [↓导出]             │
├───────────────────────────────────────────────────────────┤
│ 表格：grid-cols-12                                         │
│ ┌───────────────────────────────────────────────────────┐ │
│ │ ☐  名称/URL          文件夹     订阅日期    状态    操作 │ │
│ ├───────────────────────────────────────────────────────┤ │
│ │ ☐  Source Name       Folder1   2026-01-01  正常    [✎] │ │
│ │     https://example.com/rss                    [👁] [🗑] │ │
│ │ ☐  Source Name2      —         2026-01-02  超时    [✎] │ │
│ │     https://example2.com/rss                    [👁] [🗑] │ │
│ └───────────────────────────────────────────────────────┘ │
├───────────────────────────────────────────────────────────┤
│ 底部操作栏：                                               │
│ [隐藏选中] [重命名] [删除]  3 项已选中                     │
└───────────────────────────────────────────────────────────┘
```

### 3.2 表格列定义

| 列 | span | 说明 |
|----|------|------|
| 选择框 | col-span-1 | 全选/单选 checkbox |
| 名称/URL | col-span-4 | 两行：名称（13px font-medium）+ URL（11px text-muted truncate） |
| 文件夹 | col-span-2 | 显示 folderName 或「—」 |
| 订阅日期 | col-span-2 | 12px text-secondary |
| 状态 | col-span-1 | 状态 badge（正常/超时/未检测） |
| 操作 | col-span-2 | 行内 icon buttons：编辑、隐藏、删除 |

### 3.3 操作按钮说明

**顶部工具栏**（与数据管理页一致的行内按钮风格）：

| 按钮 | 类型 | 说明 |
|------|------|------|
| 检测可用性 | SmallBtn + RefreshCw 图标 | 点击后所有行状态变为「检测中」 |
| 筛选下拉 | Select 控件 | 全部 / 可用 / 超时 |
| 添加订阅源 | IconBtn + Plus 图标 | 打开添加订阅源弹窗 |
| 导入 OPML | IconBtn + Upload 图标 | 桌面端：PickOPMLFile；Web 端：file input |
| 导出 OPML | IconBtn + Download 图标 | 桌面端：SaveOPMLFile；Web 端：下载 |

**行内操作**（每行右侧）：

| 按钮 | 图标 | 说明 |
|------|------|------|
| 编辑 | Pencil | 弹出编辑弹窗（非行内编辑），修改名称和 URL |
| 隐藏 | EyeOff | 切换 hideInTimeline，即时生效 |
| 删除 | Trash2 | 确认弹窗后删除 |

**底部操作栏**（批量操作，选中项 > 0 时显示）：

| 按钮 | 说明 |
|------|------|
| 批量隐藏 | 选中多个源统一隐藏 |
| 重命名 | 选中单个源时显示，弹出编辑弹窗 |
| 批量删除 | 确认弹窗，删除选中源 |
| 选中计数 | 显示「N 项已选中」 |

### 3.4 编辑弹窗（替代行内编辑）

当前行内编辑导致表格行高突变，改为弹窗编辑：

```
┌─ 编辑订阅源 ────────────────────────┐
│                                      │
│  名称                                │
│  [________________________]          │
│                                      │
│  RSS 链接                             │
│  [________________________]          │
│                                      │
│  [取消]  [保存]                       │
└──────────────────────────────────────┘
```

### 3.5 交互状态

| 状态 | 行样式 | 状态 badge |
|------|--------|------------|
| 正常 | 默认样式 | bg-success/10, text-success, 绿色圆点 |
| 超时(>=3次失败) | 名称 text-danger | bg-danger/10, text-danger, 红色圆点，title 显示失败次数和错误信息 |
| 未检测 | 默认样式 | bg-border/30, text-muted, 灰色圆点 |
| 检测中 | 全部禁用 | 旋转图标 +「检测中」文字 |
| 选中 | bg-primary-subtle | 勾选框选中 |
| 隐藏(hideInTimeline) | 名称 text-muted | 无特殊 badge，通过文字颜色暗示 |

---

## 四、需求优先级

### P0 — 布局统一（核心变更）

| 变更 | 文件 | 说明 |
|------|------|------|
| 移除 `flex h-full` 撑满布局 | SettingsSourcesTab.tsx | 改为固定高度内容区，与数据管理页一致 |
| 表格改为 grid-cols-12 | SettingsSourcesTab.tsx | 与备份列表一致的表格结构 |
| 新增「文件夹」列 | SettingsSourcesTab.tsx | 显示源所在文件夹名 |
| 行内操作列（编辑/隐藏/删除） | SettingsSourcesTab.tsx | 替换纯表格行，增加操作按钮 |
| 行内编辑改为弹窗编辑 | SettingsSourcesTab.tsx + SettingsModal.tsx | 新增 EditSourceModal 组件 |

### P1 — 交互优化

| 变更 | 说明 |
|------|------|
| 状态 badge 间距调整 | 统一 badge 内边距和字号 |
| 底部工具栏改为条件显示 | 仅选中项 > 0 时显示，无选中时隐藏 |
| 筛选下拉位置调整 | 跟随顶部工具栏，保持与检测按钮对齐 |

### P2 — 细节完善

| 变更 | 说明 |
|------|------|
| 空状态提示 | 无订阅源时显示「暂无订阅源，点击添加订阅源开始使用」 |
| 选中计数样式 | 与数据管理页一致的 12px text-muted |
| 按钮禁用状态 | 操作按钮在无选中或选中多个时正确禁用 |

---

## 五、实施步骤

### Step 1: 重写 SettingsSourcesTab.tsx

1. 移除 `flex h-full` 容器，改为普通 `div` 内容区
2. 顶部工具栏统一为 `flex items-center gap-2 mb-3` 布局
3. 表格改为 `grid grid-cols-12` 结构
4. 新增「操作」列（编辑/隐藏/删除）
5. 新增「文件夹」列
6. 表格容器改为 `rounded-lg border border-border overflow-hidden`
7. 底部工具栏改为条件渲染

### Step 2: 新增 EditSourceModal.tsx

1. 基于 ModalLayout 的编辑弹窗
2. 名称 + URL 两个输入框
3. 保存/取消按钮
4. 空的编辑状态管理

### Step 3: 更新 SettingsModal.tsx

1. 新增 `editModalSource` 状态
2. 新增 `openEditModal` / `closeEditModal` 方法
3. 将 `onStartRename` / `onConfirmRename` / `onCancelRename` 替换为弹窗逻辑
4. 传递 editModal 回调到 SettingsSourcesTab

### 验证清单

- [ ] `tsc --noEmit` 通过
- [ ] `vite build` 通过
- [ ] 表格 hover 高亮正确
- [ ] 编辑弹窗打开/关闭/保存正常
- [ ] 批量操作（隐藏/删除）正常
- [ ] 筛选下拉切换正常
- [ ] 检测可用性正常
- [ ] 导入/导出 OPML 正常