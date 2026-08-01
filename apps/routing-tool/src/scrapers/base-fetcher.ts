import { ofetch } from 'ofetch';
import * as cheerio from 'cheerio';
import type { AnyNode } from 'domhandler';

// 统一 re-export，供子类直接使用，避免各站点文件重复导入第三方依赖
export { ofetch } from 'ofetch';
export { cheerio };
export type { AnyNode } from 'domhandler';

export interface ScrapeResult {
  title: string;
  link: string;
  desc?: string;
  author?: string;
  pubDate?: Date;
}

/**
 * 基类：提供通用的抓取工具方法
 *
 * 子类只需实现 fetch() 方法，使用 this.fetchHtml() 等工具。
 */
export abstract class BaseFetcher {
  /**
   * HTTP 请求默认头
   */
  protected defaultHeaders: Record<string, string> = {
    'User-Agent':
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    Accept: 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
    'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
  };

  /**
   * 获取 HTML 并加载为 cheerio 实例
   */
  protected async fetchHtml(url: string, headers?: Record<string, string>): Promise<cheerio.CheerioAPI> {
    const html = await ofetch(url, {
      headers: { ...this.defaultHeaders, ...headers },
    });
    return cheerio.load(html);
  }

  /**
   * 获取 JSON 数据
   */
  protected async fetchJson<T>(url: string, headers?: Record<string, string>): Promise<T> {
    return ofetch(url, {
      headers: { ...this.defaultHeaders, ...headers },
      parseResponse: JSON.parse,
    });
  }

  /**
   * 解析相对链接为绝对链接
   */
  protected resolveUrl(href: string, baseUrl: string): string {
    if (!href) return '';
    if (href.startsWith('http://') || href.startsWith('https://')) return href;
    try {
      return new URL(href, baseUrl).href;
    } catch {
      return '';
    }
  }

  /**
   * 尝试解析日期字符串
   * 支持 ISO 8601、YYYY-MM-DD、datetime 属性等多种格式
   */
  protected parseDate(str: string): Date | undefined {
    if (!str) return undefined;

    // 尝试直接用 Date.parse
    const ts = Date.parse(str);
    if (!isNaN(ts)) return new Date(ts);

    // 尝试匹配 YYYY-MM-DD HH:mm:ss
    const match = str.match(/(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:\s+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/);
    if (match) {
      const [, y, m, d, hh, mm, ss] = match;
      return new Date(Number(y), Number(m) - 1, Number(d), Number(hh || 0), Number(mm || 0), Number(ss || 0));
    }

    return undefined;
  }

  /**
   * 从元素中提取日期：优先取 datetime 属性，否则取文本
   */
  protected extractDate($el: cheerio.Cheerio<AnyNode>, selector: string): Date | undefined {
    const $found = selector ? $el.find(selector).first() : $el;
    const datetime = $found.attr('datetime');
    if (datetime) {
      const d = this.parseDate(datetime);
      if (d) return d;
    }
    const text = $found.text().trim();
    return this.parseDate(text);
  }

  /**
   * 创建标准化的 ScrapeResult
   */
  protected makeItem(params: {
    title: string;
    link: string;
    desc?: string;
    author?: string;
    pubDate?: Date;
  }): ScrapeResult | null {
    if (!params.title || !params.link) return null;
    return {
      title: params.title.trim(),
      link: params.link,
      desc: params.desc?.trim(),
      author: params.author?.trim(),
      pubDate: params.pubDate,
    };
  }
}