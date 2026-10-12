package updater

import (
	"fmt"
	"os"
)

// UpdateInfo 返回给前端的更新信息。
type UpdateInfo struct {
	CurrentVersion string   `json:"currentVersion"`
	LatestVersion  string   `json:"latestVersion"`
	Notes          string   `json:"notes"`
	Size           int64    `json:"size"`
	FileName       string   `json:"fileName"`
	SHA256         string   `json:"sha256"`
	URLs           []string `json:"urls"`
	// Forced 表示当前版本低于 manifest.minSupported，属于强制更新场景，
	// 前端应提升提示优先级（如禁止静默忽略）。
	Forced bool `json:"forced"`
}

// CheckForUpdate 拉取 manifest 并比对当前版本；无更新返回 (nil, nil)。
func CheckForUpdate(currentVersion string) (*UpdateInfo, error) {
	manifest, err := FetchManifest()
	if err != nil {
		return nil, err
	}
	return buildUpdateInfo(manifest, currentVersion)
}

// buildUpdateInfo 由 manifest 与当前版本构造更新信息（纯函数，便于单测）。
// 无可用更新返回 (nil, nil)；manifest 缺字段或无平台资产时返回错误。
func buildUpdateInfo(m *Manifest, currentVersion string) (*UpdateInfo, error) {
	if m.Latest == "" {
		return nil, fmt.Errorf("manifest 缺少 latest 字段")
	}
	if CompareVersion(m.Latest, currentVersion) <= 0 {
		return nil, nil
	}
	asset := matchAsset(m)
	if asset == nil {
		return nil, fmt.Errorf("未找到适配平台 %s 的更新资产", currentPlatform())
	}
	forced := m.MinSupported != "" && CompareVersion(currentVersion, m.MinSupported) < 0
	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  m.Latest,
		Notes:          m.Notes,
		Size:           asset.Size,
		FileName:       asset.FileName,
		SHA256:         asset.SHA256,
		URLs:           asset.URLs,
		Forced:         forced,
	}, nil
}

// ApplyUpdate 下载并准备应用更新：下载→解压→生成外部脚本→启动脚本后由调用方退出进程。
// 真正的文件替换由外部脚本在进程退出后完成（避免替换正在运行的自身可执行文件）。
// onProgress 报告下载进度（0~1），可为 nil。
func ApplyUpdate(info *UpdateInfo, onProgress func(float64)) error {
	asset := &Asset{
		FileName: info.FileName,
		Size:     info.Size,
		SHA256:   info.SHA256,
		URLs:     info.URLs,
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位当前可执行文件: %w", err)
	}
	return applyUpdateProgress(asset, exePath, onProgress)
}
