import { Trash2 } from '../icons';
import { Toggle, NumberInput } from './SettingsShared';
import type { AppSettings } from '../../utils/settings';

const BACKUP_INTERVAL_OPTIONS = [
  { value: '360', label: '每 6 小时' },
  { value: '720', label: '每 12 小时' },
  { value: '1440', label: '每天' },
  { value: '2880', label: '每 2 天' },
  { value: '10080', label: '每周' },
];

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
  cleaning: boolean;
  onCleanup: () => void;
}

/** 备份策略配置：保留数量 / 天数 / 自动备份开关 / 间隔 / 立即清理 */
export default function BackupPolicyForm({ settings, updateSetting, cleaning, onCleanup }: Props) {
  return (
    <div>
      <h3 className="mb-3 text-[14px] font-semibold text-primary">备份策略</h3>
      <div className="flex flex-col gap-1">
        {/* 启用自动备份 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">启用自动备份</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">按固定间隔自动创建压缩备份</span>
          </div>
          <div className="ml-4 shrink-0">
            <Toggle
              checked={settings.backupAutoEnabled}
              onChange={(v) => updateSetting('backupAutoEnabled', v)}
            />
          </div>
        </div>

        {settings.backupAutoEnabled && (
          <div className="flex items-center justify-between py-3 border-b border-border-subtle">
            <div className="flex flex-col">
              <span className="text-[13px] text-primary">自动备份间隔</span>
            </div>
            <div className="ml-4 shrink-0">
              <select
                value={settings.backupAutoInterval.toString()}
                onChange={(e) => updateSetting('backupAutoInterval', Number(e.target.value))}
                className="px-2.5 py-1.5 text-[13px] border border-border rounded-md bg-surface text-primary outline-none focus:border-primary"
              >
                {BACKUP_INTERVAL_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>
          </div>
        )}

        {/* 最大保留数量 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">最大保留数量</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">超过此数量的旧备份将被自动清理</span>
          </div>
          <div className="ml-4 shrink-0">
            <NumberInput
              value={settings.backupMaxKeep}
              min={1}
              max={100}
              unit="个"
              onChange={(v) => updateSetting('backupMaxKeep', v)}
            />
          </div>
        </div>

        {/* 最大保留天数 */}
        <div className="flex items-center justify-between py-3 border-b border-border-subtle">
          <div className="flex flex-col">
            <span className="text-[13px] text-primary">最大保留天数</span>
            <span className="mt-0.5 text-[12px] leading-snug text-muted">超过此天数的备份将被自动清理</span>
          </div>
          <div className="ml-4 shrink-0">
            <NumberInput
              value={settings.backupMaxDays}
              min={1}
              max={365}
              unit="天"
              onChange={(v) => updateSetting('backupMaxDays', v)}
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
          立即清理过期备份
        </button>
      </div>
    </div>
  );
}