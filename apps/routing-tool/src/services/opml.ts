import * as cheerio from 'cheerio';
import type { AnyNode } from 'domhandler';
import prisma from '../utils/prisma.js';
import { createSource } from './source.js';

interface OPMLOutline {
  text: string;
  title?: string;
  xmlUrl?: string;
  htmlUrl?: string;
  type?: string;
  children: OPMLOutline[];
}

function parseOutlines($: cheerio.CheerioAPI, $parent: cheerio.Cheerio<AnyNode>): OPMLOutline[] {
  const result: OPMLOutline[] = [];
  $parent.children('outline').each((_index: number, el: AnyNode) => {
    const $el = $(el);
    const text = $el.attr('text') || $el.attr('title') || '';
    const title = $el.attr('title') || text;
    const xmlUrl = $el.attr('xmlUrl') || '';
    const htmlUrl = $el.attr('htmlUrl') || '';
    const type = $el.attr('type') || '';
    const children = parseOutlines($, $el);
    result.push({ text, title, xmlUrl, htmlUrl, type, children });
  });
  return result;
}

function isFeed(outline: OPMLOutline): boolean {
  return Boolean(outline.xmlUrl || outline.type === 'rss');
}

async function importOutlines(outlines: OPMLOutline[], parentFolderId: number | null) {
  for (const outline of outlines) {
    if (isFeed(outline)) {
      // 叶子节点：创建 Source，使用 rss-feed 路由解析标准 RSS/Atom
      await createSource({
        name: outline.text || outline.title || '未命名订阅',
        url: outline.xmlUrl || outline.htmlUrl || '',
        folderId: parentFolderId,
        routeId: 'rss-feed',
        routeParams: '{}',
        listRule: '',
      });
    } else if (outline.children.length > 0) {
      // 文件夹节点：创建 Folder，再递归导入子节点
      const folder = await prisma.folder.create({
        data: { name: outline.text || outline.title || '未命名文件夹' },
      });
      await importOutlines(outline.children, folder.id);
    }
  }
}

export async function importOPML(xml: string) {
  const $ = cheerio.load(xml, { xmlMode: true });
  const body = $('body');
  if (body.length === 0) {
    throw new Error('OPML 格式错误：缺少 body 节点');
  }
  const outlines = parseOutlines($, body);
  await importOutlines(outlines, null);
  return { imported: outlines.length };
}

function buildOutlineXML(outline: OPMLOutline, indent: string): string {
  const attrs: string[] = [];
  if (outline.text) attrs.push(`text="${escapeXml(outline.text)}"`);
  if (outline.title && outline.title !== outline.text) attrs.push(`title="${escapeXml(outline.title)}"`);
  if (outline.type) attrs.push(`type="${escapeXml(outline.type)}"`);
  if (outline.xmlUrl) attrs.push(`xmlUrl="${escapeXml(outline.xmlUrl)}"`);
  if (outline.htmlUrl) attrs.push(`htmlUrl="${escapeXml(outline.htmlUrl)}"`);

  if (outline.children.length > 0) {
    const childrenXML = outline.children.map((child) => buildOutlineXML(child, indent + '  ')).join('\n');
    return `${indent}<outline ${attrs.join(' ')}>\n${childrenXML}\n${indent}</outline>`;
  }
  return `${indent}<outline ${attrs.join(' ')} />`;
}

function escapeXml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

export async function exportOPML() {
  const folders = await prisma.folder.findMany({ include: { sources: true } });
  const uncategorized = await prisma.source.findMany({ where: { folderId: null } });

  const outlines: OPMLOutline[] = [];

  for (const folder of folders) {
    const children: OPMLOutline[] = folder.sources.map((s) => ({
      text: s.name,
      title: s.name,
      xmlUrl: s.url,
      htmlUrl: s.url,
      type: 'rss',
      children: [],
    }));
    outlines.push({
      text: folder.name,
      title: folder.name,
      children,
    });
  }

  for (const source of uncategorized) {
    outlines.push({
      text: source.name,
      title: source.name,
      xmlUrl: source.url,
      htmlUrl: source.url,
      type: 'rss',
      children: [],
    });
  }

  const bodyXML = outlines.map((o) => buildOutlineXML(o, '    ')).join('\n');

  return `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>Flore Subscriptions</title>
    <dateCreated>${new Date().toISOString()}</dateCreated>
  </head>
  <body>
${bodyXML}
  </body>
</opml>`;
}
