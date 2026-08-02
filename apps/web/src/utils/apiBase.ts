// API 地址的唯一数据源。
// 单独成文件是为了让 fetchData 与 api 都能依赖它而不产生循环引用：
//   apiBase ← fetchData ← api（api 再对外 re-export getApi/setApiBase）
// 桌面端会在启动时通过 Wails 绑定动态覆盖为本地后端实际端口。

/** 后端默认监听端口，仅在未配置 VITE_GO_API_URL 且非桌面端时作为回退 */
const FALLBACK_PORT = '3002';

/** 回退到硬编码端口时给出可诊断的提示（生产环境同样输出，便于排查连不上后端的问题） */
function warnFallback(base: string): void {
  console.warn(
    `[api] 未配置 VITE_GO_API_URL，回退到硬编码端口 ${FALLBACK_PORT}（${base}）。` +
    '若后端监听在其它端口，请在构建时设置 VITE_GO_API_URL。'
  );
}

function detectDefaultBase(): string {
  const configured = import.meta.env.VITE_GO_API_URL;
  if (configured) return configured;
  // Web 端：复用当前页面主机名，避免局域网访问时被硬编码的 localhost 挡住
  if (typeof window !== 'undefined' && /^https?:$/.test(window.location.protocol)) {
    const { protocol, hostname } = window.location;
    const base = `${protocol}//${hostname}:${FALLBACK_PORT}`;
    warnFallback(base);
    return base;
  }
  // 桌面端会在 initApiBase 中被 Wails 注入的真实端口覆盖，此处仅为启动前的占位
  const base = `http://localhost:${FALLBACK_PORT}`;
  warnFallback(base);
  return base;
}

let _apiBase = detectDefaultBase();
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

// 桌面端从 Wails 绑定 GetAPIToken 取得，用于访问后端敏感接口（写操作 / OPML 导入 / 代理）的 Bearer 鉴权。
// Web 端不设置，后端在未配置 FLORE_API_TOKEN 时也不要求鉴权，因此保持为 null 即可。
let _apiToken: string | null = null;

export function setApiToken(token: string | null): void {
  _apiToken = token;
}

export function getApiToken(): string | null {
  return _apiToken;
}
