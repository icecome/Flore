# Flore RSS 阅读器 — 设计一致性分析报告

> 基于代码库分析生成 | 建议先阅读，不直接修改代码

---

## 一、项目设计现状概览

### 1.1 技术栈与样式方案

| 项目 | 内容 |
|------|------|
| 前端框架 | React 19 + Vite 6 + TypeScript 5 |
| 样式方案 | Tailwind CSS 3 + CSS 变量 (`--bg-*`, `--text-*`, `--primary` 等) |
| 图标库 | lucide-react (通过 `icons.tsx` 统一导出) |
| 桌面壳 | Wails v2.13+ |

### 1.2 品牌方向（来自设计上下文简报）

推荐 **"Obsidian + Apple" 混合风格**：
- Obsidian 负责"知识工具"身份认同：深色主题、紫色调、高密度信息
- Apple 负责"阅读体验"内容质感：留白、字体、圆角、动画

但当前代码实现与这一目标存在明显差距。

---

## 二、已发现的设计不一致问题

### ISSUE 1: 圆角系统未统一

CSS 变量定义了 `--radius-sm: 4px` / `--radius-md: 6px` / `--radius-lg: 8px`，但**组件中从未使用这些 CSS 变量**，而是混用 Tailwind 的圆角类。

| 元素 | 使用的圆角 | 实际值 |
|------|-----------|--------|
| CSS 变量 `--radius-sm` | 未使用 | 4px |
| CSS 变量 `--radius-md` | 未使用 | 6px |
| CSS 变量 `--radius-lg` | 未使用 | 8px |
| ModalLayout 关闭按钮 | `rounded-sm` | 2px |
| SettingsModal 关闭按钮 | `rounded-md` | 6px |
| 输入框 (SearchBox) | `rounded-md` | 6px |
| 输入框 (AddSourceModal) | `rounded-sm` | 2px |
| 输入框 (App.tsx 创建文件夹) | `rounded-sm` | 2px |
| 输入框 (RenameModal) | `rounded-sm` | 2px |
| 确认按钮 (ModalLayout) | `rounded-lg` | 8px |
| 确认按钮 (SettingsModal) | `rounded-md` | 6px |
| EmptyState 图标容器 | `rounded-2xl` | 16px |
| ContextMenu | `rounded-md` | 6px |
| 移动端标题栏按钮 | `rounded-md` | 6px |

**影响**：模态框、弹窗、输入框、按钮之间的圆角不统一，视觉上缺乏一致性。

### ISSUE 2: 遮罩层颜色不一致

不同组件使用不同的遮罩层颜色：

| 组件 | 遮罩层颜色 |
|------|-----------|
| ModalLayout | `rgba(10,10,10,0.6)` + `backdrop-blur-sm` |
| SettingsModal | `rgba(10,10,10,0.6)` + `backdrop-blur-sm` |
| App.tsx 移动端侧边栏遮罩 | `rgba(26,26,26,0.45)` 无毛玻璃 |

**影响**：移动端侧边栏遮罩与弹窗遮罩颜色/透明度不同，且缺少 CSS 变量定义。

### ISSUE 3: 按钮系统缺乏统一规范

按钮的尺寸、圆角、内边距在组件间不一致：

| 组件 | 按钮高度 | 圆角 | 内边距 |
|------|---------|------|--------|
| ModalLayout header 关闭 | 28px (h-7) | 2px (rounded-sm) | 居中 |
| SettingsModal header 关闭 | 32px (h-8) | 6px (rounded-md) | 居中 |
| IconButton sm | 28px (h-7) | 6px (rounded-md) | 居中 |
| IconButton md | 32px (h-8) | 6px (rounded-md) | 居中 |
| 模态框取消按钮 | ~36px (py-2) | 2px (rounded-sm) | px-5 / px-[18px] |
| 模态框确认按钮 | ~36px (py-2) | 2px (rounded-sm) | px-5 / px-6 |
| 设置面板按钮 | ~36px (py-2) | 6px (rounded-md) | px-4 |
| 搜索按钮 | ~36px (py-2.5) | 6px (rounded-md) | px-2.5 |

**影响**：缺乏 Primary / Secondary / Ghost 按钮的标准化定义。

### ISSUE 4: 阴影系统未应用

CSS 变量定义了 `--shadow-sm` / `--shadow-md` / `--shadow-lg`，但**组件中从未使用这些变量**，而是使用 Tailwind 的 `shadow-md` / `shadow-lg`。

| CSS 变量 | 定义值 | 是否使用 |
|---------|--------|---------|
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,0.04)` | 否 |
| `--shadow-md` | `0 2px 6px rgba(0,0,0,0.04)` | 否 |
| `--shadow-lg` | `0 4px 12px rgba(0,0,0,0.06)` | 否 |

### ISSUE 5: 模态框样式碎片化

`ModalLayout.tsx` 和 `SettingsModal.tsx` 分别实现了各自的模态框，有重复代码：

- 两者都重复了遮罩层 HTML 结构
- `ModalLayout.tsx` 有焦点陷阱、Escape 关闭、焦点恢复
- `SettingsModal.tsx` 没有焦点陷阱，但有自己的布局逻辑
- 模态框宽度不一致：`ModalLayout` 默认 480px，`SettingsModal` 920px
- 模态框高度：`ModalLayout` 90vh，`SettingsModal` 固定 680px

### ISSUE 6: 颜色系统不完整

当前 CSS 变量缺少以下实用颜色：

| 缺失的 Token | 建议用途 |
|-------------|---------|
| `--overlay` 或 `--backdrop` | 模态框/侧边栏遮罩层标准化 |
| `--focus-ring` | 键盘焦点指示器 |
| `--sidebar-bg` | 侧边栏专用背景色 |
| `--primary-active` | 按钮/交互组件按下态 |
| `--selection` | 文本选中色 |
| 暗色模式 `--primary-hover` | 当前暗色模式的 `--primary-hover` 使用 `#A99BFC`，但浅色模式使用 `color-mix()`（可能兼容性问题） |

### ISSUE 7: 主题切换的 `accentColor` 与深色模式冲突

`index.css` 中：
- `:root` 定义默认主题（浅色）的 `--primary: #7B68EE`
- `[data-theme="dark"]` 定义深色主题的 `--primary: #9B8AFB`
- `applyAccentColor()` 在 `:root` 上设置 `--primary`（不区分主题）

这意味着：如果用户设置了自定义强调色，深色模式下 `[data-theme="dark"]` 的 `--primary` 会被覆盖，无法正确显示深色适配的强调色。

### ISSUE 8: ContextMenu 样式不一致

- 使用 `border border-border rounded-md shadow-lg` 比模态框 (`rounded-lg`) 圆角小
- 使用字符串拼接 className 而非 `cn()` 工具函数
- Danger 状态使用 `text-danger` 但 hover 态无视觉反馈
- 缺少 CSS 过渡动画

### ISSUE 9: Toast 通知系统未渲染

`toast.ts` 实现了一个完整的 toast store（订阅/通知/超时），但**没有对应的 Toast 渲染组件**。ShowToast 调用只会添加数据到 store，但 UI 上不可见。

### ISSUE 10: 移动端侧边栏过渡效果

- 使用 `duration-250` 自定义过渡时间（CSS 中没有 `--duration-250` 变量）
- 移动端侧边栏遮罩使用固定颜色，而非 CSS 变量
- 移动端顶部栏硬编码 "RSS 阅读器" 标题

### ISSUE 11: 暗色模式颜色对比度较弱

当前暗色模式分析：
- `--bg-canvas: #202020` 与 `--bg-surface: #262626` 差异仅 6 个灰度级别，辨识度低
- `--text-muted: #737373` 在深色背景上对比度不足
- `--border-subtle: #2E2E2E` 与背景过于接近，边界几乎不可见

### ISSUE 12: Apple 设计库参考与当前实现差距

Apple Design Library 提供的参考与当前实现的主要差异：

| 维度 | Apple 设计库 | 当前实现 |
|------|-------------|---------|
| 主色 | `#007aff` 蓝色 | `#7B68EE` 紫色 |
| 品牌圆角 | 19.2px (大圆角) | 4-8px (小圆角) |
| 间距单元 | 3.84px 步进 | Tailwind 标准 |
| 阴影层级 | 7 级分层，含多层阴影 | 3 级简单阴影 |
| 背景色 | 纯白/纯黑 | 灰白/深灰 |

---

## 三、改进方案建议

### 优先级 1: 修复 CSS 变量系统

```css
/* 新增缺失变量 */
:root {
  --overlay: rgba(10, 10, 10, 0.6);
  --overlay-mobile: rgba(10, 10, 10, 0.45);
  --focus-ring: 0 0 0 2px var(--primary-subtle);
  --sidebar-bg: var(--bg-elevated);
  --primary-active: color-mix(in srgb, var(--primary) 75%, black);
  --selection-bg: var(--primary-subtle);
  --duration-fast: 150ms;
  --duration-normal: 200ms;
  --duration-slow: 300ms;
}

[data-theme="dark"] {
  --overlay: rgba(0, 0, 0, 0.7);
  --overlay-mobile: rgba(0, 0, 0, 0.55);
}
```

### 优先级 2: 统一圆角系统

将 CSS 变量与 Tailwind 配置关联，或在组件中直接使用 CSS 变量：

```css
/* 方案：使用 Tailwind 的 arbitrary values 或统一使用 CSS 变量 */
/* 建议统一使用 CSS 变量：border-radius: var(--radius-md) */
```

建议组件圆角映射：
- 按钮/输入框 → `--radius-sm` (4px)
- 卡片/面板 → `--radius-md` (6px)
- 模态框/弹窗 → `--radius-lg` (8px)
- 标签/徽章 → `--radius-sm` (4px) 或 `--radius-md` (6px)

### 优先级 3: 统一按钮系统

定义标准化的按钮组件：

| 类型 | 高度 | 圆角 | 内边距 | 字体 |
|------|------|------|--------|------|
| Primary (主要) | 32px | 6px | `px-4 py-1.5` | 13px semibold |
| Secondary (次要) | 32px | 6px | `px-4 py-1.5` | 13px medium |
| Ghost (轻量) | 32px | 6px | `px-2` | 13px |
| Icon (图标) | 28-32px | 6px | 居中 | - |

### 优先级 4: 修复 `accentColor` 主题冲突

```typescript
// 在 applyAccentColor 中区分主题
function applyAccentColor(color: string, isDark: boolean) {
  const root = document.documentElement;
  if (isDark) {
    root.style.setProperty('--primary', lightenColor(color, 20)); // 在深色模式下调亮
  } else {
    root.style.setProperty('--primary', color);
  }
}
```

### 优先级 5: 实现 Toast 组件

基于现有的 `toast.ts` store，创建 `ToastContainer.tsx` 组件，在 `App.tsx` 中渲染。

### 优先级 6: 优化暗色模式对比度

```css
[data-theme="dark"] {
  --bg-canvas: #1A1A1A;      /* 更深 */
  --bg-surface: #232323;      /* 适当拉开差距 */
  --bg-elevated: #2C2C2C;    /* 提高辨识度 */
  --text-muted: #8A8A8A;      /* 提高对比度 */
  --border-subtle: #353535;   /* 让边界可见 */
}
```

### 优先级 7: 整合 Apple 设计参考

将 Apple 设计库的参考元素策略性地融入：
- 采用 Apple 的阴影层级系统（7 级分层阴影）
- 借鉴 Apple 的内容优先排版哲学
- 保留紫色主色（Obsidian 风格），但借鉴 Apple 的间距和圆角处理

---

## 四、视觉预览（建议）

下面对关键界面提出改进建议：

### 4.1 主界面三栏布局

```
┌─────────────────────────────────────────────────────────────┐
│ TitleBar (34px)                                              │
├──────────┬──────────────────────┬────────────────────────────┤
│ Sidebar  │ ArticleList          │ Reader                     │
│ (260px)  │ (360px)              │ (flex-1)                   │
│          │                      │                            │
│ Search   │ Header: 标题 + 计数  │ 工具栏: 收藏/稍后/阅读模式   │
│ Filters  │                      │                            │
│ 全部文章 │ [未读点] 来源 · 时间  │ 标题 h1                     │
│ 未读文章 │ 文章标题 (bold)       │ 元信息: 来源 · 日期 · 链接   │
│ 稍后阅读 │ 摘要 (2行)           │                            │
│ 收藏     │ 稍后 收藏 图标        │ 正文内容 (衬线/无衬线)       │
│ ─────── │                      │                            │
│ 订阅源   │ [选中态: 左侧3px紫条]  │ 引用块 / 代码 / 图片        │
│ 文件夹1  │                      │                            │
│  ├─ 源A  │ 空状态: 图标 + 文案   │                            │
│  └─ 源B  │                      │                            │
│ 设置     │                      │                            │
└──────────┴──────────────────────┴────────────────────────────┘
```

### 4.2 模态框改进

**当前问题**：弹窗没有统一的圆角、遮罩色、标题栏高度。

**改进方案**：
- 统一遮罩层：`rgba(10,10,10,0.6)` + `backdrop-blur(4px)`
- 统一弹窗圆角：`--radius-lg` (8px)
- 统一标题栏高度：52px（当前 ModalLayout 标准）
- 统一关闭按钮位置和行为

### 4.3 颜色系统改进

**浅色主题**：
- 背景：`#FAFAFA` → `#FFFFFF` → `#F2F2F2`（保持）
- 主色：`#7B68EE`（保持紫色，增强品牌识别）
- 未读：`#3B82F6`（蓝色，与紫色形成对比）

**深色主题**：
- 背景：`#1A1A1A` → `#232323` → `#2C2C2C`（调整对比度）
- 主色：`#9B8AFB`（调亮以适应深色背景）
- 文字：`#E8E8E8` → `#A0A0A0` → `#8A8A8A`（提高可读性）

---

## 五、总结

### 问题汇总

| 严重度 | 问题 | 涉及组件 |
|--------|------|---------|
| **高** | 暗色模式对比度不足 | 全局 |
| **高** | Toast 系统无 UI 渲染 | 全局 |
| **高** | `accentColor` 与暗色模式冲突 | 全局 |
| **中** | 圆角系统未统一使用 CSS 变量 | 所有弹窗/按钮/输入框 |
| **中** | 遮罩层颜色不一致 | ModalLayout / SettingsModal / App |
| **中** | 阴影变量未使用 | 模态框/菜单 |
| **中** | 按钮系统缺乏标准化 | 所有弹窗按钮 |
| **低** | ContextMenu 使用字符串拼接样式 | ContextMenu |
| **低** | 移动端遮罩使用固定颜色 | App.tsx |
| **低** | CSS 变量定义但未使用 | 圆角/阴影变量 |

### 建议实施顺序

1. 修复 CSS 变量系统（新增缺失变量 + 在组件中引用）
2. 实现 Toast 组件
3. 修复 `accentColor` 主题冲突
4. 统一按钮系统（创建标准化 Button 组件）
5. 统一弹窗模态框样式
6. 优化暗色模式对比度
7. 整合 Apple 设计库参考