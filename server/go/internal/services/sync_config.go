package services

import (
	"fmt"
	"strconv"

	"github.com/rss/go-server/internal/sync"
)

// SyncConfig 同步配置（面向 API 的视图）
type SyncConfig struct {
	APIEnabled  bool   `json:"apiEnabled"`
	APIProvider string `json:"apiProvider"`
	APIEndpoint string `json:"apiEndpoint"`
	APIUsername string `json:"apiUsername"`
	APIPassword string `json:"apiPassword"` // 脱敏返回
	APIToken    string `json:"apiToken"`    // 脱敏返回

	FileEnabled  bool   `json:"fileEnabled"`
	FileProvider string `json:"fileProvider"`
	FileURL      string `json:"fileUrl"`
	// WebDAV 凭证
	FileUsername string `json:"fileUsername"`
	FilePassword string `json:"filePassword"` // 脱敏返回
	// S3 凭证
	FileEndpoint  string `json:"fileEndpoint"`
	FileRegion    string `json:"fileRegion"`
	FileBucket    string `json:"fileBucket"`
	FileAccessKey string `json:"fileAccessKey"`
	FileSecretKey string `json:"fileSecretKey"` // 脱敏返回
	FilePrefix    string `json:"filePrefix"`

	WindowDays   int `json:"windowDays"`
	PullInterval int `json:"pullInterval"` // 秒
}

// syncSensitiveKeys 敏感配置键（脱敏返回 + 前端回传掩码值时保留原值）
var syncSensitiveKeys = map[string]bool{
	"sync_api_password":    true,
	"sync_api_token":       true,
	"sync_file_password":   true,
	"sync_file_access_key": true,
	"sync_file_secret_key": true,
}

// syncAPIProviders 允许的 API provider（仅收录已实现的枚举）
var syncAPIProviders = map[string]bool{
	sync.ProviderMiniflux: true,
	sync.ProviderGReader:  true,
}

// syncFileProviders 允许的文件 provider（P3 交付，预留枚举）
var syncFileProviders = map[string]bool{
	"webdav": true,
	"s3":     true,
}

// GetSyncConfig 读取同步配置，敏感字段脱敏（**** + 尾 4 位，不足 4 位全掩码）
func (s *ReaderService) GetSyncConfig() *SyncConfig {
	return &SyncConfig{
		APIEnabled:    s.GetSettingBool("sync_api_enabled", false),
		APIProvider:   s.GetSetting("sync_api_provider"),
		APIEndpoint:   s.GetSetting("sync_api_endpoint"),
		APIUsername:   s.GetSetting("sync_api_username"),
		APIPassword:   maskSecret(s.GetSetting("sync_api_password")),
		APIToken:      maskSecret(s.GetSetting("sync_api_token")),
		FileEnabled:   s.GetSettingBool("sync_file_enabled", false),
		FileProvider:  s.GetSetting("sync_file_provider"),
		FileURL:       s.GetSetting("sync_file_url"),
		FileUsername:  s.GetSetting("sync_file_username"),
		FilePassword:  maskSecret(s.GetSetting("sync_file_password")),
		FileEndpoint:  s.GetSetting("sync_file_endpoint"),
		FileRegion:    s.GetSetting("sync_file_region"),
		FileBucket:    s.GetSetting("sync_file_bucket"),
		FileAccessKey: maskSecret(s.GetSetting("sync_file_access_key")),
		FileSecretKey: maskSecret(s.GetSetting("sync_file_secret_key")),
		FilePrefix:    s.GetSetting("sync_file_prefix"),
		WindowDays:    s.GetSettingInt("sync_window_days", 30),
		PullInterval:  s.GetSettingInt("sync_pull_interval", 300),
	}
}

// UpdateSyncConfig 写入同步配置。
// 敏感字段传掩码值（以****开头）时保留原值，不覆盖。
func (s *ReaderService) UpdateSyncConfig(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}

	// 校验 provider 枚举
	if v, ok := values["sync_api_provider"]; ok && v != "" && !syncAPIProviders[v] {
		return fmt.Errorf("unsupported api provider: %s", v)
	}
	if v, ok := values["sync_file_provider"]; ok && v != "" && !syncFileProviders[v] {
		return fmt.Errorf("unsupported file provider: %s", v)
	}

	// 校验 endpoint（API / WebDAV / S3），防 SSRF 跳板
	for _, key := range []string{"sync_api_endpoint", "sync_file_url", "sync_file_endpoint"} {
		if v, ok := values[key]; ok && v != "" {
			if err := ValidateURL(v); err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
		}
	}

	// 校验数值项（非负整数；0 有语义：windowDays=全部历史，pullInterval=仅手动）
	for _, key := range []string{"sync_window_days", "sync_pull_interval"} {
		if v, ok := values[key]; ok && v != "" {
			if n, err := strconv.Atoi(v); err != nil || n < 0 {
				return fmt.Errorf("invalid %s: %s", key, v)
			}
		}
	}

	// 敏感字段掩码保护：前端回传脱敏值时跳过写入
	filtered := make(map[string]string, len(values))
	for k, v := range values {
		if syncSensitiveKeys[k] && isMaskedSecret(v) {
			continue
		}
		filtered[k] = v
	}
	return s.UpdateSettings(filtered)
}

// maskSecret 脱敏：保留尾 4 位，不足 4 位全掩码，空值返回空
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}

// isMaskedSecret 判断是否为脱敏回传值（以****开头）
func isMaskedSecret(v string) bool {
	return v == "****" || (len(v) > 4 && v[:4] == "****")
}
