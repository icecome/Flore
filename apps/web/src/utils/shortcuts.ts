/** 全局键盘快捷键的唯一定义。ShortcutsHelpModal 与 SettingsShortcutsTab 均从这里渲染，
 *  新增快捷键时只改 `App.tsx` 的 keydown 分支和本文件，不再维护第二份表格。 */

export interface ShortcutEntry {
  /** 展示用的按键序列，如「↑ / ↓」。 */
  keys: string;
  /** 动作说明。 */
  desc: string;
}

export interface ShortcutGroup {
  title: string;
  entries: ShortcutEntry[];
}

export const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    title: '文章导航',
    entries: [
      { keys: '↑ / ↓ / J / K', desc: '上下切换文章' },
      { keys: 'Enter', desc: '打开选中的文章' },
      { keys: 'Esc', desc: '关闭文章详情/弹窗' },
    ],
  },
  {
    title: '文章操作',
    entries: [
      { keys: 'M', desc: '标记当前文章为已读/未读' },
      { keys: 'S', desc: '收藏/取消收藏当前文章' },
      { keys: 'L', desc: '加入/移除稍后阅读' },
    ],
  },
  {
    title: '列表操作',
    entries: [
      { keys: 'R', desc: '刷新当前列表' },
      { keys: 'X', desc: '进入/退出多选模式' },
    ],
  },
  {
    title: '其他',
    entries: [{ keys: '?', desc: '显示快捷键帮助' }],
  },
];
