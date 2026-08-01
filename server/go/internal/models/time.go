package models

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

// parseMilliTime 解析毫秒时间戳输入为 time.Time，支持 int64/int/float64/[]byte/string/time.Time。
// string 类型优先尝试毫秒时间戳字符串，再尝试 ISO 8601/RFC3339 格式。
func parseMilliTime(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case int64:
		return time.UnixMilli(v), nil
	case int:
		return time.UnixMilli(int64(v)), nil
	case float64:
		return time.UnixMilli(int64(v)), nil
	case []byte:
		return parseMilliTime(string(v))
	case string:
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(ms), nil
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t, nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("cannot parse time string %q", v)
	case time.Time:
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", value)
	}
}

// MilliTime 是兼容 Prisma SQLite DateTime（毫秒时间戳）的时间类型。
type MilliTime struct {
	T time.Time
}

// GormDataType 告诉 GORM 这是时间类型
func (MilliTime) GormDataType() string { return "datetime" }

// Time 返回底层 time.Time
func (t MilliTime) Time() time.Time { return t.T }

// IsZero 判断是否为零值时间
func (t MilliTime) IsZero() bool { return t.T.IsZero() }

// Value 存入数据库时转换为毫秒时间戳
func (t MilliTime) Value() (driver.Value, error) {
	if t.T.IsZero() {
		return nil, nil
	}
	return t.T.UnixMilli(), nil
}

// Scan 从数据库读取时解析毫秒时间戳
func (t *MilliTime) Scan(value interface{}) error {
	if t == nil {
		return fmt.Errorf("MilliTime.Scan: receiver is nil")
	}
	if value == nil {
		t.T = time.Time{}
		return nil
	}
	tm, err := parseMilliTime(value)
	if err != nil {
		return err
	}
	t.T = tm
	return nil
}

// MarshalJSON 输出 ISO 8601 字符串
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
func (NullableMilliTime) GormDataType() string { return "datetime" }

// Time 返回底层 time.Time 指针
func (t NullableMilliTime) Time() *time.Time { return t.T }

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

// Scan 从数据库读取时解析毫秒时间戳
func (t *NullableMilliTime) Scan(value interface{}) error {
	if t == nil {
		return fmt.Errorf("NullableMilliTime.Scan: receiver is nil")
	}
	if value == nil {
		t.T = nil
		return nil
	}
	tm, err := parseMilliTime(value)
	if err != nil {
		return err
	}
	t.T = &tm
	return nil
}

// MarshalJSON 输出 ISO 8601 字符串或 null
func (t NullableMilliTime) MarshalJSON() ([]byte, error) {
	if t.T == nil || t.T.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.T.Format(time.RFC3339Nano) + `"`), nil
}
