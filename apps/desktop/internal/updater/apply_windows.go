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

// applyUpdateProgress 在 Windows 上按资产 variant 分支处理更新：
//   - setup：下载 NSIS 安装器 → 生成脚本等当前进程退出 → 静默重装到原位（/S /D=安装目录）
//   - 其它（portable）：下载 zip → 解压 → 生成脚本复制覆盖安装目录
//
// 脚本独立于本进程运行，由调用方退出本进程后生效。onProgress 报告下载进度（0~1）。
func applyUpdateProgress(asset *Asset, exePath string, onProgress func(float64)) error {
	staging, err := os.MkdirTemp("", "flore-update-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	// 注意：staging 不能提前删除，bat 需要从中读取下载的资产。

	if asset.Variant == "setup" {
		return applySetup(asset, exePath, staging, onProgress)
	}
	return applyPortable(asset, exePath, staging, onProgress)
}

// applyPortable 处理便携包更新：下载 zip → 解压 → 脚本复制覆盖安装目录 → 重启。
// 成功启动 BAT 后由脚本自行清理 staging；任何前置失败都由本函数清理。
func applyPortable(asset *Asset, exePath, staging string, onProgress func(float64)) (retErr error) {
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(staging)
		}
	}()

	zipPath := filepath.Join(staging, asset.FileName)
	if err := DownloadAssetProgress(asset, zipPath, onProgress); err != nil {
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

// applySetup 处理安装包更新：下载 NSIS 安装器 → 脚本等当前进程退出 → 静默重装到原位。
// 成功启动 BAT 后由脚本自行清理 staging；任何前置失败都由本函数清理。
func applySetup(asset *Asset, exePath, staging string, onProgress func(float64)) (retErr error) {
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(staging)
		}
	}()

	setupPath := filepath.Join(staging, asset.FileName)
	if err := DownloadAssetProgress(asset, setupPath, onProgress); err != nil {
		return fmt.Errorf("下载更新失败: %w", err)
	}

	installDir := filepath.Dir(exePath)
	batPath := filepath.Join(staging, "apply-update.bat")
	script := buildSetupScript(os.Getpid(), setupPath, installDir, exePath, staging)
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

// batEscape 转义写入 BAT 脚本的字符串字面量中的 cmd 特殊字符。
// cmd.exe 对引号内的 & | < > ^ % 仍会解析，路径中若包含这些字符可注入任意命令
// （BAT 模板注入）。路径中极少出现引号，直接剔除；其余字符加 ^ 转义或双写。
func batEscape(s string) string {
	replacer := strings.NewReplacer(
		`"`, "",
		"&", "^&",
		"|", "^|",
		"<", "^<",
		">", "^>",
		"^", "^^",
		"%", "%%",
	)
	return replacer.Replace(s)
}

// buildApplyScript 生成 Windows 便携包更新脚本。
// 流程：等待当前进程(PID)退出 → 结束可能残留的后端子进程（Flore.exe --backend）→ 复制解压文件覆盖安装目录 → 重新拉起主程序。
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
taskkill /F /IM Flore.exe >nul 2>&1
timeout /t 1 /nobreak >nul
xcopy /y /e /i "{SRC}\*" "{DST}\" >nul
start "" "{SELF}"
rem 清理临时目录（bat 自身所在目录），失败不影响更新结果
rmdir /s /q "%~dp0"
`
	s := strings.ReplaceAll(tpl, "{PID}", strconv.Itoa(pid))
	s = strings.ReplaceAll(s, "{SRC}", batEscape(srcDir))
	s = strings.ReplaceAll(s, "{DST}", batEscape(installDir))
	s = strings.ReplaceAll(s, "{SELF}", batEscape(exePath))
	return s
}

// buildSetupScript 生成 Windows 安装包更新脚本。
// 流程：等待当前进程(PID)退出 → 静默运行 NSIS 安装器（/S 静默、/D=安装目录覆盖默认位置）→ 重新拉起主程序 → 清理临时目录。
// 注意：/D 的值不得加引号（NSIS 要求），且必须放在命令行最后。
func buildSetupScript(pid int, setupExe, installDir, exePath, staging string) string {
	const tpl = `@echo off
set PID={PID}
set SETUPEXE={SETUPEXE}
set INSTDIR={INSTDIR}
set SELF={SELF}
set STAGE={STAGE}
:wait
tasklist /fi "PID eq %PID%" | find "PID" >nul
if %errorlevel%==0 (
  timeout /t 1 /nobreak >nul
  goto wait
)
"%SETUPEXE%" /S /D={INSTDIR}
start "" "%SELF%"
rmdir /s /q "%STAGE%"
`
	s := strings.ReplaceAll(tpl, "{PID}", strconv.Itoa(pid))
	s = strings.ReplaceAll(s, "{SETUPEXE}", batEscape(setupExe))
	s = strings.ReplaceAll(s, "{INSTDIR}", batEscape(installDir))
	s = strings.ReplaceAll(s, "{SELF}", batEscape(exePath))
	s = strings.ReplaceAll(s, "{STAGE}", batEscape(staging))
	return s
}
