import { useCallback, useEffect, useState } from 'react';
import {
  cleanupBackups,
  createBackup,
  deleteBackup,
  downloadBackup,
  importBackup,
  listBackups,
  restoreBackup,
  type BackupEntry,
  type RestoreScope,
} from '../utils/api';
import { showToast } from '../utils/toast';

/** 当前进行中的备份操作，用于禁用按钮避免重复提交 */
export type BackupBusy = '' | 'refresh' | 'create' | 'delete' | 'download' | 'restore' | 'import' | 'cleanup';

const RESTORE_DONE_MESSAGE: Record<RestoreScope, string> = {
  all: '已从备份恢复，建议重启应用生效',
  config: '配置已从备份恢复',
  opml: '订阅源已从备份恢复',
};

export interface UseBackupsResult {
  backups: BackupEntry[];
  busy: BackupBusy;
  refresh: () => Promise<void>;
  create: () => Promise<void>;
  remove: (name: string) => Promise<void>;
  removeMany: (names: string[]) => Promise<void>;
  download: (name: string) => Promise<void>;
  restore: (name: string, scope: RestoreScope) => Promise<void>;
  importFile: (file: File) => Promise<BackupEntry | null>;
  cleanup: () => Promise<void>;
}

/** 备份列表与备份相关操作的集中管理，所有失败路径都会 toast 提示 */
export function useBackups(): UseBackupsResult {
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [busy, setBusy] = useState<BackupBusy>('');

  const refresh = useCallback(async () => {
    try {
      const data = await listBackups();
      setBackups(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Refresh backups error:', err);
      showToast('获取备份列表失败');
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const create = useCallback(async () => {
    setBusy('create');
    try {
      const data = await createBackup();
      showToast(`备份已创建: ${data.name}`);
      await refresh();
    } catch (err) {
      console.error('Create backup error:', err);
      showToast('备份失败');
    } finally {
      setBusy('');
    }
  }, [refresh]);

  const remove = useCallback(async (name: string) => {
    setBusy('delete');
    try {
      await deleteBackup(name);
      showToast('备份已删除');
      await refresh();
    } catch (err) {
      console.error('Delete backup error:', err);
      showToast('删除失败');
    } finally {
      setBusy('');
    }
  }, [refresh]);

  const removeMany = useCallback(async (names: string[]) => {
    if (names.length === 0) return;
    setBusy('delete');
    let failed = 0;
    for (const name of names) {
      try {
        await deleteBackup(name);
      } catch (err) {
        failed += 1;
        console.error('Delete backup error:', err);
      }
    }
    const succeeded = names.length - failed;
    if (failed === 0) showToast(`${succeeded} 个备份已删除`);
    else if (succeeded === 0) showToast('删除失败');
    else showToast(`部分删除失败（${failed}/${names.length}）`);
    await refresh();
    setBusy('');
  }, [refresh]);

  const download = useCallback(async (name: string) => {
    setBusy('download');
    try {
      await downloadBackup(name);
    } catch (err) {
      console.error('Download backup error:', err);
      showToast('下载失败');
    } finally {
      setBusy('');
    }
  }, []);

  const restore = useCallback(async (name: string, scope: RestoreScope) => {
    setBusy('restore');
    try {
      await restoreBackup(name, scope);
      showToast(RESTORE_DONE_MESSAGE[scope]);
      await refresh();
    } catch (err) {
      console.error('Restore backup error:', err);
      showToast('恢复失败');
    } finally {
      setBusy('');
    }
  }, [refresh]);

  const importFile = useCallback(async (file: File): Promise<BackupEntry | null> => {
    setBusy('import');
    try {
      const data = await importBackup(file);
      await refresh();
      return data;
    } catch (err) {
      console.error('Import backup error:', err);
      showToast('导入失败：备份文件无效或损坏');
      return null;
    } finally {
      setBusy('');
    }
  }, [refresh]);

  const cleanup = useCallback(async () => {
    setBusy('cleanup');
    try {
      await cleanupBackups();
      showToast('已清理过期备份');
      await refresh();
    } catch (err) {
      console.error('Cleanup backups error:', err);
      showToast('清理失败');
    } finally {
      setBusy('');
    }
  }, [refresh]);

  return { backups, busy, refresh, create, remove, removeMany, download, restore, importFile, cleanup };
}
