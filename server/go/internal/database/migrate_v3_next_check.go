package database

import (
	"log/slog"
)

// migrateV3NextCheckAtUnix 为 SourceHealth 添加 NextCheckAtUnix 列，用于自适应调度。
// 新列由 GORM AutoMigrate 自动创建，此迁移仅确保版本号一致。
func migrateV3NextCheckAtUnix() error {
	slog.Info("migration v3: ensuring SourceHealth.NextCheckAtUnix column exists")
	// AutoMigrate 已创建列，此处仅作版本推进
	return nil
}
