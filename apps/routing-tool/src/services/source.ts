import prisma from '../utils/prisma.js';
import type { Prisma } from '@prisma/client';
import { getConfig } from './config.js';
import { getFetcher } from '../scrapers/registry.js';

const GO_API = process.env.GO_API_URL || 'http://localhost:3002/api';

export interface CreateSourceInput {
  name: string;
  url: string;
  folderId?: number | null;
  routeId?: string;
  routeParams?: string;
  listRule?: string;
  titleRule?: string;
  linkRule?: string;
  descRule?: string;
  dateRule?: string;
  authorRule?: string;
  interval?: number;
}

// 从 URL 生成 RSSHub 风格路径：https://www.chinawriter.com.cn/404057/ → /chinawriter/404057
function generateFeedPath(url: string): string {
  try {
    const u = new URL(url);
    let domain = u.hostname.replace(/^www\./, '');
    domain = domain.split('.')[0];
    let path = u.pathname.replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
    return `${domain}/${path}`.replace(/\/+/g, '/');
  } catch {
    return url.replace(/[^a-z0-9\/]/gi, '/').replace(/\/+/g, '/').slice(1) || 'unknown';
  }
}

// 根据 URL 精确查找订阅源
async function findSourceByUrl(url: string) {
  const all = await prisma.source.findMany();
  let match = all.find((s) => s.url === url);
  if (match) return match;
  match = all.find((s) => s.url === url.replace(/\/$/, ''));
  if (match) return match;
  try {
    const targetUrl = new URL(url);
    targetUrl.pathname = '';
    targetUrl.search = '';
    match = all.find((s) => {
      try {
        const sourceUrl = new URL(s.url);
        sourceUrl.pathname = '';
        sourceUrl.search = '';
        return sourceUrl.href === targetUrl.href;
      } catch {
        return false;
      }
    });
    if (match) return match;
  } catch {
    // 忽略
  }
  return null;
}

export async function createSource(input: CreateSourceInput) {
  const config = await getConfig();
  const data: Prisma.SourceUncheckedCreateInput = {
    name: input.name,
    url: input.url,
    listRule: input.listRule || '',
    interval: input.interval ?? config.defaultInterval,
  };

  if (input.folderId !== undefined && input.folderId !== null) {
    data.folderId = input.folderId;
  }

  // 如果指定了路由，保存路由信息
  if (input.routeId) {
    data.routeId = input.routeId;
    data.routeParams = input.routeParams || '{}';
  }

  // 可选的选择器字段
  if (input.titleRule) data.titleRule = input.titleRule;
  if (input.linkRule) data.linkRule = input.linkRule;
  if (input.descRule) data.descRule = input.descRule;
  if (input.dateRule) data.dateRule = input.dateRule;
  if (input.authorRule) data.authorRule = input.authorRule;

  return prisma.source.create({ data });
}

export async function getAllSources() {
  return prisma.source.findMany({
    orderBy: { createdAt: 'desc' },
    include: { _count: { select: { items: true } } },
  });
}

export async function getSource(id: number) {
  return prisma.source.findUnique({ where: { id } });
}

export async function updateSource(id: number, input: Partial<CreateSourceInput>) {
  const data: Record<string, unknown> = {};
  if (input.name !== undefined) data.name = input.name;
  if (input.url !== undefined) data.url = input.url;
  if (input.folderId !== undefined) data.folderId = input.folderId;
  if (input.interval !== undefined) data.interval = input.interval;
  if (input.listRule !== undefined) data.listRule = input.listRule;
  if (input.titleRule !== undefined) data.titleRule = input.titleRule;
  if (input.linkRule !== undefined) data.linkRule = input.linkRule;
  if (input.descRule !== undefined) data.descRule = input.descRule;
  if (input.dateRule !== undefined) data.dateRule = input.dateRule;
  if (input.authorRule !== undefined) data.authorRule = input.authorRule;
  if (input.routeId !== undefined) data.routeId = input.routeId;
  if (input.routeParams !== undefined) data.routeParams = input.routeParams;

  return prisma.source.update({ where: { id }, data });
}

export async function deleteSource(id: number) {
  return prisma.source.delete({ where: { id } });
}

async function indexItemForSearch(id: number) {
  try {
    await fetch(`${GO_API}/items/${id}/index-search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
  } catch (err) {
    console.error(`[Search] Failed to index item ${id}:`, err);
  }
}

async function reportSourceHealth(id: number, success: boolean, error?: string) {
  try {
    await fetch(`${GO_API}/sources/${id}/health`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ success, error: error || '' }),
    });
  } catch (err) {
    console.error(`[Health] Failed to report health for source ${id}:`, err);
  }
}

async function applyFilterRules(id: number) {
  try {
    await fetch(`${GO_API}/items/${id}/apply-filters`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
  } catch (err) {
    console.error(`[Filter] Failed to apply rules for item ${id}:`, err);
  }
}

export async function fetchSource(id: number) {
  try {
    const result = await doFetchSource(id);
    await reportSourceHealth(id, true);
    return result;
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    await reportSourceHealth(id, false, msg);
    throw err;
  }
}

async function doFetchSource(id: number) {
  const source = await prisma.source.findUnique({ where: { id } });
  if (!source) throw new Error('Source not found');

  let items: Array<{ title: string; link: string; desc?: string; author?: string; pubDate?: Date }> = [];

  if (source.routeId) {
    // 路由模式：使用注册的抓取器
    const fetcher = getFetcher(source.routeId);
    if (!fetcher) throw new Error(`Fetcher "${source.routeId}" not found`);

    let params: Record<string, string> = {};
    if (source.routeParams) {
      try {
        params = JSON.parse(source.routeParams);
      } catch {
        params = {};
      }
    }
    // 确保 url 在 params 中
    params.url = source.url;

    items = await fetcher.fetch(source.url, params);
  } else {
    // 旧版 CSS 选择器模式
    const { default: genericCss } = await import('../scrapers/sites/generic-css.js');
    const params: Record<string, string> = {
      listRule: source.listRule,
      url: source.url,
    };
    if (source.titleRule) params.titleRule = source.titleRule;
    if (source.linkRule) params.linkRule = source.linkRule;
    if (source.descRule) params.descRule = source.descRule;
    if (source.dateRule) params.dateRule = source.dateRule;
    if (source.authorRule) params.authorRule = source.authorRule;

    items = await genericCss.fetch(source.url, params);
  }

  let added = 0;
  for (const item of items) {
    try {
      const upserted = await prisma.item.upsert({
        where: { link: item.link },
        update: {
          title: item.title,
          desc: item.desc ?? null,
          author: item.author ?? null,
          pubDate: item.pubDate ?? null,
        },
        create: {
          sourceId: id,
          title: item.title,
          link: item.link,
          desc: item.desc ?? null,
          author: item.author ?? null,
          pubDate: item.pubDate ?? null,
        },
      });
      await indexItemForSearch(upserted.id);
      await applyFilterRules(upserted.id);
      added++;
    } catch (e) {
      // 重复项跳过
    }
  }

  return { total: items.length, added };
}

export { generateFeedPath, findSourceByUrl };