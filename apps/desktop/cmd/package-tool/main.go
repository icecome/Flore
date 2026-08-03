// package-tool 跨平台生成 Flore 分发包，替代原 Windows 专用的 package-portable.ps1。
//
// 命名规范（统一所有分发包）：
//
//	portable: flore-portable-<os>-<arch>-<version>.zip
//	setup:    flore-setup-<os>-<arch>-<version>.exe
//
// 版本号由 -version 传入，代码中不硬编码。版本号格式示例：0.1.0-20260802。
// 版本进文件名，确保文件脱离 Release 后仍可追溯。
//
// 便携包仅包含可执行文件（Flore.exe + florebackend.exe），
// data/ backups/ webview2/ 等目录由软件首次运行时自己创建。
//
// 用法：
//
//	# 构建便携版（默认）
//	package-tool -edition portable -version 0.1.0-20260802 -os windows -arch amd64
//
//	# 构建安装版（需先构建 NSIS 安装器）
//	package-tool -edition setup -setupBin build/Flore-installer.exe -version 0.1.0-20260802 -os windows -arch amd64
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	backendName := flag.String("backendName", "florebackend", "后端可执行文件名（不含扩展名）")
	feBin := flag.String("feBin", "", "前端二进制路径（默认 <binDir>/<appName>[.exe]）；macOS 传 <binDir>/Flore.app")
	beBin := flag.String("beBin", "", "后端二进制路径（默认 <binDir>/<backendName>[.exe]）")
	setupBin := flag.String("setupBin", "", "NSIS 安装器路径（-edition setup 时必填）")
	clean := flag.Bool("clean", true, "构建完成后清理便携目录（仅 portable edition）")
	flag.Parse()

	if *version == "" {
		fatal("--version 必填")
	}

	switch *edition {
	case "portable":
		buildPortable(version, osArg, arch, binDir, outDir, portableDir, appName, backendName, feBin, beBin, clean)
	case "setup":
		buildSetup(version, osArg, arch, outDir, setupBin)
	default:
		fatal("不支持的 edition: %s（仅支持 portable|setup）", *edition)
	}
}

func buildPortable(version, osArg, arch, binDir, outDir, portableDir, appName, backendName, feBin, beBin *string, clean *bool) {
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

	// 拷贝后端二进制
	bePath := *beBin
	if bePath == "" {
		n := *backendName
		if *osArg == "windows" {
			n += ".exe"
		}
		bePath = filepath.Join(*binDir, n)
	}
	if _, err := os.Stat(bePath); err != nil {
		fatal("后端二进制未找到: %s（请先 build:go）", bePath)
	}
	if err := copyPath(bePath, filepath.Join(portablePath, filepath.Base(bePath))); err != nil {
		fatal("拷贝后端二进制失败: %v", err)
	}

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
