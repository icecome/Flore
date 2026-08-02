//go:build !windows

package updater

import "fmt"

// applyUpdate 非 Windows 平台暂不支持自动更新（当前仅发布 Windows 桌面端）。
func applyUpdate(asset *Asset, exePath string) error {
	return fmt.Errorf("当前平台不支持自动更新")
}
