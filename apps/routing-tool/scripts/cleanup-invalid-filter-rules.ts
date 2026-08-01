import prisma from '../src/utils/prisma.js';

interface FilterCondition {
  field?: string;
  operator?: string;
  value?: string;
}

async function main() {
  const rules = await prisma.filterRule.findMany();
  const toDelete: number[] = [];

  for (const rule of rules) {
    let conditions: FilterCondition[] = [];
    try {
      conditions = JSON.parse(rule.conditions) as FilterCondition[];
    } catch {
      toDelete.push(rule.id);
      console.log(`Rule #${rule.id} has invalid JSON conditions, will delete.`);
      continue;
    }

    if (!Array.isArray(conditions) || conditions.length === 0) {
      toDelete.push(rule.id);
      console.log(`Rule #${rule.id} has empty conditions, will delete.`);
      continue;
    }

    if (conditions.some((c) => typeof c.value !== 'string' || c.value.trim() === '')) {
      toDelete.push(rule.id);
      console.log(`Rule #${rule.id} has empty condition value, will delete.`);
    }
  }

  if (toDelete.length === 0) {
    console.log('No invalid filter rules found.');
    return;
  }

  const { count } = await prisma.filterRule.deleteMany({
    where: { id: { in: toDelete } },
  });

  console.log(`Deleted ${count} invalid filter rule(s).`);
}

main()
  .catch((err) => {
    console.error(err);
    process.exit(1);
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
