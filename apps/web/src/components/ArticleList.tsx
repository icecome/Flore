import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef } from 'react';
import type { Folder, Item, Source } from '../types';
import { cn } from '../lib/cn';
import { formatDate, formatTime, formatRelative } from '../lib/format';
import type { AppSettings } from '../utils/settings';
import ContextMenu from './ContextMenu';
import IconButton from './IconButton';
import Loading from './Loading';
import SourceAvatar from './SourceAvatar';
import { useContextMenu } from '../hooks/useContextMenu';
import { buildArticleRowMenu, buildArticleListHeaderMenu } from '../utils/contextMenu';
import { StarIcon, StarFilledIcon, RefreshCw, Circle, CheckCheck, Clock, Download, Square, CheckSquare, Check } from './icons';

type FilterType = 'all' | 'unread' | 'starred' | 'readLater';

function getArticleListTitle({
  isSearchMode,
  filter,
  currentSource,
  currentFolder,
}: {
  isSearchMode: boolean;
  filter: FilterType;
  currentSource: Source | null;
  currentFolder: Folder | null;
}): string {
  if (isSearchMode) return '搜索结果';
  if (filter === 'starred') return '收藏文章';
  if (filter === 'unread') return '未读文章';
  if (filter === 'readLater') return '稍后阅读';
  if (currentSource) return currentSource.name;
  if (currentFolder) return currentFolder.name;
  return '全部文章';
}

interface Props {
  items: Item[];
  selectedId: number | null;
  focusedId?: number | null;
  loading: boolean;
  refreshing?: boolean;
  currentSource: Source | null;
  currentFolder: Folder | null;
  filter: FilterType;
  totalCount: number;
  globalUnreadCount: number;
  settings: AppSettings;
  searchKeyword?: string;
  isSearchMode?: boolean;
  multiSelectMode?: boolean;
  selectedIds?: number[];
  onSelectItem: (item: Item) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleRead: (id: number, read: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onFetch: (scope: { sourceId?: number; folderId?: number }) => void;
  onSelectFilter: (filter: FilterType) => void;
  onMarkAllRead: (scope: { sourceId?: number; folderId?: number }) => void;
  onBatchMarkRead: (ids: number[], read: boolean) => void;
  onToggleMultiSelect?: () => void;
  onToggleSelect?: (id: number) => void;
  onSelectAll?: (ids: number[]) => void;
  hasMore?: boolean;
  loadingMore?: boolean;
  onLoadMore?: () => void;
  onExport?: () => void;
}

export default function ArticleList({
  items,
  selectedId,
  focusedId,
  loading,
  refreshing = false,
  currentSource,
  currentFolder,
  filter,
  totalCount,
  globalUnreadCount,
  settings,
  searchKeyword = '',
  isSearchMode = false,
  multiSelectMode = false,
  selectedIds = [],
  onSelectItem,
  onToggleStar,
  onToggleRead,
  onToggleReadLater,
  onFetch,
  onSelectFilter,
  onMarkAllRead,
  onBatchMarkRead,
  onToggleMultiSelect,
  onToggleSelect,
  onSelectAll,
  onExport,
  hasMore = false,
  loadingMore = false,
  onLoadMore,
}: Props) {
  const { menuProps, showMenu } = useContextMenu();
  const itemRefs = useRef<Map<number, HTMLDivElement>>(new Map());

  // 记忆文章列表滚动位置：存在 ref 中，避免「滚动 → setState → 重渲染 → 复位滚动」的回环
  const scrollPositionRef = useRef(0);
  const listContainerRef = useRef<HTMLDivElement | null>(null);

  // 无限滚动：用 ref 持有最新状态，避免 useEffect([]) 闭包捕获过期值导致自动加载失效
  const loadMoreStateRef = useRef<{
    hasMore: boolean;
    loadingMore: boolean;
    onLoadMore?: () => void;
  }>({ hasMore, loadingMore, onLoadMore });
  loadMoreStateRef.current = { hasMore, loadingMore, onLoadMore };

  // 右键菜单需要完整列表，用 ref 持有以保证回调引用稳定（否则每次 items 变化都会让所有行重渲染）
  const itemsRef = useRef(items);
  itemsRef.current = items;

  // 监听容器滚动，保存当前位置 + 触底自动加载下一页
  useEffect(() => {
    const container = listContainerRef.current;
    if (!container) return;

    const handleScroll = () => {
      scrollPositionRef.current = container.scrollTop;
      // 触底自动加载下一页（无限滚动）；状态从 ref 读取，避免闭包捕获过期值
      const state = loadMoreStateRef.current;
      if (state.onLoadMore && state.hasMore && !state.loadingMore) {
        const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 240;
        if (nearBottom) state.onLoadMore();
      }
    };
    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => container.removeEventListener('scroll', handleScroll);
  }, []);

  // items 变化（刷新 / 加载更多）后恢复滚动位置；behavior:'auto' 保证一帧内完成，不产生新的平滑滚动事件流
  useLayoutEffect(() => {
    const container = listContainerRef.current;
    if (!container) return;
    const top = scrollPositionRef.current;
    if (top <= 0 || Math.abs(container.scrollTop - top) < 1) return;
    container.scrollTo({ top, behavior: 'auto' });
  }, [items]);

  useEffect(() => {
    if (!focusedId) return;
    const el = itemRefs.current.get(focusedId);
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }, [focusedId]);

  // 单个 IntersectionObserver 监听所有文章条目的滚动可见性
  useEffect(() => {
    if (settings.markReadMode !== 'scroll') return;

    // 跟踪已可见的条目，仅当从可见变为不可见时才标记已读
    const visibleItems = new Set<number>();
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          const id = Number(entry.target.getAttribute('data-item-id'));
          if (!id || isNaN(id)) return;
          if (entry.isIntersecting) {
            visibleItems.add(id);
          } else if (visibleItems.has(id)) {
            visibleItems.delete(id);
            onToggleRead(id, true);
          }
        });
      },
      { threshold: 0.1 }
    );

    itemRefs.current.forEach((el) => observer.observe(el));

    return () => observer.disconnect();
  }, [settings.markReadMode, onToggleRead, items]);

  // 刷新旋转：直接操作 DOM，彻底绕过 CSS 动画问题
  const refreshIconRef = useRef<SVGSVGElement>(null);
  useEffect(() => {
    const el = refreshIconRef.current;
    if (!el) return;
    if (!refreshing) {
      el.style.transform = 'rotate(0deg)';
      return;
    }
    let degrees = 0;
    let lastTime = 0;
    // 6 秒一圈 = 360deg / 6000ms = 0.06deg/ms
    let rafId: number;
    const tick = (time: number) => {
      if (!lastTime) lastTime = time;
      const delta = time - lastTime;
      lastTime = time;
      degrees = (degrees + delta * 0.5) % 360;
      el.style.transform = `rotate(${degrees}deg)`;
      rafId = requestAnimationFrame(tick);
    };
    rafId = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafId);
  }, [refreshing]);

  const getScope = useCallback(() => {
    if (currentSource) return { sourceId: currentSource.id };
    if (currentFolder) return { folderId: currentFolder.id };
    return {};
  }, [currentSource, currentFolder]);

  const canMarkAllRead = filter !== 'starred';

  const handleMarkAllRead = useCallback(() => {
    if (!canMarkAllRead) return;
    onMarkAllRead(getScope());
  }, [canMarkAllRead, getScope, onMarkAllRead]);

  const handleFetch = useCallback(() => onFetch(getScope()), [getScope, onFetch]);

  // ---- 行级稳定回调：保证 ArticleRow 的 React.memo 生效 ----
  const registerRef = useCallback((id: number, el: HTMLDivElement | null) => {
    if (el) itemRefs.current.set(id, el);
    else itemRefs.current.delete(id);
  }, []);

  const handleActivate = useCallback((item: Item) => {
    if (multiSelectMode) onToggleSelect?.(item.id);
    else onSelectItem(item);
  }, [multiSelectMode, onSelectItem, onToggleSelect]);

  const handleToggleSelect = useCallback((id: number) => {
    onToggleSelect?.(id);
  }, [onToggleSelect]);

  const handleRowContextMenu = useCallback((e: React.MouseEvent, item: Item) => {
    showMenu(e, buildArticleRowMenu(item, itemsRef.current, !!multiSelectMode, {
      onToggleMultiSelect,
      onExport,
      onToggleRead,
      onToggleStar,
      onToggleReadLater,
      onBatchMarkRead,
    }));
  }, [multiSelectMode, onBatchMarkRead, onExport, onToggleMultiSelect, onToggleRead, onToggleReadLater, onToggleStar, showMenu]);

  const handleListContextMenu = useCallback((e: React.MouseEvent) => {
    showMenu(e, buildArticleListHeaderMenu({
      onFetch: handleFetch,
      onMarkAllRead: handleMarkAllRead,
      onToggleMultiSelect,
    }, canMarkAllRead));
  }, [canMarkAllRead, handleFetch, handleMarkAllRead, onToggleMultiSelect, showMenu]);

  const handleSelectAll = useCallback(() => {
    onSelectAll?.(itemsRef.current.map((it) => it.id));
  }, [onSelectAll]);

  const handleToggleUnreadFilter = useCallback(() => {
    onSelectFilter(filter === 'unread' ? 'all' : 'unread');
  }, [filter, onSelectFilter]);

  const title = getArticleListTitle({ isSearchMode, filter, currentSource, currentFolder });

  // 显示计数信息：仅全部文章 / 源 / 文件夹视图显示"· N 未读"
  const showHeaderCount = useMemo(() => {
    // 未读视图：显示未读数作为总数（不显示"未读"后缀，因为本身就是未读）
    if (filter === 'unread') {
      return { total: globalUnreadCount, unread: 0 };
    }
    const isGlobalScope = !isSearchMode && filter === 'all' && !currentSource && !currentFolder;
    const isSourceOrFolder = !isSearchMode && (currentSource || currentFolder);
    if (isGlobalScope || isSourceOrFolder) {
      return { total: totalCount, unread: globalUnreadCount };
    }
    // 未读 / 收藏 / 稍后阅读 / 搜索：只显示总数，不追加未读数
    return { total: totalCount, unread: 0 };
  }, [totalCount, globalUnreadCount, isSearchMode, filter, currentSource, currentFolder]);

  return (
    <section className="relative flex h-full w-[360px] shrink-0 flex-col border-r border-border bg-surface">
      {/* Header */}
      <header
        className="flex shrink-0 flex-col gap-2 border-b border-border-subtle px-4 pb-2.5 pt-3"
        onContextMenu={handleListContextMenu}
      >
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-baseline gap-2">
            <h2 className="truncate text-[17px] font-semibold tracking-tight text-primary">
              {title}
            </h2>
            {isSearchMode && (
              <span className="shrink-0 rounded bg-primary-subtle px-1.5 py-0.5 text-[11px] font-medium text-primary">
                搜索
              </span>
            )}
          </div>
          <div className="flex items-center gap-0.5">
            <IconButton size="sm" onClick={handleFetch} title="刷新当前列表">
              <RefreshCw size={15} ref={refreshIconRef} />
            </IconButton>
            <IconButton
              size="sm"
              onClick={handleToggleUnreadFilter}
              title={filter === 'unread' ? '显示全部' : '只看未读'}
            >
              <Circle size={15} fill={filter === 'unread' ? 'currentColor' : 'none'} />
            </IconButton>
            <IconButton
              size="sm"
              onClick={handleMarkAllRead}
              disabled={!canMarkAllRead || multiSelectMode}
              title="全部标为已读"
            >
              <CheckCheck size={15} />
            </IconButton>
          </div>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-[12px] text-muted">
            {showHeaderCount.total} 篇
            {showHeaderCount.unread > 0 && (
              <span className="text-secondary"> · {showHeaderCount.unread} 未读</span>
            )}
          </span>
        </div>
      </header>

      {/* List */}
      <div
        className="flex-1 overflow-y-auto"
        ref={listContainerRef}
        onContextMenu={handleListContextMenu}
      >
        {loading ? (
          <Loading text="正在加载文章列表..." fullHeight />
        ) : items.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
            <CheckCheck size={28} className="text-border-strong" />
            <p className="text-[13px] text-muted">没有更多文章了</p>
          </div>
        ) : (
          items.map((item, idx) => (
            <ArticleRow
              key={item.id}
              item={item}
              index={idx}
              selected={selectedId === item.id}
              focused={focusedId === item.id}
              checked={selectedIds.includes(item.id)}
              multiSelectMode={multiSelectMode}
              settings={settings}
              highlightKeyword={searchKeyword}
              registerRef={registerRef}
              onActivate={handleActivate}
              onToggleRead={onToggleRead}
              onToggleStar={onToggleStar}
              onToggleReadLater={onToggleReadLater}
              onToggleSelect={handleToggleSelect}
              onContextMenu={handleRowContextMenu}
            />
          ))
        )}
      </div>

      {multiSelectMode && items.length > 0 && (
        <div className="absolute bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-md border border-border bg-surface px-2.5 py-1.5 shadow-md">
          <IconButton size="sm" onClick={onToggleMultiSelect} title="完成">
            <Check size={15} />
          </IconButton>
          <span className="whitespace-nowrap px-1 text-[13px] text-secondary">
            {selectedIds.length} / {items.length} 已选
          </span>
          <IconButton
            size="sm"
            onClick={handleSelectAll}
            title={selectedIds.length === items.length ? '取消全选' : '全选当前列表'}
          >
            {selectedIds.length === items.length ? (
              <CheckSquare size={15} />
            ) : (
              <Square size={15} />
            )}
          </IconButton>
          <button
            onClick={onExport}
            disabled={selectedIds.length === 0}
            className="inline-flex h-7 items-center gap-1 rounded-sm border border-border-strong bg-surface px-2.5 text-[13px] font-medium text-secondary disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Download size={14} />
            <span>导出</span>
          </button>
        </div>
      )}

      {menuProps && <ContextMenu {...menuProps} />}
    </section>
  );
}

// ---- 纯函数与模块级缓存（避免每行、每次渲染重复创建） ----

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function highlight(text: string, keyword: string): React.ReactNode {
  if (!keyword || !text) return text;
  const parts = text.split(new RegExp(`(${escapeRegExp(keyword)})`, 'gi'));
  return parts.map((part, i) =>
    part.toLowerCase() === keyword.toLowerCase() ? (
      <mark key={i} className="rounded-sm bg-primary-subtle px-0.5 text-primary">
        {part}
      </mark>
    ) : (
      part
    )
  );
}

// stripHtml 缓存：相同 desc 输入只解析一次，避免每次 render 都用 DOMParser
const stripHtmlCache = new Map<string, string>();

function stripHtml(html: string): string {
  const cached = stripHtmlCache.get(html);
  if (cached !== undefined) return cached;
  let result: string;
  try {
    const doc = new DOMParser().parseFromString(html, 'text/html');
    result = (doc.body.textContent || '').trim();
  } catch {
    result = html.replace(/<[^>]+>/g, '').trim();
  }
  // 限制缓存大小，避免无限增长
  if (stripHtmlCache.size > 500) stripHtmlCache.clear();
  stripHtmlCache.set(html, result);
  return result;
}

interface ArticleRowProps {
  item: Item;
  index: number;
  selected: boolean;
  focused: boolean;
  checked: boolean;
  multiSelectMode: boolean;
  settings: AppSettings;
  highlightKeyword: string;
  registerRef: (id: number, el: HTMLDivElement | null) => void;
  onActivate: (item: Item) => void;
  onToggleRead: (id: number, read: boolean) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onToggleSelect: (id: number) => void;
  onContextMenu: (e: React.MouseEvent, item: Item) => void;
}

const ArticleRow = React.memo(function ArticleRow({
  item,
  index,
  selected,
  focused,
  checked,
  multiSelectMode,
  settings,
  highlightKeyword,
  registerRef,
  onActivate,
  onToggleRead,
  onToggleStar,
  onToggleReadLater,
  onToggleSelect,
  onContextMenu,
}: ArticleRowProps) {
  // 卸载时 React 会以 null 回调，registerRef 内部据此从 Map 中删除，避免只增不删的内存泄漏
  const setRef = useCallback(
    (el: HTMLDivElement | null) => registerRef(item.id, el),
    [registerRef, item.id]
  );

  const hoverTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const handleMouseEnter = useCallback(() => {
    if (settings.markReadMode !== 'hover' || item.isRead) return;
    hoverTimerRef.current = setTimeout(() => {
      onToggleRead(item.id, true);
    }, settings.markReadHoverDelay);
  }, [item.id, item.isRead, onToggleRead, settings.markReadHoverDelay, settings.markReadMode]);

  const handleMouseLeave = useCallback(() => {
    if (!hoverTimerRef.current) return;
    clearTimeout(hoverTimerRef.current);
    hoverTimerRef.current = null;
  }, []);

  // 组件卸载时清理悬停计时器，防止卸载后仍触发标记已读
  useEffect(() => {
    return () => {
      if (hoverTimerRef.current) {
        clearTimeout(hoverTimerRef.current);
        hoverTimerRef.current = null;
      }
    };
  }, []);

  const handleClick = useCallback(() => onActivate(item), [onActivate, item]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault();
    onActivate(item);
  }, [onActivate, item]);

  const handleContextMenu = useCallback((e: React.MouseEvent) => onContextMenu(e, item), [onContextMenu, item]);

  const handleToggleSelect = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect(item.id);
  }, [onToggleSelect, item.id]);

  const handleToggleStar = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleStar(item.id, !item.isStarred);
  }, [onToggleStar, item.id, item.isStarred]);

  const handleToggleReadLater = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleReadLater(item.id, !item.isReadLater);
  }, [onToggleReadLater, item.id, item.isReadLater]);

  const descText = useMemo(() => {
    if (!item.desc || !settings.listShowPreview) return '';
    const text = stripHtml(item.desc);
    return text.length > 120 ? text.slice(0, 120) + '...' : text;
  }, [item.desc, settings.listShowPreview]);

  // 根据列表密度调整 padding/字号
  const densityClass = settings.listDensity === 'compact'
    ? 'px-4 py-2'
    : settings.listDensity === 'comfortable'
    ? 'px-4 py-4'
    : 'px-4 py-3';

  // 日期格式：始终显示 yyyy-mm-dd，当天或绝对模式下追加 HH:MM
  const dateText = useMemo(() => {
    const datePart = formatDate(item.pubDate ?? '');
    if (!datePart) return '—';
    const d = new Date(item.pubDate ?? '');
    const now = new Date();
    const isSameDay = d.toDateString() === now.toDateString();
    if (settings.listDateFormat === 'absolute' || isSameDay) {
      return `${datePart} ${formatTime(item.pubDate ?? '')}`;
    }
    return `${datePart} ${formatRelative(item.pubDate ?? '')}`;
  }, [item.pubDate, settings.listDateFormat]);

  return (
    <article
      ref={setRef}
      data-item-id={item.id}
      onClick={handleClick}
      onContextMenu={handleContextMenu}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      style={{
        animationDelay: `${Math.min(index, 12) * 18}ms`,
        opacity: settings.dimRead && item.isRead ? 0.55 : 1,
        boxShadow: focused && !selected ? 'inset 0 0 0 1.5px var(--primary)' : 'none',
      }}
      className={cn(
        'group relative cursor-pointer border-b border-border-subtle animate-list-item-in',
        'transition-colors duration-150',
        // 视口外的条目跳过渲染（浏览器级"虚拟化"），长列表下大幅降低布局与绘制成本
        '[content-visibility:auto] [contain-intrinsic-size:auto_112px]',
        densityClass,
        selected ? 'bg-active' : 'hover:bg-hover'
      )}
    >
      {/* selection accent */}
      <span
        className={cn(
          'absolute inset-y-0 left-0 w-[3px] rounded-r bg-primary transition-opacity duration-150',
          selected ? 'opacity-100' : 'opacity-0'
        )}
      />

      <div className="flex items-center gap-2">
        {multiSelectMode ? (
          <button
            onClick={handleToggleSelect}
            className={cn(
              'inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm transition-colors',
              checked ? 'text-primary' : 'text-muted hover:text-secondary'
            )}
            title={checked ? '取消选择' : '选择'}
          >
            {checked ? <CheckSquare size={16} /> : <Square size={16} />}
          </button>
        ) : (
          !item.isRead && (
            <span className="h-2 w-2 shrink-0 rounded-full bg-unread" aria-label="未读" />
          )
        )}
        <SourceAvatar
          name={item.sourceName}
          url={item.sourceUrl}
          size={16}
          className="shrink-0"
        />
        <span
          className={cn(
            'truncate text-[12px] font-medium',
            item.isRead ? 'text-secondary' : 'text-primary'
          )}
        >
          {item.sourceName}
        </span>
      </div>

      <h3
        className={cn(
          'mt-1.5 line-clamp-2 text-[14px] leading-snug text-pretty',
          item.isRead ? 'font-normal text-secondary' : 'font-semibold text-primary'
        )}
      >
        {highlight(item.title, highlightKeyword)}
      </h3>

      {descText && (
        <p className="mt-1 line-clamp-2 text-[12.5px] leading-relaxed text-muted">
          {highlight(descText, highlightKeyword)}
        </p>
      )}

      <div className="mt-2 flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-muted">
          <Clock size={11} className="shrink-0" />
          <span className="text-[11px]">{dateText || '—'}</span>
        </div>
        <div className="flex items-center gap-0.5 opacity-0 transition-opacity duration-150 group-hover:opacity-100 [&:has(.on)]:opacity-100">
          <button
            onClick={handleToggleReadLater}
            className={cn(
              'inline-flex h-6 w-6 items-center justify-center rounded transition-colors hover:bg-active',
              item.isReadLater ? 'on text-primary' : 'text-muted'
            )}
            title={item.isReadLater ? '取消稍后阅读' : '稍后阅读'}
          >
            <Clock size={14} fill={item.isReadLater ? 'currentColor' : 'none'} />
          </button>
          <button
            onClick={handleToggleStar}
            className={cn(
              'inline-flex h-6 w-6 items-center justify-center rounded transition-colors hover:bg-active',
              item.isStarred ? 'on text-primary' : 'text-muted'
            )}
            title={item.isStarred ? '取消收藏' : '收藏'}
          >
            {item.isStarred ? (
              <StarFilledIcon size={14} />
            ) : (
              <StarIcon size={14} />
            )}
          </button>
        </div>
      </div>
    </article>
  );
});
