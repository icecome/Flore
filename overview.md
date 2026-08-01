# 计数器统计修复（最终版）

## 问题
恢复备份后，侧边栏计数显示不正确：
- "全部文章" 显示未读计数（而非全文总数）
- 切换到"未读文章"时，"全部文章"仍显示未读计数

## 根因

### 1. fetchItemCount 使用了包含 filter 的参数构建
```typescript
// 问题代码
const params = buildItemParams(); // 包含 filter 参数
params.delete('limit');
```

当 `filter === 'unread'` 时，`buildItemParams()` 会添加 `unread=true`，导致计数请求返回未读计数而非全文总数。

### 2. Sidebar 缺少正确的 props
- Sidebar 只有 `totalUnread`（来源未读数之和），没有 `totalCount` 和 `unreadCountInScope`
- "全部文章" 和 "未读文章" 都显示 `totalUnread`

## 修复方案

### useItemsData.ts - 独立构建 count 参数
```typescript
const fetchItemCount = useCallback(async (signal?: AbortSignal) => {
  const params = new URLSearchParams();
  if (selectedSourceId !== null) params.append('sourceId', ...);
  if (selectedFolderId !== null) params.append('folderId', ...);
  // 注意：不添加 filter 参数，确保始终获取全文总数
  if (settings.hidePrivateInTimeline && ...) params.append('hidePrivate', 'true');
  // ...
}, [selectedSourceId, selectedFolderId, settings.hidePrivateInTimeline, getApi, showToast]);
```

### Sidebar.tsx - 新增 props
```typescript
interface Props {
  // ... existing props ...
  totalCount?: number;           // 全文总数
  unreadCountInScope?: number;   // 当前范围未读数
}
```

### App.tsx - 传递 props
```tsx
<Sidebar
  totalCount={totalCount}
  unreadCountInScope={unreadCountInScope}
  // ...
/>
```

## 验证结果
- TypeScript 编译：✅ 通过
- 构建：✅ 代码层面成功
