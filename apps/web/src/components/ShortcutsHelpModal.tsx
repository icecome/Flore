import { Keyboard } from './icons';
import ModalLayout from './ModalLayout';

interface Props {
  onClose: () => void;
}

const shortcuts = [
  { keys: ['↑', '↓'], desc: '上下切换文章' },
  { keys: ['Enter'], desc: '打开选中的文章' },
  { keys: ['M'], desc: '标记当前文章为已读/未读' },
  { keys: ['S'], desc: '收藏/取消收藏当前文章' },
  { keys: ['L'], desc: '添加到稍后阅读/取消' },
  { keys: ['R'], desc: '刷新当前列表' },
  { keys: ['X'], desc: '进入/退出多选模式' },
  { keys: ['?'], desc: '显示快捷键帮助' },
  { keys: ['Esc'], desc: '关闭文章详情/弹窗' },
];

export default function ShortcutsHelpModal({ onClose }: Props) {
  return (
    <ModalLayout
      title="键盘快捷键"
      titleIcon={<Keyboard size={18} />}
      onClose={onClose}
      width={420}
    >
      <div className="px-5 pt-3 pb-5 flex flex-col gap-3">
        {shortcuts.map((shortcut, index) => (
          <div key={shortcut.keys.join("")} className="flex items-center justify-between min-h-[36px]">
            <div className="flex gap-1.5">
              {shortcut.keys.map((key, i) => (
                <span key={i} className="inline-flex items-center justify-center min-w-[28px] h-7 px-2 bg-hover border border-border rounded-md text-sm font-mono text-primary shadow-[0_1px_0_var(--border)]">
                  {key}
                </span>
              ))}
            </div>
            <span className="text-secondary text-base">{shortcut.desc}</span>
          </div>
        ))}
      </div>
    </ModalLayout>
  );
}