-- CreateTable
CREATE TABLE "Source" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL,
    "url" TEXT NOT NULL,
    "listRule" TEXT NOT NULL,
    "titleRule" TEXT,
    "linkRule" TEXT,
    "descRule" TEXT,
    "dateRule" TEXT,
    "authorRule" TEXT,
    "interval" INTEGER NOT NULL DEFAULT 30,
    "active" BOOLEAN NOT NULL DEFAULT true,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL
);

-- CreateTable
CREATE TABLE "Item" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "sourceId" INTEGER NOT NULL,
    "title" TEXT NOT NULL,
    "link" TEXT NOT NULL,
    "desc" TEXT,
    "author" TEXT,
    "pubDate" DATETIME,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "Item_sourceId_fkey" FOREIGN KEY ("sourceId") REFERENCES "Source" ("id") ON DELETE CASCADE ON UPDATE CASCADE
);

-- CreateIndex
CREATE UNIQUE INDEX "Item_link_key" ON "Item"("link");

-- CreateIndex
CREATE INDEX "Item_sourceId_idx" ON "Item"("sourceId");

-- CreateIndex
CREATE INDEX "Item_link_idx" ON "Item"("link");
