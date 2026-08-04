import type { ContextMenuItem } from '../components/ContextMenu';
import type { Folder, Item, Source } from '../types';
import { getItemMarkdownUrl, openExternal, getDesktopApp } from './api';
import { showToast } from './toast';

// 通用：安全打开外部 URL（仅允许 http/https）
// 已合并到 openExternal（utils/api.ts），此处保留别名兼容
export const safeOpenUrl = openExternal;

/** 触发单篇文章的 Markdown 下载（桌面端用资源管理器保存对话框，Web端使用保存对话框） */
async function downloadItemMarkdown(itemId: number): Promise<void> {
  const desktopApp = getDesktopApp();
  
  // 方案1：桌面端专用 SaveMarkdownFile 方法
  if (desktopApp?.SaveMarkdownFile) {
    const markdownUrl = getItemMarkdownUrl(itemId);
    try {
      await desktopApp.SaveMarkdownFile(markdownUrl);
      showToast('Markdown 已保存');
    } catch (err) {
      console.error('Failed to save Markdown file via Wails:', err);
      showToast('保存失败，请重试');
    }
    return;
  }

  // 方案2：使用浏览器原生保存对话框（showSaveFilePicker）
  try {
    const handle = await window.showSaveFilePicker({
      suggestedName: `${itemId}.md`,
      types: [{ description: 'Markdown 文件', accept: { 'text/markdown': ['.md'] } }],
    });
    const writable = await handle.createWritable();
    const response = await fetch(getItemMarkdownUrl(itemId));
    const content = await response.text();
    await writable.write(content);
    await writable.close();
    showToast('Markdown 已保存');
    return;
  } catch (err) {
    console.warn('showSaveFilePicker not supported or cancelled', err);
  }

  // 方案3：回退到 fetch + Blob 下载。
  // 目标 URL 是本应用后端 API（非外部 URL），不受 openExternal 协议白名单约束；
  // 用 Blob 下载避免跨源时 download 属性失效导致页面跳转，同时补全错误反馈。
  try {
    const response = await fetch(getItemMarkdownUrl(itemId));
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = objectUrl;
    a.download = `${itemId}.md`;
    a.click();
    URL.revokeObjectURL(objectUrl);
    showToast('正在下载 Markdown...');
  } catch (err) {
    console.error('Failed to download markdown:', err);
    showToast('下载失败，请重试');
  }
}

// ---------- Sidebar: 订阅源 ----------

export interface SidebarSourceMenuHandlers {
  onMarkAllRead: (scope: { sourceId?: number; folderId?: number }) => void;
  onEditSource: (source: Source) => void;
  onRenameSource: (source: Source) => void;
  onMoveSource: (source: Source) => void;
  onRemoveFromFolder: (sourceId: number) => void;
  onDeleteSource: (id: number) => void;
  onFetchSource: (scope: { sourceId?: number; folderId?: number }) => void;
}

export function buildSidebarSourceMenu(source: Source, h: SidebarSourceMenuHandlers): ContextMenuItem[] {
  const base: ContextMenuItem[] = [
    { id: 'read-all', label: '全部标为已读', onClick: () => h.onMarkAllRead({ sourceId: source.id }) },
    { id: 'sep-1', label: '', separator: true },
    { id: 'edit', label: '编辑订阅', onClick: () => h.onEditSource(source) },
    { id: 'rename', label: '重命名', onClick: () => h.onRenameSource(source) },
    { id: 'move', label: '移动到文件夹', onClick: () => h.onMoveSource(source) },
  ];
  if (source.folderId !== null) {
    base.push({ id: 'remove-from-folder', label: '移出文件夹', onClick: () => h.onRemoveFromFolder(source.id) });
  }
  return [
    ...base,
    { id: 'sep-2', label: '', separator: true },
    { id: 'open-feed', label: '新标签页打开订阅', onClick: () => safeOpenUrl(source.url) },
    {
      id: 'open-site',
      label: '新标签页打开原网站',
      onClick: () => {
        try {
          const u = new URL(source.url);
          safeOpenUrl(`https://${u.hostname}`);
        } catch {
          safeOpenUrl(source.url);
        }
      },
    },
    { id: 'fetch', label: '立即抓取', onClick: () => h.onFetchSource({ sourceId: source.id }) },
    { id: 'sep-3', label: '', separator: true },
    { id: 'delete', label: '取消订阅', danger: true, onClick: () => h.onDeleteSource(source.id) },
  ];
}

// ---------- Sidebar: 文件夹 ----------

export interface SidebarFolderMenuHandlers {
  onMarkAllRead: (scope: { sourceId?: number; folderId?: number }) => void;
  onFetchSource: (scope: { sourceId?: number; folderId?: number }) => void;
  onClearFolderSources: (folderId: number) => void;
  onRenameFolder: (folder: Folder) => void;
  onDeleteFolder: (id: number) => void;
}

export function buildSidebarFolderMenu(folder: Folder, h: SidebarFolderMenuHandlers): ContextMenuItem[] {
  return [
    { id: 'read-all', label: '全部标为已读', onClick: () => h.onMarkAllRead({ folderId: folder.id }) },
    { id: 'fetch-all', label: '刷新文件夹', onClick: () => h.onFetchSource({ folderId: folder.id }) },
    { id: 'clear-folder', label: '清空文件夹内订阅', onClick: () => h.onClearFolderSources(folder.id) },
    { id: 'sep-1', label: '', separator: true },
    { id: 'rename', label: '重命名', onClick: () => h.onRenameFolder(folder) },
    { id: 'delete', label: '删除文件夹', danger: true, onClick: () => h.onDeleteFolder(folder.id) },
  ];
}

// ---------- Sidebar: 订阅源面板空白处 ----------

export interface SidebarFeedsAreaMenuHandlers {
  onAddSource: () => void;
  onAddFolder: () => void;
  onImport: () => void;
  onExport: () => void;
  onFetchSource: (scope: { sourceId?: number; folderId?: number }) => void;
  onMarkAllRead: (scope: { sourceId?: number; folderId?: number }) => void;
}

export function buildSidebarFeedsAreaMenu(h: SidebarFeedsAreaMenuHandlers): ContextMenuItem[] {
  return [
    { id: 'add-source', label: '添加订阅源', onClick: h.onAddSource },
    { id: 'add-folder', label: '添加文件夹', onClick: h.onAddFolder },
    { id: 'sep-1', label: '', separator: true },
    { id: 'import', label: '导入 OPML', onClick: h.onImport },
    { id: 'export', label: '导出 OPML', onClick: h.onExport },
    { id: 'sep-2', label: '', separator: true },
    { id: 'fetch-all', label: '刷新所有订阅源', onClick: () => h.onFetchSource({}) },
    { id: 'read-all', label: '全部标为已读', onClick: () => h.onMarkAllRead({}) },
  ];
}

// ---------- ArticleList: 文章项 ----------

export interface ArticleRowMenuHandlers {
  onToggleRead: (id: number, read: boolean) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onBatchMarkRead: (ids: number[], read: boolean) => void;
  onToggleMultiSelect?: () => void;
  onExport?: () => void;
}

export function buildArticleRowMenu(
  item: Item,
  items: Item[],
  multiSelectMode: boolean,
  h: ArticleRowMenuHandlers,
): ContextMenuItem[] {
  const i = items.findIndex((it) => it.id === item.id);
  const aboveIds = items.slice(0, i).map((it) => it.id);
  const belowIds = items.slice(i + 1).map((it) => it.id);
  return [
    { id: 'multi-select', label: multiSelectMode ? '退出多选模式' : '进入多选模式', onClick: () => h.onToggleMultiSelect?.() },
    { id: 'export', label: '批量导出文章', onClick: () => h.onExport?.() },
    { id: 'sep-0', label: '', separator: true },
    { id: 'read', label: item.isRead ? '标记为未读' : '标记为已读', onClick: () => h.onToggleRead(item.id, !item.isRead) },
    { id: 'star', label: item.isStarred ? '取消收藏' : '收藏', onClick: () => h.onToggleStar(item.id, !item.isStarred) },
    { id: 'read-later', label: item.isReadLater ? '取消稍后阅读' : '稍后阅读', onClick: () => h.onToggleReadLater(item.id, !item.isReadLater) },
    { id: 'sep-1', label: '', separator: true },
    {
      id: 'open', label: '在新标签页打开', onClick: () => safeOpenUrl(item.link),
    },
    {
      id: 'copy', label: '复制链接',
      onClick: async () => {
        try { await navigator.clipboard.writeText(item.link); showToast('链接已复制'); }
        catch { showToast('复制失败'); }
      },
    },
    {
      id: 'export-md', label: '导出为 Markdown',
      onClick: () => downloadItemMarkdown(item.id),
    },
    { id: 'sep-2', label: '', separator: true },
    { id: 'read-above', label: '将以上标记为已读', disabled: aboveIds.length === 0, onClick: () => h.onBatchMarkRead(aboveIds, true) },
    { id: 'unread-above', label: '将以上标记为未读', disabled: aboveIds.length === 0, onClick: () => h.onBatchMarkRead(aboveIds, false) },
    { id: 'read-below', label: '将以下标记为已读', disabled: belowIds.length === 0, onClick: () => h.onBatchMarkRead(belowIds, true) },
    { id: 'unread-below', label: '将以下标记为未读', disabled: belowIds.length === 0, onClick: () => h.onBatchMarkRead(belowIds, false) },
  ];
}

// ---------- ArticleList: 头部/空白处 ----------

export interface ArticleListHeaderMenuHandlers {
  onFetch: () => void;
  onMarkAllRead: () => void;
  onToggleMultiSelect?: () => void;
}

export function buildArticleListHeaderMenu(
  h: ArticleListHeaderMenuHandlers,
  canMarkAllRead: boolean,
): ContextMenuItem[] {
  return [
    { id: 'refresh', label: '刷新当前列表', onClick: h.onFetch },
    { id: 'read-all', label: '全部标为已读', disabled: !canMarkAllRead, onClick: h.onMarkAllRead },
    { id: 'sep-1', label: '', separator: true },
    { id: 'multi-select', label: '进入多选模式', onClick: () => h.onToggleMultiSelect?.() },
  ];
}

// ---------- Reader: 正文区 ----------

export interface ReaderContentMenuHandlers {
  onToggleRead: (id: number, read: boolean) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onToggleReaderMode: () => void;
  onToggleViewOriginal: () => void;
  openExternal: (url: string) => void;
}

export function buildReaderContentMenu(
  item: Item,
  displayMode: 'rss' | 'readability' | 'iframe',
  hasSelection: boolean,
  h: ReaderContentMenuHandlers,
): ContextMenuItem[] {
  const items: ContextMenuItem[] = [];
  if (hasSelection) {
    items.push({
      id: 'copy-selection',
      label: '复制选中文字',
      onClick: () => {
        // TODO: document.execCommand 已废弃，未来迁移至 navigator.clipboard.writeText（需同步获取 Selection 文本）
        try { document.execCommand('copy'); } catch { showToast('复制失败'); }
      },
    });
    items.push({ id: 'sep-sel', label: '', separator: true });
  }
  items.push(
    { id: 'reader-mode', label: displayMode === 'readability' ? '退出全文模式' : '全文模式', onClick: h.onToggleReaderMode },
    { id: 'view-original', label: displayMode === 'iframe' ? '退出网页模式' : '网页模式', onClick: h.onToggleViewOriginal },
    { id: 'sep-1', label: '', separator: true },
    { id: 'toggle-read', label: item.isRead ? '标为未读' : '标为已读', onClick: () => h.onToggleRead(item.id, !item.isRead) },
    { id: 'star', label: item.isStarred ? '取消收藏' : '收藏', onClick: () => h.onToggleStar(item.id, !item.isStarred) },
    { id: 'read-later', label: item.isReadLater ? '取消稍后阅读' : '稍后阅读', onClick: () => h.onToggleReadLater(item.id, !item.isReadLater) },
    { id: 'sep-2', label: '', separator: true },
    {
      id: 'copy-link', label: '复制文章链接',
      onClick: async () => {
        try { await navigator.clipboard.writeText(item.link); showToast('链接已复制'); }
        catch { showToast('复制失败'); }
      },
    },
  );
  if (item.link) {
    items.push({ id: 'open-browser', label: '在浏览器打开', onClick: () => h.openExternal(item.link) });
  }
  items.push(
    { id: 'sep-3', label: '', separator: true },
    {
      id: 'export-md', label: '导出为 Markdown',
      onClick: () => downloadItemMarkdown(item.id),
    },
  );
  return items;
}

// ---------- Input: 输入框 ----------

export interface InputMenuHandlers {
  onClear?: () => void;
}

export function buildInputMenu(
  ctx: { hasSelection: boolean; hasValue: boolean; readOnly: boolean },
  h: InputMenuHandlers,
): ContextMenuItem[] {
  const items: ContextMenuItem[] = [];
  if (ctx.hasSelection && !ctx.readOnly) {
    // TODO: document.execCommand 已废弃，未来迁移至 Clipboard API（需处理 cut 场景的选区删除）
    items.push({ id: 'cut', label: '剪切', onClick: () => { try { document.execCommand('cut'); } catch { /* ignore */ } } });
  }
  if (ctx.hasSelection) {
    items.push({ id: 'copy', label: '复制', onClick: () => { try { document.execCommand('copy'); } catch { /* ignore */ } } });
  }
  if (!ctx.readOnly) {
    items.push({
      id: 'paste', label: '粘贴',
      onClick: async () => {
        try {
          const text = await navigator.clipboard.readText();
          const el = document.activeElement as HTMLInputElement | HTMLTextAreaElement | null;
          if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')) {
            const start = el.selectionStart ?? el.value.length;
            const end = el.selectionEnd ?? el.value.length;
            const newValue = el.value.slice(0, start) + text + el.value.slice(end);
            const setter = Object.getOwnPropertyDescriptor(el.constructor.prototype, 'value')?.set;
            setter?.call(el, newValue);
            el.dispatchEvent(new Event('input', { bubbles: true }));
            el.selectionStart = el.selectionEnd = start + text.length;
          }
        } catch {
          showToast('粘贴失败，请使用 Ctrl+V');
        }
      },
    });
  }
  items.push({
    id: 'select-all', label: '全选', onClick: () => {
      const el = document.activeElement as HTMLInputElement | HTMLTextAreaElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')) el.select();
    },
  });
  if (ctx.hasValue && h.onClear) {
    items.push({ id: 'sep-1', label: '', separator: true });
    items.push({ id: 'clear', label: '清空', onClick: () => h.onClear?.() });
  }
  return items;
}
