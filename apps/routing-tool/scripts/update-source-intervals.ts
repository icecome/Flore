import prisma from '../src/utils/prisma.js';

async function main() {
  const interval = Number(process.argv[2]) || 120;

  const before = await prisma.source.findMany({
    select: { id: true, name: true, interval: true },
  });

  console.log(`Found ${before.length} sources. Updating interval to ${interval} minutes...`);

  const { count } = await prisma.source.updateMany({
    where: { active: true },
    data: { interval },
  });

  console.log(`Updated ${count} active sources to ${interval} minutes.`);

  const nonActiveCount = await prisma.source.count({ where: { active: false } });
  if (nonActiveCount > 0) {
    console.log(`Skipped ${nonActiveCount} inactive sources.`);
  }
}

main()
  .catch((err) => {
    console.error(err);
    process.exit(1);
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
