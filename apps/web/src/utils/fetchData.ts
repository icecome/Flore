// 通用 fetch 工具：统一注入超时、拼接 API 地址、错误处理和 toast 提示
// 所有网络请求都应经过这里，避免各处重复实现超时与错误分支

import { getApi, getApiToken } from './apiBase';

/** 统一请求超时时间，避免后端无响应时请求永久挂起 */
export const DEFAULT_TIMEOUT_MS = 30000;

export interface FetchOptions extends RequestInit {
  /** 覆盖默认 30s 超时 */
  timeoutMs?: number;
}

/** 绝对地址原样使用，相对路径拼接当前 API 前缀 */
export function resolveUrl(path: string): string {
  return path.startsWith('http') ? path : `${getApi()}${path}`;
}

/**
 * 统一注入超时信号。
 * 调用方已传入 signal（如全局 AbortController）时保持其取消语义，
 * 但仍通过 AbortSignal.any 叠加超时，确保任何请求都不会无限等待。
 */
function withTimeout(options: FetchOptions): { init: RequestInit; timeoutMs: number } {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, ...init } = options;
  // 桌面端注入的 Bearer Token：仅当存在时才附加，Web 端（无 token）不影响非鉴权接口
  const token = getApiToken();
  // 写请求附加自定义头 X-Requested-With：跨域简单请求无法携带，配合后端 CSRFProtection
  // 中间件阻断恶意网页对本机后端的写请求。GET/HEAD 为只读，不附加以避免触发预检。
  const method = init.method ?? 'GET';
  const isWrite = method !== 'GET' && method !== 'HEAD';
  const headers = {
    ...(init.headers ?? {}),
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(isWrite ? { 'X-Requested-With': 'XMLHttpRequest' } : {}),
  };
  const base: RequestInit = { ...init, headers };
  const timeoutSignal = AbortSignal.timeout(timeoutMs);
  if (!base.signal) return { init: { ...base, signal: timeoutSignal }, timeoutMs };
  // AbortSignal.any 在部分旧运行时缺失，缺失时退回调用方信号（外层仍有 catch 兜底）
  const anyOf = (AbortSignal as unknown as { any?: (signals: AbortSignal[]) => AbortSignal }).any;
  if (!anyOf) return { init: base, timeoutMs };
  return { init: { ...base, signal: anyOf([base.signal, timeoutSignal]) }, timeoutMs };
}

/** 统一发起请求（不校验 ok）：桌面端优先经 Wails 绑定 ApiRequest 由壳进程原生 HTTP
 * 转发到本地后端，Web 端走原生 fetch。
 * Wails v2 macOS 生产 webview 使用 wails:// 自定义 scheme，直接 fetch
 * http://127.0.0.1:port 会被 WebKit 拦截（报 "Load failed"，请求到不了后端），
 * 绑定转发可彻底规避。
 */
async function doRequest(path: string, init: RequestInit, timeoutMs: number = DEFAULT_TIMEOUT_MS): Promise<Response> {
  const url = resolveUrl(path);
  const apiReq = getDesktopApiRequest();
  if (apiReq) {
    let pathAndQuery = url;
    try {
      const u = new URL(url);
      pathAndQuery = u.pathname + u.search;
    } catch { /* url 非标准时原样透传 */ }
    const hdrs = (init.headers ?? {}) as Record<string, string>;
    const { data, ctype } = await bodyToPayload(init.body);
    const finalCtype = hdrs['Content-Type'] || ctype || '';
    try {
      // 将前端超时透传给壳绑定，由 Go 端按该超时控制单次请求（兜底 30s）。
      const res = await apiReq(init.method ?? 'GET', pathAndQuery, data, finalCtype, timeoutMs);
      const bin = Uint8Array.from(atob(res.body), (c) => c.charCodeAt(0));
      // 回填壳透传的响应头（下载文件名、分页、缓存校验等），缺失 Content-Type 时回退 ctype。
      const respHeaders = new Headers();
      for (const [k, v] of Object.entries(res.headers ?? {})) {
        respHeaders.set(k, v);
      }
      if (!respHeaders.has('Content-Type')) {
        respHeaders.set('Content-Type', res.ctype || 'application/json');
      }
      return new Response(bin, { status: res.status, headers: respHeaders });
    } catch (err) {
      // 绑定调用失败（如后端尚未就绪）时回退原生 fetch，由调用方统一报错
      console.error('Desktop API proxy failed, falling back to fetch:', err);
    }
  }
  return fetch(url, init);
}

/** 把请求体编码为 base64 并探测 Content-Type（兼容 XML/JSON/FormData/二进制） */
async function bodyToPayload(body: BodyInit | null | undefined): Promise<{ data: string; ctype: string }> {
  if (body == null) return { data: '', ctype: '' };
  if (typeof body === 'string') {
    const bytes = new TextEncoder().encode(body);
    return { data: base64FromBytes(bytes), ctype: 'text/plain;charset=UTF-8' };
  }
  // FormData / Blob / ArrayBuffer / URLSearchParams：经 Response 归一化获取字节与 Content-Type
  const resp = new Response(body);
  const ctype = resp.headers.get('Content-Type') ?? '';
  const buf = await resp.arrayBuffer();
  return { data: base64FromBytes(new Uint8Array(buf)), ctype };
}

function base64FromBytes(bytes: Uint8Array): string {
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

/** 发起请求（统一超时），非 2xx 抛错 */
export async function request(path: string, options: FetchOptions = {}): Promise<Response> {
  const { init, timeoutMs } = withTimeout(options);
  const res = await doRequest(path, init, timeoutMs);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res;
}

/** 取 Wails 绑定 ApiRequest（桌面端有，Web 端无），避免与 api.ts 循环依赖而直接访问 window.go */
function getDesktopApiRequest():
  | ((method: string, path: string, body: string, contentType: string, timeoutMs: number) => Promise<{ status: number; ctype: string; headers?: Record<string, string>; body: string }>)
  | undefined {
  if (typeof window === 'undefined') return undefined;
  const app = (window as { go?: { main?: { App?: Record<string, unknown> } } }).go?.main?.App;
  return app?.ApiRequest as
    | ((method: string, path: string, body: string, contentType: string, timeoutMs: number) => Promise<{ status: number; ctype: string; headers?: Record<string, string>; body: string }>)
    | undefined;
}

/**
 * 发起请求，非 2xx 时优先抛出后端返回的 `error` 文案（供需要透传后端提示的接口使用）。
 * 与 request 的区别仅在于错误信息更友好，超时注入完全一致。
 */
export async function requestOrServerError(
  path: string,
  options: FetchOptions & { fallbackError: string }
): Promise<Response> {
  const { fallbackError, ...init } = options;
  const { init: reqInit, timeoutMs } = withTimeout(init);
  const res = await doRequest(path, reqInit, timeoutMs);
  if (!res.ok) {
    const payload = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(payload?.error || fallbackError);
  }
  return res;
}

/** 发起请求并解析 JSON，非 2xx 抛错 */
export async function fetchData<T>(path: string, options: FetchOptions = {}): Promise<T> {
  const res = await request(path, options);
  return (await res.json()) as T;
}

/** 发起请求但不关心响应体（POST/PUT/DELETE 等） */
export async function requestVoid(path: string, options: FetchOptions = {}): Promise<void> {
  await request(path, options);
}

/** 发送 JSON 请求体并解析 JSON 响应 */
export async function requestJson<T>(
  path: string,
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE',
  body?: unknown,
  options: FetchOptions = {}
): Promise<T> {
  const res = await request(path, {
    ...options,
    method,
    headers: { 'Content-Type': 'application/json', ...(options.headers ?? {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  // 后端部分接口返回 204/空体，此处容错解析
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}
