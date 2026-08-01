import prisma from '../src/utils/prisma.js';

async function main() {
  await prisma.$executeRawUnsafe(`
    CREATE TABLE IF NOT EXISTS "Config" (
      "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
      "defaultInterval" INTEGER NOT NULL DEFAULT 120
    )
  `);

  const count = await prisma.$queryRawUnsafe<{ count: number }[]>(
    `SELECT COUNT(*) as count FROM "Config"`
  );

  if (Number(count[0].count) === 0) {
    await prisma.$executeRawUnsafe(`
      INSERT INTO "Config" ("id", "defaultInterval") VALUES (1, 120)
    `);
    console.log('Created default Config row with interval 120 minutes.');
  } else {
    console.log('Config table already exists.');
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
