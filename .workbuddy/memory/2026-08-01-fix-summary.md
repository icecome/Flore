# Flore RSS Reader 修复总结

## 已完成修复

### 1. 计数器统计问题 ✅
**问题**: 侧边栏"全部文章"显示未读计数，切换到"未读文章"时"全部文章"仍显示未读计数

**修复内容**:
- `useItemsData.ts` - `fetchItemCount` 使用独立参数构建，不包含 filter 参数
- `useItemsData.ts` - `useEffect` 依赖数组包含 `filter`，切换视图时正确刷新计数
- `Sidebar.tsx` - 新增 `totalCount` 和 `unreadCountInScope` props
- `App.tsx` - 正确传递计数 props 给 Sidebar 和 ArticleList
- `ArticleList.tsx` - 未读视图显示 `globalUnreadCount`，其他视图显示 `totalCount`

**验证**: TypeScript 编译通过，Vite 构建成功

### 2. 自动加载功能 ✅
**状态**: 已正常工作

- `useItemsData.ts` - `loadMore` 函数，检测 `hasMore` 防止重复加载
- `ArticleList.tsx` - scroll 事件监听，触底时（距离底部 < 240px）调用 `onLoadMore`
- `App.tsx` - 已传递 `hasMore`、`loadingMore`、`onLoadMore` props

### 3. 窗口最大化按钮修复 ✅
**问题**: 软件启动时窗口最大化，但按钮显示为"非最大化状态"

**修复内容**:
- `TitleBar.tsx` - 挂载时调用 `app.WindowIsMaximised()` 检查初始状态
- `TitleBar.tsx` - 状态变化时调用 `app.SaveWindowState()` 保存到 localStorage
- `app.go` - 新增 `SaveWindowState()` 和 `LoadWindowState()` 方法
- `main.go` - 启动时读取上次保存的状态，正确设置 `WindowStartState`

**验证**: TypeScript 编译通过，Go 编译成功

### 4. Go 编译错误修复 ✅
- 删除 `backup.go` 中未使用的 `encoding/json` 和 `models` 导入

## 构建状态

| 组件 | 状态 |
|------|------|
| TypeScript 编译 | ✅ 通过 |
| Go 编译 | ✅ 成功 |
| Web 构建 | ✅ 成功 |
| 前端资源 | ✅ 已复制到 desktop/frontend/dist |

## 修改文件清单

1. `apps/web/src/hooks/useItemsData.ts` - 修复计数逻辑
2. `apps/web/src/components/Sidebar.tsx` - 新增计数 props
3. `apps/web/src/components/ArticleList.tsx` - 调整计数显示
4. `apps/web/src/App.tsx` - 传递计数 props
5. `apps/web/src/components/TitleBar.tsx` - 窗口状态检查和持久化
6. `apps/web/src/utils/api.ts` - 添加 `SaveWindowState` 类型
7. `apps/desktop/app.go` - 新增窗口状态持久化方法
8. `apps/desktop/main.go` - 启动时读取窗口状态
9. `server/go/internal/services/backup.go` - 删除未使用导入

## 使用说明

1. 重新运行应用即可体验修复效果
2. 窗口状态会自动记忆：上次关闭前如果是最大化状态，下次启动会自动最大化
3. 切换"全部文章"和"未读文章"视图时，计数会正确更新
