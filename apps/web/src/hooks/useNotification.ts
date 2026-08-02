import { useEffect, useCallback } from 'react';
import { getDesktopApp } from '../utils/api.js';
import { showToast } from '../utils/toast';

// 系统通知标题（抽为常量，避免散落硬编码，n-02）
export const NOTIFICATION_TITLE = 'Flore 新文章';

interface UseNotificationParams {
  notifyEnabled: boolean;
  notifyBatchMin: number;
  unreadCountInScope: number;
}

// 抓取完成后的系统通知逻辑与权限请求。
// 桌面版（Wails）由桌面壳 startNotifyWatcher 统一发送原生 Toast，此处直接返回避免重复；
// Web 版回退到浏览器 Notification API（m-02/m-03 移除 WebView2 定时器与硬编码标题）。
export function useNotification({ notifyEnabled, notifyBatchMin, unreadCountInScope }: UseNotificationParams) {
  // 请求通知权限（仅 Web 版；桌面版由原生 Toast 处理，无权限概念）
  useEffect(() => {
    if (getDesktopApp()) return;
    if (notifyEnabled && 'Notification' in window && Notification.permission === 'default') {
      // 首次进入时静默预请求，失败不打扰用户；真正需要通知时会再次请求并提示
      Notification.requestPermission().catch((err) => {
        console.error('Failed to request notification permission:', err);
      });
    }
  }, [notifyEnabled]);

  const notifyFetchComplete = useCallback(async (newItems: number) => {
    if (!notifyEnabled || newItems <= 0) return;

    // 桌面版通知交由原生 Toast 处理，不在此重复发送
    if (getDesktopApp()) return;

    if (!('Notification' in window)) {
      console.warn('[notification] Web Notification API unavailable'); // m-08
      return;
    }
    if (Notification.permission === 'denied') {
      showToast('通知权限已被拒绝，无法推送新文章通知'); // m-01 反馈
      return;
    }
    if (Notification.permission === 'default') {
      try {
        const perm = await Notification.requestPermission();
        if (perm !== 'granted') {
          showToast('未授予通知权限，无法推送新文章通知'); // m-01
          return;
        }
      } catch (err) {
        console.error('Failed to request notification permission:', err);
        showToast('通知权限请求失败，无法推送新文章通知');
        return;
      }
    }
    try {
      const batchMin = notifyBatchMin || 5;
      const body = newItems >= batchMin
        ? `新增 ${newItems} 篇文章，共 ${unreadCountInScope} 篇未读`
        : `新增 ${newItems} 篇文章`;
      new Notification(NOTIFICATION_TITLE, { body });
    } catch (err) {
      console.warn('[notification] failed to show', err); // m-08
    }
  }, [notifyEnabled, notifyBatchMin, unreadCountInScope]);

  return { notifyFetchComplete };
}
