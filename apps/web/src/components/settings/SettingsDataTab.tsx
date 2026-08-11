import { useCallback, useRef, useState, type ChangeEvent } from "react";
import { RefreshCw, Database, Upload, RotateCw } from "../icons";
import ConfirmDialog from "../ConfirmDialog";
import BackupList from "./BackupList";
import BackupPolicyForm from "./BackupPolicyForm";
import RetentionForm from "./RetentionForm";
import { useBackups } from "../../hooks/useBackups";
import { cleanupArticles, type BackupEntry, type RestoreScope } from "../../utils/api";
import { formatBytes } from "../../lib/format";
import { showToast } from "../../utils/toast";
import type { AppSettings } from "../../utils/settings";

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
  onRestartApp: () => Promise<void>;
}

/** 待用户二次确认的破坏性操作 */
type PendingAction =
  | { kind: "restore"; backup: BackupEntry; scope: RestoreScope }
  | { kind: "delete"; name: string }
  | { kind: "import-restore"; backup: BackupEntry }
  | { kind: "restart" };

const RESTORE_CONFIRM_MESSAGE: Record<RestoreScope, (name: string) => string> = {
  all: (name) =>
    `确定要从备份「${name}」恢复吗？\n当前数据将先自动备份，再被该备份替换。恢复后建议重启应用生效。`,
  config: (name) => `确定要从备份「${name}」仅恢复配置吗？\n当前设置项将被该备份的配置覆盖。`,
  opml: (name) => `确定要从备份「${name}」仅恢复订阅源吗？\n当前订阅列表将被该备份的订阅源覆盖。`,
};

function confirmMessage(action: PendingAction): string {
  if (action.kind === "restore") return RESTORE_CONFIRM_MESSAGE[action.scope](action.backup.name);
  if (action.kind === "import-restore")
    return `已导入备份「${action.backup.name}」，是否立即恢复？\n恢复前当前数据将先自动备份。`;
  if (action.kind === "restart")
    return "重启将关闭并重新打开应用。未保存的窗口状态会丢失，确定要继续吗？";
  return `确定要删除备份「${action.name}」吗？此操作不可恢复。`;
}

export default function SettingsDataTab({ settings, updateSetting, onRestartApp }: Props) {
  const { backups, busy, refresh, create, remove, download, restore, importFile, cleanup } =
    useBackups();
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [cleaningArticles, setCleaningArticles] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const handleRestore = useCallback((backup: BackupEntry, scope: RestoreScope) => {
    setPending({ kind: "restore", backup, scope });
  }, []);

  const handleDelete = useCallback((backup: BackupEntry) => {
    setPending({ kind: "delete", name: backup.name });
  }, []);

  const handleConfirm = useCallback(async () => {
    const action = pending;
    setPending(null);
    if (!action) return;
    if (action.kind === "restore") await restore(action.backup.name, action.scope);
    else if (action.kind === "import-restore") await restore(action.backup.name, "all");
    else if (action.kind === "restart") await onRestartApp();
    else await remove(action.name);
  }, [pending, remove, restore, onRestartApp]);

  const handleRestart = useCallback(() => {
    setPending({ kind: "restart" });
  }, []);

  const handleFileChange = useCallback(
    async (e: ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (!file) return;
      const imported = await importFile(file);
      if (!imported) return;
      // 导入成功后弹出确认框，让用户选择是否立即恢复
      setPending({ kind: "import-restore", backup: imported });
    },
    [importFile]
  );

  const handleCleanupArticles = useCallback(async () => {
    setCleaningArticles(true);
    try {
      await cleanupArticles({
        retentionDays: settings.articleRetentionDays,
        retentionMax: settings.articleRetentionMax,
        excludeStarred: settings.retentionExcludeStarred,
        excludeReadLater: settings.retentionExcludeReadLater,
      });
      showToast("已清理过期文章");
    } catch (err) {
      console.error("Cleanup articles error:", err);
      showToast("清理失败");
    } finally {
      setCleaningArticles(false);
    }
  }, [
    settings.articleRetentionDays,
    settings.articleRetentionMax,
    settings.retentionExcludeStarred,
    settings.retentionExcludeReadLater,
  ]);

  const working = busy !== "";

  return (
    <div className="min-h-0 flex-1 space-y-6 overflow-y-auto">
      {/* 备份与恢复 */}
      <div>
        <h3 className="mb-3 text-[14px] font-semibold text-primary">备份与恢复</h3>

        {/* 顶部操作栏 */}
        <div className="flex items-center gap-2 mb-3">
          <button
            onClick={create}
            disabled={working}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[13px] border border-border rounded-md bg-elevated hover:bg-hover hover:text-primary transition-colors disabled:opacity-50"
          >
            <Database size={14} />
            立即备份
          </button>
          <button
            onClick={() => fileRef.current?.click()}
            disabled={working}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[13px] border border-border rounded-md bg-elevated hover:bg-hover hover:text-primary transition-colors disabled:opacity-50"
          >
            <Upload size={14} />
            导入备份
          </button>
          <button
            onClick={refresh}
            disabled={working}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[13px] border border-border rounded-md bg-elevated hover:bg-hover hover:text-primary transition-colors disabled:opacity-50"
          >
            <RefreshCw size={14} />
            刷新
          </button>
          <button
            onClick={handleRestart}
            disabled={working}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[13px] border border-border rounded-md bg-elevated hover:bg-hover hover:text-primary transition-colors disabled:opacity-50"
          >
            <RotateCw size={14} />
            重启应用
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".zip"
            className="hidden"
            onChange={handleFileChange}
          />
        </div>

        {backups.length > 0 && (
          <p className="text-[12px] text-muted mb-3">
            最近备份: {backups[0].modTime}（{formatBytes(backups[0].size)}）
          </p>
        )}

        <BackupList
          backups={backups}
          disabled={working}
          onDownload={download}
          onRestore={handleRestore}
          onDelete={handleDelete}
        />
      </div>

      <BackupPolicyForm
        settings={settings}
        updateSetting={updateSetting}
        cleaning={busy === "cleanup"}
        onCleanup={cleanup}
      />

      <RetentionForm
        settings={settings}
        updateSetting={updateSetting}
        cleaning={cleaningArticles}
        onCleanup={handleCleanupArticles}
      />

      {pending && (
        <ConfirmDialog
          title={
            pending.kind === "delete"
              ? "确认删除"
              : pending.kind === "restart"
                ? "重启应用"
                : "导入并恢复"
          }
          message={confirmMessage(pending)}
          confirmText={
            pending.kind === "delete" ? "删除" : pending.kind === "restart" ? "重启" : "立即恢复"
          }
          danger={pending.kind === "delete"}
          onConfirm={handleConfirm}
          onCancel={() => setPending(null)}
        />
      )}
    </div>
  );
}
