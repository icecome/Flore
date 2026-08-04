## 2026-08-02 设置面板 UI 重新设计（进行中）

- 任务：基于现有 Flore 设置面板代码，重新设计数据管理标签页 + 统一所有标签页视觉风格
- 目标文件：https://ardot.tencent.com/file/710466762144799
- 分析结果：
  - 现有结构：9 个标签页（通用/外观/订阅源/规则过滤/数据管理/通知与驻留/网络设置/快捷键/关于）
  - 侧边栏宽度 200px，内容区高度 740px
  - 设计语言：卡片式 Section、Row 行、Toggle/Switch、Select、Slider
  - 配色：var(--primary)/var(--secondary)/var(--muted)/var(--accent) 等 CSS 变量
  - 按钮样式：border border-border rounded-md px-3 py-1.5 text-[13px]
  - 字体：Inter 13.5px 正文，14px 小标题
