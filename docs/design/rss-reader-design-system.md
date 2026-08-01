# RSS 阅读器主题设计系统提案

## 1. 项目现状与问题

### 1.1 当前技术栈
- 前端：React 19 + Vite，无 CSS 框架，全部使用内联 `React.CSSProperties`。
- 图标：已迁移为内联 SVG（`src/components/icons.tsx`），但尚未统一颜色与尺寸规范。
- 布局：三栏固定布局（Sidebar 260px / ArticleList 360px / Reader 自适应）。
- 当前视觉：以 `#1976d2` 蓝、纯白、浅灰为主，偏工具化，缺乏阅读器应有的“纸质/编辑感”。

### 1.2 体验痛点
1. **色彩单调**：蓝色作为唯一强调色，长期使用易产生视觉疲劳；且蓝色与“阅读”情感关联较弱。
2. **字体单一**：全站使用系统无衬线字体，长文阅读缺乏“书卷气”。
3. **层次不足**：三栏之间仅靠 1px `#e0e0e0` 分隔，信息层级靠间距和背景色硬撑。
4. **文章列表识别度低**：不同来源的文章视觉差异小，难以一眼定位。
5. **阅读区拥挤**：文字撑满宽度、行高与字号尚未针对中文长文优化。
6. **暗色模式缺失**：夜间阅读只能依赖系统反色或浏览器插件。

---

## 2. 设计方向：「Typestream / 字流」

> 以“信息流中的慢阅读”为核心理念，把 RSS 阅读器设计成一份**可随时翻阅的个人杂志**。视觉上参考 Folo 的通透感，但避免照搬其圆角卡片与强阴影，转而采用**纸质感、暖中性色、编辑级排版**。

### 2.1 关键词
- **纸感**（Paper-like）：暖白背景、轻微纹理、柔和阴影。
- **低饱和**（Muted）：避免高饱和原色，使用赭石、鼠尾草、炭黑等自然色。
- **可识别来源**（Source Identity）：每个订阅源拥有稳定色相，帮助快速扫视。
- **专注阅读**（Focus Reading）：阅读区像一张干净书页，减少 UI 噪音。

---

## 3. 布局设计

### 3.1 三栏结构（保持不变，优化比例与层次）

```
┌──────────────────────────────────────────────────────────────────────┐
│  [RSS 阅读器]    [⚙] [+]  │  [全部文章]    [⟳] [○] [✓] 100篇  │  [文章标题]            [★] [阅读] [原文] [↗] [已读] │
├───────────────────────────┼─────────────────────────────────────┼─────────────────────────────────────────────────────┤
│ ★ 收藏                    │ BlogsClub           17:55           │                                                     │
│ 全部文章              100 │ 修复一下 Handsome 主题顶部统计图... │                                                     │
│                           │                                     │                                                     │
│ ▼ 博客                12  │ V2EX-最热主题       17:24           │              选择一篇文章开始阅读                   │
│    少数派              3  │ 吃瓜 openai gpt 全网 503            │                                                     │
│    阮一峰的网络日志    9  │                                     │                                                     │
│ ▶ 豆瓣                 5  │                                     │                                                     │
│                           │                                     │                                                     │
│ 未分类                 8  │                                     │                                                     │
│                           │                                     │                                                     │
└───────────────────────────┴─────────────────────────────────────┴─────────────────────────────────────────────────────┘

Sidebar: 260px        ArticleList: 380px        Reader: flex 1
```

### 3.2 关键布局调整
- **三栏分隔**：用 1px 暖灰色纵线代替冷灰，营造“装订线”感。
- **标题栏**：保持 52px 高度，但按钮统一为 28px 图标按钮，操作区宽度固定，标题自动截断。
- **文章列表**：条目之间用 1px 细线分隔，替代当前卡片之间的空白，形成“信息流”连续感。
- **阅读区**：内容宽度限制在 `max-width: 680px` 并居中，模拟纸质书页。

---

## 4. 主题设计（Theme）

### 4.1 色彩系统（CSS Variables）

采用 `oklch()` 定义现代、易维护的语义化变量，同时提供 Hex 回退。

```css
:root {
  /* 背景层：由远及近 */
  --bg-canvas: oklch(0.97 0.006 80);          /* 全局画布，暖米色 */
  --bg-surface: oklch(1 0 0);                 /* 栏背景，纯白 */
  --bg-elevated: oklch(0.99 0.004 80);        /* 弹窗、浮层 */
  --bg-hover: oklch(0.955 0.008 80);          /* 悬停 */
  --bg-active: oklch(0.93 0.012 80);          /* 按下/选中 */

  /* 文字 */
  --text-primary: oklch(0.22 0.01 60);        /* 主文字，炭黑 */
  --text-secondary: oklch(0.45 0.015 60);     /* 次要文字 */
  --text-muted: oklch(0.62 0.01 60);          /* 元信息、时间 */
  --text-on-primary: oklch(1 0 0);            /* 主按钮上的文字 */

  /* 强调色：赭石陶土 */
  --primary: oklch(0.55 0.14 35);             /* 赭石 */
  --primary-hover: oklch(0.48 0.15 35);
  --primary-subtle: oklch(0.94 0.03 45);      /* 选中背景 */

  /* 状态色 */
  --success: oklch(0.55 0.12 145);            /* 鼠尾草绿 */
  --warning: oklch(0.7 0.12 85);              /* 琥珀 */
  --danger: oklch(0.55 0.16 25);              /* 暗红 */

  /* 边框 */
  --border: oklch(0.9 0.01 70);               /* 暖灰分隔线 */
  --border-strong: oklch(0.78 0.02 60);       /* 强分隔 */

  /* 阴影：纸质感 */
  --shadow-sm: 0 1px 2px oklch(0.6 0 0 / 0.04);
  --shadow-md: 0 4px 12px oklch(0.5 0 0 / 0.06);
  --shadow-lg: 0 12px 32px oklch(0.4 0 0 / 0.08);

  /* 圆角 */
  --radius-sm: 6px;
  --radius-md: 10px;
  --radius-lg: 14px;

  /* 字号与行高 */
  --font-sans: Inter, "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --font-serif: "Noto Serif SC", "Source Han Serif SC", Georgia, serif;
  --font-mono: "JetBrains Mono", "Fira Code", Consolas, monospace;
}
```

### 4.2 暗色模式

```css
[data-theme="dark"] {
  --bg-canvas: oklch(0.18 0.01 60);
  --bg-surface: oklch(0.22 0.01 60);
  --bg-elevated: oklch(0.26 0.015 60);
  --bg-hover: oklch(0.28 0.02 60);
  --bg-active: oklch(0.32 0.02 60);

  --text-primary: oklch(0.94 0.005 60);
  --text-secondary: oklch(0.72 0.01 60);
  --text-muted: oklch(0.55 0.01 60);

  --primary: oklch(0.65 0.12 45);
  --primary-hover: oklch(0.72 0.13 45);
  --primary-subtle: oklch(0.3 0.03 45);

  --border: oklch(0.32 0.01 60);
  --border-strong: oklch(0.42 0.02 60);

  --shadow-sm: 0 1px 2px oklch(0 0 0 / 0.25);
  --shadow-md: 0 4px 12px oklch(0 0 0 / 0.35);
  --shadow-lg: 0 12px 32px oklch(0 0 0 / 0.45);
}
```

### 4.3 字体策略

| 用途 | 字体 | 说明 |
|------|------|------|
| UI 控件、标题栏、菜单 | `--font-sans` (Inter) | 清晰、中性、高密度 |
| 文章标题 | `--font-serif` | 编辑感，区分列表与正文 |
| 阅读正文 | `--font-serif` | 长文阅读更舒适 |
| 代码块 | `--font-mono` | 等宽，已有 |

引入方式（`index.html` head）：
```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&family=Noto+Serif+SC:wght@400;600;700&display=swap" rel="stylesheet">
```

---

## 5. 组件级设计要点

### 5.1 Sidebar（订阅栏）

- **背景**：`--bg-surface`。
- **标题栏**：底部 1px `--border`；logo/标题使用衬线字体「RSS 阅读器」？可改为「Feed」或保留中文但使用 `--font-sans` 加字间距。
- **收藏 / 全部文章**：选中态背景 `--primary-subtle`，文字 `--primary`，左侧无竖线。
- **文件夹**：每个文件夹左侧放置 8px 色条（来源色），增强识别。展开箭头使用 16px Chevron，旋转动画 150ms。
- **未读徽章**： Pill 形状，背景 `--primary`，文字 `--text-on-primary`，字号 11px。

### 5.2 ArticleList（文章列表）

- **条目**：去除当前“卡片”背景，改为白底 + 底部 1px `--border`；悬停时背景变为 `--bg-hover`。
- **选中态**：左侧 3px `--primary` 竖线 + 背景 `--primary-subtle`。
- **来源标识**：标题行左侧放置 6px 圆点，颜色由订阅源名称哈希生成，稳定不变。
- **摘要**：保持 2 行截断，颜色 `--text-secondary`。
- **收藏按钮**：空心星（默认）/ 实心星（已收藏），颜色 `--primary`，无文字。

### 5.3 Reader（阅读区）

- **标题栏**：底部 1px `--border`；标题使用衬线、字号 20px、行高 1.35；元信息 `--text-muted`。
- **操作按钮**：统一 ghost 风格，图标 + 文字， hover 背景 `--bg-hover`；已收藏按钮为 `--primary` 实心。
- **正文区域**：
  - 背景 `--bg-canvas`（暖米色），与两侧栏形成层次。
  - 正文容器 `max-width: 680px`，内边距 `48px 40px`。
  - 正文字体 `--font-serif`，字号 17px，行高 1.9，字色 `--text-primary`。
  - 段落间距 `1.6em`。
  - 引用块左侧 3px `--primary`，背景 `--bg-hover`。
  - 图片圆角 `--radius-md`，阴影 `--shadow-sm`。

### 5.4 模态框与右键菜单

- **模态框**：`--bg-elevated`、圆角 `--radius-lg`、阴影 `--shadow-lg`、1px `--border`。
- **右键菜单**：无投影，使用 1px `--border` + `--bg-elevated`；危险操作使用 `--danger` 文字色。

---

## 6. 动效设计

所有动效使用 `ease-out` 与 `cubic-bezier(0.16, 1, 0.3, 1)`（Apple 风格缓出）。

```
hover-bg:      150ms ease-out  background-color
button-press:  100ms ease-out  scale(0.97)
card-lift:     200ms ease-out  translateY(-1px) + shadow-md
chevron-spin:  150ms ease-out  rotate(90deg)
modal-fade:    250ms ease-out  opacity 0→1 + scale 0.98→1
list-item-in:  300ms ease-out  opacity 0→1 + translateY(6px)→0
reader-fade:   350ms ease-out  opacity 0→1
```

---

## 7. 来源色生成规则（Source Identity）

为每个订阅源生成稳定的低饱和度色相，避免高饱和破坏整体氛围。

```ts
function sourceHue(name: string): number {
  let hash = 0;
  for (const c of name) hash = (hash * 31 + c.charCodeAt(0)) % 360;
  return hash;
}

function sourceColor(name: string, mode: 'light' | 'dark'): string {
  const h = sourceHue(name);
  return mode === 'light'
    ? `oklch(0.58 0.1 ${h})`
    : `oklch(0.68 0.08 ${h})`;
}
```

用途：文章列表左侧圆点、订阅源列表 favicon 占位背景、文件夹色条。

---

## 8. 实现路径建议

### Phase 1：变量与基础样式（低风险，立竿见影）
1. 在 `index.css` 中引入上述 CSS 变量与字体。
2. 为 `<html>` 添加 `data-theme` 属性并监听系统偏好。
3. 调整 `body` 背景为 `--bg-canvas`，文字色为 `--text-primary`。

### Phase 2：栏与标题栏
1. 更新 `Sidebar`、`ArticleList`、`Reader` 的 `container` / `header` 样式，使用变量。
2. 统一标题栏按钮为 28px 图标按钮，调整 hover/active 态。

### Phase 3：文章列表与阅读区
1. 重构 `ArticleList` 条目样式：白底细线、来源色圆点、选中态左侧竖线。
2. 重构 `Reader` 内容区：暖米色背景、衬线正文、最大宽度 680px。

### Phase 4：来源色与动效
1. 实现 `sourceColor()` 工具函数，并在列表中应用。
2. 为按钮、 Chevron、模态框添加 CSS transition。

### Phase 5：暗色模式
1. 补齐 `data-theme="dark"` 变量，测试三栏、阅读区、模态框。

---

## 9. 与 Folo 的差异点（避免照搬）

| 维度 | Folo | 本方案 Typestream |
|------|------|-------------------|
| 主色 | 亮蓝 / 紫渐变 | 低饱和赭石 |
| 背景 | 纯白 / 深灰 | 暖米色画布 |
| 文章列表 | 圆角卡片、强阴影 | 扁平信息流、细线分隔 |
| 字体 | 全站无衬线 | UI 无衬线 + 阅读衬线 |
| 来源识别 | 彩色 favicon | 低饱和稳定色相圆点/色条 |
| 阅读区 | 白色卡片、窄边距 | 纸质书页、大边距、居中 |

---

## 10. 下一步

本提案可作为「设计系统 v1.0」。如果你认可方向，我可以：
1. 先实现 Phase 1–2（变量 + 栏/标题栏改造），提供前后对比截图；
2. 或一次性实现完整主题并生成暗色模式切换。

请确认是否按此方案推进，或需要调整色彩/字体/布局方向。
