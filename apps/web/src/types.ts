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
  /** 非空表示该源来自远端同步服务端（如 miniflux） */
  remoteOrigin: string;
  remoteId: string;
  createdAt: string;
  updatedAt: string;
  unreadCount: number;
  lastFetchAt: string | null;
  lastSuccessAt: string | null;
  fetchFailCount: number;
  lastError: string | null;
}

/** 服务端同步配置（敏感字段脱敏返回） */
export interface SyncConfig {
  apiEnabled: boolean;
  apiProvider: string;
  apiEndpoint: string;
  apiUsername: string;
  apiPassword: string;
  apiToken: string;
  fileEnabled: boolean;
  fileProvider: string;
  fileUrl: string;
  fileUsername: string;
  filePassword: string;
  filePrefix: string;
  windowDays: number;
  pullInterval: number;
}

/** 同步健康状态 */
export interface SyncStatus {
  enabled: boolean;
  provider: string;
  syncing: boolean;
  lastPullAt: string;
  lastResult: string;
  lastError: string;
}

/** 一次手动同步的结果 */
export interface SyncResult {
  feedsImported: number;
  feedsMerged: number;
  itemsUpserted: number;
  localOnlyUrls: string[];
}

/** 对齐向导执行结果 */
export interface SyncAlignResult {
  pushed: number;
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
