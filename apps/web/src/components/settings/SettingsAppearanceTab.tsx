import { useState } from 'react';
import { Sun, Moon, Monitor } from '../icons';
import { cn } from '../../lib/cn';
import {
  type AppSettings,
  THEME_OPTIONS,
  LETTER_SPACING_OPTIONS,
  ACCENT_PRESETS,
  applyAccentColor,
  setThemePersist,
} from '../../utils/settings';
import { Section, Row, Select, Slider } from './SettingsShared';

export type Theme = 'light' | 'dark' | 'system';

const FONT_FAMILY_OPTIONS: { value: AppSettings['readerFontFamily']; label: string }[] = [
  { value: 'serif', label: '衬线体' },
  { value: 'sans', label: '无衬线体' },
  { value: 'mono', label: '等宽字体' },
];

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

export default function SettingsAppearanceTab({ settings, updateSetting }: Props) {
  // 用 AppSettings.appTheme 初始化，避免弹窗内状态与全局不一致。
  // 持久化写入 localStorage 与 appTheme 设置项（供后端同步），由 setThemePersist 统一处理。
  const [theme, setTheme] = useState<Theme>(() => {
    return (settings.appTheme === 'light' || settings.appTheme === 'dark') ? settings.appTheme : 'system';
  });

  const applyTheme = (value: Theme) => {
    setTheme(value);
    // 主题持久化失败不应阻断当前会话的视觉切换
    setThemePersist(value, (next) => updateSetting('appTheme', next.appTheme), settings);
  };

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="主题">
        <Row
          title="主题"
          desc="跟随系统时，随操作系统深浅色设置自动切换。"
          control={<ThemeSelector theme={theme} onChange={applyTheme} />}
        />
      </Section>
      <Section title="强调色">
        <Row
          title="强调色"
          desc="用于按钮、选中态与链接的高亮色，立即生效。"
          control={
            <div className="flex items-center gap-1.5">
              {ACCENT_PRESETS.map((preset) => (
                <button
                  key={preset.color}
                  onClick={() => {
                    updateSetting('accentColor', preset.color);
                    applyAccentColor(preset.color);
                  }}
                  className={cn(
                    'h-7 w-7 rounded-md border-2 transition-transform hover:scale-110',
                    (settings.accentColor || '#7B68EE') === preset.color
                      ? 'border-text-primary scale-110'
                      : 'border-transparent'
                  )}
                  style={{ backgroundColor: preset.color }}
                  title={preset.name}
                />
              ))}
              <label
                className="relative flex h-7 w-7 cursor-pointer items-center justify-center rounded-md border-2 border-dashed border-border transition-colors hover:border-border-strong"
                title="自定义颜色"
              >
                <input
                  type="color"
                  value={settings.accentColor || '#7B68EE'}
                  onInput={(e) => {
                    applyAccentColor((e.target as HTMLInputElement).value);
                  }}
                  onChange={(e) => {
                    updateSetting('accentColor', e.target.value);
                  }}
                  className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
                />
                <span className="text-[10px] text-muted">+</span>
              </label>
            </div>
          }
        />
      </Section>
      <Section title="排版设置">
        <Row
          title="默认字体"
          control={
            <Select
              value={settings.readerFontFamily}
              options={FONT_FAMILY_OPTIONS}
              onChange={(v) =>
                updateSetting('readerFontFamily', v as AppSettings['readerFontFamily'])
              }
            />
          }
        />
        <Row
          title="默认字号"
          control={
            <Slider
              value={settings.readerFontSize}
              min={14}
              max={24}
              step={1}
              unit="px"
              onChange={(v) => updateSetting('readerFontSize', v)}
            />
          }
        />
        <Row
          title="默认行高"
          control={
            <Slider
              value={settings.readerLineHeight}
              min={1.2}
              max={2.4}
              step={0.05}
              onChange={(v) => updateSetting('readerLineHeight', v)}
            />
          }
        />
        <Row
          title="最大宽度"
          control={
            <Slider
              value={settings.readerMaxWidth}
              min={500}
              max={1000}
              step={10}
              unit="px"
              onChange={(v) => updateSetting('readerMaxWidth', v)}
            />
          }
        />
        <Row
          title="字间距"
          control={
            <Select
              value={settings.readerLetterSpacing}
              options={LETTER_SPACING_OPTIONS}
              onChange={(v) =>
                updateSetting('readerLetterSpacing', v as AppSettings['readerLetterSpacing'])
              }
            />
          }
        />
        <Row
          title="阅读主题"
          control={
            <Select
              value={settings.readerTheme}
              options={THEME_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
              onChange={(v) => updateSetting('readerTheme', v as AppSettings['readerTheme'])}
            />
          }
        />
      </Section>
    </div>
  );
}

function ThemeSelector({ theme, onChange }: { theme: Theme; onChange: (t: Theme) => void }) {
  const opts: { value: Theme; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: '浅色', icon: <Sun size={14} /> },
    { value: 'dark', label: '深色', icon: <Moon size={14} /> },
    { value: 'system', label: '跟随系统', icon: <Monitor size={14} /> },
  ];

  return (
    <div className="flex gap-1.5">
      {opts.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-[12.5px] transition-colors',
            theme === o.value
              ? 'border-primary bg-primary-subtle text-primary'
              : 'border-border text-secondary hover:border-border-strong hover:text-primary'
          )}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  );
}
