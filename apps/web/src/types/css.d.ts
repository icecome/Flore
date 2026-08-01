import 'csstype';

declare module 'csstype' {
  interface Properties {
    // Wails/Electron 无边框窗口拖拽所需 CSS 属性
    WebkitAppRegion?: 'drag' | 'no-drag';
  }
}
