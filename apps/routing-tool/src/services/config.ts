import prisma from '../utils/prisma.js';

const CONFIG_ID = 1;

export async function getConfig() {
  let config = await prisma.config.findUnique({ where: { id: CONFIG_ID } });
  if (!config) {
    config = await prisma.config.create({
      data: { id: CONFIG_ID, defaultInterval: 120 },
    });
  }
  return config;
}

export async function updateDefaultInterval(minutes: number) {
  const clamped = Math.max(5, Math.min(1440, minutes));
  await prisma.config.upsert({
    where: { id: CONFIG_ID },
    create: { id: CONFIG_ID, defaultInterval: clamped },
    update: { defaultInterval: clamped },
  });
  return clamped;
}
