import { useState, useEffect, useCallback } from 'react';
import { fetchRecoveryInfo, listRecoveryBackups, restoreFromRecovery } from '../utils/api';

export interface RecoveryBackup {
  name: string;
  type: 'zip' | 'snapshot';
  size: number;
  modTime: string;
  path: string;
}

export interface RecoveryInfo {
  dbPath: string;
  dataDir: string;
}

// 后端恢复模式检测（P0-006）：数据库损坏时后端进入降级恢复服务，
// 仍暴露 /api/recovery/info。前端据此展示恢复界面而非 onboarding 或空列表。
export function useRecovery() {
  const [recovery, setRecovery] = useState<RecoveryInfo | null>(null);
  const [backups, setBackups] = useState<RecoveryBackup[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [restoring, setRestoring] = useState(false);

  const check = useCallback(async () => {
    try {
      const info = await fetchRecoveryInfo();
      setRecovery(info);
      return true;
    } catch {
      setRecovery(null);
      return false;
    }
  }, []);

  const refreshBackups = useCallback(async () => {
    setLoading(true);
    try {
      setBackups(await listRecoveryBackups());
    } catch {
      setBackups([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const restore = useCallback(async (path: string) => {
    setRestoring(true);
    try {
      await restoreFromRecovery(path);
      return true;
    } finally {
      setRestoring(false);
    }
  }, []);

  useEffect(() => {
    void check();
  }, [check]);

  return { recovery, backups, loading, restoring, check, refreshBackups, restore };
}
