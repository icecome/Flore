import { useCallback, useEffect, useRef, useState } from 'react';
import {
  checkForUpdate,
  confirmRestart,
  getUpdateSnapshot,
  startUpdate,
  updateStateEvent,
  type UpdateState,
} from '../utils/api';

/**
 * 全局更新状态仓库：订阅后端事件 + 挂载时拉取快照。
 *
 * 设计要点：
 * - 状态属主在桌面壳（updatelet 状态机），前端只是投影。弹窗关闭 → 组件卸载 →
 *   订阅释放，但后端下载照常进行；下次挂载通过快照复原"正在下载 + 进度"。
 * - 事件回调与快照拉取都经 setState 收敛，天然防重入；并发点击同一操作
 *   由后端 inFlight 拒绝并给出 toast。
 */
export default function useUpdate() {
  const [snapshot, setSnapshot] = useState<UpdateState>(() => ({
    status: 'idle',
    info: null,
    progress: 0,
  }));
  const [ready, setReady] = useState(false);
  const stateRef = useRef(snapshot);
  const syncedRef = useRef(false);

  const apply = useCallback((st: UpdateState) => {
    stateRef.current = st;
    setSnapshot(st);
  }, []);

  useEffect(() => {
    if (syncedRef.current) return;
    syncedRef.current = true;

    let cancelled = false;
    (async () => {
      const st = await getUpdateSnapshot().catch(() => null);
      if (!cancelled && st) apply(st);
    })();

    let off: (() => void) | undefined;
    const win = window as unknown as {
      runtime?: { EventsOn?: (name: string, cb: (...data: unknown[]) => void) => () => void };
    };
    if (win.runtime?.EventsOn) {
      off = win.runtime.EventsOn(updateStateEvent, (data) => {
        const st = Array.isArray(data) ? data[0] : data;
        if (st && typeof st === 'object' && 'status' in st) {
          apply(st as unknown as UpdateState);
        }
      });
    }
    return () => {
      cancelled = true;
      off?.();
    };
  }, [apply]);

  useEffect(() => {
    if (snapshot.status === 'ready') {
      setReady(true);
      return;
    }
    setReady(false);
  }, [snapshot.status]);

  const check = useCallback(async () => {
    try {
      await checkForUpdate();
    } catch (err) {
      console.error('Update check failed:', err);
      throw err;
    }
  }, []);

  const download = useCallback(async () => {
    try {
      await startUpdate();
    } catch (err) {
      console.error('Update download failed:', err);
      throw err;
    }
  }, []);

  const restart = useCallback(async () => {
    try {
      await confirmRestart();
    } catch (err) {
      console.error('Update restart failed:', err);
      throw err;
    }
  }, []);

  return {
    snapshot,
    ready,
    check,
    download,
    restart,
  };
}
