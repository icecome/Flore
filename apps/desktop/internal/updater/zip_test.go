package updater

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestUnzipSafeSkipsUserData 验证 unzipSafe 跳过顶层 data/ 与 backups/，
// 避免更新覆盖用户数据与备份；同时防御 zip slip（路径穿越）。
func TestUnzipSafeSkipsUserData(t *testing.T) {
	src, err := os.MkdirTemp("", "uzsrc-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(src)

	zipPath := filepath.Join(src, "t.zip")
	makeTestZip(t, zipPath)

	dest, err := os.MkdirTemp("", "uzdst-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dest)

	if err := unzipSafe(zipPath, dest); err != nil {
		t.Fatalf("unzipSafe failed: %v", err)
	}

	// 应被解出的文件
	for _, f := range []string{"Flore.exe", "florebackend.exe", filepath.Join("webview2", ".keep")} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("expected %s extracted: %v", f, err)
		}
	}
	// 用户数据目录必须被跳过（不被覆盖）
	for _, d := range []string{"data", "backups"} {
		if _, err := os.Stat(filepath.Join(dest, d)); err == nil {
			t.Errorf("user data dir %s must be skipped", d)
		}
	}
	// 防 zip slip：escape.txt 不应出现在 dest 根（路径穿越被拒绝）
	if _, err := os.Stat(filepath.Join(dest, "escape.txt")); err == nil {
		t.Errorf("zip slip path should not be extracted")
	}
}

// makeTestZip 构造含正常文件与一条路径穿越 entry 的测试 zip。
// entry 名直接以字符串写入，不经由文件系统路径规范化，以真实模拟 zip slip 攻击载荷。
func makeTestZip(t *testing.T, out string) {
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	add := func(rel, content string) {
		fw, err := w.Create(rel)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	add("Flore.exe", "payload")
	add("florebackend.exe", "payload")
	add("webview2/.keep", "")
	add("data/.keep", "")
	add("backups/.keep", "")
	add("evil/../escape.txt", "slip") // 路径穿越，应被拒绝
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
