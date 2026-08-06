package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 下载相关常量
const (
	chunkSize           int64         = 4 << 20 // 单分片大小：4MB
	maxConcurrency      = 4              // 并行分片数
	chunkRetries        = 3              // 单分片失败重试次数（每轮遍历全部下载源）
	chunkTimeout        = 60 * time.Second // 单分片请求超时（含连接与读取，防 stall 卡死）
	overallTimeout      = 30 * time.Minute // 整体超时兜底
	maxUpdateAssetBytes = 2 << 30            // 2GB 绝对上限（防 zip-bomb）
	minProgressStep     = 0.02               // 进度回调最小步进，避免过频
)

// errNoRange 表示所有下载源都不支持 HTTP Range，需退化为单流整文件下载。
var errNoRange = errors.New("下载源不支持 HTTP Range")

// DownloadAsset 兼容旧调用：无进度回调的分片下载。
func DownloadAsset(asset *Asset, dest string) error {
	return DownloadAssetProgress(asset, dest, nil)
}

// DownloadAssetProgress 分片并行下载资产到 dest，具备：
//   - HTTP Range 断点续传（分片级：中断后重跑可跳过已完成的片）
//   - 每分片独立超时与重试；超时/失败后切换到下一个下载源(asset.URLs)
//   - 所有源均不可用时返回错误，由调用方提示用户手动下载
//   - 下载完成后校验 sha256 与 Ed25519 签名
//
// onProgress 报告 0~1 的下载进度，可为 nil。
func DownloadAssetProgress(asset *Asset, dest string, onProgress func(float64)) error {
	if onProgress != nil {
		onProgress(0)
	}
	client := newHTTPClient()
	size, err := resolveSize(asset, client)
	if err != nil {
		return err
	}
	// 无法获知可靠大小或超出上限：退化为单流整文件下载（仍带超时与多源切换）。
	if size <= 0 || size > maxUpdateAssetBytes {
		return downloadSingleStream(asset, dest, client, onProgress)
	}

	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()

	partDir := dest + ".parts"
	if err := os.MkdirAll(partDir, 0o755); err != nil {
		return fmt.Errorf("创建分片目录失败: %w", err)
	}

	chunks := splitChunks(size, chunkSize)
	total := int64(len(chunks))
	var completed int64
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, c := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			partPath := filepath.Join(partDir, strconv.Itoa(i)+".part")
			if err := downloadChunk(ctx, client, asset, size, c, partPath); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				cancel()
				return
			}
			mu.Lock()
			completed++
			if onProgress != nil {
				onProgress(float64(completed) / float64(total))
			}
			mu.Unlock()
		}(i, c)
	}
	wg.Wait()

	if firstErr != nil {
		// 保留 partDir 以便后续续传；清理可能不完整的 dest
		if err := os.Remove(dest); err != nil {
			fmt.Fprintf(os.Stderr, "[updater] 清理不完整文件 %s: %v\n", dest, err)
		}
		if errors.Is(firstErr, errNoRange) {
			// 所有源均不支持 Range：退化为单流整文件下载
			return downloadSingleStream(asset, dest, client, onProgress)
		}
		return fmt.Errorf("分片下载失败: %w", firstErr)
	}

	if err := assemble(dest, partDir, chunks); err != nil {
		if rerr := os.Remove(dest); rerr != nil {
			fmt.Fprintf(os.Stderr, "[updater] 清理不完整文件 %s: %v\n", dest, rerr)
		}
		return err
	}
	if err := os.RemoveAll(partDir); err != nil {
		fmt.Fprintf(os.Stderr, "[updater] 清理分片目录 %s: %v\n", partDir, err)
	}

	if err := verifyFile(dest, asset); err != nil {
		if rerr := os.Remove(dest); rerr != nil {
			fmt.Fprintf(os.Stderr, "[updater] 清理校验失败文件 %s: %v\n", dest, rerr)
		}
		return err
	}
	if onProgress != nil {
		onProgress(1)
	}
	return nil
}

type chunk struct {
	index int
	start int64
	end   int64 // 含端点
}

func splitChunks(size, cs int64) []chunk {
	n := (size + cs - 1) / cs
	chunks := make([]chunk, 0, n)
	for i := int64(0); i < n; i++ {
		start := i * cs
		end := start + cs - 1
		if end >= size {
			end = size - 1
		}
		chunks = append(chunks, chunk{index: int(i), start: start, end: end})
	}
	return chunks
}

// resolveSize 确定资产总字节数：优先用 manifest 声明的 asset.Size；
// 若缺失则对各下载源发 HEAD 取 Content-Length。
func resolveSize(asset *Asset, client *http.Client) (int64, error) {
	if asset.Size > 0 {
		return asset.Size, nil
	}
	for _, u := range asset.URLs {
		ctx, cancel := context.WithTimeout(context.Background(), chunkTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		cl := resp.Header.Get("Content-Length")
		resp.Body.Close()
		cancel()
		if cl != "" {
			if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > 0 {
				return n, nil
			}
		}
	}
	return 0, nil
}

// downloadChunk 下载单个分片：先判断已完整则跳过（续传），否则按
// 重试次数遍历所有下载源；每源带独立超时，超时/失败即切下一源。
func downloadChunk(ctx context.Context, client *http.Client, asset *Asset, total int64, c chunk, partPath string) error {
	// 续传：分片已完整则跳过
	if fi, err := os.Stat(partPath); err == nil && fi.Size() == (c.end-c.start+1) {
		return nil
	}
	lastErr := fmt.Errorf("无可用下载源")
	for attempt := 0; attempt < chunkRetries; attempt++ {
		for _, u := range asset.URLs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := fetchRange(ctx, client, u, total, c, partPath); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("分片 %d 下载失败: %w", c.index, lastErr)
}

// fetchRange 对单个源发起带 Range 的 GET，写入 partPath（按分片偏移 seek）。
// 要求服务端返回 206；若返回 200 整文件则仅在「本片即整个文件」时接受，否则报错（切下一源）。
func fetchRange(ctx context.Context, client *http.Client, url string, total int64, c chunk, partPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", c.start, c.end))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
		// 正常分片响应
	case http.StatusOK:
		// 仅当本分片就是整个文件（单分片资产）时接受 200
		if c.start != 0 || c.end != total-1 {
			return errNoRange
		}
	default:
		return fmt.Errorf("下载 %s 返回 HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(partPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(c.start, io.SeekStart); err != nil {
		return err
	}
	limit := c.end - c.start + 1
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit))
	if err != nil {
		return err
	}
	if n != limit {
		return fmt.Errorf("分片 %d 写入字节数不符: 实际 %d 期望 %d", c.index, n, limit)
	}
	return nil
}

// downloadSingleStream 退化路径：单流整文件下载，带每读超时、多源切换与进度。
// 用于无法获知大小或下载源不支持 Range 的场景。
func downloadSingleStream(asset *Asset, dest string, client *http.Client, onProgress func(float64)) error {
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < chunkRetries; attempt++ {
		for _, u := range asset.URLs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := fetchFull(ctx, client, u, asset, dest, onProgress); err != nil {
				lastErr = err
				continue
			}
			if err := verifyFile(dest, asset); err != nil {
				if rerr := os.Remove(dest); rerr != nil {
					fmt.Fprintf(os.Stderr, "[updater] 清理校验失败文件 %s: %v\n", dest, rerr)
				}
				lastErr = err
				continue
			}
			if onProgress != nil {
				onProgress(1)
			}
			return nil
		}
	}
	return fmt.Errorf("下载失败（所有源已尝试）: %w", lastErr)
}

// fetchFull 单流下载整文件，边下边报告进度（按 manifest 声明大小）。
func fetchFull(ctx context.Context, client *http.Client, url string, asset *Asset, dest string, onProgress func(float64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 返回 HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	var written int64
	var lastp float64
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if asset.Size > 0 && onProgress != nil {
				p := float64(written) / float64(asset.Size)
				if p-lastp >= minProgressStep || p >= 1 {
					onProgress(p)
					lastp = p
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
		if written > maxUpdateAssetBytes {
			return fmt.Errorf("下载文件超过大小上限")
		}
	}
	if asset.Size > 0 && written != asset.Size {
		return fmt.Errorf("下载文件不完整: 实际 %d 字节, 期望 %d 字节", written, asset.Size)
	}
	return nil
}

// assemble 将分片按序拼接为目标文件。
func assemble(dest, partDir string, chunks []chunk) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, c := range chunks {
		partPath := filepath.Join(partDir, strconv.Itoa(c.index)+".part")
		pf, err := os.Open(partPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, pf); err != nil {
			pf.Close()
			return err
		}
		pf.Close()
	}
	return nil
}

// verifyFile 校验下载文件的尺寸、sha256 与 Ed25519 签名。
func verifyFile(dest string, asset *Asset) error {
	f, err := os.Open(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if asset.Size > 0 && info.Size() != asset.Size {
		return fmt.Errorf("下载文件大小与清单不符: 实际 %d 期望 %d", info.Size(), asset.Size)
	}
	if info.Size() > maxUpdateAssetBytes {
		return fmt.Errorf("下载文件超过大小上限: %d bytes", info.Size())
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, asset.SHA256) {
		return fmt.Errorf("sha256 校验失败: 实际 %s 期望 %s", got, asset.SHA256)
	}
	if err := verifyAssetSignature(asset, h.Sum(nil)); err != nil {
		return err
	}
	return nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: chunkTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
		},
	}
}
