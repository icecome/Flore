import { Section, ShortcutRow } from './SettingsShared';

export default function SettingsShortcutsTab() {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="文章导航">
        <ShortcutRow keys="↑ / ↓" desc="上下切换文章" />
        <ShortcutRow keys="Enter" desc="打开选中的文章" />
        <ShortcutRow keys="Esc" desc="关闭文章详情/弹窗" />
      </Section>

      <Section title="文章操作">
        <ShortcutRow keys="M" desc="标记当前文章为已读/未读" />
        <ShortcutRow keys="S" desc="收藏/取消收藏当前文章" />
        <ShortcutRow keys="L" desc="加入/移除稍后阅读" />
      </Section>

      <Section title="列表操作">
        <ShortcutRow keys="R" desc="刷新当前列表" />
        <ShortcutRow keys="X" desc="进入/退出多选模式" />
      </Section>

      <Section title="其他">
        <ShortcutRow keys="?" desc="显示快捷键帮助" />
      </Section>
    </div>
  );
}
