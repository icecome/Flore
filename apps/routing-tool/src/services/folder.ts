import prisma from '../utils/prisma.js';

export interface CreateFolderInput {
  name: string;
}

export async function createFolder(input: CreateFolderInput) {
  return prisma.folder.create({ data: { name: input.name } });
}

export async function getAllFolders() {
  return prisma.folder.findMany({
    orderBy: { createdAt: 'asc' },
    include: { sources: true },
  });
}

export async function getFolder(id: number) {
  return prisma.folder.findUnique({
    where: { id },
    include: { sources: true },
  });
}

export async function updateFolder(id: number, input: Partial<CreateFolderInput>) {
  const data: Record<string, unknown> = {};
  if (input.name !== undefined) data.name = input.name;
  return prisma.folder.update({
    where: { id },
    data,
  });
}

export async function deleteFolder(id: number) {
  // 删除文件夹时，将其下订阅源的 folderId 置空
  await prisma.source.updateMany({
    where: { folderId: id },
    data: { folderId: null },
  });
  return prisma.folder.delete({ where: { id } });
}
