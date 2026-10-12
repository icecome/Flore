import React, { useCallback, useEffect, useMemo, useState } from 'react';

import type { Folder, Source } from '../types';
import { cn } from '../lib/cn';
import ContextMenu, { type ContextMenuItem } from './ContextMenu';
import SettingsModal, { type TabId } from './SettingsModal';
import SearchBox from './SearchBox';
import type { AppSettings } from '../utils/settings';
import SourceAvatar from './SourceAvatar';
import { useContextMenu } from '../hooks/useContextMenu';
import { usePlatform } from '../hooks/usePlatform';
import { useSyncStatus } from '../hooks/useSyncStatus';
import {
  buildSidebarSourceMenu,
  buildSidebarFolderMenu,
  buildSidebarFeedsAreaMenu,
} from '../utils/contextMenu';
import {
  Settings,
  Plus,
  StarFilledIcon,
  ChevronDown,
  ChevronRight,
  Clock,
  Inbox,
  Folder as FolderIcon,
  AlertTriangle,
  Circle,
  FolderOpenIcon,
  Search,
  RefreshCw,
  PauseCircle,
} from './icons';

interface Props {
  sources: Source[];
  folders: Folder[];
  settings: AppSettings;
  onSettingsChange: (settings: AppSettings) => void;
  selectedSourceId: number | null;
  selectedFolderId: number | null;
  filter: 'all' | 'unread' | 'starred' | 'readLater';
  onSelectSource: (id: number | null) => void;
  onSelectFolder: (id: number | null) => void;
  onSelectFilter: (filter: 'all' | 'unread' | 'starred' | 'readLater') => void;
  onAddSource: () => void;
  onAddFolder: () => void;
  onImport: () => void;
  onExport: () => void;
  onRenameFolder: (folder: Folder) => void;
  onDeleteFolder: (id: number) => void;
  onRenameSource: (source: Source) => void;
  onMoveSource: (source: Source) => void;
  onFetchSource: (scope: { sourceId?: number; folderId?: number }) => void;
  onSourcesChanged: () => void;
  onMarkAllRead: (scope: { sourceId?: number; folderId?: number }) => void;
  onDeleteSource: (id: number) => void;
  onClearFolderSources: (folderId: number) => void;
  onEditSource: (source: Source) => void;
  onRemoveFromFolder: (sourceId: number) => void;
  onToggleSourceActive: (sourceId: number, active: boolean) => void;
  readLaterCount?: number;
  totalCount?: number; // 全文总数（用于"全部文章"计数）
  unreadCountInScope?: number; // 当前范围未读数（用于"未读文章"计数）
  searchKeyword?: string;
  isSearchMode?: boolean;
  onSearch?: (keyword: string) => void;
  onClearSearch?: () => void;
}

function computeFolderUnread(
  folderId: number,
  sourcesByFolder: Map<number, Source[]>,
  childrenByFolder: Map<number, Folder[]>,
  cache: Map<number, number>
): number {
  const cached = cache.get(folderId);
  if (cached !== undefined) return cached;
  let total = (sourcesByFolder.get(folderId) || []).reduce(
    (sum, s) => sum + (s.unreadCount || 0),
    0
  );
  const children = childrenByFolder.get(folderId) || [];
  for (const cf of children) {
    total += computeFolderUnread(cf.id, sourcesByFolder, childrenByFolder, cache);
  }
  cache.set(folderId, total);
  return total;
}

const EXPANDED_FOLDERS_KEY = 'flore-expanded-folders';

// 从 localStorage 恢复已展开文件夹集合；无记录（首次启动）时返回空集合（默认全部折叠）
function loadExpandedFolders(): Set<number> {
  try {
    const raw = localStorage.getItem(EXPANDED_FOLDERS_KEY);
    if (!raw) return new Set<number>();
    const arr = JSON.parse(raw);
    if (Array.isArray(arr)) return new Set<number>(arr.filter((n) => typeof n === 'number'));
  } catch {
    // 解析失败回退到默认折叠
  }
  return new Set<number>();
}

// 持久化展开/折叠状态到 localStorage
function saveExpandedFolders(set: Set<number>): void {
  try {
    localStorage.setItem(EXPANDED_FOLDERS_KEY, JSON.stringify([...set]));
  } catch {
    // 隐私模式等写入失败时静默忽略
  }
}

export default function Sidebar({
  sources,
  folders,
  settings,
  onSettingsChange,
  selectedSourceId,
  selectedFolderId,
  filter,
  onSelectSource,
  onSelectFolder,
  onSelectFilter,
  onAddSource,
  onAddFolder,
  onImport,
  onExport,
  onRenameFolder,
  onDeleteFolder,
  onRenameSource,
  onMoveSource,
  onFetchSource,
  onSourcesChanged,
  onMarkAllRead,
  onDeleteSource,
  onClearFolderSources,
  onEditSource,
  onRemoveFromFolder,
  onToggleSourceActive,
  readLaterCount = 0,
  searchKeyword = '',
  isSearchMode = false,
  onSearch,
  onClearSearch,
  totalCount = 0,
  unreadCountInScope = 0,
}: Props) {
  // macOS 端独立标题栏已移除，侧边栏顶到窗口顶部；顶栏搜索按钮与交通灯共用一行
  const { isMac, ready } = usePlatform();
  const [showSearch, setShowSearch] = useState(false);

  // 根据设置过滤订阅源（当前选中的源始终显示）
  // hideRead 保留"从未抓取过"的源（lastSuccessAt 为空），避免新导入源在抓取前被隐藏
  const visibleSources = useMemo(() => {
    return sources.filter((source) => {
      if (source.id === selectedSourceId) return true;
      if (settings.hidePrivateInSidebar && source.isPrivate) return false;
      if (settings.hideRead && (source.unreadCount || 0) === 0 && source.lastSuccessAt) return false;
      return true;
    });
  }, [sources, settings.hidePrivateInSidebar, settings.hideRead, selectedSourceId]);

  // 展开状态记忆：从 localStorage 恢复，首次启动默认全部折叠（不再强制展开所有文件夹）
  const [expandedFolders, setExpandedFolders] = useState<Set<number>>(loadExpandedFolders);

  // 持久化展开/折叠状态，重启后恢复用户上次的折叠操作
  useEffect(() => {
    saveExpandedFolders(expandedFolders);
  }, [expandedFolders]);

  const [showSettings, setShowSettings] = useState(false);
  const [settingsTab, setSettingsTab] = useState<TabId | null>(null);
  const syncStatus = useSyncStatus();
  const { menuProps, showMenu } = useContextMenu();

  const uncategorized = useMemo(
    () => visibleSources.filter((source) => source.folderId === null),
    [visibleSources]
  );

  // 按文件夹 ID 索引直接子源与子文件夹，避免每次 filter O(n)
  const { folderSourcesMap, folderChildrenMap, folderUnreadMap } = useMemo(() => {
    const sourcesByFolder = new Map<number, Source[]>();
    const childrenByFolder = new Map<number, Folder[]>();
    for (const s of visibleSources) {
      if (s.folderId === null) continue;
      const arr = sourcesByFolder.get(s.folderId);
      if (arr) arr.push(s);
      else sourcesByFolder.set(s.folderId, [s]);
    }
    for (const f of folders) {
      if (f.parentId === null) continue;
      const arr = childrenByFolder.get(f.parentId);
      if (arr) arr.push(f);
      else childrenByFolder.set(f.parentId, [f]);
    }
    const unreadByFolder = new Map<number, number>();
    for (const f of folders) {
      computeFolderUnread(f.id, sourcesByFolder, childrenByFolder, unreadByFolder);
    }
    return {
      folderSourcesMap: sourcesByFolder,
      folderChildrenMap: childrenByFolder,
      folderUnreadMap: unreadByFolder,
    };
  }, [visibleSources, folders]);

  const folderSources = useCallback((folderId: number) => folderSourcesMap.get(folderId) || [], [folderSourcesMap]);
  const folderChildren = useCallback((folderId: number) => folderChildrenMap.get(folderId) || [], [folderChildrenMap]);
  const folderUnread = useCallback((folderId: number) => folderUnreadMap.get(folderId) || 0, [folderUnreadMap]);

  const isUnhealthySource = useCallback(
    (s: { fetchFailCount?: number; lastError?: string | null }): boolean => {
      return (s.fetchFailCount || 0) >= 3 || s.lastError != null;
    },
    []
  );

  // 仅当存在异常源时显示「N 个源异常」提示，全部健康时隐藏
  const abnormalCount = useMemo(
    () => visibleSources.filter((s) => sourceHealthState(s) !== 'ok').length,
    [visibleSources]
  );

  const sortByUnreadDescThenName = useCallback(
    <T extends { name: string; unreadCount?: number }>(
      items: T[],
      isUnhealthy?: (item: T) => boolean
    ): T[] => {
      const compareFn = (a: T, b: T) => {
        const unreadDiff = (b.unreadCount || 0) - (a.unreadCount || 0);
        if (unreadDiff !== 0) return unreadDiff;
        const nameCmp = a.name.localeCompare(b.name, 'zh-CN');
        if (nameCmp !== 0) return nameCmp;
        return compareByHealth(a, b, isUnhealthy);
      };
      return [...items].sort(compareFn);
    },
    []
  );

  // 文件夹本身没有 unreadCount 字段，需要从 folderUnreadMap 查未读数再排序
  const sortFoldersByUnreadDescThenName = useCallback(
    (foldersToSort: Folder[]): Folder[] => {
      return [...foldersToSort].sort((a, b) => {
        const unreadDiff = folderUnread(b.id) - folderUnread(a.id);
        if (unreadDiff !== 0) return unreadDiff;
        return a.name.localeCompare(b.name, 'zh-CN');
      });
    },
    [folderUnread]
  );

  function compareByHealth<T extends { name: string; unreadCount?: number }>(
    a: T, b: T, isUnhealthy?: (item: T) => boolean
  ): number {
    if (!isUnhealthy) return 0;
    const badA = isUnhealthy(a);
    const badB = isUnhealthy(b);
    if (badA !== badB) return badA ? 1 : -1;
    return 0;
  }

  const sortedUncategorized = useMemo(
    () => sortByUnreadDescThenName(uncategorized, isUnhealthySource),
    [uncategorized]
  );

  const toggleFolder = useCallback((folderId: number) => {
    setExpandedFolders((prev) => {
      const next = new Set(prev);
      if (next.has(folderId)) next.delete(folderId);
      else next.add(folderId);
      return next;
    });
  }, []);

  const handleSettingsClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setSettingsTab(null);
    setShowSettings(true);
  };

  // 同步指示器：同步中转圈 / 异常红标 / 正常灰标；点击跳转同步设置
  const openSyncSettings = (e: React.MouseEvent) => {
    e.stopPropagation();
    setSettingsTab('sync');
    setShowSettings(true);
  };

  const isFilterActive = (f: 'all' | 'unread' | 'starred' | 'readLater') =>
    filter === f && selectedSourceId === null && selectedFolderId === null;

  const sourceMenuItems = useCallback((source: Source): ContextMenuItem[] => {
    return buildSidebarSourceMenu(source, {
      onMarkAllRead,
      onEditSource,
      onRenameSource,
      onMoveSource,
      onRemoveFromFolder,
      onDeleteSource,
      onFetchSource,
      onToggleActive: onToggleSourceActive,
    });
  }, [onMarkAllRead, onEditSource, onRenameSource, onMoveSource, onRemoveFromFolder, onDeleteSource, onFetchSource, onToggleSourceActive]);

  const buildFolderMenuItems = useCallback((folder: Folder): ContextMenuItem[] => {
    return buildSidebarFolderMenu(folder, {
      onMarkAllRead,
      onFetchSource,
      onClearFolderSources,
      onRenameFolder,
      onDeleteFolder,
    });
  }, [onMarkAllRead, onFetchSource, onClearFolderSources, onRenameFolder, onDeleteFolder]);

  // 递归渲染文件夹节点：renderChildFolder 指向自身，支持任意深度嵌套。
  // useCallback 稳定函数身份，避免每次渲染重建导致 React.memo（FolderNode）整树失效（m-14）。
  const renderFolderNode = useCallback((folder: Folder): React.ReactNode => {
    const expanded = expandedFolders.has(folder.id);
    const unread = folderUnread(folder.id);
    const isSelected = selectedFolderId === folder.id;
    const sources = sortByUnreadDescThenName(folderSources(folder.id), isUnhealthySource);
    const childFolders = sortFoldersByUnreadDescThenName(folderChildren(folder.id));

    return (
      <FolderNode
        key={folder.id}
        folder={folder}
        expanded={expanded}
        isSelected={isSelected}
        unread={unread}
        sources={sources}
        childFolders={childFolders}
        onSelect={() => { onSelectFolder(folder.id); onSelectSource(null); }}
        onToggle={() => toggleFolder(folder.id)}
        onContextMenu={(e) => showMenu(e, buildFolderMenuItems(folder))}
        renderChildFolder={renderFolderNode}
        sourceMenuItems={sourceMenuItems}
        selectedSourceId={selectedSourceId}
        onSelectSource={onSelectSource}
        onSelectFolder={onSelectFolder}
        onContextMenuHandler={showMenu}
      />
    );
  }, [
    expandedFolders,
    selectedFolderId,
    selectedSourceId,
    folderUnread,
    folderSources,
    folderChildren,
    sortFoldersByUnreadDescThenName,
    sortByUnreadDescThenName,
    isUnhealthySource,
    onSelectFolder,
    onSelectSource,
    toggleFolder,
    showMenu,
    buildFolderMenuItems,
    sourceMenuItems,
  ]);

  const renderFolderTree = (rootFolders: Folder[]) => {
    return sortFoldersByUnreadDescThenName(rootFolders).map(renderFolderNode);
  };

  // 顶栏（搜索按钮行）为 macOS 专属：与交通灯共用一行，仅 mac 渲染。
  // 非 mac 永不渲染该行，避免平台未知期出现 28px 空占位行造成布局跳动。
  // ready 仅用于门控非 mac 搜索框的出现时机（加载完成后才显示）。
  const showMacTop = isMac;
  return (
    <aside className="flex h-full w-[260px] shrink-0 flex-col border-r border-border bg-canvas">
      {/* macOS 顶栏：与交通灯共用一行，搜索按钮右对齐，不挤占交通灯 */}
      {showMacTop && (
        <div className="flex h-[28px] items-center justify-end px-3">
          {isMac && (
            <button
              type="button"
              onClick={() => setShowSearch((v) => !v)}
              className="inline-flex h-6 w-6 items-center justify-center rounded text-muted transition-colors hover:bg-hover hover:text-primary"
              title={showSearch ? '收起搜索' : '搜索'}
              aria-label={showSearch ? '收起搜索' : '搜索'}
            >
              <Search size={15} />
            </button>
          )}
        </div>
      )}
      {isMac && ready && showSearch && (
        <div className="px-3 pb-2">
          <SearchBox
            query={searchKeyword}
            onSearch={(kw) => onSearch?.(kw)}
            onClear={() => onClearSearch?.()}
            placeholder="搜索文章"
            autoFocus
          />
        </div>
      )}
      {!isMac && ready && (
        <div className="flex flex-col gap-2.5 px-3 pb-2 pt-3">
          <SearchBox
            query={searchKeyword}
            onSearch={(kw) => onSearch?.(kw)}
            onClear={() => onClearSearch?.()}
            placeholder="搜索文章"
          />
        </div>
      )}

      {/* Filters */}
      <nav className="flex flex-col gap-0.5 px-2 pb-1.5">
        <FilterRow
          icon={<Inbox size={16} />}
          label="全部文章"
          count={totalCount}
          active={isFilterActive('all')}
          onClick={() => {
            onSelectFilter('all');
            onSelectSource(null);
            onSelectFolder(null);
          }}
        />
        <FilterRow
          icon={<Circle size={16} />}
          label="未读文章"
          count={unreadCountInScope}
          countTone="muted"
          active={isFilterActive('unread')}
          onClick={() => {
            onSelectFilter('unread');
            onSelectSource(null);
            onSelectFolder(null);
          }}
        />
        <FilterRow
          icon={<Clock size={16} />}
          label="稍后阅读"
          count={readLaterCount}
          countTone="muted"
          active={isFilterActive('readLater')}
          onClick={() => {
            onSelectFilter('readLater');
            onSelectSource(null);
            onSelectFolder(null);
          }}
        />
        <FilterRow
          icon={<StarFilledIcon size={16} />}
          label="收藏"
          active={isFilterActive('starred')}
          onClick={() => {
            onSelectFilter('starred');
            onSelectSource(null);
            onSelectFolder(null);
          }}
        />
      </nav>

      <div className="mx-3 my-1 h-px bg-border-subtle" />

      {/* Feeds */}
      <div className="flex items-center justify-between px-4 py-1.5">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-muted">订阅源</span>
        <div className="flex items-center gap-0.5">
          <button
            onClick={onAddFolder}
            className="inline-flex h-5 w-5 items-center justify-center rounded text-muted transition-colors hover:bg-hover hover:text-primary"
            title="添加文件夹"
          >
            <FolderIcon size={14} />
          </button>
          <button
            onClick={onAddSource}
            className="inline-flex h-5 w-5 items-center justify-center rounded text-muted transition-colors hover:bg-hover hover:text-primary"
            title="添加订阅源"
          >
            <Plus size={14} />
          </button>
        </div>
      </div>

      <div
        className="flex-1 overflow-y-auto px-2 pb-2"
        onContextMenu={(e) => showMenu(e, buildSidebarFeedsAreaMenu({
          onAddSource,
          onAddFolder,
          onImport,
          onExport,
          onFetchSource,
          onMarkAllRead,
        }))}
      >
        {sources.length === 0 ? (
          <div className="flex h-[120px] flex-col items-center justify-center gap-1 px-4 text-center">
            <p className="text-[13px] text-muted">还没有订阅源</p>
            <p className="text-[11px] text-muted/70">点击上方 + 号添加，或在设置中导入 OPML</p>
          </div>
        ) : (
        <>
        {abnormalCount > 0 && (
          <button
            type="button"
            onClick={() => onFetchSource({})}
            title="部分订阅源暂时无法连接，点击立即重试全部"
            className="mx-1.5 mb-1 flex w-[calc(100%-12px)] items-center gap-1.5 rounded-md px-2 py-1 text-[11px] text-warning transition-colors hover:bg-hover"
          >
            <AlertTriangle size={12} className="shrink-0" />
            <span>{abnormalCount} 个源异常，点击重试</span>
          </button>
        )}
        {renderFolderTree(folders.filter((f) => f.parentId === null))}

        {sortedUncategorized.length > 0 && (
          <>
            <div className="mx-1.5 my-1.5 h-px bg-border-subtle" />
            {sortedUncategorized.map((source) => (
              <SourceRow
                key={source.id}
                source={source}
                health={sourceHealthState(source)}
                active={selectedSourceId === source.id}
                onClick={() => {
                  onSelectSource(source.id);
                  onSelectFolder(null);
                }}
                onContextMenu={(e) => showMenu(e, sourceMenuItems(source))}
              />
            ))}
          </>
        )}
        </>)}
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 border-t border-border-subtle p-2">
        <button
          onClick={handleSettingsClick}
          className="flex flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-[13px] text-secondary transition-colors hover:bg-hover hover:text-primary"
        >
          <Settings size={16} />
          <span>设置</span>
        </button>
        {syncStatus && (syncStatus.enabled || syncStatus.syncing) && (
          <button
            onClick={openSyncSettings}
            title={
              syncStatus.syncing
                ? '同步进行中'
                : syncStatus.lastError
                  ? `同步异常：${syncStatus.lastError}`
                  : '服务端同步已启用'
            }
            className={cn(
              'flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-hover',
              syncStatus.syncing ? 'text-primary' : syncStatus.lastError ? 'text-danger' : 'text-muted'
            )}
          >
            <RefreshCw size={14} className={syncStatus.syncing ? 'animate-spin' : ''} />
          </button>
        )}
      </div>

      {menuProps && <ContextMenu {...menuProps} />}

      {showSettings && (
        <SettingsModal
          settings={settings}
          onSettingsChange={onSettingsChange}
          onClose={() => setShowSettings(false)}
          onSourcesChanged={onSourcesChanged}
          onAddSource={onAddSource}
          sources={sources}
          folders={folders}
          initialTab={settingsTab ?? undefined}
        />
      )}

    </aside>
  );
}

// 源健康分级（T-v03-04）：bad=连续失败≥3（红标）、warn=退避中（橙标）、ok=健康（零标记）
function sourceHealthState(s: { fetchFailCount?: number; lastError?: string | null }): 'ok' | 'warn' | 'bad' {
  if ((s.fetchFailCount || 0) >= 3) return 'bad';
  if (s.lastError != null) return 'warn';
  return 'ok';
}

function FilterRow({
  icon,
  label,
  count,
  countTone = 'primary',
  active,
  onClick,
  onContextMenu,
}: {
  icon: React.ReactNode;
  label: string;
  count?: number;
  countTone?: 'primary' | 'muted';
  active: boolean;
  onClick: () => void;
  onContextMenu?: (e: React.MouseEvent) => void;
}) {
  return (
    <button
      onClick={onClick}
      onContextMenu={onContextMenu}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] transition-colors',
        active ? 'bg-active font-medium text-primary' : 'text-secondary hover:bg-hover'
      )}
    >
      <span className={cn(active ? 'text-primary' : 'text-muted')}>{icon}</span>
      <span className="truncate">{label}</span>
      {count !== undefined && count > 0 && (
        <span
          className={cn(
            'ml-auto rounded-full px-1.5 text-[11px] font-semibold',
            countTone === 'primary' ? 'bg-primary-subtle text-primary' : 'bg-hover text-muted'
          )}
        >
          {count}
        </span>
      )}
    </button>
  );
}

const SourceRow = React.memo(function SourceRow({
  source,
  active,
  health,
  indent,
  onClick,
  onContextMenu,
}: {
  source: Source;
  active: boolean;
  health: 'ok' | 'warn' | 'bad';
  indent?: boolean;
  onClick: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}) {
  let hostname = source.url;
  try {
    hostname = new URL(source.url).hostname;
  } catch {
    // keep original url
  }

  const healthTitle =
    health === 'bad'
      ? source.lastError ?? '订阅源连续失败'
      : health === 'warn'
        ? source.lastError ?? '订阅源暂时不可用'
        : source.name;

  // 暂停的源：不参与抓取，未读徽标清零、名称降权、显示暂停标识
  const paused = !source.active;

  return (
    <button
      onClick={onClick}
      onContextMenu={onContextMenu}
      title={paused ? `${healthTitle}（已暂停）` : healthTitle}
      className={cn(
        'group flex w-full flex-col rounded-md py-1.5 pr-2 text-[13px] transition-colors',
        indent ? 'pl-7' : 'pl-2',
        active ? 'bg-active text-primary' : 'text-secondary hover:bg-hover',
        paused && 'opacity-50'
      )}
    >
      <div className="flex w-full items-center gap-2">
        <SourceAvatar name={source.name} url={source.url} size={18} />
        <span
          className={cn(
            'truncate',
            health === 'bad' && 'text-danger',
            !paused && health === 'ok' && source.unreadCount > 0 ? 'font-medium text-primary' : ''
          )}
        >
          {source.name}
        </span>
        {paused ? (
          <PauseCircle size={13} className="ml-auto shrink-0 text-muted" />
        ) : health === 'bad' ? (
          <AlertTriangle size={13} className="ml-auto shrink-0 text-danger" />
        ) : health === 'warn' ? (
          <AlertTriangle size={13} className="ml-auto shrink-0 text-warning" />
        ) : source.unreadCount > 0 ? (
          <span className="ml-auto shrink-0 rounded-full bg-hover px-1.5 text-[11px] font-medium text-muted">
            {source.unreadCount}
          </span>
        ) : null}
      </div>
      <span className="truncate pl-7 text-left text-[10px] text-muted">{hostname}</span>
    </button>
  );
});

const FolderNode = React.memo(function FolderNode({
  folder,
  expanded,
  isSelected,
  unread,
  sources,
  childFolders,
  onSelect,
  onToggle,
  onContextMenu,
  renderChildFolder,
  sourceMenuItems,
  selectedSourceId,
  onSelectSource,
  onSelectFolder,
  onContextMenuHandler,
}: {
  folder: Folder;
  expanded: boolean;
  isSelected: boolean;
  unread: number;
  sources: Source[];
  childFolders: Folder[];
  onSelect: () => void;
  onToggle: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  renderChildFolder: (folder: Folder) => React.ReactNode;
  sourceMenuItems: (source: Source) => ContextMenuItem[];
  selectedSourceId: number | null;
  onSelectSource: (id: number | null) => void;
  onSelectFolder: (id: number | null) => void;
  onContextMenuHandler: (e: React.MouseEvent, items: ContextMenuItem[]) => void;
}) {
  return (
    <div className="mb-0.5">
      {/* 外层用 div + role="button" 承载「选中文件夹」语义，
          避免 <button> 嵌套 <button>（P2-007：无效 HTML，破坏屏幕阅读器与键盘导航）。 */}
      <div
        role="button"
        tabIndex={0}
        onClick={onSelect}
        onContextMenu={onContextMenu}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onSelect();
          }
        }}
        className={cn(
          'group flex w-full items-center gap-1.5 rounded-md py-1.5 pl-1.5 pr-2 text-[13px] transition-colors',
          isSelected ? 'bg-active text-primary' : 'text-secondary hover:bg-hover'
        )}
      >
        <button
          onClick={(e) => {
            e.stopPropagation();
            onToggle();
          }}
          className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-muted transition-colors hover:bg-hover"
          aria-label={expanded ? '折叠文件夹' : '展开文件夹'}
        >
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </button>
        {expanded ? (
          <FolderOpenIcon size={15} className="shrink-0 text-muted" />
        ) : (
          <FolderIcon size={15} className="shrink-0 text-muted" />
        )}
        <span className="truncate font-medium">{folder.name}</span>
        {unread > 0 && (
          <span className="ml-auto rounded-full bg-hover px-1.5 text-[11px] font-medium text-muted">
            {unread}
          </span>
        )}
      </div>

      {expanded && (
        <div className="ml-3 border-l border-border-subtle pl-1.5">
          {childFolders.map((cf) => renderChildFolder(cf))}
          {sources.map((source) => (
            <SourceRow
              key={source.id}
              source={source}
              health={sourceHealthState(source)}
              active={selectedSourceId === source.id}
              indent
              onClick={() => {
                onSelectSource(source.id);
                onSelectFolder(null);
              }}
              onContextMenu={(e) => onContextMenuHandler(e, sourceMenuItems(source))}
            />
          ))}
        </div>
      )}
    </div>
  );
});
