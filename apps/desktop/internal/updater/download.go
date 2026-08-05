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
	// 大小双保险：与 manifest 声明 size 精确比对（防篡改清单谎报大小），
	// 另设 2GB 绝对上限（防 zip-bomb，即使清单被伪造也能拦截）。
	const maxUpdateAssetBytes = 2 << 30
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if n > maxUpdateAssetBytes {
		_ = os.Remove(dest)
		return fmt.Errorf("下载文件超过大小上限: %d bytes", maxUpdateAssetBytes)
	}
	if asset.Size > 0 && n != asset.Size {
		_ = os.Remove(dest)
		return fmt.Errorf("下载文件大小与清单不符: 实际 %d 期望 %d", n, asset.Size)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, asset.SHA256) {
		_ = os.Remove(dest)
		return fmt.Errorf("sha256 校验失败: 实际 %s 期望 %s", got, asset.SHA256)
	}

	// 签名校验：确认文件由持私钥的发布方签发（SHA256 只保证完整性，不证明发布者身份）。
	// 验签失败同样删除已下载文件并拒绝更新，防止被篡改的安装包落地执行。
	if err := verifyAssetSignature(asset, h.Sum(nil)); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}
