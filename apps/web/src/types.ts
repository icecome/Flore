export interface Folder {
  id: number;
  name: string;
  parentId: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface Source {
  id: number;
  name: string;
  url: string;
  folderId: number | null;
  listRule: string;
  interval: number;
  active: boolean;
  isPrivate: boolean;
  hideInTimeline: boolean;
  createdAt: string;
  updatedAt: string;
  unreadCount: number;
  lastFetchAt: string | null;
  lastSuccessAt: string | null;
  fetchFailCount: number;
  lastError: string | null;
}

export interface Item {
  id: number;
  sourceId: number;
  title: string;
  link: string;
  desc: string | null;
  author: string | null;
  pubDate: string | null;
  isRead: boolean;
  isStarred: boolean;
  isReadLater: boolean;
  createdAt: string;
  sourceName: string;
  sourceUrl: string;
}

export interface FilterCondition {
  field: 'title' | 'desc' | 'author' | 'link';
  operator: 'contains' | 'notContains' | 'equals' | 'notEquals';
  value: string;
}

export interface FilterRule {
  id: number;
  name: string;
  enabled: boolean;
  priority: number;
  scope: 'global' | 'source' | 'folder';
  sourceId: number | null;
  folderId: number | null;
  conditions: FilterCondition[];
  action: 'markRead' | 'star' | 'readLater';
  createdAt: string;
}
