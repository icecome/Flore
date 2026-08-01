-- RedefineTables
PRAGMA defer_foreign_keys=ON;
PRAGMA foreign_keys=OFF;
CREATE TABLE "new_Item" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "sourceId" INTEGER NOT NULL,
    "title" TEXT NOT NULL,
    "link" TEXT NOT NULL,
    "desc" TEXT,
    "author" TEXT,
    "pubDate" DATETIME,
    "isRead" BOOLEAN NOT NULL DEFAULT false,
    "isStarred" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "Item_sourceId_fkey" FOREIGN KEY ("sourceId") REFERENCES "Source" ("id") ON DELETE CASCADE ON UPDATE CASCADE
);
INSERT INTO "new_Item" ("author", "createdAt", "desc", "id", "link", "pubDate", "sourceId", "title") SELECT "author", "createdAt", "desc", "id", "link", "pubDate", "sourceId", "title" FROM "Item";
DROP TABLE "Item";
ALTER TABLE "new_Item" RENAME TO "Item";
CREATE UNIQUE INDEX "Item_link_key" ON "Item"("link");
CREATE INDEX "Item_sourceId_idx" ON "Item"("sourceId");
CREATE INDEX "Item_link_idx" ON "Item"("link");
CREATE INDEX "Item_isRead_idx" ON "Item"("isRead");
CREATE INDEX "Item_isStarred_idx" ON "Item"("isStarred");
PRAGMA foreign_keys=ON;
PRAGMA defer_foreign_keys=OFF;
