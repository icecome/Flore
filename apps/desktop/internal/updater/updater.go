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
}

// CheckForUpdate 拉取 manifest 并比对当前版本；无更新返回 (nil, nil)。
func CheckForUpdate(currentVersion string) (*UpdateInfo, error) {
	manifest, err := FetchManifest()
	if err != nil {
		return nil, err
	}
	if manifest.Latest == "" {
		return nil, fmt.Errorf("manifest 缺少 latest 字段")
	}
	if CompareVersion(manifest.Latest, currentVersion) <= 0 {
		return nil, nil
	}
	// 低于最小支持版本时仍返回更新信息，由上层强制提示更新。
	if manifest.MinSupported != "" && CompareVersion(currentVersion, manifest.MinSupported) < 0 {
		_ = manifest.MinSupported
	}
	asset := matchAsset(manifest)
	if asset == nil {
		return nil, fmt.Errorf("未找到适配平台 %s 的更新资产", currentPlatform())
	}
	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  manifest.Latest,
		Notes:          manifest.Notes,
		Size:           asset.Size,
		FileName:       asset.FileName,
		SHA256:         asset.SHA256,
		URLs:           asset.URLs,
	}, nil
}

// ApplyUpdate 下载并准备应用更新：下载→解压→生成外部脚本→启动脚本后由调用方退出进程。
// 真正的文件替换由外部脚本在进程退出后完成（避免替换正在运行的自身可执行文件）。
func ApplyUpdate(info *UpdateInfo) error {
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
	return applyUpdate(asset, exePath)
}
