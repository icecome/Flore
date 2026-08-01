import { Hono } from 'hono';
import { generateFeed } from '../services/feed.js';
import { generateFeedPath, getAllSources } from '../services/source.js';

const feedRouter = new Hono();

// 生成 RSS/Atom 订阅（RSSHub 风格：/feed/chinawriter/404057）
feedRouter.get('/*', async (c) => {
  // c.req.path 在 feedRouter 内是完整路径如 /feed/chinawriter/404057
  const fullPath = c.req.path.replace(/^\/feed\/?/, '').replace(/\/+$/g, '');
  if (!fullPath) {
    return c.json({ error: 'Invalid path' }, 400);
  }

  try {
    const allSources = await getAllSources();
    let matchedSource = null;
    for (const s of allSources) {
      const expectedPath = generateFeedPath(s.url);
      if (expectedPath === fullPath) {
        matchedSource = s;
        break;
      }
    }

    if (!matchedSource) {
      return c.json({ error: 'Source not found', path: fullPath, sources: allSources.map(s => ({ name: s.name, url: s.url, expected: generateFeedPath(s.url) })) }, 404);
    }

    const baseUrl = `${c.req.url.match(/^https?:\/\/[^\/]+/)?.[0] || ''}`;
    const xml = await generateFeed(matchedSource.id, baseUrl, `feed/${fullPath}`);
    c.header('Content-Type', 'application/rss+xml; charset=utf-8');
    return c.body(xml);
  } catch (err) {
    console.error('[Feed] error:', err);
    return c.json({ error: err instanceof Error ? err.message : 'Not found' }, 500);
  }
});

export default feedRouter;