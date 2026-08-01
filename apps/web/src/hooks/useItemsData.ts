import { useState, useEffect, useCallback, useRef } from 'react';
import type { Dispatch, SetStateAction, MutableRefObject } from 'react';
import type { Item } from '../types';
import type { AppSettings } from '../utils/settings';
import { getApi } from '../utils/api.js';
import { showToast } from '../utils/toast';

export type ItemFilter = 'all' | 'unread' | 'starred' | 'readLater';

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
  loadingItems: boolean;
  searchKeyword: string;
  isSearchMode: boolean;
  setSearchKeyword: Dispatch<SetStateAction<string>>;
  setIsSearchMode: Dispatch<SetStateAction<boolean>>;
  setItems: Dispatch<SetStateAction<Item[]>>;
  fetchItems: (signal?: AbortSignal) => Promise<void>;
  loadMore: (signal?: AbortSignal) => Promise<void>;
  hasMore: boolean;
  loadingMore: boolean;
  fetchItemCount: (signal?: AbortSignal) => Promise<void>;
  fetchReadLaterCount: (signal?: AbortSignal) => Promise<void>;
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
    // 全部文章视图下，根据设置隐藏私密订阅源
    if (settings.hidePrivateInTimeline && selectedSourceId === null && selectedFolderId === null) {
      params.append('hidePrivate', 'true');
    }
    // 分页加载：每页拉取 PAGE_SIZE 条，offset 由 offsetRef 控制（替代原 limit=100 硬截断）
    params.append('limit', String(PAGE_SIZE));
    params.append('offset', String(offsetRef.current));
    // 文章列表排序
    if (settings.listSortOrder === 'oldest') {
      params.append('orderBy', 'oldest');
    }
    return params;
  }, [selectedSourceId, selectedFolderId, filter, settings.hidePrivateInTimeline, settings.listSortOrder, PAGE_SIZE]);

  // 获取文章总数
  const fetchItemCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = buildItemParams();
      params.delete('limit');
      const res = await fetch(`${getApi()}/items/count?${params.toString()}`, { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { count: number };
      setTotalCount(typeof data.count === 'number' ? data.count : 0);
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch item count:', err);
      showToast('获取文章总数失败');
    }
  }, [buildItemParams]);

  // 获取稍后阅读未读数
  const fetchReadLaterCount = useCallback(async (signal?: AbortSignal) => {
    try {
      const params = new URLSearchParams();
      params.append('readLater', 'true');
      if (settings.hidePrivateInTimeline) {
        params.append('hidePrivate', 'true');
      }
      const res = await fetch(`${getApi()}/items/count?${params.toString()}`, { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { count: number };
      setReadLaterCount(typeof data.count === 'number' ? data.count : 0);
    } catch (err) {
      if (signal?.aborted) return;
      console.error('Failed to fetch read later count:', err);
      showToast('获取稍后阅读数失败');
    }
  }, [settings.hidePrivateInTimeline]);

  // 获取文章列表（分页）
  // 默认从头加载（重置 offset）；opts.append=true 时追加下一页
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
      const res = await fetch(`${getApi()}/items?${params.toString()}`, { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as Item[];
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
        showToast('获取文章列表失败');
      }
    } finally {
      setLoadingItems(false);
      setLoadingMore(false);
    }
  }, [buildItemParams]);

  // 加载下一页（滚动触底或点击"加载更多"时调用）
  const loadMore = useCallback(async (signal?: AbortSignal) => {
    if (loadingMore || loadingItems || !hasMore) return;
    await fetchItems(signal, { append: true });
  }, [fetchItems, hasMore, loadingMore, loadingItems]);

  // 搜索文章
  const fetchSearchItems = useCallback(async (keyword: string, signal?: AbortSignal) => {
    setLoadingItems(true);
    try {
      const params = new URLSearchParams();
      params.append('q', keyword);
      params.append('limit', '50');
      const res = await fetch(`${getApi()}/items/search?${params.toString()}`, { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as Item[];
      setItems(Array.isArray(data) ? data : []);
      setTotalCount(Array.isArray(data) ? data.length : 0);
    } catch (err) {
      if (signal?.aborted) { setLoadingItems(false); return; }
      console.error('Failed to search items:', err);
      showToast('搜索文章失败');
      setItems([]);
      setTotalCount(0);
    } finally {
      setLoadingItems(false);
    }
  }, []);

  // 当选择源、文件夹或筛选条件变化时重新加载文章与计数
  // 搜索模式下仅加载搜索结果
  useEffect(() => {
    const signal = abortRef.current?.signal;
    if (isSearchMode && searchKeyword) {
      fetchSearchItems(searchKeyword, signal);
      setSelectedItem(null);
    } else {
      fetchItems(signal);
      fetchItemCount(signal);
      fetchUnreadCount(signal);
      fetchReadLaterCount(signal);
      setSelectedItem(null);
    }
    // abortRef 与 setSelectedItem 为稳定引用，无需作为依赖
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isSearchMode, searchKeyword, fetchSearchItems, fetchItems, fetchItemCount, fetchUnreadCount, fetchReadLaterCount]);

  return {
    items,
    totalCount,
    readLaterCount,
    loadingItems,
    searchKeyword,
    isSearchMode,
    setSearchKeyword,
    setIsSearchMode,
    setItems,
    fetchItems,
    loadMore,
    hasMore,
    loadingMore,
    fetchItemCount,
    fetchReadLaterCount,
    fetchSearchItems,
  };
}
