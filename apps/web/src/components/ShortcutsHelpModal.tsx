import { Keyboard } from './icons';
import ModalLayout from './ModalLayout';
import { SHORTCUT_GROUPS } from '../utils/shortcuts';

interface Props {
  onClose: () => void;
}

function Key({ label }: { label: string }) {
  return (
    <span className="inline-flex items-center justify-center min-w-[28px] h-7 px-2 bg-hover border border-border rounded-md text-sm font-mono text-primary shadow-[0_1px_0_var(--border)]">
      {label}
    </span>
  );
}

export default function ShortcutsHelpModal({ onClose }: Props) {
  return (
    <ModalLayout
      title="键盘快捷键"
      titleIcon={<Keyboard size={18} />}
      onClose={onClose}
      width={460}
    >
      <div className="px-5 pt-3 pb-5 flex flex-col gap-4">
        {SHORTCUT_GROUPS.map((group) => (
          <div key={group.title}>
            <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted">
              {group.title}
            </div>
            <div className="flex flex-col gap-2">
              {group.entries.map((shortcut) => (
                <div key={shortcut.keys} className="flex items-center justify-between min-h-[36px]">
                  <div className="flex flex-wrap gap-1.5">
                    {shortcut.keys.split(' / ').map((key) => (
                      <Key key={key} label={key} />
                    ))}
                  </div>
                  <span className="text-secondary text-sm">{shortcut.desc}</span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </ModalLayout>
  );
}