import { Section, ShortcutRow } from './SettingsShared';
import { SHORTCUT_GROUPS } from '../../utils/shortcuts';

export default function SettingsShortcutsTab() {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {SHORTCUT_GROUPS.map((group) => (
        <Section key={group.title} title={group.title}>
          {group.entries.map((entry) => (
            <ShortcutRow key={entry.keys} keys={entry.keys} desc={entry.desc} />
          ))}
        </Section>
      ))}
    </div>
  );
}