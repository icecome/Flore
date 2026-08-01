import type { ScrapeResult } from './base-fetcher.js';

export interface FetcherParam {
  key: string;
  label: string;
  type: 'text' | 'number' | 'url';
  required?: boolean;
  placeholder?: string;
}

export interface FetcherDefinition {
  /** 路由唯一标识，如 chinawriter */
  id: string;
  /** 显示名称，如 "中国作家网 - 新闻列表" */
  name: string;
  /** 描述文字 */
  description: string;
  /** 匹配的网站域名模式，如 chinawriter.com.cn */
  sitePattern: string;
  /** 该路由需要的参数定义 */
  params: FetcherParam[];
  /** 执行抓取 */
  fetch(url: string, params: Record<string, string>): Promise<ScrapeResult[]>;
}

const registry = new Map<string, FetcherDefinition>();

export function registerFetcher(def: FetcherDefinition) {
  if (registry.has(def.id)) {
    console.warn(`[Registry] Fetcher "${def.id}" already registered, overwriting`);
  }
  registry.set(def.id, def);
  console.log(`[Registry] Registered fetcher: ${def.id} (${def.name})`);
}

export function getFetcher(id: string): FetcherDefinition | undefined {
  return registry.get(id);
}

export function listFetchers(): FetcherDefinition[] {
  return Array.from(registry.values()).map((def) => ({
    ...def,
    // 返回定义时不包含 fetch 实现（仅元数据），供前端展示
    fetch: undefined as unknown as FetcherDefinition['fetch'],
  }));
}

/** 初始化：注册所有内置路由 */
export async function initRegistry() {
  // 动态注册所有站点路由
  const { default: genericCss } = await import('./sites/generic-css.js');
  registerFetcher(genericCss);

  const { default: rssFeed } = await import('./sites/rss-feed.js');
  registerFetcher(rssFeed);

  const { default: chinawriter } = await import('./sites/chinawriter.js');
  registerFetcher(chinawriter);

  console.log(`[Registry] Initialized with ${registry.size} fetchers`);
}