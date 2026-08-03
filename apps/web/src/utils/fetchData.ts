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
function withTimeout(options: FetchOptions): RequestInit {
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
  if (!base.signal) return { ...base, signal: timeoutSignal };
  // AbortSignal.any 在部分旧运行时缺失，缺失时退回调用方信号（外层仍有 catch 兜底）
  const anyOf = (AbortSignal as unknown as { any?: (signals: AbortSignal[]) => AbortSignal }).any;
  if (!anyOf) return base;
  return { ...base, signal: anyOf([base.signal, timeoutSignal]) };
}

/** 发起请求（统一超时），非 2xx 抛错 */
export async function request(path: string, options: FetchOptions = {}): Promise<Response> {
  const res = await fetch(resolveUrl(path), withTimeout(options));
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res;
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
  const res = await fetch(resolveUrl(path), withTimeout(init));
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
