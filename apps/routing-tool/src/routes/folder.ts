import { Hono } from 'hono';
import { createFolder, getAllFolders, getFolder, updateFolder, deleteFolder } from '../services/folder.js';

const folderRouter = new Hono();

folderRouter.get('/', async (c) => {
  const folders = await getAllFolders();
  return c.json(folders);
});

folderRouter.get('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  const folder = await getFolder(id);
  if (!folder) return c.json({ error: 'Not found' }, 404);
  return c.json(folder);
});

folderRouter.post('/', async (c) => {
  const body = await c.req.json();
  const folder = await createFolder({ name: body.name });
  return c.json(folder, 201);
});

folderRouter.put('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  const body = await c.req.json();
  const folder = await updateFolder(id, { name: body.name });
  return c.json(folder);
});

folderRouter.delete('/:id', async (c) => {
  const id = Number(c.req.param('id'));
  await deleteFolder(id);
  return c.json({ ok: true });
});

export default folderRouter;
