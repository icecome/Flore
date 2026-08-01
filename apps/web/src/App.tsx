import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { MenuIcon } from './components/icons';
import Sidebar from './components/Sidebar';
import ArticleList from './components/ArticleList';
import Reader from './components/Reader';
import TitleBar from './components/TitleBar';
import AddSourceModal, { type SourceFormData } from './components/AddSourceModal';
import MoveSourceModal from './components/MoveSourceModal';
import EditSourceModal from './components/EditSourceModal';
import ImportOPMLModal from './components/ImportOPMLModal';
import RenameModal from './components/RenameModal';
import ShortcutsHelpModal from './components/ShortcutsHelpModal';
import ExportArticlesModal, { type ExportScope } from './components/ExportArticlesModal';
import Button from './components/Button';
import ToastContainer from './components/ToastContainer';
import ConfirmDialog from './components/ConfirmDialog';
import ModalLayout from './components/ModalLayout';
import type { Item, Source, Folder } from './types';
import type { AppSettings } from './utils/settings';
import { loadSettings, loadSettingsFromServer, saveSettings, applyAccentColor, migrateLegacySettingsIfNeeded } from './utils/settings';
import { showToast } from './utils/toast';

import { getApi, downloadBlob, openExternal } from './utils/api.js';
import { useSourcesData } from './hooks/useSourcesData';
import { useItemsData, type ItemFilter } from './hooks/useItemsData';
import { useNotification } from './hooks/useNotification';
import { useFetchOrchestration } from './hooks/useFetchOrchestration';

function App() {
  const [settings, setSettings] = useState<AppSettings>(() => loadSettings());

  const handleSettingsChange = useCallback((next: AppSettings) => {
    setSettings(next);
    saveSettings(next);
    // applyAccentColor 由下方 useEffect 监听 settings.accentColor 触发，避免重复调用
  }, []);

  // 应用强调色（启动时及主题色变更时）
  useEffect(() => {
    applyAccentColor(settings.accentColor || '#7B68EE');
  }, [settings.accentColor]);

  // 启动后从后端加载设置，并执行一次性 localStorage 迁移
  // 失败时静默，保留 localStorage 中的配置继续使用
  useEffect(() => {
    let cancelled = false;
    (async () => {
      await migrateLegacySettingsIfNeeded();
      const serverSettings = await loadSettingsFromServer();
      if (cancelled || !serverSettings) return;
      // 后端有配置时合并到 state，覆盖本地默认值
      setSettings(serverSettings);
      saveSettings(serverSettings);
    })();
    return () => { cancelled = true; };
  }, []);

  const [selectedSourceId, setSelectedSourceId] = useState<number | null>(null);
  const [selectedFolderId, setSelectedFolderId] = useState<number | null>(null);
  const [selectedItem, setSelectedItem] = useState<Item | null>(null);
  const [filter, setFilter] = useState<ItemFilter>(
    settings.unreadOnStart ? 'unread' : 'all'
  );

  const {
    sources, folders, unreadCountInScope, loadingSources,
    setSources, setFolders, setUnreadCountInScope,
    fetchSources, fetchFolders, fetchUnreadCount,
  } = useSourcesData(selectedSourceId, selectedFolderId, settings);

  const [loadingReader, setLoadingReader] = useState(false);
  const [showAddModal, setShowAddModal] = useState(false);

  const [moveSource, setMoveSource] = useState<Source | null>(null);
  const [editSource, setEditSource] = useState<Source | null>(null);
  const [renameTarget, setRenameTarget] = useState<{ type: 'folder' | 'source'; item: Folder | Source } | null>(null);
  const [showImportModal, setShowImportModal] = useState(false);
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(false);
  const [showExportModal, setShowExportModal] = useState(false);
  const [focusedItemId, setFocusedItemId] = useState<number | null>(null);
  const [multiSelectMode, setMultiSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [isMobile, setIsMobile] = useState(false);
  const [mobileView, setMobileView] = useState<'list' | 'reader'>('list');
  const [showMobileSidebar, setShowMobileSidebar] = useState(false);
  const [showCreateFolder, setShowCreateFolder] = useState(false);
  const [createFolderName, setCreateFolderName] = useState('');
  const createFolderInputRef = useRef<HTMLInputElement>(null);
  const [confirmAction, setConfirmAction] = useState<{ message: string; danger?: boolean; onConfirm: () => void } | null>(null);

  useEffect(() => {
    if (showCreateFolder) {
      const frame = requestAnimationFrame(() => {
        createFolderInputRef.current?.focus();
      });
      return () => cancelAnimationFrame(frame);
    }
  }, [showCreateFolder]);

  // 响应式检测
  useEffect(() => {
    const check = () => {
      const mobile = window.innerWidth < 768;
      setIsMobile(mobile);
      if (!mobile) {
        setMobileView('list');
        setShowMobileSidebar(false);
      }
    };
    check();
    window.addEventListener('resize', check);
    return () => window.removeEventListener('resize', check);
  }, []);

  const currentSource = useMemo(
    () => sources.find((s) => s.id === selectedSourceId) || null,
    [sources, selectedSourceId]
  );
  const currentFolder = useMemo(
    () => folders.find((f) => f.id === selectedFolderId) || null,
    [folders, selectedFolderId]
  );

  // AbortController: 切换 source/folder 时取消旧请求，避免竞态
  const abortRef = useRef<AbortController | null>(null);
  useEffect(() => {
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;
    return () => { ac.abort(); };
  }, [currentSource, currentFolder]);

  const {
    items, totalCount, readLaterCount, loadingItems,
    searchKeyword, isSearchMode,
    setSearchKeyword, setIsSearchMode, setItems,
    fetchItems, loadMore, hasMore, loadingMore, fetchItemCount, fetchReadLaterCount, fetchSearchItems,
  } = useItemsData({
    selectedSourceId, selectedFolderId, filter, settings,
    fetchUnreadCount, abortRef, setSelectedItem,
  });

  // 当文章列表变化时，保持聚焦项有效；若失效则聚焦首条
  useEffect(() => {
    setFocusedItemId((prev) => {
      if (items.length === 0) return null;
      const exists = items.some((i) => i.id === prev);
      return exists ? prev : items[0].id;
    });
  }, [items]);

  // 搜索
  const handleSearch = (keyword: string) => {
    setSearchKeyword(keyword);
    setIsSearchMode(true);
    setSelectedSourceId(null);
    setSelectedFolderId(null);
    setFilter('all');
  };

  // 清空搜索
  const handleClearSearch = () => {
    setSearchKeyword('');
    setIsSearchMode(false);
  };

  // 切换多选模式
  const handleToggleMultiSelect = () => {
    setMultiSelectMode((v) => !v);
    setSelectedIds([]);
  };

  // 移动端返回文章列表
  const handleBackToList = () => {
    setMobileView('list');
    setSelectedItem(null);
  };

  // 切换文章选中
  const handleToggleSelect = (id: number) => {
    setSelectedIds((prev) =>
      prev.includes(id) ? prev.filter((i) => i !== id) : [...prev, id]
    );
  };

  // 全选当前列表文章
  const handleSelectAll = (ids: number[]) => {
    setSelectedIds(ids);
  };

  // 批量导出文章
  const triggerBlobDownload = useCallback((blob: Blob, format: 'markdown' | 'json') => {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `articles-${new Date().toISOString().slice(0, 10)}.${format === 'json' ? 'json' : 'zip'}`;
    a.click();
    URL.revokeObjectURL(url);
  }, []);

  const handleExportArticles = async (scope: ExportScope, format: 'markdown' | 'json') => {
    let res: Response;
    try {
      res = await fetch(`${getApi()}/items/export?format=${format}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(scope),
      });
    } catch (err) {
      console.error('Failed to export articles:', err);
      showToast('导出失败');
      return;
    }
    if (!res.ok) {
      const err = (await res.json().catch(() => ({ error: '导出失败' }))) as { error: string };
      console.error('Failed to export articles:', err);
      showToast(err.error || '导出失败');
      return;
    }
    try {
      triggerBlobDownload(await res.blob(), format);
    } catch (err) {
      console.error('Failed to export articles:', err);
      showToast('导出失败');
    }
  };

  // 选择文章时标记为已读
  const handleSelectItem = async (item: Item) => {
    // 浏览器打开模式：直接在外部浏览器打开，不进入阅读器
    if (settings.openArticleMode === 'browser') {
      if (item.link) openExternal(item.link);
      showToast('已在浏览器打开');
      return;
    }

    setSelectedItem(item);
    setFocusedItemId(item.id);
    if (isMobile) {
      setMobileView('reader');
    }
    setLoadingReader(true);

    if (!item.isRead) {
      try {
        const readRes = await fetch(`${getApi()}/items/${item.id}/read`, { method: 'POST' });
        if (!readRes.ok) throw new Error(`HTTP ${readRes.status}`);
        setItems((prev) =>
          prev.map((i) => (i.id === item.id ? { ...i, isRead: true } : i))
        );
        setSources((prev) =>
          prev.map((s) =>
            s.id === item.sourceId
              ? { ...s, unreadCount: Math.max(0, (s.unreadCount || 0) - 1) }
              : s
          )
        );
      } catch (err) {
        console.error('Failed to mark read:', err);
      }
    }

    setLoadingReader(false);
  };

  // 切换收藏
  const handleToggleStar = async (id: number, starred: boolean) => {
    const endpoint = starred ? 'star' : 'unstar';
    try {
      const res = await fetch(`${getApi()}/items/${id}/${endpoint}`, { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
      console.error('Failed to toggle star:', err);
      showToast(starred ? '收藏失败' : '取消收藏失败');
      return;
    }
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, isStarred: starred } : i)));
    if (selectedItem?.id === id) {
      setSelectedItem((prev) => (prev ? { ...prev, isStarred: starred } : null));
    }
  };

  // 切换稍后阅读
  const handleToggleReadLater = async (id: number, readLater: boolean) => {
    const endpoint = readLater ? 'read-later' : 'unread-later';
    try {
      const res = await fetch(`${getApi()}/items/${id}/${endpoint}`, { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
      console.error('Failed to toggle read later:', err);
      showToast(readLater ? '添加稍后阅读失败' : '取消稍后阅读失败');
      return;
    }
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, isReadLater: readLater } : i)));
    if (selectedItem?.id === id) {
      setSelectedItem((prev) => (prev ? { ...prev, isReadLater: readLater } : null));
    }
  };

  // 切换已读（阅读区按钮）
  const handleToggleRead = async (id: number, read: boolean) => {
    const prevItem = items.find((i) => i.id === id);
    const wasRead = prevItem?.isRead ?? true;
    const sourceId = prevItem?.sourceId;

    try {
      const res = await fetch(`${getApi()}/items/${id}/${read ? 'read' : 'unread'}`, { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
      console.error('Failed to toggle read:', err);
      showToast(read ? '标记已读失败' : '标记未读失败');
      return;
    }
    updateItemReadState(id, read);
    if (selectedItem?.id === id) {
      setSelectedItem((prev) => (prev ? { ...prev, isRead: read } : null));
    }
    if (sourceId !== undefined && read !== wasRead) {
      updateSourceUnreadCount(sourceId, read ? -1 : 1);
    }
  };

  const updateItemReadState = (id: number, read: boolean) => {
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, isRead: read } : i)));
  };

  const updateSourceUnreadCount = (sourceId: number, delta: number) => {
    setSources((prev) =>
      prev.map((s) =>
        s.id === sourceId
          ? { ...s, unreadCount: Math.max(0, (s.unreadCount || 0) + delta) }
          : s
      )
    );
  };

  const isAnyModalOpen = () =>
    showAddModal ||
    moveSource !== null ||
    editSource !== null ||
    showImportModal ||
    renameTarget !== null ||
    showShortcutsHelp ||
    showExportModal ||
    confirmAction !== null ||
    showCreateFolder;

  const handleKeyboardShortcut = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowDown':
        handleArrowDown(e);
        break;
      case 'ArrowUp':
        handleArrowUp(e);
        break;
      case 'Enter':
        if (focusedItemId !== null) {
          e.preventDefault();
          const item = items.find((i) => i.id === focusedItemId);
          if (item) handleSelectItem(item);
        }
        break;
      case 'm':
      case 'M':
        toggleItemAction(e, 'read');
        break;
      case 's':
      case 'S':
        toggleItemAction(e, 'star');
        break;
      case 'l':
      case 'L':
        toggleItemAction(e, 'readLater');
        break;
      case 'r':
      case 'R':
        e.preventDefault();
        fetchItems();
        fetchItemCount();
        break;
      case 'x':
      case 'X':
        e.preventDefault();
        handleToggleMultiSelect();
        break;
      case '?':
        e.preventDefault();
        setShowShortcutsHelp(true);
        break;
      case 'Escape':
        if (selectedItem) {
          e.preventDefault();
          setSelectedItem(null);
        }
        break;
    }
  };

  const handleArrowDown = (e: KeyboardEvent) => {
    e.preventDefault();
    setFocusedItemId((prev) => {
      if (items.length === 0) return null;
      const idx = items.findIndex((i) => i.id === prev);
      const nextIdx = idx < items.length - 1 ? idx + 1 : 0;
      return items[nextIdx].id;
    });
  };

  const handleArrowUp = (e: KeyboardEvent) => {
    e.preventDefault();
    setFocusedItemId((prev) => {
      if (items.length === 0) return null;
      const idx = items.findIndex((i) => i.id === prev);
      const prevIdx = idx > 0 ? idx - 1 : items.length - 1;
      return items[prevIdx].id;
    });
  };

  const toggleItemAction = (e: KeyboardEvent, action: 'read' | 'star' | 'readLater') => {
    const item = items.find((i) => i.id === focusedItemId);
    if (!item) return;
    e.preventDefault();
    if (action === 'read') handleToggleRead(item.id, !item.isRead);
    else if (action === 'star') handleToggleStar(item.id, !item.isStarred);
    else if (action === 'readLater') handleToggleReadLater(item.id, !item.isReadLater);
  };

  // 用 ref 保存最新的键盘 handler，使 listener 只注册一次
  // 每次渲染都会更新 ref.current 到最新闭包，避免 17 项依赖频繁重注册
  const keyboardHandlerRef = useRef<(e: KeyboardEvent) => void>(() => {});
  keyboardHandlerRef.current = (e: KeyboardEvent) => {
    const activeTag = document.activeElement?.tagName;
    if (activeTag === 'INPUT' || activeTag === 'TEXTAREA' || activeTag === 'SELECT') return;
    if (isAnyModalOpen()) return;
    handleKeyboardShortcut(e);
  };

  // 全局键盘快捷键：listener 只注册一次，通过 ref 调用最新 handler
  useEffect(() => {
    const handler = (e: KeyboardEvent) => keyboardHandlerRef.current(e);
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  // 用 ref 保存最新引用，避免回调（如 refreshAllData）使用过期闭包
  const fetchItemsRef = useRef(fetchItems);
  const fetchSourcesRef = useRef(fetchSources);
  const fetchFoldersRef = useRef(fetchFolders);
  const fetchItemCountRef = useRef(fetchItemCount);
  const fetchUnreadCountRef = useRef(fetchUnreadCount);
  const fetchReadLaterCountRef = useRef(fetchReadLaterCount);
  useEffect(() => { fetchItemsRef.current = fetchItems; }, [fetchItems]);
  useEffect(() => { fetchSourcesRef.current = fetchSources; }, [fetchSources]);
  useEffect(() => { fetchFoldersRef.current = fetchFolders; }, [fetchFolders]);
  useEffect(() => { fetchItemCountRef.current = fetchItemCount; }, [fetchItemCount]);
  useEffect(() => { fetchUnreadCountRef.current = fetchUnreadCount; }, [fetchUnreadCount]);
  useEffect(() => { fetchReadLaterCountRef.current = fetchReadLaterCount; }, [fetchReadLaterCount]);

  const refreshAllData = useCallback(async () => {
    await Promise.all([
      fetchItemsRef.current(),
      // 优化：抓取完成后只需刷新文章列表和相关计数
      // 不再调用 fetchSourcesRef/fetchFoldersRef，因为它们仅在源/文件夹结构变更时才需要
      // fetchSourcesRef.current(),
      // fetchFoldersRef.current(),
      fetchItemCountRef.current(),
      fetchUnreadCountRef.current(),
      fetchReadLaterCountRef.current(),
    ]);
  }, []);

  // 通知与抓取编排下沉到独立 hook，保持 App 顶层可读性
  const { notifyFetchComplete } = useNotification({
    notifyEnabled: settings.notifyEnabled,
    notifyBatchMin: settings.notifyBatchMin,
    unreadCountInScope,
  });

  const { refreshing, handleFetch } = useFetchOrchestration({
    autoFetchOnStart: settings.autoFetchOnStart,
    sourcesLength: sources.length,
    refreshAllData,
    notifyFetchComplete,
  });

  // 源结构变更（增/删/改/移动/导入）后，统一刷新所有派生视图状态。
  // 与 refreshAllData 的区别：本函数额外刷新 sources/folders（结构），用于结构变更场景。
  // 单一数据源：全局 state 之外不保留副本，所有变更入口收敛到本函数。
  const refreshSourceStructure = useCallback(async () => {
    await Promise.all([
      fetchSourcesRef.current(),
      fetchFoldersRef.current(),
      fetchUnreadCountRef.current(undefined),
      fetchItemsRef.current(),
      fetchItemCountRef.current(),
    ]);
  }, []);

  // 从 URL 提取域名作为文件夹名称
  const getDomainFolderName = (url: string): string => {
    try {
      const hostname = new URL(url).hostname;
      return hostname.replace(/^www\./, '').split('.')[0];
    } catch {
      return '未分类';
    }
  };

  // 查找或创建文件夹（用于自动分组）
  const ensureFolder = async (name: string): Promise<number> => {
    const existing = folders.find((f) => f.name === name);
    if (existing) return existing.id;
    const res = await fetch(`${getApi()}/folders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    if (!res.ok) throw new Error('创建文件夹失败');
    const created: { id: number } = await res.json();
    await fetchFolders();
    return created.id;
  };

  // 添加订阅源
  const buildSourcePayload = useCallback(async (data: SourceFormData): Promise<SourceFormData> => {
    const payload: SourceFormData = { ...data };
    if (settings.autoGroup && !payload.folderId) {
      payload.folderId = await ensureFolder(getDomainFolderName(data.url));
    }
    return payload;
  }, [settings.autoGroup, ensureFolder]);

  const handleAddSource = async (data: SourceFormData) => {
    let payload: SourceFormData;
    try {
      payload = await buildSourcePayload(data);
    } catch (err) {
      console.error('Failed to prepare source:', err);
      showToast('创建失败');
      return;
    }
    try {
      const res = await fetch(`${getApi()}/sources/create`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = (await res.json().catch(() => ({ error: '创建失败' }))) as { error: string };
        showToast(err.error || '创建失败');
        return;
      }
    } catch (err) {
      console.error('Failed to create source:', err);
      showToast('创建失败');
      return;
    }
    setShowAddModal(false);
    await fetchSources();
  };

  // 创建文件夹
  const handleCreateFolder = () => {
    setCreateFolderName('');
    setShowCreateFolder(true);
  };

  const handleCreateFolderSubmit = async () => {
    const name = createFolderName.trim();
    setShowCreateFolder(false);
    if (!name) return;
    try {
      const res = await fetch(`${getApi()}/folders`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      if (!res.ok) throw new Error('创建失败');
      await fetchFolders();
    } catch (err) {
      console.error('Failed to create folder:', err);
      showToast('创建文件夹失败');
    }
  };

  // 重命名文件夹
  const handleRenameFolder = async (value: string) => {
    if (!renameTarget || renameTarget.type !== 'folder') return;
    try {
      const res = await fetch(`${getApi()}/folders/${renameTarget.item.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: value }),
      });
      if (!res.ok) throw new Error('重命名失败');
      await fetchFolders();
    } catch (err) {
      console.error('Failed to rename folder:', err);
      showToast('重命名失败');
    }
  };

  // 删除文件夹
  const handleDeleteFolder = (id: number) => {
    setConfirmAction({
      message: '确定删除该文件夹？文件夹内的订阅源将变为未分类。',
      danger: true,
      onConfirm: async () => {
        setConfirmAction(null);
        try {
          const res = await fetch(`${getApi()}/folders/${id}`, { method: 'DELETE' });
          if (!res.ok) throw new Error('删除失败');
          if (selectedFolderId === id) {
            setSelectedFolderId(null);
          }
          await Promise.all([fetchFolders(), fetchSources()]);
        } catch (err) {
          console.error('Failed to delete folder:', err);
          showToast('删除失败');
        }
      },
    });
  };

  // 重命名订阅源
  const handleRenameSource = async (value: string) => {
    if (!renameTarget || renameTarget.type !== 'source') return;
    try {
      const res = await fetch(`${getApi()}/sources/${renameTarget.item.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: value }),
      });
      if (!res.ok) throw new Error('重命名失败');
      await fetchSources();
    } catch (err) {
      console.error('Failed to rename source:', err);
      showToast('重命名失败');
    }
  };

  // 移动订阅源
  const handleMoveSource = async (sourceId: number, folderId: number | null) => {
    try {
      const res = await fetch(`${getApi()}/sources/${sourceId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ folderId }),
      });
      if (!res.ok) throw new Error('移动失败');
    } catch (err) {
      console.error('Failed to move source:', err);
      showToast('移动失败');
      return;
    }
    await fetchSources();
  };

  const executeDeleteSource = async (id: number) => {
    try {
      const res = await fetch(`${getApi()}/sources/${id}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('删除失败');
    } catch (err) {
      console.error('Failed to delete source:', err);
      showToast('删除失败');
      return;
    }
    if (selectedSourceId === id) setSelectedSourceId(null);
    await refreshSourceStructure();
  };

  const handleDeleteSource = (id: number) => {
    setConfirmAction({
      message: '确定删除该订阅源？相关文章也将被删除。',
      danger: true,
      onConfirm: async () => {
        setConfirmAction(null);
        await executeDeleteSource(id);
      },
    });
  };

  // 全部标为已读（支持全部文章 / 文件夹 / 单个订阅源）
  const handleMarkAllRead = async (scope: { sourceId?: number; folderId?: number }) => {
    try {
      const res = await fetch(`${getApi()}/items/read-all`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(scope),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await Promise.all([fetchSources(), fetchItems(), fetchItemCount()]);
    } catch (err) {
      console.error('Failed to mark all read:', err);
      showToast('标记失败');
    }
  };

  // 批量标记已读/未读（按文章 ID 列表）
  const handleBatchMarkRead = async (ids: number[], read: boolean) => {
    if (ids.length === 0) return;
    try {
      const res = await fetch(`${getApi()}/items/mark-read`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, read }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
      console.error('Failed to batch mark read:', err);
      showToast('批量标记失败');
      return;
    }
    await Promise.all([fetchSources(), fetchItems(), fetchItemCount()]);
  };

  // 将订阅源移出当前文件夹
  const handleRemoveFromFolder = async (sourceId: number) => {
    try {
      const res = await fetch(`${getApi()}/sources/${sourceId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ folderId: null }),
      });
      if (!res.ok) throw new Error('移出文件夹失败');
      await fetchSources();
    } catch (err) {
      console.error('Failed to remove source from folder:', err);
      showToast('移出文件夹失败');
    }
  };

  // 编辑订阅源（名称 + URL + 文件夹 + 私密 + 时间线隐藏）
  const handleEditSource = async (params: {
    sourceId: number;
    name: string;
    url: string;
    folderId: number | null;
    isPrivate: boolean;
    hideInTimeline: boolean;
  }) => {
    const body = {
      name: params.name.trim(),
      url: params.url.trim(),
      folderId: params.folderId,
      isPrivate: params.isPrivate,
      hideInTimeline: params.hideInTimeline,
    };
    try {
      const res = await fetch(`${getApi()}/sources/${params.sourceId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error('编辑失败');
    } catch (err) {
      console.error('Failed to edit source:', err);
      showToast('编辑订阅源失败');
      return;
    }
    setEditSource(null);
    await Promise.all([fetchSources(), fetchItems(), fetchItemCount()]);
  };

  // 清空文件夹内的所有订阅源（保留文件夹本身）
  const handleClearFolder = (folderId: number) => {
    setConfirmAction({
      message: '确定清空该文件夹内的所有订阅源？订阅源将变为未分类，文件夹本身会保留。',
      danger: true,
      onConfirm: async () => {
        setConfirmAction(null);
        try {
          const res = await fetch(`${getApi()}/folders/${folderId}/clear`, {
            method: 'POST',
          });
          if (!res.ok) throw new Error('清空分组失败');
          await fetchSources();
        } catch (err) {
          console.error('Failed to clear folder:', err);
          showToast('清空分组失败');
        }
      },
    });
  };

  // 导入 OPML
  const handleImportOPML = async (xml: string) => {
    try {
      const res = await fetch(`${getApi()}/opml/import`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/xml' },
        body: xml,
      });
      if (!res.ok) {
        const errData = (await res.json().catch(() => ({ error: '导入失败' }))) as { error: string };
        throw new Error(errData.error || '导入失败');
      }
      await Promise.all([fetchFolders(), fetchSources()]);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : '导入失败';
      // 如果错误消息是 internal_server_error，给出更友好的提示
      if (message === 'internal_server_error') {
        throw new Error('服务器处理失败，请检查后端是否正常运行');
      }
      throw err;
    }
  };

  // 导出 OPML
  const handleExportOPML = async () => {
    try {
      await downloadBlob('/opml/export', 'subscriptions.opml');
    } catch (err) {
      console.error('Failed to export OPML:', err);
      showToast('导出失败');
    }
  };

  return (
    <div className="flex flex-col h-screen w-full overflow-hidden font-sans bg-canvas">
      <TitleBar />

      <div className={`flex flex-1 min-h-0 overflow-hidden bg-canvas ${isMobile ? 'flex-col' : 'flex-row'}`}>
        {isMobile && (
          <div className="flex items-center justify-between px-3 h-12 min-h-[48px] border-b border-border-subtle bg-surface">
            <button
              onClick={() => setShowMobileSidebar(true)}
              className="w-9 h-9 rounded-md bg-transparent text-secondary cursor-pointer flex items-center justify-center hover:bg-hover"
              title="打开订阅列表"
              aria-label="打开订阅列表"
            >
              <MenuIcon size={20} />
            </button>
            <span className="text-[15px] font-semibold text-primary">RSS 阅读器</span>
            <span className="w-9" />
          </div>
        )}

        <div
          className={`w-[260px] min-w-[260px] h-full bg-elevated border-r border-border-subtle overflow-hidden transition-transform duration-200 ease-out ${
            isMobile ? 'fixed z-[1100]' : 'relative'
          }`}
          style={{
            transform: isMobile
              ? showMobileSidebar
                ? 'translateX(0)'
                : 'translateX(-100%)'
              : undefined,
          }}
        >
          <Sidebar
          sources={sources}
          folders={folders}
          settings={settings}
          onSettingsChange={handleSettingsChange}
          selectedSourceId={selectedSourceId}
          selectedFolderId={selectedFolderId}
          filter={filter}
          onSelectSource={(id) => {
            setSelectedSourceId(id);
            setIsSearchMode(false);
            setSearchKeyword('');
            setShowMobileSidebar(false);
          }}
          onSelectFolder={(id) => {
            setSelectedFolderId(id);
            setIsSearchMode(false);
            setSearchKeyword('');
            setShowMobileSidebar(false);
          }}
          onSelectFilter={(f) => {
            setFilter(f);
            setIsSearchMode(false);
            setSearchKeyword('');
            setShowMobileSidebar(false);
          }}
          onAddSource={() => setShowAddModal(true)}
          onAddFolder={handleCreateFolder}
          onImport={() => setShowImportModal(true)}
          onExport={handleExportOPML}
          onRenameFolder={(folder) => setRenameTarget({ type: 'folder', item: folder })}
          onDeleteFolder={handleDeleteFolder}
          onRenameSource={(source) => setRenameTarget({ type: 'source', item: source })}
          onMoveSource={(source) => setMoveSource(source)}
          onFetchSource={handleFetch}
          onSourcesChanged={refreshSourceStructure}
          onMarkAllRead={handleMarkAllRead}
          onDeleteSource={handleDeleteSource}
          onClearFolderSources={handleClearFolder}
          onEditSource={(source) => setEditSource(source)}
          onRemoveFromFolder={handleRemoveFromFolder}
          readLaterCount={readLaterCount}
          searchKeyword={searchKeyword}
          isSearchMode={isSearchMode}
          onSearch={handleSearch}
          onClearSearch={handleClearSearch}
        />
      </div>

      {isMobile && showMobileSidebar && (
        <div
          className="fixed inset-0 bg-[var(--overlay-mobile)] z-[1050]"
          onClick={() => setShowMobileSidebar(false)}
          role="presentation"
          aria-hidden="true"
        />
      )}

      <div className="flex flex-1 min-w-0 overflow-hidden">
        <div
          className={`w-[360px] min-w-[360px] h-full bg-canvas border-r border-border-subtle overflow-hidden ${
            isMobile && mobileView === 'reader' ? 'hidden' : 'flex'
          }`}
        >
          <ArticleList
            items={items}
            selectedId={selectedItem?.id || null}
            focusedId={focusedItemId}
            loading={loadingItems}
            refreshing={refreshing}
            currentSource={currentSource}
            currentFolder={currentFolder}
            filter={filter}
            totalCount={totalCount}
            globalUnreadCount={unreadCountInScope}
            settings={settings}
            searchKeyword={searchKeyword}
            isSearchMode={isSearchMode}
            multiSelectMode={multiSelectMode}
            selectedIds={selectedIds}
            onSelectItem={handleSelectItem}
            onToggleStar={handleToggleStar}
            onToggleRead={handleToggleRead}
            onToggleReadLater={handleToggleReadLater}
            onFetch={handleFetch}
            onSelectFilter={(f) => {
              setFilter(f);
              setIsSearchMode(false);
              setSearchKeyword('');
            }}
            onMarkAllRead={handleMarkAllRead}
            onBatchMarkRead={handleBatchMarkRead}
            onToggleMultiSelect={handleToggleMultiSelect}
            onToggleSelect={handleToggleSelect}
            onSelectAll={handleSelectAll}
            onExport={() => setShowExportModal(true)}
            hasMore={hasMore}
            loadingMore={loadingMore}
            onLoadMore={loadMore}
          />
        </div>

        <div
          className={`flex-1 min-w-0 h-full bg-surface overflow-hidden ${
            isMobile && mobileView === 'list' ? 'hidden' : 'flex'
          }`}
        >
          <Reader
            item={selectedItem}
            loading={loadingReader}
            settings={settings}
            onSettingsChange={handleSettingsChange}
            onToggleRead={handleToggleRead}
            onToggleStar={handleToggleStar}
            onToggleReadLater={handleToggleReadLater}
            onBack={isMobile ? handleBackToList : undefined}
          />
        </div>
      </div>

      {showAddModal && (
        <AddSourceModal
          folders={folders}
          onClose={() => setShowAddModal(false)}
          onSubmit={handleAddSource}
        />
      )}

      {moveSource && (
        <MoveSourceModal
          source={moveSource}
          folders={folders}
          onClose={() => setMoveSource(null)}
          onMove={handleMoveSource}
        />
      )}

      {editSource && (
        <EditSourceModal
          source={editSource}
          folders={folders}
          onClose={() => setEditSource(null)}
          onSubmit={handleEditSource}
        />
      )}

      {showImportModal && (
        <ImportOPMLModal
          onClose={() => setShowImportModal(false)}
          onImport={handleImportOPML}
        />
      )}

      {renameTarget && (
        <RenameModal
          title={renameTarget.type === 'folder' ? '重命名文件夹' : '重命名订阅源'}
          initialValue={renameTarget.item.name}
          onClose={() => setRenameTarget(null)}
          onSubmit={renameTarget.type === 'folder' ? handleRenameFolder : handleRenameSource}
        />
      )}

      {showShortcutsHelp && (
        <ShortcutsHelpModal onClose={() => setShowShortcutsHelp(false)} />
      )}

      {showExportModal && (
        <ExportArticlesModal
          isOpen={showExportModal}
          onClose={() => setShowExportModal(false)}
          items={items}
          selectedIds={selectedIds}
          currentSource={currentSource}
          currentFolder={currentFolder}
          filter={filter}
          settings={settings}
          onExport={handleExportArticles}
        />
      )}

      {confirmAction && (
        <ConfirmDialog
          message={confirmAction.message}
          danger={confirmAction.danger}
          onConfirm={confirmAction.onConfirm}
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {showCreateFolder && (
        <ModalLayout title="新建文件夹" onClose={() => setShowCreateFolder(false)} width={360}>
          <div className="px-6 pt-5 pb-6">
            <input
              ref={createFolderInputRef}
              type="text"
              value={createFolderName}
              onChange={(e) => setCreateFolderName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleCreateFolderSubmit(); if (e.key === 'Escape') setShowCreateFolder(false); }}
              placeholder="文件夹名称"
              className="w-full px-3 py-2.5 border border-border rounded-sm text-sm bg-surface text-primary outline-none focus:border-primary mb-4"
              autoFocus
            />
            <div className="flex justify-end gap-3">
              <Button variant="secondary" onClick={() => setShowCreateFolder(false)}>
                取消
              </Button>
              <Button variant="primary" onClick={handleCreateFolderSubmit}>
                保存
              </Button>
            </div>
          </div>
        </ModalLayout>
      )}
      </div>
      <ToastContainer />
    </div>
  );
}

export default App;
