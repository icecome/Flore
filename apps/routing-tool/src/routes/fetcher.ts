import { Hono } from 'hono';
import { listFetchers } from '../scrapers/registry.js';

const fetcherRouter = new Hono();

// 获取所有可用抓取路由
fetcherRouter.get('/', async (c) => {
  const fetchers = listFetchers();
  return c.json(fetchers);
});

export default fetcherRouter;