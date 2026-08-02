package database

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// migrateV2PubDate 将 pubDate 列中所有 text 格式的日期统一转换为 integer 毫秒时间戳
func migrateV2PubDate() error {
	type pubDateRecord struct {
		ID      int
		PubDate string
	}

	// 查询所有 pubDate 为 text 格式的文章（非 NULL、非纯数字）
	var records []pubDateRecord
	if err := DB.Raw(`
		SELECT id, pubDate FROM Item
		WHERE pubDate IS NOT NULL
		AND typeof(pubDate) = 'text'
		AND pubDate GLOB '[0-9]*' = 0
	`).Scan(&records).Error; err != nil {
		return fmt.Errorf("failed to query pubDate records: %w", err)
	}

	if len(records) == 0 {
		slog.Info("no pubDate migration needed")
		return nil
	}

	slog.Info("migrating pubDate text values to integer milliseconds", "count", len(records))

	const batchSize = 100
	var total, skipped int64
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}

		err := DB.Transaction(func(tx *gorm.DB) error {
			for _, rec := range records[i:end] {
				// 解析 ISO 8601 格式，如 "2026-07-28 12:06:26+00:00"
				// RFC3339Nano 是 RFC3339 的超集，单次解析即可覆盖两种格式
				t, err := time.Parse(time.RFC3339Nano, rec.PubDate)
				if err != nil {
					slog.Warn("failed to parse pubDate, skipping", "id", rec.ID, "pubDate", rec.PubDate, "error", err)
					skipped++
					continue
				}
				milli := t.UnixMilli()
				if err := tx.Exec("UPDATE Item SET pubDate = ? WHERE id = ?", milli, rec.ID).Error; err != nil {
					return err
				}
				total++
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to migrate batch: %w", err)
		}
	}

	slog.Info("pubDate migration completed", "migrated", total, "skipped", skipped)
	return nil
}
