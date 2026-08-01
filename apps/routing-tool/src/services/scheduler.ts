import cron from 'node-cron';
import prisma from '../utils/prisma.js';
import { fetchSource } from './source.js';
import { getConfig } from './config.js';

export function startScheduler() {
  // 每分钟检查一次需要抓取的源
  cron.schedule('* * * * *', async () => {
    const config = await getConfig();
    const intervalMinutes = config.defaultInterval;

    const sources = await prisma.source.findMany({
      where: { active: true },
    });

    for (const source of sources) {
      const lastItem = await prisma.item.findFirst({
        where: { sourceId: source.id },
        orderBy: { createdAt: 'desc' },
      });

      const elapsed = lastItem
        ? (Date.now() - lastItem.createdAt.getTime()) / 1000 / 60
        : Infinity;

      if (elapsed >= intervalMinutes) {
        try {
          const result = await fetchSource(source.id);
          console.log(`[Scheduler] Fetched "${source.name}": ${result.added} new items`);
        } catch (err) {
          console.error(`[Scheduler] Failed to fetch "${source.name}":`, err);
        }
      }
    }
  });

  console.log('[Scheduler] Started');
}