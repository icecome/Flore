// 默认 API 地址（Web / 开发模式）
// 桌面端会在启动时通过 Wails 绑定动态覆盖为本地后端实际端口
// 内部私有变量，外部仅通过 getApiBase()/getApi()/setApiBase() 访问
import { showToast } from './toast';

let _apiBase = import.meta.env.VITE_GO_API_URL || 'http://localhost:3002';
let _api = `${_apiBase}/api`;

export function getApiBase(): string {
  return _apiBase;
}

export function getApi(): string {
  return _api;
}

export function setApiBase(base: string): void {
  _apiBase = base;
  _api = `${base}/api`;
}

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
  SaveConfigFile?: (configJSON: string) => Promise<string>;
  SavePNGFile?: (data: string) => Promise<string>;
  WindowMinimise?: () => void;
  WindowMaximise?: () => void;
  WindowUnmaximise?: () => void;
  WindowToggleMaximise?: () => void;
  WindowIsMaximised?: () => boolean;
  WindowClose?: () => void;
  RefreshWindowSettings?: () => void;
  RestartApp?: () => Promise<void>;
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

/** 从 Wails 后端获取状态并设置 API 地址 */
async function applyBackendStatus(): Promise<void> {
  const app = getWailsApp();
  if (!app?.GetBackendStatus) return;
  try {
    const status = await app.GetBackendStatus();
    if (status.goBaseURL) setApiBase(status.goBaseURL);
  } catch (err) {
    console.error('Failed to initialize desktop API base:', err);
  }
}

export async function initApiBase(maxWaitMs = 3000): Promise<void> {
  if (!isDesktop() && !maybeWailsEnv()) return;
  const ready = await waitForDesktop(maxWaitMs);
  if (!ready) return;
  applyBackendStatus();
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

/** 在 Web 端触发浏览器下载：从 API 获取 blob 并下载为文件 */
export async function downloadBlob(apiPath: string, filename: string): Promise<void> {
  const res = await fetch(`${getApi()}${apiPath}`);
  if (!res.ok) throw new Error('导出失败');
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
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
