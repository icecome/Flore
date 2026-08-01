import { Hono } from 'hono';
import { importOPML, exportOPML } from '../services/opml.js';

const opmlRouter = new Hono();

opmlRouter.post('/import', async (c) => {
  try {
    // Hono 的 c.req.text() 在某些 Content-Type 下会以 latin1 解码，
    // 导致中文乱码。这里手动按 UTF-8 解码请求体。
    const buffer = await c.req.arrayBuffer();
    const body = new TextDecoder('utf-8').decode(buffer);
    const result = await importOPML(body);
    return c.json(result);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Import failed';
    return c.json({ error: msg }, 400);
  }
});

opmlRouter.get('/export', async (c) => {
  try {
    const xml = await exportOPML();
    c.header('Content-Type', 'application/xml');
    c.header('Content-Disposition', 'attachment; filename="subscriptions.opml"');
    return c.body(xml);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Export failed';
    return c.json({ error: msg }, 500);
  }
});

export default opmlRouter;
