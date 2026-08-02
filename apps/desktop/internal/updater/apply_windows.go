//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyUpdate 在 Windows 上：下载资产→解压到临时目录→生成 apply-update.bat→
// 启动该脚本（独立于本进程）→由调用方退出本进程，脚本等待本进程释放后覆盖文件并重启。
func applyUpdate(asset *Asset, exePath string) error {
	staging, err := os.MkdirTemp("", "flore-update-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	// 注意：staging 不能提前删除，bat 需要从中读取解压后的文件。

	zipPath := filepath.Join(staging, asset.FileName)
	if err := DownloadAsset(asset, zipPath); err != nil {
		return fmt.Errorf("下载更新失败: %w", err)
	}

	extractDir := filepath.Join(staging, "extracted")
	if err := unzipSafe(zipPath, extractDir); err != nil {
		return fmt.Errorf("解压更新失败: %w", err)
	}

	installDir := filepath.Dir(exePath)
	batPath := filepath.Join(staging, "apply-update.bat")
	script := buildApplyScript(os.Getpid(), extractDir, installDir, exePath)
	if err := os.WriteFile(batPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("写入更新脚本失败: %w", err)
	}

	cmd := exec.Command("cmd", "/c", batPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

// buildApplyScript 生成 Windows 批处理更新脚本。
// 流程：等待当前进程(PID)退出 → 结束可能残留的后端子进程 → 复制解压文件覆盖安装目录 → 重新拉起主程序。
func buildApplyScript(pid int, srcDir, installDir, exePath string) string {
	const tpl = `@echo off
set PID={PID}
set SRC={SRC}
set DST={DST}
set SELF={SELF}
:wait
tasklist /fi "PID eq %PID%" | find "PID" >nul
if %errorlevel%==0 (
  timeout /t 1 /nobreak >nul
  goto wait
)
taskkill /F /IM florebackend.exe >nul 2>&1
timeout /t 1 /nobreak >nul
xcopy /y /e /i "{SRC}\*" "{DST}\" >nul
start "" "{SELF}"
`
	s := strings.ReplaceAll(tpl, "{PID}", strconv.Itoa(pid))
	s = strings.ReplaceAll(s, "{SRC}", srcDir)
	s = strings.ReplaceAll(s, "{DST}", installDir)
	s = strings.ReplaceAll(s, "{SELF}", exePath)
	return s
}
