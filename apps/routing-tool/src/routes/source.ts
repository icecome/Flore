import { Hono } from 'hono';
import { createSource, getAllSources, getSource, updateSource, deleteSource, fetchSource } from '../services/source.js';

const sourceRouter = new Hono();

// 获取所有订阅源
sourceRouter.get('/', async (c) => {
  const sources = await getAllSources();
  return c.json(sources);
});

// 获取单个订阅源
sourceRouter.get('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  const source = await getSource(id);
  if (!source) return c.json({ error: 'Not found' }, 404);
  return c.json(source);
});

// 创建订阅源
sourceRouter.post('/', async (c) => {
  const body = await c.req.json();
  const source = await createSource(body);
  return c.json(source, 201);
});

// 更新订阅源
sourceRouter.put('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  const body = await c.req.json();
  const source = await updateSource(id, body);
  return c.json(source);
});

// 删除订阅源
sourceRouter.delete('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  await deleteSource(id);
  return c.json({ ok: true });
});

// 手动触发抓取
sourceRouter.post('/:id/fetch', async (c) => {
  const id = Number(c.req.param('id'));
  try {
    const result = await fetchSource(id);
    return c.json(result);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Fetch failed';
    return c.json({ error: msg }, 500);
  }
});

export default sourceRouter;