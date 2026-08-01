import { ofetch } from 'ofetch';
import type { FetcherDefinition } from '../registry.js';
import { BaseFetcher, type ScrapeResult } from '../base-fetcher.js';
import * as cheerio from 'cheerio';

/**
 * 通用 RSS/Atom 订阅源抓取器
 * 用于解析标准的 RSS 2.0 / Atom 源，供 OPML 导入的订阅使用
 */
class RssFeedFetcher extends BaseFetcher {
  async fetch(url: string, _params: Record<string, string>): Promise<ScrapeResult[]> {
    console.log(`[rss-feed] Fetching ${url}`);
    let xml: string;
    try {
      xml = await ofetch(url, {
        headers: {
          ...this.defaultHeaders,
          Accept: 'application/rss+xml,application/atom+xml,application/xml;q=0.9,*/*;q=0.8',
        },
        responseType: 'text',
        // 部分源会屏蔽非浏览器 UA，使用默认 UA 已足够；失败时继续尝试
        retry: 1,
      });
    } catch (err: any) {
      console.error(`[rss-feed] Failed to fetch ${url}:`, err?.message || err);
      throw new Error(`无法获取订阅源: ${err?.message || String(err)}`);
    }

    console.log(`[rss-feed] Received ${xml.length} bytes from ${url}`);

    const $ = cheerio.load(xml, { xmlMode: true });
    const items: ScrapeResult[] = [];

    // 优先判断根元素类型（使用字符串包含，避免 cheerio 解析带命名空间的标签名选择器报错）
    const lowerXml = xml.slice(0, 500).toLowerCase();
    const rssRoot = lowerXml.includes('<rss') || lowerXml.includes('<channel');
    const atomRoot = lowerXml.includes('<feed');
    const rdfRoot = lowerXml.includes('<rdf:rdf') || lowerXml.includes('<rdf ');
    console.log(`[rss-feed] Detected ${rssRoot ? 'RSS' : atomRoot ? 'Atom' : rdfRoot ? 'RSS(RDF)' : 'Unknown'} feed`);

    // RSS 2.0 / RDF 解析增强
    if (rssRoot || rdfRoot || !atomRoot) {
      $('item').each((_, el) => {
        const $el = $(el);

        // 标题：优先 title，兼容 CDATA
        const title = this.cleanText($el.find('title').first().text());

        // 链接：多种来源回退
        let link = this.cleanText($el.find('link').first().text());
        if (!link) {
          const linkHref = $el.find('link[href]').first().attr('href');
          link = linkHref ? this.cleanText(linkHref) : '';
        }
        if (!link) {
          const guid = $el.find('guid').first();
          const isPermaLink = (guid.attr('ispermalink') || guid.attr('isPermaLink') || 'true').toLowerCase();
          const guidText = this.cleanText(guid.text());
          if (guidText && isPermaLink === 'true' && /^https?:\/\//.test(guidText)) {
            link = guidText;
          }
        }

        // 描述：优先 content:encoded（HTML 全文），其次 description
        let desc =
          this.cleanText($el.find('content\\:encoded').first().text()) ||
          this.cleanText($el.find('description').first().text());
        desc = this.stripCdata(desc);

        // 作者：兼容 author / dc:creator / dc:author
        const author =
          this.cleanText($el.find('dc\\:creator').first().text()) ||
          this.cleanText($el.find('dc\\:author').first().text()) ||
          this.cleanText($el.find('author').first().text());

        // 日期：兼容 pubDate / dc:date
        const dateStr =
          this.cleanText($el.find('dc\\:date').first().text()) ||
          this.cleanText($el.find('pubDate').first().text());
        const pubDate = this.parseDate(dateStr);

        const resolvedLink = this.resolveUrl(link, url);
        const item = this.makeItem({ title, link: resolvedLink, desc, author, pubDate });
        if (item) items.push(item);
      });
    }

    // Atom 解析增强
    if (atomRoot && items.length === 0) {
      $('entry').each((_, el) => {
        const $el = $(el);

        const title = this.cleanText($el.find('title').first().text());

        // Atom 链接通常在 <link rel="alternate" href="..."/>
        let link = '';
        const alternateLink = $el.find('link[rel="alternate"]').first().attr('href');
        if (alternateLink) {
          link = this.cleanText(alternateLink);
        } else {
          const anyLink = $el.find('link').first().attr('href');
          link = anyLink ? this.cleanText(anyLink) : '';
        }
        if (!link) {
          link = this.cleanText($el.find('id').first().text());
        }

        // 内容：优先 content（HTML 或 text），其次 summary
        let desc =
          this.cleanText($el.find('content').first().text()) ||
          this.cleanText($el.find('summary').first().text());
        desc = this.stripCdata(desc);

        const author = this.cleanText($el.find('author > name').first().text());

        const pubDate = this.parseDate(
          this.cleanText($el.find('published').first().text()) ||
            this.cleanText($el.find('updated').first().text())
        );

        const resolvedLink = this.resolveUrl(link, url);
        const item = this.makeItem({ title, link: resolvedLink, desc, author, pubDate });
        if (item) items.push(item);
      });
    }

    console.log(`[rss-feed] Parsed ${items.length} items from ${url}`);
    if (items.length === 0) {
      console.warn(`[rss-feed] No items parsed from ${url}; root snapshot:`, {
        rss: rssRoot,
        atom: atomRoot,
        rdf: rdfRoot,
        firstChars: xml.slice(0, 200).replace(/\s+/g, ' '),
      });
    }

    return items;
  }

  /**
   * 去除 CDATA 包裹
   */
  private stripCdata(str: string): string {
    if (!str) return str;
    return str.replace(/<!\[CDATA\[(.*?)\]\]>/gs, '$1').trim();
  }

  /**
   * 清理文本：去除多余空白与 HTML 注释
   */
  private cleanText(str: string): string {
    if (!str) return '';
    return str.replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ').trim();
  }

}

const fetcher = new RssFeedFetcher();

const definition: FetcherDefinition = {
  id: 'rss-feed',
  name: 'RSS/Atom 订阅源',
  description: '解析标准 RSS 2.0 或 Atom 格式的订阅源，适合导入现有 RSS 订阅',
  sitePattern: '*.xml,*rss*,*atom*',
  params: [
    { key: 'url', label: '订阅源 URL', type: 'url', required: true, placeholder: 'https://example.com/feed.xml' },
  ],
  fetch: (url, params) => fetcher.fetch(url, params),
};

export default definition;
