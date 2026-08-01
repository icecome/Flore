import { useState, useEffect, useCallback, useRef } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import type { Source, Folder } from '../types';
import type { AppSettings } from '../utils/settings';
import { getApi } from '../utils/api.js';
import { showToast } from '../utils/toast';

export interface UseSourcesDataResult {
  sources: Source[];
  folders: Folder[];
  unreadCountInScope: number;
  loadingSources: boolean;
  setSources: Dispatch<SetStateAction<Source[]>>;
  setFolders: Dispatch<SetStateAction<Folder[]>>;
  setUnreadCountInScope: Dispatch<SetStateAction<number>>;
  fetchSources: () => Promise<void>;
  fetchFolders: () => Promise<void>;
  fetchUnreadCount: (signal?: AbortSignal) => Promise<void>;
}

// 封装文件夹、订阅源、未读数的数据获取逻辑
export function useSourcesData(
  selectedSourceId: number | null,
  selectedFolderId: number | null,
  settings: AppSettings
): UseSourcesDataResult {
  const [sources, setSources] = useState<Source[]>([]);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [unreadCountInScope, setUnreadCountInScope] = useState(0);
  const [loadingSources, setLoadingSources] = useState(false);

  // 获取文件夹列表
  const fetchFolders = useCallback(async () => {
    try {
      const res = await fetch(`${getApi()}/folders`, { cache: 'no-store' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as Folder[];
      setFolders(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch folders:', err);
      showToast('获取文件夹列表失败');
    }
  }, []);

  // 获取订阅源列表
  const fetchSources = useCallback(async () => {
    setLoadingSources(true);
    try {
      const res = await fetch(`${getApi()}/sources`, { cache: 'no-store' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as Source[];
      setSources(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch sources:', err);
      showToast('获取订阅源列表失败');
    } finally {
      setLoadingSources(false);
    }
  }, []);

  // 获取未读总数（仅基于 source/folder/hidePrivate 范围，不管当前 filter）
  const fetchUnreadCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = new URLSearchParams();
      if (selectedSourceId !== null) {
        params.append('sourceId', String(selectedSourceId));
      } else if (selectedFolderId !== null) {
        params.append('folderId', String(selectedFolderId));
      }
      if (settings.hidePrivateInTimeline && selectedSourceId === null && selectedFolderId === null) {
        params.append('hidePrivate', 'true');
      }
      params.append('unread', 'true');
      const res = await fetch(`${getApi()}/items/count?${params.toString()}`, { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { count: number };
      setUnreadCountInScope(typeof data.count === 'number' ? data.count : 0);
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch unread count:', err);
      showToast('获取未读数失败');
    }
  }, [selectedSourceId, selectedFolderId, settings.hidePrivateInTimeline]);

  // 初始加载
  useEffect(() => {
    fetchSources();
    fetchFolders();
  }, [fetchSources, fetchFolders]);

  // 源健康启动检测：首次加载源后，若存在异常源则提示用户
  const healthCheckedRef = useRef(false);
  useEffect(() => {
    if (healthCheckedRef.current || sources.length === 0) return;
    healthCheckedRef.current = true;
    const badCount = sources.filter((s) => s.fetchFailCount >= 3).length;
    if (badCount > 0) {
      showToast(`${badCount} 个订阅源异常，请在设置中查看详情`);
    }
  }, [sources]);

  return {
    sources,
    folders,
    unreadCountInScope,
    loadingSources,
    setSources,
    setFolders,
    setUnreadCountInScope,
    fetchSources,
    fetchFolders,
    fetchUnreadCount,
  };
}
