import type { AppSettings, CloseBehavior, MinimizeBehavior } from '../../utils/settings';
import { Section, Row, Toggle, Select, Slider } from './SettingsShared';

const CLOSE_BEHAVIOR_OPTIONS: { value: CloseBehavior; label: string }[] = [
  { value: 'quit', label: '退出应用' },
  { value: 'tray', label: '最小化到托盘' },
];

const MINIMIZE_BEHAVIOR_OPTIONS: { value: MinimizeBehavior; label: string }[] = [
  { value: 'taskbar', label: '最小化到任务栏' },
  { value: 'tray', label: '最小化到托盘' },
];

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

export default function SettingsNotifyTab({ settings, updateSetting }: Props) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="窗口行为">
        <Row
          title="关闭按钮行为"
          desc="点击窗口关闭按钮时的行为。"
          control={
            <Select
              value={settings.closeBehavior}
              options={CLOSE_BEHAVIOR_OPTIONS}
              onChange={(v) => updateSetting('closeBehavior', v as CloseBehavior)}
            />
          }
        />
        <Row
          title="最小化行为"
          desc="点击窗口最小化按钮时的行为。"
          control={
            <Select
              value={settings.minimizeBehavior}
              options={MINIMIZE_BEHAVIOR_OPTIONS}
              onChange={(v) => updateSetting('minimizeBehavior', v as MinimizeBehavior)}
            />
          }
        />
        <Row
          title="启用系统托盘"
          desc="在系统托盘显示图标，允许应用在后台运行。"
          control={
            <Toggle
              checked={settings.trayEnabled}
              onChange={(v) => updateSetting('trayEnabled', v)}
            />
          }
        />
      </Section>

      <Section title="桌面通知">
        <Row
          title="启用通知"
          desc="有新文章时推送系统通知。"
          control={
            <Toggle
              checked={settings.notifyEnabled}
              onChange={(v) => updateSetting('notifyEnabled', v)}
            />
          }
        />
        <Row
          title="通知聚合阈值"
          desc="当新文章超过此数量时，在通知中显示未读总数。"
          control={
            <Slider
              value={settings.notifyBatchMin}
              min={1}
              max={20}
              step={1}
              unit=" 篇"
              onChange={(v) => updateSetting('notifyBatchMin', v)}
            />
          }
        />
      </Section>
    </div>
  );
}
