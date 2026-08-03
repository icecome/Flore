import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { BookOpen, Globe } from './icons';
import DOMPurify, { type Config as PurifyConfig } from 'dompurify';
import type { Item } from '../types';
import { cn } from '../lib/cn';
import { formatFull } from '../lib/format';
import { type AppSettings, resolveReaderTheme } from '../utils/settings';
import {
  openExternal,
  savePNGFile,
  getReadability,
  getImageProxyBase,
  getArticleProxyUrl,
  checkArticleFrameable,
} from '../utils/api';
import { showToast } from '../utils/toast';
import EmptyState from './EmptyState';
import Loading from './Loading';
import ReaderToolbar from './ReaderToolbar';
import ContextMenu from './ContextMenu';
import { useContextMenu } from '../hooks/useContextMenu';
import { buildReaderContentMenu } from '../utils/contextMenu';

interface Props {
  item: Item | null;
  loading: boolean;
  settings: AppSettings;
  onToggleRead: (id: number, read: boolean) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onBack?: () => void;
  onSettingsChange?: (settings: AppSettings) => void;
}

/**
 * DOMPurify 配置：显式声明允许的标签与属性，避免使用默认配置时
 * 把 style/class 等可能被用于 CSS 注入（如 exfil）的属性放行。
 *
 * - FORBID_ATTR: 显式禁止 style 与所有 on* 事件属性
 * - ALLOWED_ATTR: 通过白名单方式约束可用属性
 * - ALLOWED_TAGS: 通过白名单方式约束可用标签（涵盖常见阅读内容）
 */
const PURIFY_CONFIG: PurifyConfig = {
  USE_PROFILES: { html: true },
  FORBID_ATTR: ['style', 'class', 'id'],
  FORBID_TAGS: ['form', 'input', 'button', 'textarea', 'select', 'style', 'script', 'iframe', 'object', 'embed'],
  ALLOW_DATA_ATTR: false,
  // 允许 target=_blank 但强制 rel=noopener（由下方 afterSanitizeAttributes 钩子落实）
  ADD_ATTR: ['target', 'rel'],
};

// 强制所有外链带 target=_blank + rel="noopener noreferrer"，杜绝反向 tabnabbing。
// 钩子是全局的，模块加载时注册一次即可。
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.nodeName !== 'A' || !node.hasAttribute('href')) return;
  node.setAttribute('target', '_blank');
  node.setAttribute('rel', 'noopener noreferrer');
});

/** 提取文章全文内容，供 useEffect 和手动切换模式复用 */
async function fetchReadability(itemId: number, signal: AbortSignal): Promise<{ content: string; title: string }> {
  const data = await getReadability(itemId, signal);
  return {
    content: data.content || '<p>无法提取全文内容</p>',
    title: data.title || '',
  };
}

/** 把正文里的链接解析为绝对地址；无法解析时返回 null */
function resolveArticleHref(href: string, base?: string): string | null {
  try {
    return new URL(href, base || window.location.href).href;
  } catch {
    return null;
  }
}

/** 全文模式请求失败时的兜底文案 */
const READABILITY_ERROR_HTML = '<p>提取全文内容失败，请检查网络连接或尝试在浏览器中打开。</p>';

/** 重写 HTML 中的图片 URL 为后端代理地址，绕过 CDN 防盗链。
 *  覆盖 <img src>、<img srcset>、<picture>/<source srcset>，并将相对路径解析为绝对路径；
 *  同时在 DOMPurify 剥离 data-* 之前，把懒加载 data-src 等归一到标准 src。
 *  @param html 原始 HTML 内容
 *  @param articleUrl 文章原文 URL，作为图片代理的 referer 参数（用于防盗链）与相对路径 base
 */
function rewriteImageUrls(html: string, articleUrl?: string): string {
  const proxyBase = getImageProxyBase();

  function resolveSrc(src: string): string | null {
    if (src.startsWith('data:') || src.startsWith('blob:') || src.includes('/image-proxy')) return null;
    if (src.startsWith('http://') || src.startsWith('https://')) return src;
    if (src.startsWith('//')) return 'https:' + src;
    if (articleUrl) {
      try {
        return new URL(src, articleUrl).href;
      } catch {
        return null;
      }
    }
    return null;
  }

  function buildProxyUrl(absSrc: string): string {
    let url = `${proxyBase}?url=${encodeURIComponent(absSrc)}`;
    if (articleUrl) url += `&ref=${encodeURIComponent(articleUrl)}`;
    return url;
  }

  let result = html;

  // 1. 懒加载：data-src 等自定义属性归一到标准 src（在 DOMPurify 剥离 data-* 之前处理）。
  //    仅当标签缺少标准 src 时，才用懒加载地址兜底，避免覆盖已有的真实 src。
  //    支持自关闭标签 <img ... /> 和标准标签 <img ... >
  result = result.replace(
    /<img\s([^>]*?)(?:data-(?:src|original|lazy-src|original-src|lazy|load-src|loading-src|true-src|default-src))\s*=\s*("([^"]+)"|'([^']+)'|([^\s>]+))([^>]*?)\s*\/?>/gi,
    (match, before, _attr, dq, sq, unq, after) => {
      const lazy = dq ?? sq ?? unq;
      if (!lazy || /(?:\s)src\s*=/.test(match)) return match;
      const abs = resolveSrc(lazy) ?? lazy;
      return `<img ${before}src="${buildProxyUrl(abs)}"${after}>`;
    }
  );

  // 2. <source srcset>（picture 响应式图）
  //    支持自关闭标签
  result = result.replace(
    /<source\s([^>]*?)srcset\s*=\s*("([^"]+)"|'([^']+)')([^>]*?)\s*\/?>/gi,
    (match, _before, _q, dqSrcset, sqSrcset, _after) => {
      const srcset = dqSrcset ?? sqSrcset;
      const rewritten = rewriteSrcset(srcset, articleUrl);
      return match.replace(
        new RegExp(`srcset\\s*=\\s*("${escapeRegex(srcset)}"|'${escapeRegex(srcset)}')`, 'i'),
        `srcset="${rewritten}"`
      );
    }
  );

  // 3. <img src> 与 <img srcset>
  //    支持自关闭标签 <img ... /> 和标准标签 <img ... >
  result = result.replace(
    /<img\s([^>]*?)src\s*=\s*("([^"]+)"|'([^']+)'|([^\s>]+))([^>]*?)\s*\/?>/gi,
    (match, _before, _q, dqSrc, sqSrc, unqSrc, _after) => {
      const src = dqSrc ?? sqSrc ?? unqSrc;
      const absSrc = resolveSrc(src);
      if (!absSrc) return match;
      const proxyUrl = buildProxyUrl(absSrc);
      let newMatch = match.replace(
        new RegExp(`src\\s*=\\s*("${escapeRegex(src)}"|'${escapeRegex(src)}'|${escapeRegex(src)})`, 'i'),
        `src="${proxyUrl}"`
      );
      const srcsetMatch = newMatch.match(/srcset\s*=\s*("([^"]+)"|'([^']+)')/i);
      if (srcsetMatch) {
        const originalSrcset = srcsetMatch[2] ?? srcsetMatch[3];
        const rewritten = rewriteSrcset(originalSrcset, articleUrl);
        newMatch = newMatch.replace(
          new RegExp(`srcset\\s*=\\s*("${escapeRegex(originalSrcset)}"|'${escapeRegex(originalSrcset)}')`, 'i'),
          `srcset="${rewritten}"`
        );
      }
      return newMatch;
    }
  );

  return result;
}

/** 解析 srcset 属性值中的每个 URL，并将其替换为代理地址。
 *  srcset 格式： "image-320w.jpg 320w, image-640w.jpg 640w"
 *  每个条目由 URL + 可选宽度/像素密度描述符组成。
 */
function rewriteSrcset(srcset: string, articleUrl?: string): string {
  const proxyBase = getImageProxyBase();

  function resolveSrc(src: string): string | null {
    if (src.startsWith('data:') || src.startsWith('blob:') || src.includes('/image-proxy')) {
      return null;
    }
    if (src.startsWith('http://') || src.startsWith('https://')) {
      return src;
    }
    if (src.startsWith('//')) {
      return 'https:' + src;
    }
    if (articleUrl) {
      try {
        return new URL(src, articleUrl).href;
      } catch {
        return null;
      }
    }
    return null;
  }

  function buildProxyUrl(absSrc: string): string {
    let url = `${proxyBase}?url=${encodeURIComponent(absSrc)}`;
    if (articleUrl) {
      url += `&ref=${encodeURIComponent(articleUrl)}`;
    }
    return url;
  }

  return srcset
    .split(',')
    .map((entry) => {
      const trimmed = entry.trim();
      // 每个条目的格式：URL [描述符]（描述符可选，如 320w, 2x）
      const parts = trimmed.split(/\s+/);
      if (parts.length === 0) return entry;
      const src = parts[0];
      const absSrc = resolveSrc(src);
      if (!absSrc) return entry;
      const proxyUrl = buildProxyUrl(absSrc);
      // 保留描述符（如有）
      const descriptor = parts.length > 1 ? ' ' + parts.slice(1).join(' ') : '';
      return proxyUrl + descriptor;
    })
    .join(', ');
}

/** 重写 iframe 内容中的图片和 CSS url() 引用为后端代理地址。
 *  与 rewriteImageUrls 不同，此函数：
 *  1. 处理单引号、双引号、无引号三种 src 格式
 *  2. 将相对 URL 解析为绝对 URL
 *  3. 同时重写 <style> 标签内 CSS url() 中的图片引用
 *  4. 使用绝对 URL 避免 <base> 标签干扰
 *  5. 处理 <picture>/<source> 响应式图片和 srcset 属性
 *  @param html iframe 原始 HTML 内容
 *  @param articleUrl 文章原文 URL，用于解析相对路径和设置 Referer
 */
function rewriteIframeContent(html: string, articleUrl?: string): string {
  const proxyBase = getImageProxyBase();

  function resolveSrc(src: string): string | null {
    if (src.startsWith('data:') || src.startsWith('blob:') || src.includes('/image-proxy')) {
      return null;
    }
    if (src.startsWith('http://') || src.startsWith('https://')) {
      return src;
    }
    if (src.startsWith('//')) {
      return 'https:' + src;
    }
    if (articleUrl) {
      try {
        return new URL(src, articleUrl).href;
      } catch {
        return null;
      }
    }
    return null;
  }

  function buildProxyUrl(absSrc: string): string {
    let url = `${proxyBase}?url=${encodeURIComponent(absSrc)}`;
    if (articleUrl) {
      url += `&ref=${encodeURIComponent(articleUrl)}`;
    }
    return url;
  }

  let result = html.replace(
    /<source\s([^>]*?)srcset\s*=\s*("([^"]+)"|'([^']+)')([^>]*?)\s*\/?>/gi,
    (match, _before, _quoted, dqSrcset, sqSrcset, _after) => {
      const srcset = dqSrcset ?? sqSrcset;
      const rewritten = rewriteSrcset(srcset, articleUrl);
      return match.replace(
        new RegExp(`srcset\\s*=\\s*("${escapeRegex(srcset)}"|'${escapeRegex(srcset)}')`, 'i'),
        `srcset="${rewritten}"`
      );
    }
  );

  result = result.replace(
    /<img\s([^>]*?)src\s*=\s*("([^"]+)"|'([^']+)'|([^\s>]+))([^>]*?)\s*\/?>/gi,
    (match, _before, _quoted, dqSrc, sqSrc, unqSrc, _after) => {
      const src = dqSrc ?? sqSrc ?? unqSrc;
      const absSrc = resolveSrc(src);
      if (!absSrc) return match;
      const proxyUrl = buildProxyUrl(absSrc);
      let newMatch = match.replace(
        new RegExp(`src\\s*=\\s*("${escapeRegex(src)}"|'${escapeRegex(src)}'|${escapeRegex(src)})`, 'i'),
        `src="${proxyUrl}"`
      );
      const srcsetMatch = newMatch.match(/srcset\s*=\s*("([^"]+)"|'([^']+)')/i);
      if (srcsetMatch) {
        const originalSrcset = srcsetMatch[2] ?? srcsetMatch[3];
        const rewritten = rewriteSrcset(originalSrcset, articleUrl);
        newMatch = newMatch.replace(
          new RegExp(`srcset\\s*=\\s*("${escapeRegex(originalSrcset)}"|'${escapeRegex(originalSrcset)}')`, 'i'),
          `srcset="${rewritten}"`
        );
      }
      return newMatch;
    }
  );

  result = result.replace(
    /<style[^>]*>([\s\S]*?)<\/style>/gi,
    (match, cssContent: string) => {
      const rewritten = cssContent.replace(
        /url\(\s*["']?([^"'\s\)]+)["']?\s*\)/gi,
        (urlMatch, cssUrl: string) => {
          const absSrc = resolveSrc(cssUrl);
          if (!absSrc) return urlMatch;
          const proxyUrl = buildProxyUrl(absSrc);
          return `url("${proxyUrl}")`;
        }
      );
      return match.replace(cssContent, rewritten);
    }
  );

  result = result.replace(
    /style\s*=\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')/gi,
    (match, styleValue: string) => {
      const quote = styleValue[0];
      const content = styleValue.slice(1, -1);
      const rewritten = content.replace(
        /url\(\s*["']?([^"'\s\)]+)["']?\s*\)/gi,
        (urlMatch, cssUrl: string) => {
          const absSrc = resolveSrc(cssUrl);
          if (!absSrc) return urlMatch;
          const proxyUrl = buildProxyUrl(absSrc);
          return `url("${proxyUrl}")`;
        }
      );
      if (rewritten === content) return match;
      return `style=${quote}${rewritten}${quote}`;
    }
  );

  return result;
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export default function Reader({
  item,
  loading,
  settings,
  onToggleRead,
  onToggleStar,
  onToggleReadLater,
  onBack,
  onSettingsChange,
}: Props) {
  const [progress, setProgress] = useState(0);
  // 显示模式: 'rss' | 'readability' | 'iframe'
  const [displayMode, setDisplayMode] = useState<'rss' | 'readability' | 'iframe'>('rss');
  const { menuProps, showMenu } = useContextMenu();
  const [readabilityContent, setReadabilityContent] = useState<string | null>(null);
  const [readabilityTitle, setReadabilityTitle] = useState<string>('');
  const [loadingReadability, setLoadingReadability] = useState(false);
  const [loadingIframe, setLoadingIframe] = useState(false);
  const [iframeSrc, setIframeSrc] = useState<string>('');
  // 是否走后端最小代理（原文设了 X-Frame-Options/CSP frame-ancestors，或混合内容）。true 时 sandbox 不含 allow-same-origin。
  const [iframeUsesProxy, setIframeUsesProxy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const editable = Boolean(onSettingsChange);

  // 跟踪当前 readability fetch 的 AbortController，便于切换文章或模式时取消
  const readabilityAbortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    setProgress(0);
    setReadabilityContent(null);
    setReadabilityTitle('');

    if (!item) return;
    if (settings.openArticleMode === 'readability') {
      setDisplayMode('readability');
      if (item.link) {
        // 取消上一次未完成的全文请求，避免快速切换文章时竞态覆盖
        readabilityAbortRef.current?.abort();
        const controller = new AbortController();
        readabilityAbortRef.current = controller;
        setLoadingReadability(true);
        (async () => {
          try {
            const { content, title } = await fetchReadability(item.id, controller.signal);
            setReadabilityContent(content);
            setReadabilityTitle(title);
          } catch (err) {
            if ((err as Error).name === 'AbortError') return;
            console.error('Failed to fetch full content:', err);
            setReadabilityContent(READABILITY_ERROR_HTML);
          } finally {
            // 仅当本次请求仍为最新时才清除 loading
            if (readabilityAbortRef.current === controller) {
              setLoadingReadability(false);
              readabilityAbortRef.current = null;
            }
          }
        })();
      }
    } else {
      setDisplayMode('rss');
    }
    return () => {
      readabilityAbortRef.current?.abort();
      readabilityAbortRef.current = null;
    };
  }, [item?.id, item?.link, settings.openArticleMode]);

  // 清理 rAF
  useEffect(() => {
    return () => {
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
    };
  }, []);

  // 进入阅读区时根据设置标记为已读
  useEffect(() => {
    if (!item) return;
    if (settings.markReadMode !== 'view') return;
    if (item.isRead) return;
    onToggleRead(item.id, true);
  }, [item?.id, item?.isRead, settings.markReadMode, onToggleRead]);

  const rafRef = useRef<number | null>(null);
  const onScroll = useCallback(() => {
    if (rafRef.current !== null) return;
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = null;
      const el = scrollRef.current;
      if (!el) return;
      const max = el.scrollHeight - el.clientHeight;
      setProgress(max <= 0 ? 0 : Math.min(100, (el.scrollTop / max) * 100));
    });
  }, []);

  const [effectiveAppTheme, setEffectiveAppTheme] = useState<'light' | 'dark'>(() =>
    document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light'
  );

  useEffect(() => {
    const observer = new MutationObserver(() => {
      const theme = document.documentElement.getAttribute('data-theme');
      setEffectiveAppTheme(theme === 'dark' ? 'dark' : 'light');
    });
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    return () => observer.disconnect();
  }, []);

  const resolvedReaderTheme = useMemo(
    () => resolveReaderTheme(settings.readerTheme, effectiveAppTheme),
    [settings.readerTheme, effectiveAppTheme]
  );

  const readerStyle: React.CSSProperties = useMemo(() => ({
    '--reader-font-family': settings.readerFontFamily === 'serif' ? 'var(--font-serif)' : settings.readerFontFamily === 'sans' ? 'var(--font-sans)' : settings.readerFontFamily === 'mono' ? 'var(--font-mono)' : 'var(--font-serif)',
    '--reader-font-size': `${settings.readerFontSize}px`,
    '--reader-line-height': String(settings.readerLineHeight),
    '--reader-max-width': `${settings.readerMaxWidth}px`,
    '--reader-letter-spacing': settings.readerLetterSpacing === 'wide' ? '0.05em' : 'normal',
    '--reader-bg': resolvedReaderTheme.bg,
    '--reader-color': resolvedReaderTheme.color,
    '--reader-muted': resolvedReaderTheme.muted,
    '--reader-link': resolvedReaderTheme.link,
    '--reader-border': resolvedReaderTheme.border,
    background: 'var(--reader-bg)',
    color: 'var(--reader-color)',
  } as React.CSSProperties), [settings.readerFontFamily, settings.readerFontSize, settings.readerLineHeight, settings.readerMaxWidth, settings.readerLetterSpacing, resolvedReaderTheme]);

  /** 导出当前文章为 PNG 图片 */
  const handleExportPNG = useCallback(async (): Promise<boolean> => {
    if (!item || !scrollRef.current) return false;
    const el = scrollRef.current.querySelector('article');
    if (!el) {
      showToast('没有可导出的内容');
      return false;
    }
    try {
      showToast('正在生成图片...');
      // 动态 import 避免 html2canvas 拖累初始包体积
      const { default: html2canvas } = await import('html2canvas');
      const canvas = await html2canvas(el, {
        useCORS: true,
        backgroundColor: '#ffffff',
        scale: 2,
      });
      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
      if (!blob) { showToast('导出失败'); return false; }
      const ok = await savePNGFile(blob, `${item.title.slice(0, 50)}.png`);
      if (ok) {
        showToast('PNG 已导出');
      }
      return ok;
    } catch {
      showToast('导出失败');
      return false;
    }
  }, [item]);

  /** 导出当前文章为 PDF（通过浏览器原生打印） */
  const handleExportPDF = useCallback(async (): Promise<boolean> => {
    if (!item || !scrollRef.current) return false;
    const articleEl = scrollRef.current.querySelector('article');
    if (!articleEl) {
      showToast('没有可导出的内容');
      return false;
    }
    try {
      showToast('正在生成 PDF...');
      // 创建一个临时的打印窗口，包含完整的文章样式
      const printWin = window.open('', '_blank');
      if (!printWin) {
        showToast('请允许弹出窗口');
        return false;
      }

      const escapedTitle = (item.title || '').replace(/[&<>"']/g, (c) =>
        ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] || c)
      );

      // 构造完整的打印页面，直接渲染文章HTML
      printWin.document.write(`
        <!DOCTYPE html>
        <html>
        <head>
          <meta charset="utf-8">
          <title>${escapedTitle}</title>
          <style>
            @page { margin: 15mm; }
            body { font-family: ${settings.readerFontFamily}; max-width: 720px; margin: 20px auto; padding: 20px; }
            img { max-width: 100%; height: auto; }
            .reader-content { line-height: 1.6; }
          </style>
        </head>
        <body>
          ${articleEl.innerHTML}
        </body>
        </html>
      `);
      printWin.document.close();
      printWin.focus();
      // 等待页面加载后触发打印
      setTimeout(() => {
        if (!printWin.closed) {
          printWin.print();
        }
      }, 100);
      showToast('PDF打印窗口已打开，请选择"另存为PDF"');
      return true;
    } catch (err) {
      console.error('Export PDF failed:', err);
      showToast('导出失败');
      return false;
    }
  }, [item, settings]);

  /** 切换全文模式（摘要 / 全文） */
  const onToggleReaderMode = useCallback(async () => {
    if (displayMode === 'readability') {
      setDisplayMode('rss');
      return;
    }
    setDisplayMode('readability');
    if (readabilityContent || !item?.link) return;
    // 取消上一次未完成的全文请求
    readabilityAbortRef.current?.abort();
    const controller = new AbortController();
    readabilityAbortRef.current = controller;
    setLoadingReadability(true);
    try {
      const { content, title } = await fetchReadability(item.id, controller.signal);
      setReadabilityContent(content);
      setReadabilityTitle(title);
    } catch (err) {
      if ((err as Error).name === 'AbortError') return;
      console.error('Failed to fetch full content:', err);
      showToast('提取全文失败');
      setReadabilityContent(READABILITY_ERROR_HTML);
    } finally {
      if (readabilityAbortRef.current === controller) {
        setLoadingReadability(false);
        readabilityAbortRef.current = null;
      }
    }
  }, [displayMode, readabilityContent, item?.id, item?.link]);

  /** 切换网页模式（摘要 / 网页） */
  // 注意：此处的 displayMode 是闭包旧值，但条件判断恰好符合预期：
  // 旧值非 iframe → 切换后将是 iframe → 需要加载 iframe 内容
  const onToggleViewOriginal = useCallback(async () => {
    setDisplayMode((prev) => (prev === 'iframe' ? 'rss' : 'iframe'));
    if (displayMode === 'iframe' || !item?.link) return;
    // 预检原文是否可被 iframe 直接嵌入，决定直连还是走最小代理：
    // - 可嵌入 → 直连 item.link（CSS/JS/图片全由原站加载，最稳；sandbox 含 allow-same-origin）
    // - 不可嵌入（XFO/CSP）或混合内容 → 走 /proxy/:id 最小代理（清头 + 注入 base；sandbox 去 allow-same-origin）
    setLoadingIframe(true);
    try {
      const data = await checkArticleFrameable(item.id);
      const mixed = typeof data.url === 'string' && data.url.startsWith('http://') && window.location.protocol === 'https:';
      if (data.frameable && !mixed) {
        setIframeUsesProxy(false);
        setIframeSrc(data.url || item.link);
        return;
      }
      setIframeUsesProxy(true);
      setIframeSrc(getArticleProxyUrl(item.id));
    } catch (err) {
      // 预检失败 → 保守走代理
      console.error('Frameable precheck failed:', err);
      showToast('原文嵌入预检失败，已改用代理模式');
      setIframeUsesProxy(true);
      setIframeSrc(getArticleProxyUrl(item.id));
    }
  }, [displayMode, item?.id, item?.link]);

  /** 拦截正文内链接点击：交给系统浏览器打开，避免桌面壳整窗跳外站后无法返回 */
  const handleContentClick = useCallback((e: React.MouseEvent) => {
    const anchor = (e.target as HTMLElement | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
    if (!anchor) return;
    const href = anchor.getAttribute('href') ?? '';
    // 页内锚点交给浏览器默认行为
    if (!href || href.startsWith('#')) return;
    e.preventDefault();
    const resolved = resolveArticleHref(href, item?.link);
    if (!resolved) {
      showToast('无效的链接');
      return;
    }
    openExternal(resolved);
  }, [item?.link]);

  const handleContentContextMenu = useCallback((e: React.MouseEvent) => {
    if (displayMode === 'iframe' || !item) return;
    showMenu(e, buildReaderContentMenu(item, displayMode, !!window.getSelection()?.toString(), {
      onToggleRead,
      onToggleStar,
      onToggleReadLater,
      onToggleReaderMode,
      onToggleViewOriginal,
      openExternal,
    }));
  }, [displayMode, item, showMenu, onToggleRead, onToggleStar, onToggleReadLater, onToggleReaderMode, onToggleViewOriginal]);

  if (loading) {
    return (
      <div className="relative flex h-full flex-1 flex-col bg-canvas">
        <Loading text="正在加载文章..." fullHeight />
      </div>
    );
  }

  if (!item) {
    return (
      <div className="flex h-full flex-1 items-center justify-center bg-canvas">
        <EmptyState
          icon={<BookOpen size={30} />}
          title="选择一篇文章开始阅读"
          description="从左侧列表挑选一篇文章，或按 J / K 上下浏览。"
        />
      </div>
    );
  }

  return (
    <div className="relative flex h-full flex-1 flex-col bg-canvas">
      {/* progress bar */}
      <div className="absolute inset-x-0 top-0 z-20 h-[2px] bg-transparent">
        <div className="h-full bg-primary transition-[width] duration-150" style={{ width: `${progress}%` }} />
      </div>

      {/* Toolbar — 跟随阅读主题背景 */}
      <ReaderToolbar
        item={item}
        settings={settings}
        editable={editable}
        resolvedReaderTheme={resolvedReaderTheme}
        displayMode={displayMode}
        onBack={onBack}
        onToggleRead={onToggleRead}
        onToggleStar={onToggleStar}
        onToggleReadLater={onToggleReadLater}
        onToggleReaderMode={onToggleReaderMode}
        onToggleViewOriginal={onToggleViewOriginal}
        onOpenExternal={openExternal}
        onSettingsChange={onSettingsChange}
        onExportPNG={handleExportPNG}
        onExportPDF={handleExportPDF}
      />

      {/* Content */}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        onClick={handleContentClick}
        onContextMenu={handleContentContextMenu}
        className="flex-1 overflow-y-auto animate-reader-fade-in"
        style={readerStyle}
      >
        {displayMode === 'readability' ? (
          /* 全文模式：从后端提取的全文内容 */
          loadingReadability ? (
            <Loading text="正在提取全文内容..." fullHeight />
          ) : (
            <article
              className="mx-auto w-full px-6 py-12 md:px-10"
              style={{ maxWidth: 'var(--reader-max-width, 720px)' }}
            >
              {readabilityTitle && (
                <h1
                  className="text-balance text-[32px] font-bold leading-tight tracking-tight"
                  style={{ color: 'var(--reader-color, var(--text-primary))', fontFamily: 'var(--font-sans)' }}
                >
                  {readabilityTitle}
                </h1>
              )}
              {readabilityTitle && <div className="my-8 h-px bg-border-subtle" />}
              <div
                className="reader-content"
                dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(rewriteImageUrls(readabilityContent || '', item.link), PURIFY_CONFIG) }}
              />
              <div className="mt-8 flex items-center justify-between border-t border-border-subtle pt-6">
                <span className="text-[13px] text-muted">全文模式 · 内容由算法提取，可能与原文存在差异</span>
                <button
                  onClick={() => { if (item.link) openExternal(item.link); }}
                  className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-[13px] text-secondary transition-colors hover:border-border-strong hover:text-primary"
                >
                  <Globe size={14} />
                  在浏览器打开
                </button>
              </div>
            </article>
          )
        ) : displayMode === 'iframe' && item.link ? (
          /* 网页模式：src 由预检结果决定（直连原文 or 走最小代理）。
             直连时为跨 origin 加载，sandbox 含 allow-same-origin（安全）；
             代理时为同源加载，sandbox 去掉 allow-same-origin，使代理页 JS 运行在 opaque origin，无法访问父窗口。 */
          <div className="relative h-full w-full">
            {loadingIframe && <Loading text="正在加载网页..." fullHeight />}
            {iframeSrc && (
              <iframe
                src={iframeSrc}
                sandbox={iframeUsesProxy
                  ? 'allow-scripts allow-popups allow-forms'
                  : 'allow-same-origin allow-scripts allow-popups allow-forms'}
                className={cn('h-full w-full border-0', loadingIframe ? 'hidden' : '')}
                title="网页"
                onLoad={() => setLoadingIframe(false)}
              />
            )}
          </div>
        ) : (
          <article
            className="mx-auto w-full px-6 py-12 md:px-10"
            style={{ maxWidth: 'var(--reader-max-width, 720px)' }}
          >
            {/* meta */}
            <div className="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-[13px] text-muted">
              <span className="font-medium" style={{ color: 'var(--reader-color, var(--text-primary))' }}>
                {item.sourceName}
              </span>
              {item.author && (
                <>
                  <span>·</span>
                  <span>{item.author}</span>
                </>
              )}
              <span>·</span>
              <span>{item.pubDate ? formatFull(item.pubDate) : ''}</span>
            </div>

            <h1
              className="text-balance text-[32px] font-bold leading-tight tracking-tight"
              style={{ color: 'var(--reader-color, var(--text-primary))', fontFamily: 'var(--font-sans)' }}
            >
              {item.title}
            </h1>

            <div className="my-8 h-px bg-border-subtle" />

            <div
              className="reader-content"
              dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(rewriteImageUrls(item.desc || '', item.link), PURIFY_CONFIG) }}
            />

            <div className="mt-14 flex items-center justify-between border-t border-border-subtle pt-6">
              <span className="text-[13px] text-muted">来自 {item.sourceName}</span>
              <button
                onClick={() => {
                  if (item.link) openExternal(item.link);
                }}
                className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-[13px] text-secondary transition-colors hover:border-border-strong hover:text-primary"
                style={{ color: 'var(--reader-color, var(--text-secondary))' }}
              >
                <Globe size={14} />
                在浏览器打开
              </button>
            </div>
          </article>
        )}
      </div>
      {menuProps && <ContextMenu {...menuProps} />}
    </div>
  );
}

