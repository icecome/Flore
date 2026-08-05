import type { AppSettings } from '../../utils/settings';
import { Section, Row, Toggle, Slider } from './SettingsShared';

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

export default function SettingsNetworkTab({ settings, updateSetting }: Props) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="抓取设置">
        <Row
          title="抓取超时"
          desc="单个订阅源抓取的超时时间。"
          control={
            <Slider
              value={settings.fetchTimeout}
              min={5}
              max={120}
              step={5}
              unit="s"
              onChange={(v) => updateSetting('fetchTimeout', v)}
            />
          }
        />
        <Row
          title="最大并发数"
          desc="同时抓取的订阅源数量。"
          control={
            <Slider
              value={settings.fetchConcurrency}
              min={1}
              max={20}
              step={1}
              unit=""
              onChange={(v) => updateSetting('fetchConcurrency', v)}
            />
          }
        />
      </Section>

      <Section title="代理">
        <Row
          title="启用代理"
          desc="通过 HTTP 代理访问订阅源。适用于无法直接访问的站点。"
          control={
            <Toggle
              checked={settings.proxyEnabled}
              onChange={(v) => updateSetting('proxyEnabled', v)}
            />
          }
        />
        {settings.proxyEnabled && (
          <Row
            title="代理地址"
            desc="HTTP 代理地址，如 http://127.0.0.1:7890"
            control={
              <input
                type="text"
                value={settings.proxyUrl}
                onChange={(e) => updateSetting('proxyUrl', e.target.value)}
                placeholder="http://127.0.0.1:7890"
                className="w-[240px] rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
              />
            }
          />
        )}
      </Section>

      <Section title="隐私">
        <Row
          title="头像来源"
          desc="选择订阅源头像的获取方式。Yandex 服务快速但依赖第三方；直接抓取从源站获取，更准确但可能较慢。"
          control={
            <select
              value={settings.faviconMode ?? 'off'}
              onChange={(e) => updateSetting('faviconMode', e.target.value as 'off' | 'yandex' | 'direct')}
              className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
            >
              <option value="off">关闭（字母头像）</option>
              <option value="yandex">Yandex 图标服务</option>
              <option value="direct">直接抓取（后端代理）</option>
            </select>
          }
        />
      </Section>
    </div>
  );
}
