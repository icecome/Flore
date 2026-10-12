import {
  type AppSettings,
  type MarkReadMode,
  type ListDensity,
  type ListSortOrder,
  type ListDateFormat,
  MARK_READ_OPTIONS,
  LIST_DENSITY_OPTIONS,
  LIST_SORT_OPTIONS,
  LIST_DATE_FORMAT_OPTIONS,
} from '../../utils/settings';
import { Section, Row, Toggle, Select, Slider } from './SettingsShared';

const INTERVAL_OPTIONS: { value: number; label: string }[] = [
  { value: 30, label: '30 分钟' },
  { value: 60, label: '1 小时' },
  { value: 120, label: '2 小时' },
  { value: 180, label: '3 小时' },
  { value: 360, label: '6 小时' },
  { value: 720, label: '12 小时' },
  { value: 1440, label: '24 小时' },
];

// 首次抓取时间窗口选项（T-v02-02）：0=全部
const INITIAL_FETCH_WINDOW_OPTIONS: { value: number; label: string }[] = [
  { value: 7, label: '最近 7 天' },
  { value: 30, label: '最近 30 天' },
  { value: 0, label: '全部历史' },
];

const OPEN_MODE_OPTIONS: { value: AppSettings['openArticleMode']; label: string }[] = [
  { value: 'rss', label: '原文模式' },
  { value: 'readability', label: '全文模式' },
  { value: 'iframe', label: '网页模式' },
  { value: 'browser', label: '浏览器打开' },
];

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

export default function SettingsGeneralTab({ settings, updateSetting }: Props) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="时间线">
        <Row
          title="启动时仅显示未读"
          desc="打开应用后默认进入「未读文章」，需要查看历史记录时再切回「全部文章」。"
          control={
            <Toggle
              checked={settings.unreadOnStart}
              onChange={(v) => updateSetting('unreadOnStart', v)}
            />
          }
        />
        <Row
          title="淡化已读条目"
          desc="降低已读条目的显示亮度，未读条目保持常规亮度，便于在长列表中定位。"
          control={
            <Toggle checked={settings.dimRead} onChange={(v) => updateSetting('dimRead', v)} />
          }
        />
        <Row
          title="侧边栏隐藏私密订阅"
          desc="不在侧边栏列出标记为私密的订阅源，已选中的私密订阅源仍会显示。"
          control={
            <Toggle
              checked={settings.hidePrivateInSidebar}
              onChange={(v) => updateSetting('hidePrivateInSidebar', v)}
            />
          }
        />
        <Row
          title="时间线隐藏私密文章"
          desc="标记为私密的订阅源，其文章不进入「全部文章」时间线。直接打开该源时不受影响。"
          control={
            <Toggle
              checked={settings.hidePrivateInTimeline}
              onChange={(v) => updateSetting('hidePrivateInTimeline', v)}
            />
          }
        />
        <Row
          title="隐藏已读源"
          desc="不列出未读数为 0 的订阅源。抓取从未成功过的源始终显示，避免新订阅被隐藏。"
          control={
            <Toggle checked={settings.hideRead} onChange={(v) => updateSetting('hideRead', v)} />
          }
        />
      </Section>

      <Section title="自动标记已读">
        <Row
          title="标记方式"
          desc="选择触发时机。三种方式的触发点不同，按阅读习惯选择其一。"
          control={
            <Select
              value={settings.markReadMode}
              options={MARK_READ_OPTIONS}
              onChange={(v) => updateSetting('markReadMode', v as MarkReadMode)}
            />
          }
        />
        {settings.markReadMode === 'hover' && (
          <Row
            title="悬停延迟"
            desc="鼠标停留在条目上的时长。延迟过短会导致快速划过时误标记。"
            control={
              <Slider
                value={settings.markReadHoverDelay}
                min={200}
                max={3000}
                step={100}
                unit=" ms"
                onChange={(v) => updateSetting('markReadHoverDelay', v)}
              />
            }
          />
        )}
      </Section>

      <Section title="文章列表">
        <Row
          title="显示密度"
          desc="紧凑模式可在一屏内看到更多条目标题。"
          control={
            <Select
              value={settings.listDensity}
              options={LIST_DENSITY_OPTIONS}
              onChange={(v) => updateSetting('listDensity', v as ListDensity)}
            />
          }
        />
        <Row
          title="排序方式"
          desc="最新优先按发布时间倒序，最旧优先按发布时间正序。"
          control={
            <Select
              value={settings.listSortOrder}
              options={LIST_SORT_OPTIONS}
              onChange={(v) => updateSetting('listSortOrder', v as ListSortOrder)}
            />
          }
        />
        <Row
          title="显示摘要预览"
          desc="在标题下方显示文章摘要，可用于判断是否需要点开。"
          control={
            <Toggle
              checked={settings.listShowPreview}
              onChange={(v) => updateSetting('listShowPreview', v)}
            />
          }
        />
        <Row
          title="日期格式"
          desc="相对时间显示「3 小时前」，绝对时间显示具体日期。"
          control={
            <Select
              value={settings.listDateFormat}
              options={LIST_DATE_FORMAT_OPTIONS}
              onChange={(v) => updateSetting('listDateFormat', v as ListDateFormat)}
            />
          }
        />
      </Section>

      <Section title="抓取">
        <Row
          title="自动抓取周期"
          desc="所有订阅源按此周期统一轮询。内容未变化的源会通过 HTTP 304 协商跳过，不重复下载。"
          control={
            <Select
              value={settings.defaultInterval}
              options={INTERVAL_OPTIONS}
              onChange={(v) => updateSetting('defaultInterval', Number(v))}
            />
          }
        />
        <Row
          title="首次抓取范围"
          desc="新订阅源首次抓取时只回填此天数内的文章，避免历史文章淹没列表。修改仅影响此后新增的订阅源。"
          control={
            <Select
              value={settings.initialFetchWindowDays}
              options={INITIAL_FETCH_WINDOW_OPTIONS}
              onChange={(v) => updateSetting('initialFetchWindowDays', Number(v))}
            />
          }
        />
        <Row
          title="启动时自动抓取"
          desc="打开应用后立即抓取一次，不等待下一个轮询周期。"
          control={
            <Toggle
              checked={settings.autoFetchOnStart}
              onChange={(v) => updateSetting('autoFetchOnStart', v)}
            />
          }
        />
      </Section>

      <Section title="文章打开方式">
        <Row
          title="默认打开方式"
          desc="点击文章时使用的打开方式。阅读器工具栏可在单篇文章上临时切换。"
          control={
            <Select
              value={settings.openArticleMode}
              options={OPEN_MODE_OPTIONS}
              onChange={(v) =>
                updateSetting('openArticleMode', v as AppSettings['openArticleMode'])
              }
            />
          }
        />
      </Section>

      <Section title="自动分组">
        <Row
          title="按域名自动分组"
          desc="添加订阅源时，按网站域名自动创建并归入同名文件夹。域名解析失败时保持未分组。"
          control={
            <Toggle
              checked={settings.autoGroup}
              onChange={(v) => updateSetting('autoGroup', v)}
            />
          }
        />
      </Section>
    </div>
  );
}