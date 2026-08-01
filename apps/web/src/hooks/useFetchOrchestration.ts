import { useState, useEffect, useCallback, useRef, type Dispatch, type SetStateAction } from 'react';
import { getApi } from '../utils/api.js';

export interface FetchScope {
  sourceId?: number;
  folderId?: number;
}

interface UseFetchOrchestrationParams {
  autoFetchOnStart: boolean;
  sourcesLength: number;
  refreshAllData: () => Promise<void>;
  notifyFetchComplete: (newItems: number) => Promise<void>;
}

interface UseFetchOrchestrationResult {
  refreshing: boolean;
  setRefreshing: Dispatch<SetStateAction<boolean>>;
  handleFetch: (scope: FetchScope) => Promise<void>;
}

// 抓取编排：统一处理单次/文件夹/全量抓取的提交、轮询后端 fetch-status、
// 抓取完成后的数据刷新与系统通知触发。所有抓取均为异步，由后端 Coordinator 决定完成时机。
export function useFetchOrchestration({
  autoFetchOnStart,
  sourcesLength,
  refreshAllData,
  notifyFetchComplete,
}: UseFetchOrchestrationParams): UseFetchOrchestrationResult {
  const [refreshing, setRefreshing] = useState(false);

  const buildFetchUrl = useCallback((scope: FetchScope) => {
    if (scope.sourceId !== undefined) return `${getApi()}/sources/${scope.sourceId}/fetch`;
    if (scope.folderId !== undefined) return `${getApi()}/folders/${scope.folderId}/fetch`;
    return `${getApi()}/sources/fetch-all`;
  }, []);

  // 立即抓取（支持全部文章 / 文件夹 / 单个订阅源）。
  // 抓取为异步（POST 仅提交任务），后端尚未写入数据库，故此处不发通知；
  // 通知在轮询检测到 fetch-status.fetching=false（抓取真正完成）时发送，修复 C-01。
  const handleFetch = useCallback(async (scope: FetchScope) => {
    setRefreshing(true);
    try {
      const fetchRes = await fetch(buildFetchUrl(scope), { method: 'POST' });
      if (!fetchRes.ok) throw new Error(`HTTP ${fetchRes.status}`);
    } catch (err) {
      console.error('Failed to fetch:', err);
      setRefreshing(false);
    }
  }, [buildFetchUrl]);

  // 启动后自动抓取一次（仅执行一次）
  const autoFetchedRef = useRef(false);
  useEffect(() => {
    if (autoFetchedRef.current) return;
    if (!autoFetchOnStart) return;
    if (sourcesLength === 0) return;
    autoFetchedRef.current = true;
    handleFetch({});
  }, [autoFetchOnStart, sourcesLength, handleFetch]);

  // 抓取期间轮询：每 3s 查询后端抓取状态，fetching=false 时停止旋转并刷新数据。
  // 所有抓取（单源/文件夹/全量）统一异步，由 Coordinator 决定何时完成。
  const pollingRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollBackoffRef = useRef(3000);

  useEffect(() => {
    if (!refreshing) {
      if (pollingRef.current) {
        clearTimeout(pollingRef.current);
        pollingRef.current = null;
      }
      pollBackoffRef.current = 3000;
      return;
    }
    const tick = async () => {
      try {
        const res = await fetch(`${getApi()}/sources/fetch-status`, { cache: 'no-store' });
        if (res.ok) {
          const data = (await res.json()) as { fetching: boolean; newItems: number };
          if (!data.fetching) {
            await refreshAllData();
            // 抓取完成：发送系统通知。桌面版由桌面壳统一发送，此处仅 Web 版回退。
            await notifyFetchComplete(data.newItems);
            setRefreshing(false);
            return;
          }
        }
      } catch { /* 状态查询失败时继续轮询 */ }
      // 抓取进行中：指数退避（3s→5s→8s，上限 15s）减少无效请求（m-04）
      pollBackoffRef.current = Math.min(pollBackoffRef.current + 2000, 15000);
      pollingRef.current = setTimeout(tick, pollBackoffRef.current);
    };
    pollingRef.current = setTimeout(tick, pollBackoffRef.current);
    return () => {
      if (pollingRef.current) {
        clearTimeout(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [refreshing, refreshAllData, notifyFetchComplete]);

  return { refreshing, setRefreshing, handleFetch };
}
