package updater

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unzipSafe 解压 zip 到 dest，跳过顶层 data/ 与 backups/ 目录以保护用户数据。
func unzipSafe(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := f.Name
		// 统一为 "/" 分隔，兼容 Windows 工具写出 "\" 分隔的 entry 名。
		name = filepath.ToSlash(name)
		// 阻止 zip slip：仅允许相对路径，且首段不得为 ".."
		if strings.HasPrefix(name, "..") || strings.Contains(name, "../") {
			continue
		}
		// 跳过顶层 data/ 与 backups/，避免覆盖用户数据与备份
		top := strings.SplitN(name, "/", 2)[0]
		if top == "data" || top == "backups" {
			continue
		}
		target := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}
