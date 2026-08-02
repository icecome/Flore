// 应用统一的 API 层：
//  - 运行环境桥接（Wails 桌面端能力、剪贴板、外链、文件保存）
//  - 后端 REST 接口的语义化封装（组件不得自行 fetch）
// API 地址状态位于 ./apiBase，此处 re-export 保持 `utils/api` 单一入口。
import { showToast } from './toast';
import { getApi, setApiBase, setApiToken } from './apiBase';
import { fetchData, request, requestJson, requestOrServerError, requestVoid } from './fetchData';
import type { FilterRule, Folder, Item, Source } from '../types';

export { getApi, getApiBase, setApiBase } from './apiBase';

export interface BackendStatus {
  goStarted: boolean;
  goBaseURL: string;
}

// Wails 桌面端 App 绑定的类型定义，集中管理避免重复
interface WailsApp {
  GetBackendStatus?: () => Promise<BackendStatus>;
  OpenExternal?: (url: string) => Promise<void>;
  PickOPMLFile?: () => Promise<string>;
  SaveOPMLFile?: () => Promise<string>;
  SaveDatabaseFile?: () => Promise<string>;
  SaveBackupFile?: (name: string) => Promise<string>;
  SaveConfigFile?: (configJSON: string) => Promise<string>;
  SavePNGFile?: (data: string) => Promise<string>;
  WindowMinimise?: () => void;
  WindowMaximise?: () => void;
  WindowUnmaximise?: () => void;
  WindowToggleMaximise?: () => void;
  WindowIsMaximised?: () => Promise<boolean>;
  GetWindowState?: () => Promise<{ maximised: boolean }>;
  WindowClose?: () => void;
  RefreshWindowSettings?: () => void;
  RestartApp?: () => Promise<void>;
  SaveWindowState?: (maximised: boolean) => void;
  /** 桌面端提供后端 API Token，前端访问敏感接口时附加 Bearer 鉴权头（M5 集成） */
  GetAPIToken?: () => Promise<string>;
  /** 检查更新，返回更新信息或 null（无更新）；仅桌面端可用 */
  CheckForUpdate?: () => Promise<UpdateInfo | null>;
  /** 应用已检查的更新（会触发应用重启） */
  StartUpdate?: () => Promise<void>;
}

interface WailsRuntime {
  go?: { main?: { App?: WailsApp } };
}

function getWailsApp(): WailsApp | undefined {
  if (typeof window === 'undefined') return undefined;
  return (window as unknown as WailsRuntime).go?.main?.App;
}

export function getDesktopApp(): WailsApp | undefined {
  return getWailsApp();
}

export function isDesktop(): boolean {
  return !!getWailsApp()?.GetBackendStatus;
}

/** 尝试通过桌面端 Wails runtime 写入剪贴板 */
async function clipboardWriteDesktop(text: string): Promise<boolean> {
  if (!isDesktop()) return false;
  try {
    const runtime = (window as unknown as { runtime?: { ClipboardSetText?: (t: string) => Promise<boolean> } }).runtime;
    if (!runtime?.ClipboardSetText) return false;
    await runtime.ClipboardSetText(text);
    return true;
  } catch {
    return false;
  }
}

/** 尝试通过 navigator.clipboard 写入剪贴板 */
async function clipboardWriteNavigator(text: string): Promise<boolean> {
  if (!navigator.clipboard?.writeText) return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** 尝试通过 execCommand 写入剪贴板（最旧的回退方案） */
async function clipboardWriteFallback(text: string): Promise<boolean> {
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
    return true;
  } catch {
    return false;
  }
}

/** 写入剪贴板：桌面端优先用 Wails runtime，Web 端用 navigator.clipboard */
export async function clipboardWrite(text: string): Promise<boolean> {
  if (await clipboardWriteDesktop(text)) return true;
  if (await clipboardWriteNavigator(text)) return true;
  return clipboardWriteFallback(text);
}

/** 使用系统默认浏览器打开外部链接（桌面端用 Wails runtime，Web 端用 window.open） */
export function openExternal(url: string): void {
  // 协议白名单校验，防止 javascript:/data: XSS
  try {
    const u = new URL(url);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') {
      showToast('仅支持打开 http/https 链接');
      return;
    }
  } catch {
    showToast('无效的链接');
    return;
  }
  const app = getWailsApp();
  if (app?.OpenExternal) {
    app.OpenExternal(url).catch((err) => {
      console.error('Failed to open external:', err);
      showToast('调用系统浏览器失败，已尝试在新窗口打开');
      // 回退到 window.open，避免用户点击无反应
      window.open(url, '_blank', 'noopener,noreferrer');
    });
  } else {
    window.open(url, '_blank', 'noopener,noreferrer');
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * 桌面端启动时调用，从 Wails 主进程获取 Go 后端的动态端口，
 * 覆盖默认的固定地址，避免端口占用导致服务不可用。
 * 若 window.go 尚未注入，会短暂轮询等待。
 */
// 同步快速判断当前页面是否运行在 Wails 环境中（生产 wails:// 或开发 :34115）。
// 用于避免 Web 端在 initApiBase 中空等 window.go 注入。
function maybeWailsEnv(): boolean {
  if (typeof window === 'undefined') return false;
  // 生产构建：Wails 使用自定义 scheme（wails:// 或 http://wails.localhost）
  if (window.location.protocol.startsWith('wails')) return true;
  // 开发模式：Wails dev server 固定监听 34115
  if (window.location.port === '34115') return true;
  return false;
}

/** 轮询等待 Wails 注入，超时则返回 false */
async function waitForDesktop(maxWaitMs: number): Promise<boolean> {
  const deadline = Date.now() + maxWaitMs;
  while (!isDesktop() && Date.now() < deadline) {
    await sleep(50);
  }
  return isDesktop();
}

/** 从 Wails 后端获取状态并设置 API 地址，轮询等待后端就绪 */
async function applyBackendStatus(): Promise<void> {
  const app = getWailsApp();
  if (!app?.GetBackendStatus) return;
  // 桌面端：提前取得后端 API Token，供后续敏感接口（写操作 / OPML 导入 / 代理）鉴权使用
  if (app.GetAPIToken) {
    try {
      const token = await app.GetAPIToken();
      if (token) setApiToken(token);
    } catch (err) {
      console.warn('Failed to fetch API token from desktop:', err);
    }
  }
  try {
    const status = await app.GetBackendStatus();
    // 轮询等待后端启动完成，避免设置为 http://127.0.0.1:0
    const maxRetries = 20;
    for (let i = 0; i < maxRetries; i++) {
      if (status.goStarted && status.goBaseURL) {
        setApiBase(status.goBaseURL);
        return;
      }
      await sleep(200);
      const retry = await app.GetBackendStatus();
      if (retry.goStarted && retry.goBaseURL) {
        setApiBase(retry.goBaseURL);
        return;
      }
    }
    // 超时仍未就绪，记录错误但不设置错误地址
    console.warn('Backend not ready after polling, using default API base');
  } catch (err) {
    console.error('Failed to initialize desktop API base:', err);
  }
}

export async function initApiBase(maxWaitMs = 3000): Promise<void> {
  if (!isDesktop() && !maybeWailsEnv()) return;
  const ready = await waitForDesktop(maxWaitMs);
  if (!ready) return;
  // 必须等待 API base 设置完成，否则首个请求可能拿到错误的默认地址
  await applyBackendStatus();
}

/** 将 Blob 转换为 base64 字符串（分块处理避免大文件栈溢出） */
export async function blobToBase64(blob: Blob): Promise<string> {
  const buf = await blob.arrayBuffer();
  const bytes = new Uint8Array(buf);
  const chunkSize = 8192;
  let binary = '';
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
  }
  return btoa(binary);
}

/** 触发浏览器把 Blob 保存为文件 */
export function saveBlobAsFile(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** 在 Web 端触发浏览器下载：从 API 获取 blob 并下载为文件 */
export async function downloadBlob(apiPath: string, filename: string): Promise<void> {
  const res = await request(apiPath);
  saveBlobAsFile(await res.blob(), filename);
}

/** 通过桌面端 Wails runtime 保存 PNG 文件 */
async function savePNGDesktop(blob: Blob): Promise<boolean> {
  if (!isDesktop()) return false;
  const app = getWailsApp();
  if (!app?.SavePNGFile) return false;
  try {
    const base64 = await blobToBase64(blob);
    const path = await app.SavePNGFile(base64);
    return !!path;
  } catch {
    return false;
  }
}

/** 通过 Web showSaveFilePicker 保存 PNG 文件 */
async function savePNGWeb(blob: Blob, defaultName: string): Promise<boolean> {
  let writable: FileSystemWritableFileStream | null = null;
  try {
    const handle = await window.showSaveFilePicker({
      suggestedName: defaultName,
      types: [{ description: 'PNG 图片', accept: { 'image/png': ['.png'] } }],
    });
    writable = await handle.createWritable();
    await writable.write(blob);
    return true;
  } catch {
    return false;
  } finally {
    // 异常路径也必须关闭流，否则文件句柄泄漏
    if (writable) {
      try { await writable.close(); } catch { /* ignore close error */ }
    }
  }
}

/** 保存 PNG 文件：桌面端用 Wails 原生保存对话框，Web 端用 showSaveFilePicker */
export async function savePNGFile(blob: Blob, defaultName: string): Promise<boolean> {
  if (await savePNGDesktop(blob)) return true;
  return savePNGWeb(blob, defaultName);
}

// ============================================================================
// 后端 REST 接口封装
// 组件与 hook 禁止自行 fetch，一律调用下列语义化方法；
// 超时（30s）、URL 拼接与非 2xx 抛错均由 fetchData 统一处理，调用方只需 try/catch + toast。
// ============================================================================

const JSON_HEADERS = { 'Content-Type': 'application/json' } as const;

// ---------------------------------------------------------------- 文章

export function listItems(params: URLSearchParams, signal?: AbortSignal): Promise<Item[]> {
  return fetchData<Item[]>(`/items?${params.toString()}`, { signal });
}

export function countItems(params: URLSearchParams, signal?: AbortSignal): Promise<number> {
  return fetchData<{ count: number }>(`/items/count?${params.toString()}`, { signal })
    .then((data) => (typeof data.count === 'number' ? data.count : 0));
}

export function searchItems(params: URLSearchParams, signal?: AbortSignal): Promise<Item[]> {
  return fetchData<Item[]>(`/items/search?${params.toString()}`, { signal });
}

/** 标记单篇文章已读 / 未读 */
export function setItemRead(id: number, read: boolean): Promise<void> {
  return requestVoid(`/items/${id}/${read ? 'read' : 'unread'}`, { method: 'POST' });
}

/** 收藏 / 取消收藏 */
export function setItemStarred(id: number, starred: boolean): Promise<void> {
  return requestVoid(`/items/${id}/${starred ? 'star' : 'unstar'}`, { method: 'POST' });
}

/** 加入 / 移出稍后阅读 */
export function setItemReadLater(id: number, readLater: boolean): Promise<void> {
  return requestVoid(`/items/${id}/${readLater ? 'read-later' : 'unread-later'}`, { method: 'POST' });
}

/** 按范围全部标记已读 */
export function markAllRead(scope: { sourceId?: number; folderId?: number }): Promise<void> {
  return requestVoid('/items/read-all', { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(scope) });
}

/** 按 ID 列表批量标记已读 / 未读 */
export function markItemsRead(ids: number[], read: boolean): Promise<void> {
  return requestVoid('/items/mark-read', { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ ids, read }) });
}

/** 批量导出文章，返回二进制内容（zip / json） */
export async function exportItems(scope: unknown, format: 'markdown' | 'json'): Promise<Blob> {
  const res = await requestOrServerError(`/items/export?format=${format}`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify(scope),
    timeoutMs: 120000, // 导出可能较慢，单独放宽超时
    fallbackError: '导出失败',
  });
  return res.blob();
}

/** 提取文章全文（Readability） */
export function getReadability(itemId: number, signal?: AbortSignal): Promise<{ content: string; title: string }> {
  return fetchData<{ content: string; title: string }>(`/items/${itemId}/readability`, { signal });
}

/** 单篇文章导出为 Markdown 的下载地址 */
export function getItemMarkdownUrl(itemId: number): string {
  return `${getApi()}/items/${itemId}/export.md`;
}

/** 按留存策略清理历史文章 */
export function cleanupArticles(policy: {
  retentionDays: number;
  retentionMax: number;
  excludeStarred: boolean;
  excludeReadLater: boolean;
}): Promise<void> {
  return requestVoid('/articles/cleanup', { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(policy) });
}

// ---------------------------------------------------------------- 订阅源 / 文件夹

export function listSources(signal?: AbortSignal): Promise<Source[]> {
  return fetchData<Source[]>('/sources', { cache: 'no-store', signal });
}

export function listFolders(signal?: AbortSignal): Promise<Folder[]> {
  return fetchData<Folder[]>('/folders', { cache: 'no-store', signal });
}

/** 新建订阅源，后端返回的错误文案会作为 Error.message 抛出 */
export async function createSource(payload: unknown): Promise<void> {
  await requestOrServerError('/sources/create', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify(payload),
    fallbackError: '创建失败',
  });
}

/** 更新订阅源（局部字段） */
export function updateSource(id: number, patch: Record<string, unknown>): Promise<void> {
  return requestVoid(`/sources/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(patch) });
}

export function deleteSource(id: number): Promise<void> {
  return requestVoid(`/sources/${id}`, { method: 'DELETE' });
}

export function deleteSourcesBatch(ids: number[]): Promise<void> {
  return requestVoid('/sources/delete-batch', { method: 'DELETE', headers: JSON_HEADERS, body: JSON.stringify({ ids }) });
}

export function createFolder(name: string): Promise<{ id: number }> {
  return requestJson<{ id: number }>('/folders', 'POST', { name });
}

export function renameFolder(id: number, name: string): Promise<void> {
  return requestVoid(`/folders/${id}`, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify({ name }) });
}

export function deleteFolder(id: number): Promise<void> {
  return requestVoid(`/folders/${id}`, { method: 'DELETE' });
}

/** 清空文件夹内订阅源归属（保留文件夹本身） */
export function clearFolderSources(id: number): Promise<void> {
  return requestVoid(`/folders/${id}/clear`, { method: 'POST' });
}

/** 触发抓取：指定源 / 指定文件夹 / 全部 */
export function triggerFetch(scope: { sourceId?: number; folderId?: number }): Promise<void> {
  if (scope.sourceId !== undefined) return requestVoid(`/sources/${scope.sourceId}/fetch`, { method: 'POST' });
  if (scope.folderId !== undefined) return requestVoid(`/folders/${scope.folderId}/fetch`, { method: 'POST' });
  return requestVoid('/sources/fetch-all', { method: 'POST' });
}

export interface FetchStatus {
  fetching: boolean;
  total?: number;
  done?: number;
  /** 本轮抓取新增文章数，抓取结束时由后端返回 */
  newItems?: number;
}

export function getFetchStatus(signal?: AbortSignal): Promise<FetchStatus> {
  return fetchData<FetchStatus>('/sources/fetch-status', { cache: 'no-store', signal });
}

// ---------------------------------------------------------------- OPML / 数据库

export async function importOPML(xml: string): Promise<void> {
  await requestOrServerError('/opml/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/xml' },
    body: xml,
    timeoutMs: 120000, // 大 OPML 解析较慢
    fallbackError: '导入失败',
  });
}

export function exportOPML(): Promise<void> {
  return downloadBlob('/opml/export', 'subscriptions.opml');
}

export async function restoreDatabase(file: File): Promise<void> {
  const formData = new FormData();
  formData.append('file', file);
  await request('/database/restore', { method: 'POST', body: formData, timeoutMs: 120000 });
}

/** 数据库导出地址（Web 端直接跳转下载） */
export function getDatabaseExportUrl(): string {
  return `${getApi()}/database/export`;
}

// ---------------------------------------------------------------- 过滤规则

export function listFilterRules(): Promise<FilterRule[]> {
  return fetchData<FilterRule[]>('/filter-rules');
}

export function saveFilterRule(id: number | null, payload: Record<string, unknown>): Promise<void> {
  const path = id ? `/filter-rules/${id}` : '/filter-rules';
  return requestVoid(path, { method: id ? 'PUT' : 'POST', headers: JSON_HEADERS, body: JSON.stringify(payload) });
}

export function deleteFilterRule(id: number): Promise<void> {
  return requestVoid(`/filter-rules/${id}`, { method: 'DELETE' });
}

/** 试运行规则，返回命中的文章列表 */
export function testFilterRule(id: number): Promise<Item[]> {
  return requestJson<Item[]>(`/filter-rules/${id}/test`, 'POST');
}

// ---------------------------------------------------------------- 备份

export interface BackupEntry {
  name: string;
  size: number;
  modTime: string;
  hasDb: boolean;
  hasCfg: boolean;
  hasOpml: boolean;
}

/** 备份恢复粒度 */
export type RestoreScope = 'all' | 'config' | 'opml';

const RESTORE_PATH: Record<RestoreScope, string> = {
  all: 'restore',
  config: 'restore-config',
  opml: 'restore-opml',
};

export function listBackups(): Promise<BackupEntry[]> {
  return fetchData<BackupEntry[]>('/backups');
}

export function createBackup(): Promise<{ name: string }> {
  return requestJson<{ name: string }>('/backups/create', 'POST', undefined, { timeoutMs: 120000 });
}

export function deleteBackup(name: string): Promise<void> {
  return requestVoid(`/backups/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export async function downloadBackup(name: string): Promise<void> {
  // 桌面端：使用原生保存文件对话框
  const app = getWailsApp();
  if (app?.SaveBackupFile) {
    await app.SaveBackupFile(name);
    return;
  }
  // Web 端：触发浏览器下载
  const res = await request(`/backups/${encodeURIComponent(name)}/download`, { timeoutMs: 120000 });
  saveBlobAsFile(await res.blob(), name);
}

export function restoreBackup(name: string, scope: RestoreScope): Promise<void> {
  return requestVoid(`/backups/${encodeURIComponent(name)}/${RESTORE_PATH[scope]}`, {
    method: 'POST',
    timeoutMs: 120000,
  });
}

/** 上传外部备份 zip，返回落地后的备份信息 */
export function importBackup(file: File): Promise<BackupEntry> {
  const formData = new FormData();
  formData.append('backup', file);
  return fetchData<BackupEntry>('/backups/restore/import', {
    method: 'POST',
    body: formData,
    timeoutMs: 120000,
  });
}

export function cleanupBackups(): Promise<void> {
  return requestVoid('/backups/cleanup', { method: 'POST', timeoutMs: 60000 });
}

// ---------------------------------------------------------------- 设置 / 版本 / 代理

export function getServerSettings(): Promise<Record<string, string>> {
  return fetchData<Record<string, string>>('/settings');
}

export function putServerSettings(payload: Record<string, string>): Promise<void> {
  return requestVoid('/settings', { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(payload) });
}

export function getVersion(): Promise<{ version: string }> {
  return fetchData<{ version: string }>('/version');
}

// 更新信息结构（桌面端自写更新器返回）
export interface UpdateInfo {
  currentVersion: string;
  latestVersion: string;
  notes: string;
  size: number;
  fileName: string;
  urls: string[];
}

/** 检查更新（仅桌面端），Web 端无更新能力时返回 null */
export async function checkForUpdate(): Promise<UpdateInfo | null> {
  const app = getWailsApp();
  if (!app?.CheckForUpdate) return null;
  return app.CheckForUpdate();
}

/** 应用已检查的更新（仅桌面端），会触发进程重启 */
export async function startUpdate(): Promise<void> {
  const app = getWailsApp();
  if (!app?.StartUpdate) throw new Error('当前环境不支持更新');
  await app.StartUpdate();
}

/** 图片代理前缀，供正文图片重写使用 */
export function getImageProxyBase(): string {
  return `${getApi()}/image-proxy`;
}

/** 站点图标代理前缀，后端经国内图标服务拉取 favicon，避免直接向第三方泄露订阅域名 */
export function getFaviconProxyBase(): string {
  return `${getApi()}/favicon-proxy`;
}

/** 原文最小代理地址（网页模式回退） */
export function getArticleProxyUrl(itemId: number): string {
  return `${getApi()}/proxy/${itemId}`;
}

/** 预检原文能否直接被 iframe 嵌入 */
export function checkArticleFrameable(itemId: number): Promise<{ frameable: boolean; url?: string }> {
  return fetchData<{ frameable: boolean; url?: string }>(`/proxy/check/${itemId}`);
}
