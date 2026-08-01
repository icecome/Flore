import type { FetcherDefinition } from '../registry.js';
import { BaseFetcher, type ScrapeResult } from '../base-fetcher.js';

/**
 * 通用 CSS 选择器模式
 * 将旧版 scraper/index.ts 的逻辑包装为路由，兼容已有数据
 */
class GenericCssFetcher extends BaseFetcher {
  async fetch(url: string, params: Record<string, string>): Promise<ScrapeResult[]> {
    const $ = await this.fetchHtml(url);
    const items: ScrapeResult[] = [];

    const listSelector = params.listRule || '';
    const titleSelector = params.titleRule;
    const linkSelector = params.linkRule;
    const descSelector = params.descRule;
    const dateSelector = params.dateRule;
    const authorSelector = params.authorRule;

    if (!listSelector) return [];

    // 支持逗号分隔的选择器
    const selectors = listSelector.split(',').map((s) => s.trim()).filter(Boolean);

    for (const selector of selectors) {
      $(selector).each((_, el) => {
        const $el = $(el);

        // 标题
        let title = '';
        if (titleSelector) {
          title = $el.find(titleSelector).first().text().trim();
        } else {
          let bestLen = 0;
          $el.find('a').each((_, a) => {
            const t = $(a).text().trim();
            if (t.length > bestLen) {
              bestLen = t.length;
              title = t;
            }
          });
        }

        // 链接
        let link = '';
        if (linkSelector) {
          link = $el.find(linkSelector).first().attr('href') || '';
        } else if (titleSelector) {
          link = $el.find(titleSelector).first().attr('href') || '';
        } else {
          let bestLen = 0;
          $el.find('a').each((_, a) => {
            const t = $(a).text().trim();
            if (t.length > bestLen) {
              bestLen = t.length;
              link = $(a).attr('href') || '';
            }
          });
        }

        const desc = descSelector ? $el.find(descSelector).first().text().trim() : undefined;
        const author = authorSelector ? $el.find(authorSelector).first().text().trim() : undefined;
        const pubDate = dateSelector ? this.extractDate($el, dateSelector) : undefined;
        const resolvedLink = this.resolveUrl(link, url);

        if (title && resolvedLink) {
          items.push({ title: title.trim(), link: resolvedLink, desc, author, pubDate });
        }
      });
    }

    return items;
  }
}

const fetcher = new GenericCssFetcher();

const definition: FetcherDefinition = {
  id: 'generic-css',
  name: '通用 CSS 选择器模式',
  description: '通过手动配置 CSS 选择器来提取列表页内容，适合熟悉 HTML 结构的高级用户',
  sitePattern: '*',
  params: [
    { key: 'listRule', label: '列表选择器', type: 'text', required: true, placeholder: '.news-list > .item' },
    { key: 'titleRule', label: '标题选择器', type: 'text', placeholder: 'h2 a' },
    { key: 'linkRule', label: '链接选择器', type: 'text', placeholder: 'a' },
    { key: 'descRule', label: '摘要选择器', type: 'text', placeholder: '.summary' },
    { key: 'dateRule', label: '日期选择器', type: 'text', placeholder: 'time' },
    { key: 'authorRule', label: '作者选择器', type: 'text', placeholder: '.author' },
  ],
  fetch: (url, params) => fetcher.fetch(url, params),
};

export default definition;