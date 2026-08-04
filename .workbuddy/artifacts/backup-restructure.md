# 数据管理备份恢复重构

## 目标
将原来分散的配置管理、数据库管理、备份管理三个区块合并为统一的"备份与恢复"流程，支持全量备份和选择性恢复。

## 核心改动

### 全量备份
新的备份 ZIP 包含三个文件：
- `database.db` — 数据库一致快照（VACUUM INTO 生成）
- `settings.json` — Settings 表全部配置项
- `subscriptions.opml` — 订阅源列表

### 恢复粒度
每个备份提供三种恢复方式：
| 图标 | 名称 | 作用 |
|------|------|------|
| ArchiveRestore | 全量恢复 | 替换数据库 + 配置 + 订阅 |
| Cog | 仅恢复配置 | 仅写入 Settings 表，不影响文章 |
| Rss | 仅恢复订阅源 | 仅导入 OPML，不影响数据库 |

按钮根据备份内容动态显示：有配置显示 Cog，有 OPML 显示 Rss。

### API 端点（新增）
- `GET /api/backups/:name/contents` — 读取备份 ZIP 内容清单
- `POST /api/backups/:name/restore-config` — 仅恢复配置
- `POST /api/backups/:name/restore-opml` — 仅恢复订阅源

### 前端区块顺序
1. 备份与恢复（原备份管理重构）
2. 配置导入导出
3. 数据库维护
4. 缓存清理
5. 备份策略
6. 文章留存策略
7. 数据统计

## 修改文件
- `server/go/internal/services/backup.go` — 全量备份逻辑、选择性恢复
- `server/go/internal/handlers/reader.go` — 新增路由和 handler
- `apps/web/src/components/icons.tsx` — 新增 Cog、ArchiveRestore 图标
- `apps/web/src/components/settings/SettingsDataTab.tsx` — UI 重构

## 验证结果
- Go build: 通过
- TypeScript: 无新增错误
- 旧备份仍兼容（无 settings.json/opml 时按钮不显示）
