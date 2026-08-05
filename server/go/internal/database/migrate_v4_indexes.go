package database

import (
	"fmt"
)

// migrateV4Indexes 为热排序/过滤列补充索引（幂等，IF NOT EXISTS）。
// 背景：Item 列表/搜索/导出均按 pubDate DESC 排序，此前缺索引导致大表全表排序；
// 复合索引 (sourceId, pubDate) 加速"按源+时间"的常见查询。
// 单列索引（sourceId/isRead/isStarred/isReadLater/nextCheckAtUnix 等）已由 GORM
// AutoMigrate 的 gorm:"index" 标签创建，此处仅补充缺口。
func migrateV4Indexes() error {
	statements := []string{
		"CREATE INDEX IF NOT EXISTS idx_items_pubdate ON Item(pubDate DESC)",
		"CREATE INDEX IF NOT EXISTS idx_items_source_pubdate ON Item(sourceId, pubDate DESC)",
	}
	for _, stmt := range statements {
		if err := DB.Exec(stmt).Error; err != nil {
			return fmt.Errorf("migrate v4: failed to execute %q: %w", stmt, err)
		}
	}
	return nil
}
