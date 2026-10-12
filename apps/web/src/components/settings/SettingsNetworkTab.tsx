import type { AppSettings } from '../../utils/settings';
import { Section, Row, Toggle, Slider, Select } from './SettingsShared';

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

const FAVICON_MODE_OPTIONS: { value: AppSettings['faviconMode']; label: string }[] = [
  { value: 'off', label: '关闭（使用字母头像）' },
  { value: 'direct', label: '直接抓取（后端代理访问源站）' },
  { value: 'yandex', label: 'Yandex 图标服务（第三方）' },
];

export default function SettingsNetworkTab({ settings, updateSetting }: Props) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="抓取">
        <Row
          title="抓取超时"
          desc="单个订阅源的最长等待时间。源响应超过该时间按失败计，计入退避重试。"
          control={
            <Slider
              value={settings.fetchTimeout}
              min={5}
              max={120}
              step={5}
              unit=" s"
              onChange={(v) => updateSetting('fetchTimeout', v)}
            />
          }
        />
        <Row
          title="最大并发数"
          desc="同时抓取的订阅源数量。值越大整轮抓取越快，但对本机与目标站点的瞬时压力也越大。"
          control={
            <Slider
              value={settings.fetchConcurrency}
              min={1}
              max={20}
              step={1}
              onChange={(v) => updateSetting('fetchConcurrency', v)}
            />
          }
        />
      </Section>

      <Section title="代理">
        <Row
          title="使用代理抓取"
          desc="所有订阅源请求经由 HTTP 代理发出。适用于源站无法直接访问的网络环境。"
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
            desc="格式为 http://主机:端口，例如 http://127.0.0.1:7890。修改后立即生效。"
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

      <Section title="站点图标">
        <Row
          title="图标来源"
          desc="关闭时显示字母头像，不发任何网络请求。「直接抓取」经本机后端访问源站；「Yandex 图标服务」会把你订阅的站点域名发送给第三方服务 Yandex，请确认后选择。"
          control={
            <Select
              value={settings.faviconMode ?? 'off'}
              options={FAVICON_MODE_OPTIONS}
              onChange={(v) => updateSetting('faviconMode', v as AppSettings['faviconMode'])}
            />
          }
        />
      </Section>
    </div>
  );
}