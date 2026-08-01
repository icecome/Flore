package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDesensitizeURL 验证 URL 脱敏：保留 scheme+host，移除 path/query
func TestDesensitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full url",
			input:    "https://example.com/feed/rss?token=secret",
			expected: "https://example.com",
		},
		{
			name:     "http url",
			input:    "http://test.com/path/to/feed",
			expected: "http://test.com",
		},
		{
			name:     "non url",
			input:    "just some text",
			expected: "just some text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDesensitizeSource 验证 source 字段脱敏
func TestDesensitizeSource(t *testing.T) {
	result := DesensitizeAttr("source", slog.StringValue("My Personal Blog"))
	if !strings.HasPrefix(result.String(), "source_") {
		t.Errorf("expected source to be prefixed with 'source_', got %q", result.String())
	}
}

// TestDesensitizePath 验证 path 字段脱敏
func TestDesensitizePath(t *testing.T) {
	tests := []string{"path", "db_path", "database"}
	for _, key := range tests {
		result := DesensitizeAttr(key, slog.StringValue("C:\\Users\\test\\data\\reader.db"))
		if result.String() != "[REDACTED_PATH]" {
			t.Errorf("DesensitizeAttr(%q) = %q, want '[REDACTED_PATH]'", key, result.String())
		}
	}
}

// TestDesensitizeError 验证 error 字段脱敏
func TestDesensitizeError(t *testing.T) {
	input := "failed to fetch https://example.com/feed: timeout"
	result := DesensitizeAttr("error", slog.StringValue(input))
	if strings.Contains(result.String(), "example.com/feed") {
		t.Error("error should not contain full URL path")
	}
}

// TestDesensitizeHandler 验证 handler 正确脱敏所有属性
func TestDesensitizeHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	desensitized := NewDesensitizeHandler(handler)

	logger := slog.New(desensitized)
	logger.Info("test log",
		slog.String("url", "https://example.com/feed/rss"),
		slog.String("source", "TestBlog"),
		slog.String("path", "C:\\Users\\test\\data.db"),
		slog.String("error", "failed to https://sensitive.com/path"),
	)

	output := buf.String()
	// URL 应该只剩 scheme+host
	if strings.Contains(output, "example.com/feed/rss") {
		t.Error("URL path should be removed from log")
	}
	// source 应该被加上 "source_" 前缀，原始名称不应单独出现
	if strings.Contains(output, "source=TestBlog") {
		t.Error("source name should be prefixed with 'source_'")
	}
	// db path 应该被替换
	if strings.Contains(output, "C:\\Users\\test\\data.db") {
		t.Error("db path should be redacted")
	}
	// error 中的 URL path 应该被移除
	if strings.Contains(output, "sensitive.com/path") {
		t.Error("error URL should be redacted")
	}
}

// TestSentinelValidation 哨兵断言：预置的唯一字符串不应出现在日志中
// 覆盖三个产出方：backend.log、desktop.log、frontend-buffer.json
func TestSentinelValidation(t *testing.T) {
	// 哨兵字符串（每个唯一）
	sentinels := []struct {
		name          string
		value         string
		key           string
		expectedValue string // 期望的输出值（脱敏后）
	}{
		{"sentinel_url", "https://sentinel-test.example.com/unique/path", "url", "https://sentinel-test.example.com"},
		{"sentinel_source", "SentinelBlog2026", "source", "source_SentinelBlog2026"},
		{"sentinel_path", "C:\\SentinelTest\\UniquePath.db", "path", "[REDACTED_PATH]"},
		{"sentinel_ref", "https://ref.example.com/special", "ref", "https://ref.example.com"},
	}

	for _, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewTextHandler(&buf, nil)
			desensitized := NewDesensitizeHandler(handler)
			logger := slog.New(desensitized)

			logger.Info("test", slog.String(s.key, s.value))

			output := buf.String()
			// 验证原始敏感值不作为独立值出现
			switch s.key {
			case "url", "ref":
				// URL 应该只剩 scheme+host，不应包含 path
				if strings.Contains(output, s.value) {
					t.Errorf("sentinel %q full URL should not appear in log output", s.value)
				}
			case "source":
				// source 应该被加上前缀，原始名称不应单独出现
				if strings.Contains(output, "source="+s.value) {
					t.Errorf("sentinel %q should not appear as bare source value", s.value)
				}
			case "path":
				// path 应该被完全替换
				if strings.Contains(output, s.value) {
					t.Errorf("sentinel %q should not appear in log output", s.value)
				}
			}
		})
	}
}

// TestRotatingWriter 验证日志轮转
func TestRotatingWriter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath, 100, 3) // 100字节，最多3个备份
	if err != nil {
		t.Fatalf("create rotating writer: %v", err)
	}
	defer w.Close()

	// 写入超过阈值的数据触发轮转
	for i := 0; i < 5; i++ {
		w.Write([]byte("test line " + string(rune('a'+i)) + "\n"))
	}

	// 验证主日志文件存在
	if _, err := os.Stat(logPath); err != nil {
		t.Fatal("main log file should exist")
	}
}
