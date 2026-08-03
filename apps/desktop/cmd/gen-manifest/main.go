// gen-manifest 读取构建产物目录，计算各平台资产的 sha256，并生成 update.json。
// 后续由 CI（release.yml）调用，将产物上传 R2 后发布该 manifest。
//
// 文件名规范：flore-<edition>-<os>-<arch>-<version>.<ext>
//
//	edition: portable|setup
//	os: windows|darwin|linux
//	arch: amd64|arm64
//	ext: zip|exe|dmg|deb
//
// 用法示例：
//
//	go run ./cmd/gen-manifest \
//	  -version 0.1.0-20260803 \
//	  -dir ./dist \
//	  -baseURL https://cdn-dl.icecome.com \
//	  -githubRepo icecome/Flore \
//	  -notes "修复若干已知问题" \
//	  -platforms windows \
//	  -variant setup \
//	  -out update.json
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// assetNameRe 匹配 flore-<edition>-<os>-<arch>-<version>.<ext>
var assetNameRe = regexp.MustCompile(`^flore-(portable|setup)-(windows|darwin|linux)-(amd64|arm64)-(.+)\.(zip|exe|dmg|deb)$`)

type asset struct {
	Platform  string   `json:"platform"`
	Variant   string   `json:"variant"`
	FileName  string   `json:"fileName"`
	Size      int64    `json:"size"`
	SHA256    string   `json:"sha256"`
	Signature string   `json:"signature"`
	URLs      []string `json:"urls"`
}

type manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	App           string  `json:"app"`
	Latest        string  `json:"latest"`
	MinSupported  string  `json:"minSupported"`
	PublishedAt   string  `json:"publishedAt"`
	Notes         string  `json:"notes"`
	Assets        []asset `json:"assets"`
}

func main() {
	version := flag.String("version", "", "发布版本号（必填，如 0.1.0-20260803）")
	dir := flag.String("dir", "dist", "构建产物目录，内含各平台分发包")
	baseURL := flag.String("baseURL", "https://cdn-dl.icecome.com", "R2/CDN 基础地址（manifest 主 URL 前缀）")
	githubRepo := flag.String("githubRepo", "", "GitHub 仓库 owner/name，用于生成 Release 兜底 URL（可选）")
	notes := flag.String("notes", "", "更新说明（Markdown 文本）")
	minSupported := flag.String("minSupported", "", "最低支持版本，低于此版本强制更新（可选）")
	platforms := flag.String("platforms", "", "仅纳入指定平台（逗号分隔的 goos，如 windows）；为空则纳入全部")
	variant := flag.String("variant", "", "仅纳入指定 variant（portable|setup）；为空则纳入全部")
	out := flag.String("out", "update.json", "输出文件路径")
	flag.Parse()

	if *version == "" {
		fatal("--version 必填")
	}
	if *dir == "" {
		fatal("--dir 必填")
	}

	entries, err := os.ReadDir(*dir)
	if err != nil {
		fatal("读取产物目录失败: %v", err)
	}

	var assets []asset
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)

		m := assetNameRe.FindStringSubmatch(lower)
		if m == nil {
			// 非匹配格式，静默跳过（如 Flore-portable-*.zip 等归档副本）
			continue
		}

		edition := m[1]  // portable|setup
		osName := m[2]   // windows|darwin|linux
		archName := m[3] // amd64|arm64
		platform := osName + "/" + archName

		if *platforms != "" && !platformInList(platform, *platforms) {
			fmt.Fprintf(os.Stderr, "跳过非目标平台资产: %s (%s)\n", name, platform)
			continue
		}
		if *variant != "" && edition != *variant {
			fmt.Fprintf(os.Stderr, "跳过非目标 variant %s 资产: %s (%s)\n", *variant, name, edition)
			continue
		}

		full := filepath.Join(*dir, name)
		sum, size, err := sha256AndSize(full)
		if err != nil {
			fatal("计算 %s 校验和失败: %v", name, err)
		}

		urls := []string{
			fmt.Sprintf("%s/v%s/%s", strings.TrimRight(*baseURL, "/"), *version, name),
		}
		if *githubRepo != "" {
			urls = append(urls, fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", *githubRepo, *version, name))
		}

		assets = append(assets, asset{
			Platform: platform,
			Variant:  edition,
			FileName: name,
			Size:     size,
			SHA256:   sum,
			URLs:     urls,
		})
	}

	if len(assets) == 0 {
		fatal("在 %s 中未找到任何 flore-*.zip 或 flore-*.exe 资产", *dir)
	}

	m := manifest{
		SchemaVersion: 1,
		App:           "flore",
		Latest:        *version,
		MinSupported:  *minSupported,
		PublishedAt:   time.Now().UTC().Format(time.RFC3339),
		Notes:         *notes,
		Assets:        assets,
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fatal("序列化 manifest 失败: %v", err)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		fatal("写入 %s 失败: %v", *out, err)
	}
	fmt.Printf("已生成 %s，包含 %d 个资产\n", *out, len(assets))
}

func sha256AndSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	if _, err := copyHash(h, f); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}

func copyHash(h interface{ Write([]byte) (int, error) }, src *os.File) (int64, error) {
	buf := make([]byte, 1<<20)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gen-manifest: "+format+"\n", args...)
	os.Exit(1)
}

// platformInList 判断 platform（如 windows/amd64）是否属于逗号分隔的 goos 列表（如 windows,darwin）。
func platformInList(platform, list string) bool {
	goos := strings.Split(platform, "/")[0]
	for _, p := range strings.Split(list, ",") {
		if strings.TrimSpace(p) == goos {
			return true
		}
	}
	return false
}
