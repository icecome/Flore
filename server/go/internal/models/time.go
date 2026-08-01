package models

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

// MilliTime 是兼容 Prisma SQLite DateTime（毫秒时间戳）的时间类型。
// Prisma 在 SQLite 中默认把 DateTime 存为 Unix 毫秒时间戳，Go 的 time.Time
// 无法直接扫描该格式，因此通过自定义 Scanner/Valuer 做转换。
type MilliTime struct {
	T time.Time
}

// GormDataType 告诉 GORM 这是时间类型
func (MilliTime) GormDataType() string {
	return "datetime"
}

// Time 返回底层 time.Time
func (t MilliTime) Time() time.Time {
	return t.T
}

// IsZero 判断是否为零值时间
func (t MilliTime) IsZero() bool {
	return t.T.IsZero()
}

// Value 存入数据库时转换为毫秒时间戳（与 Prisma 的 SQLite DateTime 行为一致）
func (t MilliTime) Value() (driver.Value, error) {
	if t.T.IsZero() {
		return nil, nil
	}
	return t.T.UnixMilli(), nil
}

// Scan 从数据库读取时支持 int64、string（毫秒字符串或 ISO 字符串）和 time.Time
func (t *MilliTime) Scan(value interface{}) error {
	if t == nil {
		return fmt.Errorf("MilliTime.Scan: receiver is nil")
	}
	if value == nil {
		t.T = time.Time{}
		return nil
	}
	switch v := value.(type) {
	case int64:
		t.T = time.UnixMilli(v)
	case int:
		t.T = time.UnixMilli(int64(v))
	case float64:
		t.T = time.UnixMilli(int64(v))
	case []byte:
		return t.Scan(string(v))
	case string:
		// 优先尝试毫秒时间戳字符串
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			t.T = time.UnixMilli(ms)
			return nil
		}
		// 再尝试 ISO 8601 / RFC3339 格式
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t.T = parsed
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			t.T = parsed
			return nil
		}
		return fmt.Errorf("MilliTime.Scan: cannot parse string %q", v)
	case time.Time:
		t.T = v
	default:
		return fmt.Errorf("MilliTime.Scan: unsupported type %T", value)
	}
	return nil
}

// MarshalJSON 输出 ISO 8601 字符串，方便前端解析
func (t MilliTime) MarshalJSON() ([]byte, error) {
	if t.T.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.T.Format(time.RFC3339Nano) + `"`), nil
}

// NullableMilliTime 是指针版本，用于可选时间字段（如 pubDate）
type NullableMilliTime struct {
	T *time.Time
}

// GormDataType 告诉 GORM 这是时间类型
func (NullableMilliTime) GormDataType() string {
	return "datetime"
}

// Time 返回底层 time.Time 指针
func (t NullableMilliTime) Time() *time.Time {
	return t.T
}

// IsZero 判断指针为空或为零值时间
func (t NullableMilliTime) IsZero() bool {
	return t.T == nil || t.T.IsZero()
}

// Value 存入数据库时转换为毫秒时间戳
func (t NullableMilliTime) Value() (driver.Value, error) {
	if t.T == nil || t.T.IsZero() {
		return nil, nil
	}
	return t.T.UnixMilli(), nil
}

// Scan 从数据库读取时支持 int64、string（毫秒字符串或 ISO 字符串）和 time.Time
func (t *NullableMilliTime) Scan(value interface{}) error {
	if t == nil {
		return fmt.Errorf("NullableMilliTime.Scan: receiver is nil")
	}
	if value == nil {
		t.T = nil
		return nil
	}

	switch v := value.(type) {
	case int64:
		tm := time.UnixMilli(v)
		t.T = &tm
	case int:
		tm := time.UnixMilli(int64(v))
		t.T = &tm
	case float64:
		tm := time.UnixMilli(int64(v))
		t.T = &tm
	case []byte:
		return t.Scan(string(v))
	case string:
		// 优先尝试毫秒时间戳字符串
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			tm := time.UnixMilli(ms)
			t.T = &tm
			return nil
		}
		// 再尝试 ISO 8601 / RFC3339 格式
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t.T = &parsed
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			t.T = &parsed
			return nil
		}
		return fmt.Errorf("NullableMilliTime.Scan: cannot parse string %q", v)
	case time.Time:
		t.T = &v
	default:
		return fmt.Errorf("NullableMilliTime.Scan: unsupported type %T", value)
	}
	return nil
}

// MarshalJSON 输出 ISO 8601 字符串或 null
func (t NullableMilliTime) MarshalJSON() ([]byte, error) {
	if t.T == nil || t.T.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.T.Format(time.RFC3339Nano) + `"`), nil
}
