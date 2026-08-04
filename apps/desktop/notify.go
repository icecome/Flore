package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

func (a *App) registerNotificationApp() {
	if err := toast.SetAppData(toast.AppData{AppID: "Flore"}); err != nil {
		a.logger.Printf("register notification app failed: %v", err)
	}
}

// ShowNotification 发送原生 Windows Toast 通知（应用名显示为 Flore）。
// 供后台通知监听统一使用；非 Windows 平台为 noop。
func (a *App) ShowNotification(title, body string) error {
	if a.context() == nil {
		return fmt.Errorf("app context not ready")
	}
	if title == "" {
		title = "Flore"
	}
	ntf := toast.Notification{
		AppID: "Flore",
		Title: title,
		Body:  body,
	}
	err := ntf.Push()
	if err != nil {
		a.logger.Printf("show notification failed: %v", err)
	}
	return err
}

// startNotifyWatcher 后台轮询后端抓取状态，当检测到一轮抓取完成（fetching 下降沿）
// 且本轮有新增文章时发送原生系统通知。覆盖手动刷新、托盘抓取与后台调度三路场景，
// 消除此前仅前端点击才发通知的盲区（M-A3）。
// ctx 被 cancel 时优雅退出（由 shutdown 触发）。
func (a *App) startNotifyWatcher(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var lastFetching bool
	for {
		select {
		case <-ctx.Done():
			a.logger.Printf("notify watcher: stopped")
			return
		case <-ticker.C:
		}
		port := a.getPort()
		if port == 0 {
			lastFetching = false
			continue
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/api/sources/fetch-status", port)
		resp, err := a.doRequest(httpClient, http.MethodGet, url, nil)
		if err != nil {
			lastFetching = false
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		drainAndClose(resp)
		if err != nil {
			lastFetching = false
			continue
		}
		var st struct {
			Fetching bool `json:"fetching"`
			NewItems int  `json:"newItems"`
		}
		if err := json.Unmarshal(body, &st); err != nil {
			lastFetching = false
			continue
		}
		// 下降沿：本轮抓取刚完成，且确有新增文章，且用户开启了通知
		if lastFetching && !st.Fetching && st.NewItems > 0 && a.getNotifyEnabled() {
			if err := a.ShowNotification("Flore 新文章", fmt.Sprintf("抓取到 %d 篇新文章", st.NewItems)); err != nil {
				a.logger.Printf("notify watcher: failed to show notification: %v", err)
			}
		}
		lastFetching = st.Fetching
	}
}
