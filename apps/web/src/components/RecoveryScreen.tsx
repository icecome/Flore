import { useEffect } from 'react';
import Button from './Button';
import { showToast } from '../utils/toast';
import { useRecovery, type RecoveryBackup } from '../hooks/useRecovery';

// 数据库损坏恢复界面（P0-006）。
// 当后端检测到主库不可读并进入恢复模式时展示：列出可用的备份与 .bak 快照，
// 提供一键恢复；无备份时明确告知并提供打开数据文件夹的指引（由桌面壳 OpenExternal 支持，
// Web 端退化为展示路径）。
export default function RecoveryScreen({ onRestored }: { onRestored: () => void }) {
  const { recovery, backups, loading, restoring, refreshBackups, restore } = useRecovery();

  useEffect(() => {
    if (recovery) void refreshBackups();
  }, [recovery, refreshBackups]);

  const handleRestore = async (b: RecoveryBackup) => {
    const ok = await restore(b.path);
    if (ok) {
      showToast('已恢复，正在重启应用……', 'success');
      onRestored();
    }
  };

  const formatSize = (bytes: number): string => {
    if (bytes <= 0) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div className="flex h-full w-full items-center justify-center bg-canvas p-6">
      <div className="w-full max-w-[560px] rounded-lg border border-border bg-surface p-6">
        <h1 className="mb-2 text-lg font-semibold text-primary">数据库需要恢复</h1>
        <p className="mb-4 text-[13px] leading-relaxed text-secondary">
          本地数据库文件似乎已损坏，无法正常读取。你可以从下面的备份中恢复；
          恢复前会自动把当前文件保存为 <code className="text-[12px]">.bak/时间戳</code> 快照。
        </p>

        {recovery && (
          <div className="mb-4 rounded-md border border-border-subtle bg-elevated p-3 text-[12px] text-muted">
            <div>数据目录：<span className="text-secondary">{recovery.dataDir}</span></div>
          </div>
        )}

        <div className="mb-4 flex items-center justify-between">
          <span className="text-[13px] font-medium text-primary">可用备份</span>
          <Button variant="secondary" onClick={refreshBackups} disabled={loading}>
            {loading ? '加载中……' : '刷新'}
          </Button>
        </div>

        {backups && backups.length > 0 ? (
          <div className="mb-4 flex max-h-[280px] flex-col gap-1.5 overflow-y-auto">
            {backups.map((b) => (
              <div
                key={b.path}
                className="flex items-center justify-between gap-3 rounded-md border border-border-subtle p-2.5"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[13px] text-primary" title={b.name}>{b.name}</div>
                  <div className="text-[11px] text-muted">
                    {b.type === 'snapshot' ? '自动快照' : '备份'} · {b.modTime}
                    {formatSize(b.size) ? ` · ${formatSize(b.size)}` : ''}
                  </div>
                </div>
                <Button
                  variant="primary"
                  onClick={() => handleRestore(b)}
                  disabled={restoring}
                >
                  {restoring ? '恢复中……' : '恢复'}
                </Button>
              </div>
            ))}
          </div>
        ) : (
          <div className="mb-4 rounded-md border border-border-subtle bg-elevated p-4 text-[13px] text-secondary">
            {backups === null ? '正在加载备份列表……' : '未找到可用备份。可手动打开数据目录，用系统备份替换 reader.db。'}
          </div>
        )}

        {recovery && (
          <p className="text-[12px] leading-relaxed text-muted">
            数据目录位置：<span className="break-all text-secondary">{recovery.dataDir}</span>
            ，可在其中手动管理备份文件。恢复后需重启应用。
          </p>
        )}
      </div>
    </div>
  );
}
