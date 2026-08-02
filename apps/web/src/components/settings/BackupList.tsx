import { Download, Trash2, ArchiveRestore, Cog, Rss } from '../icons';
import { formatBytes } from '../../lib/format';
import type { BackupEntry, RestoreScope } from '../../utils/api';

interface Props {
  backups: BackupEntry[];
  disabled: boolean;
  onDownload: (name: string) => void;
  onRestore: (backup: BackupEntry, scope: RestoreScope) => void;
  onDelete: (backup: BackupEntry) => void;
}

/** 备份列表表格：参照参考布局的 grid-cols-12 表格结构，使用本项目样式 */
export default function BackupList({ backups, disabled, onDownload, onRestore, onDelete }: Props) {
  if (backups.length === 0) {
    return <p className="mt-3 text-[12px] text-muted">暂无备份，点击「立即备份」或从其他设备导入备份。</p>;
  }

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      {/* 表头 */}
      <div className="grid grid-cols-12 gap-2 px-3 py-2.5 bg-elevated border-b border-border text-[11.5px] font-semibold text-muted uppercase tracking-wider">
        <div className="col-span-4 pl-1">备份名</div>
        <div className="col-span-3">创建时间</div>
        <div className="col-span-2 text-right">大小</div>
        <div className="col-span-3 text-center">操作</div>
      </div>

      {/* 数据行 */}
      {backups.map((b) => (
        <div
          key={b.name}
          className="grid grid-cols-12 gap-2 px-3 py-2.5 border-b border-border-subtle last:border-0 hover:bg-hover transition-colors items-center text-[13px] text-primary"
        >
          <div className="col-span-4 pl-1 flex items-center gap-1.5 truncate">
            <ArchiveRestore size={14} className="shrink-0 text-primary" />
            <span className="truncate">{b.name}</span>
          </div>
          <div className="col-span-3 text-muted text-[12px]">{b.modTime}</div>
          <div className="col-span-2 text-right text-muted text-[12px]">{formatBytes(b.size)}</div>
          <div className="col-span-3 flex justify-center gap-0.5">
            <button
              onClick={() => onRestore(b, 'all')}
              disabled={disabled}
              className="p-1 rounded text-muted hover:bg-primary/10 hover:text-primary transition-colors disabled:opacity-40"
              title="全部恢复"
            >
              <ArchiveRestore size={14} />
            </button>
            {b.hasCfg && (
              <button
                onClick={() => onRestore(b, 'config')}
                disabled={disabled}
                className="p-1 rounded text-muted hover:bg-primary/10 hover:text-primary transition-colors disabled:opacity-40"
                title="仅恢复设置"
              >
                <Cog size={14} />
              </button>
            )}
            {b.hasOpml && (
              <button
                onClick={() => onRestore(b, 'opml')}
                disabled={disabled}
                className="p-1 rounded text-muted hover:bg-primary/10 hover:text-primary transition-colors disabled:opacity-40"
                title="仅恢复订阅"
              >
                <Rss size={14} />
              </button>
            )}
            <button
              onClick={() => onDownload(b.name)}
              disabled={disabled}
              className="p-1 rounded text-muted hover:bg-primary/10 hover:text-primary transition-colors disabled:opacity-40"
              title="下载导出"
            >
              <Download size={14} />
            </button>
            <button
              onClick={() => onDelete(b)}
              disabled={disabled}
              className="p-1 rounded text-muted hover:bg-danger/10 hover:text-danger transition-colors disabled:opacity-40"
              title="删除"
            >
              <Trash2 size={14} />
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}