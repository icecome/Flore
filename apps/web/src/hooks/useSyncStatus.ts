import { useEffect, useState } from 'react';
import type { SyncStatus } from '../types';
import { getSyncStatus } from '../utils/api';

const POLL_INTERVAL_MS = 30_000;

/** 轮询同步健康状态；失败时静默置空。页面隐藏时暂停轮询，恢复可见后立即刷新 */
export function useSyncStatus(): SyncStatus | null {
  const [status, setStatus] = useState<SyncStatus | null>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const controller = new AbortController();

    const poll = async () => {
      // 页面隐藏时暂停下一轮调度（visibilitychange 恢复时兜底刷新）
      if (document.hidden) return;
      try {
        const data = await getSyncStatus(controller.signal);
        if (!cancelled) setStatus(data);
      } catch (err) {
        // 卸载/中止导致的 AbortError 属预期行为，静默处理不告警
        if ((err as Error).name === 'AbortError' || controller.signal.aborted) return;
        console.error('Failed to poll sync status:', err);
        if (!cancelled) setStatus(null);
      }
      if (!cancelled) timer = setTimeout(poll, POLL_INTERVAL_MS);
    };

    const onVisible = () => {
      if (document.hidden) return;
      if (timer) clearTimeout(timer);
      poll();
    };

    poll();
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
      controller.abort();
    };
  }, []);

  return status;
}
