package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DownloadAsset 按 urls 顺序尝试下载资产到 dest，并校验 sha256，全部失败返回错误。
func DownloadAsset(asset *Asset, dest string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	var lastErr error
	for _, u := range asset.URLs {
		if err := downloadAndVerify(client, u, asset, dest); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("所有下载地址均失败: %w", lastErr)
}

func downloadAndVerify(client *http.Client, url string, asset *Asset, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 返回 HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, asset.SHA256) {
		_ = os.Remove(dest)
		return fmt.Errorf("sha256 校验失败: 实际 %s 期望 %s", got, asset.SHA256)
	}
	return nil
}
