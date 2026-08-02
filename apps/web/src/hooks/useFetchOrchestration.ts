import { useState, useEffect, useCallback, useRef, type Dispatch, type SetStateAction } from 'react';
import { getFetchStatus, triggerFetch } from '../utils/api';
import { showToast } from '../utils/toast';

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

/** 轮询初始间隔与上限：抓取进行中按 +2s 退避，减少无效请求 */
const POLL_BASE_MS = 3000;
const POLL_MAX_MS = 15000;
/** 连续查询失败达到该次数时提示用户，避免后端已挂却无任何反馈 */
const POLL_FAIL_LIMIT = 3;

// 抓取编排：统一处理单次/文件夹/全量抓取的提交、轮询后端 fetch-status、
// 抓取完成后的数据刷新与系统通知触发。所有抓取均为异步，由后端 Coordinator 决定完成时机。
export function useFetchOrchestration({
  autoFetchOnStart,
  sourcesLength,
  refreshAllData,
  notifyFetchComplete,
}: UseFetchOrchestrationParams): UseFetchOrchestrationResult {
  const [refreshing, setRefreshing] = useState(false);

  // 回调存入 ref：调用方未 useCallback 时，轮询 effect 不会因函数身份变化而反复重启
  const refreshAllDataRef = useRef(refreshAllData);
  const notifyFetchCompleteRef = useRef(notifyFetchComplete);
  useEffect(() => {
    refreshAllDataRef.current = refreshAllData;
    notifyFetchCompleteRef.current = notifyFetchComplete;
  }, [refreshAllData, notifyFetchComplete]);

  // 立即抓取（支持全部文章 / 文件夹 / 单个订阅源）。
  // 抓取为异步（POST 仅提交任务），后端尚未写入数据库，故此处不发通知；
  // 通知在轮询检测到 fetch-status.fetching=false（抓取真正完成）时发送。
  const handleFetch = useCallback(async (scope: FetchScope) => {
    setRefreshing(true);
    try {
      await triggerFetch(scope);
    } catch (err) {
      console.error('Failed to fetch:', err);
      showToast('触发抓取失败，请稍后重试');
      setRefreshing(false);
    }
  }, []);

  // 启动后自动抓取一次（仅执行一次）
  const autoFetchedRef = useRef(false);
  useEffect(() => {
    if (autoFetchedRef.current) return;
    if (!autoFetchOnStart) return;
    if (sourcesLength === 0) return;
    autoFetchedRef.current = true;
    handleFetch({});
  }, [autoFetchOnStart, sourcesLength, handleFetch]);

  // 抓取期间轮询：查询后端抓取状态，fetching=false 时停止旋转并刷新数据。
  // 所有抓取（单源/文件夹/全量）统一异步，由 Coordinator 决定何时完成。
  const pollingRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollBackoffRef = useRef(POLL_BASE_MS);
  const pollFailRef = useRef(0);

  useEffect(() => {
    if (!refreshing) {
      if (pollingRef.current) {
        clearTimeout(pollingRef.current);
        pollingRef.current = null;
      }
      pollBackoffRef.current = POLL_BASE_MS;
      pollFailRef.current = 0;
      return;
    }
    let cancelled = false;
    const tick = async () => {
      try {
        const data = await getFetchStatus();
        pollFailRef.current = 0;
        if (cancelled) return;
        if (!data.fetching) {
          await refreshAllDataRef.current();
          // 抓取完成：发送系统通知。桌面版由桌面壳统一发送，此处仅 Web 版回退。
          await notifyFetchCompleteRef.current(data.newItems ?? 0);
          if (!cancelled) setRefreshing(false);
          return;
        }
      } catch (err) {
        // 状态查询失败时继续轮询，但连续失败需要给用户反馈
        console.error('Failed to poll fetch status:', err);
        pollFailRef.current += 1;
        if (pollFailRef.current === POLL_FAIL_LIMIT) {
          showToast('无法获取抓取进度，请检查后端服务');
        }
      }
      if (cancelled) return;
      pollBackoffRef.current = Math.min(pollBackoffRef.current + 2000, POLL_MAX_MS);
      pollingRef.current = setTimeout(tick, pollBackoffRef.current);
    };
    pollingRef.current = setTimeout(tick, pollBackoffRef.current);
    return () => {
      cancelled = true;
      if (pollingRef.current) {
        clearTimeout(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [refreshing]);

  return { refreshing, setRefreshing, handleFetch };
}
