import sqlite3
c = sqlite3.connect('prisma/dev.db')
c.execute('DELETE FROM Item WHERE sourceId IN (SELECT id FROM Source WHERE folderId IS NOT NULL)')
c.execute('DELETE FROM Source WHERE folderId IS NOT NULL')
c.execute('DELETE FROM Folder')
c.commit()
c.close()
