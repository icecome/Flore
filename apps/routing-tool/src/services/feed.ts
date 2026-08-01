import RSS from 'rss';
import prisma from '../utils/prisma.js';
import { generateFeedPath } from './source.js';

export async function generateFeed(sourceId: number, baseUrl = '', feedPath = ''): Promise<string> {
  const source = await prisma.source.findUnique({ where: { id: sourceId } });
  if (!source) throw new Error('Source not found');

  const items = await prisma.item.findMany({
    where: { sourceId },
    orderBy: { pubDate: 'desc' },
    take: 100,
  });

  const feedUrl = feedPath ? `${baseUrl}/${feedPath}` : `${baseUrl}/feed/${generateFeedPath(source.url)}`;

  const feed = new RSS({
    title: source.name,
    description: `RSS feed for ${source.url}`,
    feed_url: feedUrl,
    site_url: source.url,
    language: 'zh-cn',
    ttl: source.interval,
  });

  for (const item of items) {
    feed.item({
      title: item.title,
      description: item.desc || '',
      url: item.link,
      author: item.author || undefined,
      date: item.pubDate || new Date(),
    });
  }

  return feed.xml();
}