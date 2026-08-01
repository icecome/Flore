import { Hono } from 'hono';
import { getConfig, updateDefaultInterval } from '../services/config.js';

const configRouter = new Hono();

configRouter.get('/', async (c) => {
  const config = await getConfig();
  return c.json(config);
});

configRouter.put('/', async (c) => {
  const body = await c.req.json();
  const minutes = Number(body.defaultInterval);
  if (!Number.isFinite(minutes)) {
    return c.json({ error: 'defaultInterval must be a number' }, 400);
  }
  const updated = await updateDefaultInterval(minutes);
  return c.json({ defaultInterval: updated });
});

export default configRouter;
