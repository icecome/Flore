import { serve } from '@hono/node-server';
import { Hono } from 'hono';
import { cors } from 'hono/cors';
import sourceRouter from './routes/source.js';
import feedRouter from './routes/feed.js';
import fetcherRouter from './routes/fetcher.js';
import folderRouter from './routes/folder.js';
import opmlRouter from './routes/opml.js';
import configRouter from './routes/config.js';
import { startScheduler } from './services/scheduler.js';
import { initRegistry } from './scrapers/registry.js';

const app = new Hono();

// 中间件 - 允许油猴脚本跨域调用
app.use('*', cors({
  origin: '*',
  allowHeaders: ['Content-Type'],
  allowMethods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
}));

// API 路由
app.route('/sources', sourceRouter);
app.route('/feed', feedRouter);
app.route('/fetchers', fetcherRouter);
app.route('/folders', folderRouter);
app.route('/opml', opmlRouter);
app.route('/config', configRouter);

// 健康检查
app.get('/health', (c) => c.json({ status: 'ok' }));

// 启动项
const start = async () => {
  await initRegistry();
  startScheduler();
};

start();

const port = Number(process.env.PORT) || 3001;
serve({ fetch: app.fetch, port }, () => {
  console.log(`Server running at http://localhost:${port}`);
});

export default app;