# Flore 主题柔和冷白优化方案

> 基于 Obsidian + Apple 融合风格，解决色彩冷硬、阴影单薄、通知僵硬、阅读主题冲突等问题
> 策略：柔和冷白中性 + 双变体 tint 阅读主题 + 多层柔和阴影 + 毛玻璃通知
> 方向修订：偏冷但不冷（R 微低于 B），Apple 式冷白基调

---

## 一、设计理念

### 1.1 风格定位

结合 Apple 与 Obsidian 的核心共性：**冷静、克制、内容优先**。

Apple 的白色本质是冷白（系统背景 `#F5F5F7` 带 B 微高的蓝灰调），不是暖白。Obsidian 的深色是冷灰墨（`#1A1B1E` 带蓝调），不是暖墨。两者都偏冷，但通过极小的色温差（R 与 B 仅差 2-3 级）保持"冷而不寒"的柔和感。

### 1.2 色温原则

- **偏冷但不冷**：R 微低于 B（差 2-4 级），营造冷静工具感，但不到明显发冷的程度
- **不用纯灰**：纯中性灰（R=G=B）显得廉价生硬，必须注入微弱色温倾向
- **不用暖色**：暖白会让知识工具显得软糯，偏离 Obsidian+Apple 的冷静气质
- **冷调阴影**：阴影用冷色 `rgba(25,35,50)` 而非暖色或纯黑，与冷白基调统一

### 1.3 解决的问题

1. 色彩冷硬（纯中性灰、无温度）→ 柔和冷白（微弱冷色温、有呼吸感）
2. 深色模式辨识度低（层级差仅 6-8 级）→ 拉开层间距至 7-10 级
3. 阴影单薄（单层极淡）→ 多层柔和（近距+远距叠加）
4. 通知僵硬（纯色块）→ 毛玻璃 + 左边强调条
5. 阅读主题与应用主题冲突 → 双变体 tint 架构消除冲突

---

## 二、色彩系统

### 2.1 浅色模式 — 柔和冷白

```css
:root {
  /* 背景层级 — 冷白基调，R 微低于 B */
  --bg-canvas: #F7F8FA;      /* 柔和冷白，原 #FAFAFA 纯灰 */
  --bg-surface: #FFFFFF;     /* 纯白保留 */
  --bg-elevated: #EFF1F5;    /* 冷浅灰，原 #F2F2F2 */
  --bg-hover: #E9ECF0;       /* 冷悬停 */
  --bg-active: #E1E5EA;      /* 冷激活 */
  --bg-input: #F4F6F9;       /* 冷输入框 */
  --sidebar-bg: #F2F4F7;     /* 侧边栏冷灰 */

  /* 文字 — 冷灰 */
  --text-primary: #1A1D24;   /* 冷近黑，原 #1A1A1A */
  --text-secondary: #68707A; /* 冷中灰，原 #6E6E6E */
  --text-muted: #98A0AB;     /* 冷浅灰，原 #A0A0A0 */
  --text-on-primary: #FFFFFF;
  --text-disabled: #C4CAD2;  /* 原 #C0C0C0 */

  /* 强调色 — 保持紫色品牌 */
  --primary: #7B68EE;
  --primary-hover: #6A59D9;
  --primary-subtle: rgba(123, 104, 238, 0.10);
  --primary-active: #5A49C9;

  /* 状态色 — 冷调适配 */
  --unread: #4E7CFA;
  --unread-subtle: rgba(78, 124, 250, 0.12);
  --success: #2EA876;
  --success-subtle: rgba(46, 168, 118, 0.10);
  --warning: #E89545;
  --warning-subtle: rgba(232, 149, 69, 0.12);
  --danger: #E04848;
  --danger-subtle: rgba(224, 72, 72, 0.10);

  /* 边框 — 冷灰 */
  --border: #E5E8ED;         /* 原 #E5E5E5 */
  --border-strong: #D3D8E0;  /* 原 #D4D4D4 */
  --border-subtle: #EFF1F5;  /* 原 #F0F0F0 */

  /* 遮罩与玻璃 */
  --overlay: rgba(20, 30, 45, 0.4);
  --overlay-mobile: rgba(20, 30, 45, 0.5);
  --glass-bg: rgba(255, 255, 255, 0.72);
  --glass-border: rgba(255, 255, 255, 0.6);

  /* 交互态 */
  --focus-ring: 0 0 0 3px rgba(123, 104, 238, 0.2);
  --selection-bg: rgba(123, 104, 238, 0.12);
}
```

### 2.2 深色模式 — 冷灰墨

```css
[data-theme="dark"] {
  /* 背景层级 — 冷墨基调，拉开层间距 */
  --bg-canvas: #1A1B1F;      /* 冷墨，原 #202020，B 微高 */
  --bg-surface: #212226;     /* 原 #262626，间距 +7 */
  --bg-elevated: #2A2C31;    /* 原 #2E2E2E，间距 +9 */
  --bg-hover: #323439;       /* 原 #333333 */
  --bg-active: #3A3C42;      /* 原 #3A3A3A */
  --bg-input: #252629;       /* 原 #262626 */
  --sidebar-bg: #1E1F23;     /* 原 #1E1E1E，冷调 */

  /* 文字 — 冷白，提高对比 */
  --text-primary: #E6E8EC;   /* 冷白，原 #E5E5E5 */
  --text-secondary: #A8ACB4; /* 原 #A0A0A0，+8 亮度 */
  --text-muted: #7E828A;     /* 原 #888888，可读性提升 */
  --text-on-primary: #FFFFFF;
  --text-disabled: #52555A;  /* 原 #555555 */

  /* 强调色 — 深色适配色 */
  --primary: #9B8AFB;
  --primary-hover: #AC9CFC;
  --primary-subtle: rgba(155, 138, 251, 0.14);
  --primary-active: #8A7AEB;

  /* 状态色 — 深色适配色 */
  --unread: #6E94FC;
  --unread-subtle: rgba(110, 148, 252, 0.14);
  --success: #4EBA7C;
  --success-subtle: rgba(78, 186, 124, 0.12);
  --warning: #F0A858;
  --warning-subtle: rgba(240, 168, 88, 0.12);
  --danger: #F06868;
  --danger-subtle: rgba(240, 104, 104, 0.12);

  /* 边框 — 提高可见度 */
  --border: #3A3C41;         /* 原 #404040 */
  --border-strong: #4D5056;  /* 原 #525252 */
  --border-subtle: #2C2E33;  /* 原 #2E2E2E，与 canvas 拉开 */

  /* 遮罩与玻璃 */
  --overlay: rgba(0, 0, 0, 0.55);
  --overlay-mobile: rgba(0, 0, 0, 0.65);
  --glass-bg: rgba(33, 34, 38, 0.72);
  --glass-border: rgba(255, 255, 255, 0.08);

  /* 交互态 */
  --focus-ring: 0 0 0 3px rgba(155, 138, 251, 0.25);
  --selection-bg: rgba(155, 138, 251, 0.2);
}
```

### 2.3 色温对比说明

| 维度 | 旧方案（纯灰） | 新方案（柔和冷白） |
|------|--------------|------------------|
| 浅色 canvas | `#FAFAFA`（R=G=B=250，纯灰） | `#F7F8FA`（R=247<G=248<B=250，冷偏） |
| 浅色 elevated | `#F2F2F2`（纯灰） | `#EFF1F5`（R<B，冷浅灰） |
| 深色 canvas | `#202020`（纯灰） | `#1A1B1F`（R<B，冷墨） |
| 深色层间距 | canvas→surface 差 6 级 | 差 7 级（辨识度提升） |
| 深色层间距 | surface→elevated 差 8 级 | 差 9 级（辨识度提升） |
| 文字 muted | 深色 `#888888`（对比不足） | `#7E828A`（冷灰，可读性更好） |
| 阴影色调 | 纯黑 `rgba(0,0,0)` | 冷色 `rgba(25,35,50)`（浅色） |

冷白的核心：R 与 B 差值控制在 2-4 级，足够消除纯灰的生硬感，但不足以让人察觉"这是冷色"。这是 Apple 系统色的做法。

---

## 三、阴影与圆角系统

### 3.1 多层柔和阴影（冷调）

```css
:root {
  --shadow-sm: 0 1px 2px rgba(25, 35, 50, 0.04), 0 1px 3px rgba(25, 35, 50, 0.03);
  --shadow-md: 0 2px 4px rgba(25, 35, 50, 0.04), 0 4px 12px rgba(25, 35, 50, 0.03);
  --shadow-lg: 0 4px 8px rgba(25, 35, 50, 0.05), 0 12px 28px rgba(25, 35, 50, 0.06);
  --shadow-xl: 0 8px 16px rgba(25, 35, 50, 0.06), 0 20px 40px rgba(25, 35, 50, 0.08);
}

[data-theme="dark"] {
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.2), 0 1px 3px rgba(0, 0, 0, 0.15);
  --shadow-md: 0 2px 4px rgba(0, 0, 0, 0.2), 0 4px 12px rgba(0, 0, 0, 0.15);
  --shadow-lg: 0 4px 8px rgba(0, 0, 0, 0.25), 0 12px 28px rgba(0, 0, 0, 0.2);
  --shadow-xl: 0 8px 16px rgba(0, 0, 0, 0.3), 0 20px 40px rgba(0, 0, 0, 0.25);
}
```

设计要点：
- 浅色阴影用冷色 `rgba(25,35,50)` 替代纯黑，与冷白基调统一
- 每级阴影 = 近距小阴影 + 远距大阴影，模拟自然光照
- 新增 `--shadow-xl` 用于浮层/弹窗

### 3.2 圆角适度放大

| Token | 旧值 | 新值 | 用途 |
|-------|------|------|------|
| `--radius-sm` | 4px | 6px | 按钮、输入框、标签 |
| `--radius-md` | 6px | 10px | 卡片、面板、菜单 |
| `--radius-lg` | 8px | 14px | 模态框、弹窗 |
| `--radius-xl` | — | 20px | 大型浮层（新增） |
| `--radius-full` | — | 9999px | 胶囊、头像（新增） |

---

## 四、阅读主题双变体方案

### 4.1 核心思路

阅读主题只定义"色彩倾向"，不定义明暗。每个倾向内置浅色 + 深色两个变体，由应用主题决定使用哪个。从架构上消除"应用浅色 + 阅读深色"的冲突。

```
应用浅色 → 阅读主题自动用浅色变体
应用深色 → 阅读主题自动用深色变体
用户只选色彩倾向（纸白/护眼/绿意），不选明暗
```

### 4.2 主题定义

```typescript
export type ReaderTheme = 'system' | 'paper' | 'sepia' | 'green';

interface ReaderThemeVariant {
  bg: string;
  color: string;
  muted: string;
  link: string;
}

const READER_THEME_PRESETS: Record<Exclude<ReaderTheme, 'system'>, {
  label: string;
  light: ReaderThemeVariant;
  dark: ReaderThemeVariant;
}> = {
  paper: {
    label: '纸白',
    light: {
      bg: '#FFFFFF',
      color: '#1A1D24',
      muted: '#68707A',
      link: '#7B68EE',
    },
    dark: {
      bg: '#1E1F23',
      color: '#E6E8EC',
      muted: '#A8ACB4',
      link: '#9B8AFB',
    },
  },
  sepia: {
    label: '护眼',
    light: {
      bg: '#F5F1EA',
      color: '#5C5043',
      muted: '#8A7E6E',
      link: '#9B6B3E',
    },
    dark: {
      bg: '#28241F',
      color: '#D4CCBE',
      muted: '#A89A86',
      link: '#D4A574',
    },
  },
  green: {
    label: '绿意',
    light: {
      bg: '#ECF3EE',
      color: '#2B4A32',
      muted: '#5A7560',
      link: '#3D7A45',
    },
    dark: {
      bg: '#1E251F',
      color: '#BED4C4',
      muted: '#8AA088',
      link: '#7DBE7C',
    },
  },
};
```

说明：纸白主题跟随应用冷白基调；护眼/绿意作为用户主动选择的阅读氛围，保留其色彩倾向，但明暗由应用主题决定。护眼降低暖度使其更柔和。

### 4.3 解析逻辑

```typescript
function resolveReaderTheme(
  readerTheme: ReaderTheme,
  appTheme: 'light' | 'dark'
): ReaderThemeVariant {
  const mood = readerTheme === 'system' ? 'paper' : readerTheme;
  const preset = READER_THEME_PRESETS[mood];
  return appTheme === 'dark' ? preset.dark : preset.light;
}
```

### 4.4 阅读器工具栏适配

阅读器工具栏跟随阅读主题背景色，保持视觉连贯：

```tsx
<header
  style={{
    background: 'var(--reader-bg)',
    borderBottom: '1px solid var(--reader-border, var(--border-subtle))',
  }}
>
```

### 4.5 变体色值一览

| 主题 | 模式 | 背景 | 文字 | 适用场景 |
|------|------|------|------|---------|
| 纸白 | 浅色 | `#FFFFFF` | `#1A1D24` | 默认冷白阅读 |
| 纸白 | 深色 | `#1E1F23` | `#E6E8EC` | 暗色冷墨 |
| 护眼 | 浅色 | `#F5F1EA` | `#5C5043` | 柔和米黄 |
| 护眼 | 深色 | `#28241F` | `#D4CCBE` | 暗色暖棕 |
| 绿意 | 浅色 | `#ECF3EE` | `#2B4A32` | 柔和绿调 |
| 绿意 | 深色 | `#1E251F` | `#BED4C4` | 暗色墨绿 |

---

## 五、通知系统重设计

从纯色块改为毛玻璃 + 柔和左边强调条：

```tsx
<div
  style={{
    background: 'var(--glass-bg)',
    backdropFilter: 'blur(16px)',
    border: '1px solid var(--glass-border)',
    borderLeft: '3px solid var(--toast-accent)',
    borderRadius: 'var(--radius-md)',
    boxShadow: 'var(--shadow-lg)',
  }}
>
  <Icon style={{ color: 'var(--toast-accent)' }} />
  <span>{message}</span>
</div>
```

类型映射：

```typescript
const toastConfig = {
  success: { accent: 'var(--success)', icon: Check },
  error:   { accent: 'var(--danger)',  icon: AlertTriangle },
  info:    { accent: 'var(--primary)', icon: Info },
};
```

| 维度 | 旧方案 | 新方案 |
|------|--------|--------|
| 背景 | 纯色块 `bg-success-subtle` | 毛玻璃 `glass-bg` + `blur(16px)` |
| 强调 | 全背景染色 | 左边 3px 强调条 |
| 阴影 | `shadow-lg` 单层 | `--shadow-lg` 多层柔和 |
| 圆角 | `rounded-md` (6px) | `--radius-md` (10px) |

---

## 六、实施指引

### 6.1 改动范围

| 文件 | 改动内容 | 风险 |
|------|---------|------|
| `apps/web/src/index.css` | 替换全部 CSS 变量值、新增 shadow-xl/radius-xl/glass | 低 — 仅改值 |
| `apps/web/src/utils/settings.ts` | 重构 `ReaderTheme` 类型与 `THEME_OPTIONS` | 中 — 类型变更 |
| `apps/web/src/components/Reader.tsx` | 工具栏跟随阅读主题、`resolvedReaderTheme` 逻辑 | 中 — 逻辑变更 |
| `apps/web/src/components/ToastContainer.tsx` | 毛玻璃样式 + 左边强调条 | 低 — 仅样式 |
| `apps/web/src/components/SettingsModal.tsx` | 阅读主题选项 UI 更新 | 低 — 仅选项 |

### 6.2 兼容性处理

```typescript
function migrateReaderTheme(raw: string): ReaderTheme {
  if (raw === 'dark' || raw === 'light') return 'paper';
  return raw as ReaderTheme;
}
```

### 6.3 验证清单

- 浅色/深色模式切换：色温冷而不寒，柔和有呼吸感
- 阅读主题切换：纸白/护眼/绿意在浅深色应用下均有对应变体，无文字不可读情况
- 应用主题切换时阅读主题自动跟随
- Toast 通知：毛玻璃效果正常，浅深色均可读
- 阴影层级：弹窗、面板、卡片有自然纵深
- 圆角：按钮/卡片/弹窗圆角统一且柔和
