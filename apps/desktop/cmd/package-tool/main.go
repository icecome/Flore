// package-tool 跨平台生成 Flore 分发包，替代原 Windows 专用的 package-portable.ps1。
//
// 命名规范（统一所有分发包）：
//
//	portable: flore-portable-<os>-<arch>-<version>.zip
//	setup:    flore-setup-<os>-<arch>-<version>.exe
//	deb:      flore_<version>-1_<arch>.deb
//
// 版本号由 -version 传入，代码中不硬编码。版本号格式示例：0.1.0-20260802。
// 版本进文件名，确保文件脱离 Release 后仍可追溯。
//
// 便携包仅包含前端可执行文件（Flore.exe / Flore.app，后端已编入主程序内），
// data/ backups/ webview2/ 等目录由软件首次运行时自己创建。
//
// 用法：
//
//	# 构建便携版（默认）
//	package-tool -edition portable -version 0.1.0-20260802 -os windows -arch amd64
//
//	# 构建安装版（需先构建 NSIS 安装器）
//	package-tool -edition setup -setupBin build/Flore-installer.exe -version 0.1.0-20260802 -os windows -arch amd64
//
//	# 构建 deb 包（Linux）
//	package-tool -edition deb -version 0.1.0-20260802 -os linux -arch amd64 -feBin build/bin/Flore
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	edition := flag.String("edition", "portable", "portable|setup")
	version := flag.String("version", "", "发布版本号（必填，如 0.1.0-20260802）")
	osArg := flag.String("os", runtime.GOOS, "目标操作系统（默认当前）")
	arch := flag.String("arch", runtime.GOARCH, "目标架构（默认当前）")
	binDir := flag.String("binDir", "build/bin", "含前后端二进制的目录")
	outDir := flag.String("outDir", "build", "输出目录")
	portableDir := flag.String("portableDir", "build/Flore-portable", "便携目录路径（供 zip 与 NSIS 引用）")
	appName := flag.String("appName", "Flore", "前端可执行文件名（不含扩展名）")
	feBin := flag.String("feBin", "", "前端二进制路径（默认 <binDir>/<appName>[.exe]）；macOS 传 <binDir>/Flore.app")
	setupBin := flag.String("setupBin", "", "NSIS 安装器路径（-edition setup 时必填）")
	clean := flag.Bool("clean", true, "构建完成后清理便携目录（仅 portable edition）")
	flag.Parse()

	if *version == "" {
		fatal("--version 必填")
	}

	switch *edition {
	case "portable":
		buildPortable(version, osArg, arch, binDir, outDir, portableDir, appName, feBin, clean)
	case "setup":
		buildSetup(version, osArg, arch, outDir, setupBin)
	case "deb":
		buildDeb(version, osArg, arch, binDir, outDir, appName, feBin)
	default:
		fatal("不支持的 edition: %s（仅支持 portable|setup|deb）", *edition)
	}
}

func buildPortable(version, osArg, arch, binDir, outDir, portableDir, appName, feBin *string, clean *bool) {
	// 创建便携目录（持久化，供 NSIS 脚本引用）
	portablePath := *portableDir
	if err := os.MkdirAll(portablePath, 0o755); err != nil {
		fatal("创建便携目录失败: %v", err)
	}

	// 拷贝前端二进制
	fePath := *feBin
	if fePath == "" {
		n := *appName
		if *osArg == "windows" {
			n += ".exe"
		}
		fePath = filepath.Join(*binDir, n)
	}
	if _, err := os.Stat(fePath); err != nil {
		fatal("前端二进制未找到: %s（请先 wails build）", fePath)
	}
	if err := copyPath(fePath, filepath.Join(portablePath, filepath.Base(fePath))); err != nil {
		fatal("拷贝前端二进制失败: %v", err)
	}

	// 后端代码已编入 Flore 主程序（server/go/backend 包），桌面壳以
	// 「自身二进制 --backend」自衍生子进程跑后端，分发包不再含独立 florebackend。
	// 因此这里无需拷贝/签名第二个二进制；Wails 已对 Flore.app 做 ad-hoc 自签名，
	// 子进程即同一份已放行二进制，Gatekeeper 不再拦截。

	// 便携包仅包含可执行文件，data/ backups/ webview2/ 等目录由软件首次运行时自己创建。
	// 若 data/ 不存在，appDataDir() 自动回退到 %LOCALAPPDATA%/Flore，不会因缺少目录而崩溃。

	// 打包便携 zip
	zipName := fmt.Sprintf("flore-portable-%s-%s-%s.zip", *osArg, *arch, *version)
	zipPath := filepath.Join(*outDir, zipName)
	if err := zipDir(portablePath, zipPath); err != nil {
		fatal("生成 %s 失败: %v", zipName, err)
	}
	fmt.Printf("便携包已生成: %s\n", zipPath)

	// 清理便携目录（除非 -clean=false）
	if *clean {
		if err := os.RemoveAll(portablePath); err != nil {
			fmt.Fprintf(os.Stderr, "package-tool: 清理便携目录失败: %v\n", err)
		}
	} else {
		fmt.Printf("便携目录已保留: %s\n", portablePath)
	}
}

func buildSetup(version, osArg, arch, outDir, setupBin *string) {
	if *setupBin == "" {
		fatal("-edition setup 时 --setupBin 必填")
	}
	if _, err := os.Stat(*setupBin); err != nil {
		fatal("安装器未找到: %s", *setupBin)
	}

	exeName := fmt.Sprintf("flore-setup-%s-%s-%s.exe", *osArg, *arch, *version)
	exePath := filepath.Join(*outDir, exeName)
	if err := copyFile(*setupBin, exePath); err != nil {
		fatal("拷贝安装器失败: %v", err)
	}
	fmt.Printf("安装包已生成: %s\n", exePath)
}

// buildDeb 使用 dpkg-deb 构建 Linux deb 包。
// 依赖 dpkg-deb 命令（Linux 开发环境自带）。
func buildDeb(version, osArg, arch, binDir, outDir, appName, feBin *string) {
	if *osArg != "linux" {
		fatal("-edition deb 仅支持 linux")
	}
	if *feBin == "" {
		fePath := filepath.Join(*binDir, *appName)
		if _, err := os.Stat(fePath); err != nil {
			fatal("feBin 未提供且未找到默认二进制: %s", fePath)
		}
		*feBin = fePath
	}
	if _, err := os.Stat(*feBin); err != nil {
		fatal("前端二进制未找到: %s（请先 wails build）", *feBin)
	}

	// 校验 dpkg-deb 可用
	if out, err := exec.Command("dpkg-deb", "--version").CombinedOutput(); err != nil {
		fatal("dpkg-deb 不可用，请安装 dpkg-dev: %v\n%s", err, out)
	}

	debianDir := filepath.Join(*outDir, "debian-pkg")
	if err := os.MkdirAll(debianDir, 0o755); err != nil {
		fatal("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(debianDir)

	// 目录结构
	usrDir := filepath.Join(debianDir, "usr", "share", *appName)
	binDirPkg := filepath.Join(debianDir, "usr", "bin")
	if err := os.MkdirAll(usrDir, 0o755); err != nil {
		fatal("创建目录失败: %v", err)
	}
	if err := os.MkdirAll(binDirPkg, 0o755); err != nil {
		fatal("创建目录失败: %v", err)
	}

	// 拷贝主程序
	if err := copyPath(*feBin, filepath.Join(usrDir, *appName)); err != nil {
		fatal("拷贝主程序失败: %v", err)
	}
	// 创建 bin 符号链接
	linkPath := filepath.Join(binDirPkg, *appName)
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		fatal("清理旧符号链接失败: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "share", *appName, *appName), linkPath); err != nil {
		fatal("创建符号链接失败: %v", err)
	}

	// 拷贝图标到标准 icon 目录
	// 图标源在 binDir 父目录（build/favicon.png），而非 build/bin/
	iconParentDir := filepath.Dir(*binDir)
	iconDir := filepath.Join(debianDir, "usr", "share", "icons", "hicolor")
	for _, size := range []string{"48", "256"} {
		var iconSrc string
		candidates := []string{"favicon.png", "appicon.png"}
		for _, name := range candidates {
			candidate := filepath.Join(iconParentDir, name)
			if _, err := os.Stat(candidate); err == nil {
				iconSrc = candidate
				break
			}
		}
		if iconSrc == "" {
			fmt.Fprintf(os.Stderr, "package-tool: 警告，未找到图标源文件\n")
			continue
		}
		iconDst := filepath.Join(iconDir, size+"x"+size, "apps", *appName+".png")
		if err := os.MkdirAll(filepath.Dir(iconDst), 0o755); err != nil {
			fatal("创建图标目录失败: %v", err)
		}
		if err := copyFile(iconSrc, iconDst); err != nil {
			fatal("拷贝图标失败: %v", err)
		}
	}

	// 创建 .desktop 菜单项
	desktopPath := filepath.Join(debianDir, "usr", "share", "applications", *appName+".desktop")
	if err := os.MkdirAll(filepath.Dir(desktopPath), 0o755); err != nil {
		fatal("创建目录失败: %v", err)
	}
	desktopContent := fmt.Sprintf(
		"[Desktop Entry]\nName=%s\nComment=%s RSS Reader\nExec=/usr/bin/%s\nIcon=%s\nTerminal=false\nType=Application\nCategories=Network;Reader;\nStartupNotify=true\n",
		*appName, *appName, *appName, *appName,
	)
	if err := os.WriteFile(desktopPath, []byte(desktopContent), 0o644); err != nil {
		fatal("写入 .desktop 文件失败: %v", err)
	}

	// 创建 debian/control（dpkg-deb 要求 DEBIAN/ 目录全大写）
	controlDir := filepath.Join(debianDir, "DEBIAN")
	if err := os.MkdirAll(controlDir, 0o755); err != nil {
		fatal("创建目录失败: %v", err)
	}
	debVer := strings.ReplaceAll(*version, "-", ".")
	controlContent := fmt.Sprintf(
		"Package: %s\nVersion: %s-1\nSection: utils\nPriority: optional\nArchitecture: %s\nMaintainer: Lice <werrice@outlook.com>\nDescription: %s RSS Reader\n",
		*appName, debVer, *arch, *appName,
	)
	if err := os.WriteFile(filepath.Join(controlDir, "control"), []byte(controlContent), 0o644); err != nil {
		fatal("写入 control 失败: %v", err)
	}

	// 构建 deb
	outName := fmt.Sprintf("%s_%s-1_%s.deb", *appName, debVer, *arch)
	outPath := filepath.Join(*outDir, outName)
	cmd := exec.Command("dpkg-deb", "--build", debianDir, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		fatal("dpkg-deb 构建失败: %v\n%s", err, out)
	}
	fmt.Printf("deb 包已生成: %s\n", outPath)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	// 保留源文件权限（含可执行位），否则拷贝后二进制丢失 +x，压缩包解压将无法运行
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, fi.Mode().Perm())
	}
	return nil
}

func zipDir(src, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		// 用 FileInfoHeader + SetMode 保留 Unix 权限（含可执行位），
		// 否则解压后的二进制丢失 +x，macOS 无法启动应用。
		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = rel
		if info.IsDir() {
			fh.Name += "/"
			fh.Method = zip.Store
		}
		fh.SetMode(info.Mode())
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "package-tool: "+format+"\n", args...)
	os.Exit(1)
}
