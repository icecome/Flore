package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// UserDataDir 返回用户数据目录：
//   - Windows: %LOCALAPPDATA%\Flore（回退 %APPDATA%\Flore、%TEMP%\Flore）
//   - 其他:    ~/.flore
//
// HOME 不可用时（非 Windows）返回错误，调用方应据此回退到适当位置。
func UserDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		for _, envName := range []string{"LOCALAPPDATA", "APPDATA"} {
			if dir := os.Getenv(envName); dir != "" {
				return filepath.Join(dir, "Flore"), nil
			}
		}
		return filepath.Join(os.TempDir(), "Flore"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home dir: %w", err)
	}
	return filepath.Join(home, ".flore"), nil
}