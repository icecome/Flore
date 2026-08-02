import { Trash2 } from '../icons';
import { Toggle } from './SettingsShared';
import type { AppSettings } from '../../utils/settings';

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
  cleaning: boolean;
  onCleanup: () => void;
}

/** 文章留存策略：保留天数 / 最大条数 / 排除项 Toggle / 立即清理 */
export default function RetentionForm({ settings, updateSetting, cleaning, onCleanup }: Props) {
  return (
    <div>
      <h3 className="mb-3 text-[14px] font-semibold text-primary">文章留存策略</h3>
      <div className="flex flex-col gap-1">
        {/* 保留天数 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">已读文章保留天数</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">0 表示不限制，永久保留</span>
          </div>
          <div className="ml-4 shrink-0 flex items-center gap-2">
            <input
              type="number"
              min={0}
              max={3650}
              value={settings.articleRetentionDays}
              onChange={(e) => updateSetting('articleRetentionDays', Number(e.target.value))}
              className="w-[72px] px-2 py-1 text-[13px] border border-border rounded-md bg-bg-input text-primary text-center outline-none focus:border-primary"
            />
            <span className="text-[12px] text-muted">天</span>
          </div>
        </div>

        {/* 最大文章数 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">已读文章最大数量</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">0 表示不限制，超过保留最新的</span>
          </div>
          <div className="ml-4 shrink-0 flex items-center gap-2">
            <input
              type="number"
              min={0}
              max={100000}
              step={100}
              value={settings.articleRetentionMax}
              onChange={(e) => updateSetting('articleRetentionMax', Number(e.target.value))}
              className="w-[72px] px-2 py-1 text-[13px] border border-border rounded-md bg-bg-input text-primary text-center outline-none focus:border-primary"
            />
            <span className="text-[12px] text-muted">篇</span>
          </div>
        </div>

        {/* 排除已收藏文章 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">排除已收藏文章</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">保护收藏夹中的文章不被自动清理</span>
          </div>
          <div className="ml-4 shrink-0">
            <Toggle
              checked={settings.retentionExcludeStarred}
              onChange={(v) => updateSetting('retentionExcludeStarred', v)}
            />
          </div>
        </div>

        {/* 排除稍后读文章 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">排除稍后读文章</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">保护稍后读列表中的文章不被自动清理</span>
          </div>
          <div className="ml-4 shrink-0">
            <Toggle
              checked={settings.retentionExcludeReadLater}
              onChange={(v) => updateSetting('retentionExcludeReadLater', v)}
            />
          </div>
        </div>
      </div>

      {/* 立即清理 — 独立于行列表之外 */}
      <div className="mt-4">
        <button
          onClick={onCleanup}
          disabled={cleaning}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[12.5px] border border-border rounded-md bg-surface text-secondary hover:bg-hover hover:text-primary hover:border-border-strong transition-colors disabled:opacity-50"
        >
          <Trash2 size={14} />
          立即清理过期文章
        </button>
      </div>
    </div>
  );
}