import type { FetcherDefinition } from '../registry.js';
import { BaseFetcher, type ScrapeResult } from '../base-fetcher.js';
import * as cheerio from 'cheerio';
import type { AnyNode } from 'domhandler';

/**
 * 中国作家网 - 新闻列表抓取器
 *
 * 目标网站: https://www.chinawriter.com.cn/
 * 频道页: https://www.chinawriter.com.cn/404057/
 */
class ChinawriterFetcher extends BaseFetcher {
  async fetch(url: string, _params: Record<string, string>): Promise<ScrapeResult[]> {
    const $ = await this.fetchHtml(url);
    const items: ScrapeResult[] = [];

    // 中国作家网新闻列表结构多样，先尝试常见的列表选择器
    // 注意：中国作家网频道页常见结构为 .wgwy_inner_l，内部包含 <span>序号</span><a>标题</a>
    const listSelectors = [
      '.wgwy_inner_l a[href]',
      'ul.news-list li',
      '.list-content li',
      '.news_item',
      '.news-list li',
      '.list li',
      '.content-list li',
      '.article-list li',
      'dl.zjyx_News',
    ];

    for (const selector of listSelectors) {
      $(selector).each((_, el) => {
        const $el = $(el);
        // 如果选择器本身已经是 a，则 $el 就是链接
        const $link = $el.is('a[href]') ? $el : $el.find('a[href]').first();
        const title = $link.text().trim() || $el.find('dt, h1, h2, h3, h4, h5, h6').first().text().trim();
        const href = $link.attr('href') || '';
        const link = this.resolveUrl(href, url);

        // 列表页可能存在的摘要
        const listDesc = $el.find('.desc, .summary, .intro, p').first().text().trim();

        // 列表页可能存在的日期
        const pubDate = this.extractDate($el, '.date, .time, .pub-date, .pubTime, span');

        // 列表页可能存在的作者
        const author = $el.find('.author, .editor, .source').first().text().trim() || undefined;

        const item = this.makeItem({ title, link, desc: listDesc, author, pubDate });
        if (item && !items.find((i) => i.link === item.link)) {
          items.push(item);
        }
      });

      if (items.length > 0) break;
    }

    // 如果常见选择器都没匹配到，回退到所有 li
    if (items.length === 0) {
      $('li').each((_, el) => {
        const $el = $(el);
        const $link = $el.find('a[href]').first();
        const title = $link.text().trim();
        const href = $link.attr('href') || '';
        const link = this.resolveUrl(href, url);

        if (title && link && title.length > 4 && !items.find((i) => i.link === link)) {
          const listDesc = $el.find('.desc, .summary, .intro, p').first().text().trim();
          const pubDate = this.extractDate($el, '.date, .time, .pub-date');
          const author = $el.find('.author, .editor').first().text().trim() || undefined;
          const item = this.makeItem({ title, link, desc: listDesc, author, pubDate });
          if (item) items.push(item);
        }
      });
    }

    // 去重并过滤掉非文章页链接（如频道首页、图片集等）
    const articleItems = items.filter((item) => {
      if (!item.link) return false;
      // 中国作家网文章链接通常包含 /n1/ 和 -数字.html
      return /\/n1\/\d{4}\/\d{4}\/c\d+-\d+\.html/.test(item.link);
    });

    // 进入详情页补充摘要、日期、作者
    const enrichedItems = await Promise.all(
      articleItems.map(async (item) => {
        if (!item.link) return item;
        try {
          const detail = await this.fetchDetail(item.link);
          return {
            ...item,
            desc: item.desc && item.desc.length > 30 ? item.desc : detail.desc,
            author: item.author || detail.author,
            pubDate: item.pubDate || detail.pubDate,
          };
        } catch {
          return item;
        }
      })
    );

    return enrichedItems.filter((item): item is ScrapeResult => Boolean(item.title && item.link));
  }

  /**
   * 抓取文章详情页，提取摘要、作者、日期
   *
   * 中国作家网详情页特点：
   * - <meta name="description"> 中通常有高质量摘要
   * - <meta name="publishdate" content="YYYY-MM-DD"> 提供发布日期
   * - 正文来源信息通常为 "来源：XXX　|　作者姓名　　<em>2026年07月06日08:02</em>"
   */
  private async fetchDetail(url: string): Promise<Partial<ScrapeResult>> {
    const $ = await this.fetchHtml(url);
    const result: Partial<ScrapeResult> = {};

    // 1. 优先从 meta description 获取摘要（质量最高）
    const metaDesc = $('meta[name="description"]').attr('content')?.trim();
    if (metaDesc && metaDesc.length > 10) {
      result.desc = this.truncateDesc(metaDesc);
    }

    // 2. 从 meta publishdate 获取日期
    const publishDate =
      $('meta[name="publishdate"]').attr('content') ||
      $('meta[name="publishDate"]').attr('content') ||
      $('meta[name="PublishDate"]').attr('content');
    if (publishDate) {
      const d = this.parseDate(publishDate.trim());
      if (d) result.pubDate = d;
    }

    // 3. 提取作者/来源：优先匹配 "来源：XXX | 作者" 或 "来源：XXX" 格式
    const sourceText = this.extractSourceInfo($);
    if (sourceText) {
      result.author = sourceText;
    }

    // 4. 如果 meta 没有摘要，从正文容器提取
    if (!result.desc) {
      const contentSelectors = [
        '.contentMain',
        '#contentMain',
        '.article-content',
        '.content-detail',
        '.detail-content',
        '.TRS_Editor',
        '.text',
        '.article',
        '.content',
        '#article-content',
        '.main-content',
        '.left-content',
        '.news-content',
      ];

      let $content: cheerio.Cheerio<AnyNode> | null = null;
      for (const selector of contentSelectors) {
        const $found = $(selector).first();
        if ($found.length > 0 && $found.text().trim().length > 50) {
          $content = $found;
          break;
        }
      }

      if ($content) {
        $content.find('script, style, nav, header, footer, aside').remove();
        const text = $content.text().replace(/\s+/g, ' ').trim();
        result.desc = this.truncateDesc(text);
      } else {
        // 兜底：取 body 中最大文本块
        let maxText = '';
        $('div, article, section').each((_, el) => {
          const text = $(el).text().trim();
          if (text.length > maxText.length && text.length < 50000) {
            maxText = text;
          }
        });
        if (maxText) {
          result.desc = this.truncateDesc(maxText);
        }
      }
    }

    // 5. 如果 meta 没有日期，再尝试页面中的日期元素
    if (!result.pubDate) {
      const dateSelectors = [
        '.date',
        '.time',
        '.pub-date',
        '.pubTime',
        '.publish-time',
        '[class*="date"]',
        '[class*="time"]',
      ];
      for (const selector of dateSelectors) {
        const $found = $(selector).first();
        const date = this.extractDate($found, '');
        if (date) {
          result.pubDate = date;
          break;
        }
      }
    }

    return result;
  }

  /**
   * 提取来源/作者信息
   * 中国作家网常见格式："来源：文汇报　|　赵进华　　<em>2026年07月06日08:02</em>"
   */
  private extractSourceInfo($: cheerio.CheerioAPI): string | undefined {
    const html = $.html();

    // 匹配 "来源：XXX | 作者" 或 "来源：XXX"
    const patterns = [
      /来源[：:]\s*([^|<\s][^|<]*?)\s*[|｜]\s*([^<\s]{2,20})/,
      /来源[：:]\s*([^|<\s][^|<]{1,30})/,
    ];

    for (const pattern of patterns) {
      const match = html.match(pattern);
      if (match) {
        // 第一组是来源，第二组（如有）是作者
        const source = this.cleanSourceText(match[1]);
        const author = match[2] ? this.cleanSourceText(match[2]) : '';
        if (author && author !== source) {
          return `${source} · ${author}`;
        }
        return source;
      }
    }

    return undefined;
  }

  /**
   * 清理来源/作者文本中的 HTML 实体和多余空白
   */
  private cleanSourceText(text: string): string {
    return text
      .replace(/&[a-zA-Z]+;/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  }

  /**
   * 截取前 300 字符作为摘要
   */
  private truncateDesc(text: string): string {
    const cleaned = text.replace(/\s+/g, ' ').trim();
    if (cleaned.length <= 300) return cleaned;
    return cleaned.slice(0, 300) + '...';
  }
}

const fetcher = new ChinawriterFetcher();

const definition: FetcherDefinition = {
  id: 'chinawriter',
  name: '中国作家网 - 新闻列表',
  description: '抓取中国作家网（www.chinawriter.com.cn）各频道的新闻列表，自动进入详情页提取摘要',
  sitePattern: 'chinawriter.com.cn',
  params: [
    {
      key: 'url',
      label: '频道页 URL',
      type: 'url',
      required: true,
      placeholder: 'https://www.chinawriter.com.cn/404057/',
    },
  ],
  fetch: (url, params) => fetcher.fetch(url, params),
};

export default definition;
