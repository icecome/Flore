package database

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

// CurrentSchemaVersion 当前数据库 schema 版本。
// 每次有破坏性变更（列重命名、数据迁移等）时递增。
// 新增列/表由 AutoMigrate 自动处理，无需递增版本号。
const CurrentSchemaVersion = 3

// Migration 定义一次版本化迁移
type Migration struct {
	Version int
	Name    string
	Migrate func() error
}

// migrations 按版本号升序排列的迁移列表
// 新增迁移时在此追加，同时递增 CurrentSchemaVersion
var migrations = []Migration{
	// v1: 初始 schema，当前所有数据库均为此版本
	// v2: 将 pubDate text 格式统一转换为 integer 毫秒时间戳
	{Version: 2, Name: "migrate pubDate to integer milliseconds", Migrate: migrateV2PubDate},
	// v3: 为 SourceHealth 添加 NextCheckAtUnix 列（由 AutoMigrate 自动建列，此迁移推进版本）
	{Version: 3, Name: "add NextCheckAtUnix to SourceHealth", Migrate: migrateV3NextCheckAtUnix},
}

// getSchemaVersion 从 Setting 表读取当前 schema 版本号。
// 未设置时返回 0，表示全新数据库或尚未初始化。
func getSchemaVersion() (int, error) {
	var setting models.Setting
	err := DB.Where("key = ?", "schema_version").First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read schema version: %w", err)
	}
	version, err := strconv.Atoi(setting.Value)
	if err != nil {
		return 0, fmt.Errorf("invalid schema version value %q: %w", setting.Value, err)
	}
	return version, nil
}

// setSchemaVersion 写入或更新 Setting 表中的 schema 版本号
func setSchemaVersion(version int) error {
	return DB.Where("key = ?", "schema_version").
		Assign(models.Setting{Value: strconv.Itoa(version)}).
		FirstOrCreate(&models.Setting{Key: "schema_version"}).Error
}

// RunMigrations 执行版本化迁移。
// 必须在 AutoMigrate（确保所有表存在）之后调用。
// 按版本号升序执行尚未运行的迁移，并在每次迁移成功后更新版本号。
// 迁移失败时返回错误，由调用方决定是否退出进程。
func RunMigrations() error {
	currentVersion, err := getSchemaVersion()
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	if currentVersion > CurrentSchemaVersion {
		return fmt.Errorf(
			"database schema version (%d) is newer than app version (%d), downgrade not supported",
			currentVersion, CurrentSchemaVersion,
		)
	}

	if currentVersion == CurrentSchemaVersion {
		return nil
	}

	slog.Info("schema migration needed",
		"currentVersion", currentVersion,
		"targetVersion", CurrentSchemaVersion,
	)

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}
		if m.Version > CurrentSchemaVersion {
			break
		}

		slog.Info("running migration", "version", m.Version, "name", m.Name)
		if err := m.Migrate(); err != nil {
			return fmt.Errorf("migration v%d (%s) failed: %w", m.Version, m.Name, err)
		}
		if err := setSchemaVersion(m.Version); err != nil {
			return fmt.Errorf("failed to update schema version to %d: %w", m.Version, err)
		}
		slog.Info("migration completed", "version", m.Version, "name", m.Name)
	}

	// 确保最终版本号被持久化。即便 migrations 列表为空，
	// 也需要将 currentVersion (0) 推进到 CurrentSchemaVersion (1)，
	// 避免每次启动都重复走迁移流程。
	if currentVersion != CurrentSchemaVersion {
		if err := setSchemaVersion(CurrentSchemaVersion); err != nil {
			return fmt.Errorf("failed to update schema version to %d: %w", CurrentSchemaVersion, err)
		}
		slog.Info("schema version updated", "version", CurrentSchemaVersion)
	}

	return nil
}
