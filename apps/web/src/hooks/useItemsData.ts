import { useState, useEffect, useCallback, useRef } from 'react';
import type { Dispatch, SetStateAction, MutableRefObject } from 'react';
import type { Item } from '../types';
import type { AppSettings } from '../utils/settings';
import { countItems, listItems, searchItems } from '../utils/api';
import { showToast } from '../utils/toast';

export type ItemFilter = 'all' | 'unread' | 'starred' | 'readLater';

/** 搜索结果的空状态类型，用于区分「读完了」和「搜不到/搜索失败」。 */
export type SearchStatus = 'idle' | 'no-results' | 'error';

export interface UseItemsDataParams {
  selectedSourceId: number | null;
  selectedFolderId: number | null;
  filter: ItemFilter;
  settings: AppSettings;
  fetchUnreadCount: (signal?: AbortSignal) => Promise<void>;
  abortRef: MutableRefObject<AbortController | null>;
  setSelectedItem: Dispatch<SetStateAction<Item | null>>;
}

export interface UseItemsDataResult {
  items: Item[];
  totalCount: number;
  readLaterCount: number;
  starredCount: number;
  loadingItems: boolean;
  searchKeyword: string;
  isSearchMode: boolean;
  searchStatus: SearchStatus;
  setSearchKeyword: Dispatch<SetStateAction<string>>;
  setIsSearchMode: Dispatch<SetStateAction<boolean>>;
  setItems: Dispatch<SetStateAction<Item[]>>;
  fetchItems: (signal?: AbortSignal) => Promise<void>;
  loadMore: (signal?: AbortSignal) => Promise<void>;
  hasMore: boolean;
  loadingMore: boolean;
  fetchItemCount: (signal?: AbortSignal) => Promise<void>;
  fetchReadLaterCount: (signal?: AbortSignal) => Promise<void>;
  fetchStarredCount: (signal?: AbortSignal) => Promise<void>;
  fetchSearchItems: (keyword: string, signal?: AbortSignal) => Promise<void>;
}

// 封装文章列表、搜索、计数的数据获取逻辑
export function useItemsData(params: UseItemsDataParams): UseItemsDataResult {
  const {
    selectedSourceId,
    selectedFolderId,
    filter,
    settings,
    fetchUnreadCount,
    abortRef,
    setSelectedItem,
  } = params;

  const [items, setItems] = useState<Item[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [readLaterCount, setReadLaterCount] = useState(0);
  const [starredCount, setStarredCount] = useState(0);
  const [loadingItems, setLoadingItems] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [isSearchMode, setIsSearchMode] = useState(false);

  // 列表分页：分批加载以替代原 limit=100 硬截断（消除"文章列表遗漏"根因）
  const PAGE_SIZE = 50;
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const offsetRef = useRef(0);

  // 构建文章查询参数
  const buildItemParams = useCallback(() => {
    const params = new URLSearchParams();
    if (selectedSourceId !== null) {
      params.append('sourceId', String(selectedSourceId));
    } else if (selectedFolderId !== null) {
      params.append('folderId', String(selectedFolderId));
    }
    if (filter === 'unread') {
      params.append('unread', 'true');
    }
    if (filter === 'starred') {
      params.append('starred', 'true');
    }
    if (filter === 'readLater') {
      params.append('readLater', 'true');
    }
    // 根据设置隐藏私密订阅源
    if (settings.hidePrivateInTimeline) {
      params.append('hidePrivate', 'true');
    }
    // 分页加载：每页拉取 PAGE_SIZE 条，offset 由 offsetRef 控制
    params.append('limit', String(PAGE_SIZE));
    params.append('offset', String(offsetRef.current));
    // 文章列表排序
    if (settings.listSortOrder === 'oldest') {
      params.append('orderBy', 'oldest');
    }
    return params;
  }, [selectedSourceId, selectedFolderId, filter, settings.hidePrivateInTimeline, settings.listSortOrder, PAGE_SIZE]);

  // 获取文章总数（独立构建参数，不受 filter 影响）
  const fetchItemCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = new URLSearchParams();
      if (selectedSourceId !== null) {
        params.append('sourceId', String(selectedSourceId));
      } else if (selectedFolderId !== null) {
        params.append('folderId', String(selectedFolderId));
      }
      // 注意：不添加 filter 参数，确保始终获取全文总数
      if (settings.hidePrivateInTimeline) {
        params.append('hidePrivate', 'true');
      }
      setTotalCount(await countItems(params, signal));
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch item count:', err);
      showToast('获取文章总数失败', 'error');
    }
  }, [selectedSourceId, selectedFolderId, settings.hidePrivateInTimeline]);

  // 获取稍后阅读未读数
  const fetchReadLaterCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = new URLSearchParams();
      params.append('readLater', 'true');
      if (settings.hidePrivateInTimeline) {
        params.append('hidePrivate', 'true');
      }
      setReadLaterCount(await countItems(params, signal));
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch read later count:', err);
      showToast('获取稍后阅读数失败', 'error');
    }
  }, [settings.hidePrivateInTimeline]);

  // 获取收藏数
  const fetchStarredCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = new URLSearchParams();
      params.append('starred', 'true');
      if (settings.hidePrivateInTimeline) {
        params.append('hidePrivate', 'true');
      }
      setStarredCount(await countItems(params, signal));
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch starred count:', err);
      showToast('获取收藏数失败', 'error');
    }
  }, [settings.hidePrivateInTimeline]);

  // 获取文章列表（分页）
  const fetchItems = useCallback(async (signal?: AbortSignal, opts?: { append?: boolean }) => {
    const append = opts?.append ?? false;
    if (append) setLoadingMore(true);
    else setLoadingItems(true);
    try {
      const params = buildItemParams();
      if (!append) {
        offsetRef.current = 0;
      }
      params.set('offset', String(offsetRef.current));
      const data = await listItems(params, signal);
      const arr = Array.isArray(data) ? data : [];
      if (append) {
        setItems(prev => [...prev, ...arr]);
        offsetRef.current += arr.length;
      } else {
        setItems(arr);
        offsetRef.current = arr.length;
      }
      setOffset(offsetRef.current);
      setHasMore(arr.length === PAGE_SIZE);
    } catch (err) {
      if (!signal?.aborted) {
        console.error('Failed to fetch items:', err);
        showToast('获取文章列表失败', 'error');
      }
    } finally {
      setLoadingItems(false);
      setLoadingMore(false);
    }
  }, [buildItemParams, PAGE_SIZE]);

  // 加载下一页（滚动触底或点击"加载更多"时调用）
  const loadMore = useCallback(async (signal?: AbortSignal) => {
    if (loadingMore || loadingItems || !hasMore) return;
    await fetchItems(signal, { append: true });
  }, [fetchItems, hasMore, loadingMore, loadingItems]);

  // 搜索状态：区分「没有结果」和「搜索失败」，供空态渲染分支使用
  const [searchStatus, setSearchStatus] = useState<SearchStatus>('idle');

  // 搜索文章
  const fetchSearchItems = useCallback(async (keyword: string, signal?: AbortSignal) => {
    setLoadingItems(true);
    try {
      const params = new URLSearchParams();
      params.append('q', keyword);
      // 后端搜索单次最多返回 50 条；超出部分不返回也不报错，由调用方在 UI 提示截断
      params.append('limit', '50');
      const data = await searchItems(params, signal);
      const arr = Array.isArray(data) ? data : [];
      setItems(arr);
      setTotalCount(arr.length);
      setSearchStatus(arr.length === 0 ? 'no-results' : 'idle');
    } catch (err) {
      if (signal?.aborted) { setLoadingItems(false); return; }
      console.error('Failed to search items:', err);
      showToast('搜索文章失败', 'error');
      setItems([]);
      setTotalCount(0);
      setSearchStatus('error');
    } finally {
      setLoadingItems(false);
    }
  }, []);

  // 当选择源、文件夹或筛选条件变化时重新加载文章与计数
  useEffect(() => {
    const signal = abortRef.current?.signal;
    if (isSearchMode && searchKeyword) {
      fetchSearchItems(searchKeyword, signal);
      setSelectedItem(null);
    } else {
      setSearchStatus('idle');
      fetchItems(signal);
      fetchItemCount(signal);
      fetchUnreadCount(signal);
      fetchReadLaterCount(signal);
      fetchStarredCount(signal);
      setSelectedItem(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isSearchMode, searchKeyword, filter, selectedSourceId, selectedFolderId, fetchSearchItems, fetchItems, fetchItemCount, fetchUnreadCount, fetchReadLaterCount, fetchStarredCount]);

  return {
    items,
    totalCount,
    readLaterCount,
    starredCount,
    loadingItems,
    searchKeyword,
    isSearchMode,
    searchStatus,
    setSearchKeyword,
    setIsSearchMode,
    setItems,
    fetchItems,
    loadMore,
    hasMore,
    loadingMore,
    fetchItemCount,
    fetchReadLaterCount,
    fetchStarredCount,
    fetchSearchItems,
  };
}
