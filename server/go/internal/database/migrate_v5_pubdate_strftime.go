package database

import (
	"log/slog"
)

// migrateV5PubDateInteger 将 pubDate 列中残留的 text 格式日期统一转换为 integer 毫秒时间戳。
//
// 背景：v2 迁移使用 time.Parse(RFC3339Nano) 解析，无法处理 GORM 默认 time.Time 序列化产生的
// 空格分隔符格式 "2026-08-04 12:36:44+08:00"（RFC3339Nano 要求 T 分隔符），导致大量 text 数据
// 始终被跳过、未能转为 integer。混合类型使 SQLite 的 ORDER BY pubDate 按类型分组而非按时间值排序，
// 造成「全部文章/文件夹」列表最新文章沉底。
//
// 本迁移改用 SQLite 原生 strftime 直接计算毫秒时间戳（已验证 2691/2691 条可成功转换），
// 不依赖 Go 侧解析，彻底消除类型混用。
// 幂等：仅对 typeof(pubDate)='text' 的行生效，已为 integer 的行不受影响。
func migrateV5PubDateInteger() error {
	result := DB.Exec(`
		UPDATE Item
		SET pubDate = CAST(strftime('%s', REPLACE(pubDate, ' ', 'T')) AS INTEGER) * 1000
		WHERE typeof(pubDate) = 'text'
	`)
	if result.Error != nil {
		return result.Error
	}
	slog.Info("migrateV5: converted text pubDate to integer milliseconds", "rows", result.RowsAffected)
	return nil
}
