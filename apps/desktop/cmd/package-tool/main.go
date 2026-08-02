// package-tool 跨平台生成 Flore 便携分发包，替代原 Windows 专用的 package-portable.ps1。
//
// 产出两类 zip（均不含顶层包裹目录，zip 内顶层即可执行文件与 data/ backups/ webview2/）：
//   - flore-<os>-<arch>.zip        ：平台化命名，供 gen-manifest 解析与 R2/GitHub 上传
//   - Flore-portable-<version>.zip ：带版本号归档，便于人工下载与追溯
//
// 用法示例：
//
//	go run ./cmd/package-tool -version 0.0.1-20260802 -os windows -arch amd64
//	go run ./cmd/package-tool -version 0.0.1-20260802 -os darwin -arch arm64 -feBin build/bin/Flore.app
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
	version := flag.String("version", "", "发布版本号（必填，如 0.0.1-20260802）")
	osArg := flag.String("os", runtime.GOOS, "目标操作系统（默认当前）")
	arch := flag.String("arch", runtime.GOARCH, "目标架构（默认当前）")
	binDir := flag.String("binDir", "build/bin", "含前后端二进制的目录")
	outDir := flag.String("outDir", "build", "zip 输出目录")
	appName := flag.String("appName", "Flore", "前端可执行文件名（不含扩展名）")
	backendName := flag.String("backendName", "florebackend", "后端可执行文件名（不含扩展名）")
	feBin := flag.String("feBin", "", "前端二进制路径（默认 <binDir>/<appName>[.exe]）；macOS 传 <binDir>/Flore.app")
	beBin := flag.String("beBin", "", "后端二进制路径（默认 <binDir>/<backendName>[.exe]）")
	flag.Parse()

	if *version == "" {
		fatal("--version 必填")
	}

	fePath := *feBin
	if fePath == "" {
		n := *appName
		if *osArg == "windows" {
			n += ".exe"
		}
		fePath = filepath.Join(*binDir, n)
	}
	bePath := *beBin
	if bePath == "" {
		n := *backendName
		if *osArg == "windows" {
			n += ".exe"
		}
		bePath = filepath.Join(*binDir, n)
	}

	if _, err := os.Stat(fePath); err != nil {
		fatal("前端二进制未找到: %s（请先 wails build）", fePath)
	}
	if _, err := os.Stat(bePath); err != nil {
		fatal("后端二进制未找到: %s（请先 build:go）", bePath)
	}

	// 临时便携目录：可执行文件 + data/ backups/ webview2/ 占位
	tmp, err := os.MkdirTemp("", "flore-portable")
	if err != nil {
		fatal("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmp)

	if err := copyPath(fePath, filepath.Join(tmp, filepath.Base(fePath))); err != nil {
		fatal("拷贝前端二进制失败: %v", err)
	}
	if err := copyPath(bePath, filepath.Join(tmp, filepath.Base(bePath))); err != nil {
		fatal("拷贝后端二进制失败: %v", err)
	}
	for _, d := range []string{"data", "backups", "webview2"} {
		p := filepath.Join(tmp, d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			fatal("创建 %s 失败: %v", d, err)
		}
		if err := os.WriteFile(filepath.Join(p, ".keep"), []byte{}, 0o644); err != nil {
			fatal("写入 %s/.keep 失败: %v", d, err)
		}
	}

	zipName := fmt.Sprintf("flore-%s-%s.zip", *osArg, *arch)
	zipPath := filepath.Join(*outDir, zipName)
	if err := zipDir(tmp, zipPath); err != nil {
		fatal("生成 %s 失败: %v", zipName, err)
	}

	archiveName := fmt.Sprintf("Flore-portable-%s.zip", *version)
	archivePath := filepath.Join(*outDir, archiveName)
	if err := zipDir(tmp, archivePath); err != nil {
		fatal("生成 %s 失败: %v", archiveName, err)
	}

	fmt.Printf("已生成 %s 与 %s\n", zipName, archiveName)
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
	_, err = io.Copy(out, in)
	return err
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
		if info.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
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
