// Toast notification utility - non-blocking replacement for alert()
type ToastType = 'success' | 'error' | 'info';

interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
  timerId?: ReturnType<typeof setTimeout>;
}

// 最大同时显示的 toast 数量，避免堆积
const MAX_TOASTS = 5;

// 内部状态封装为闭包，外部仅通过 showToast/getToasts/addListener/removeListener 访问
const toastStore = (() => {
  let toasts: ToastItem[] = [];
  let listeners: (() => void)[] = [];
  let nextId = 0;
  // suppressed 为 true 时静默丢弃 toast：用于后端未就绪（connecting/offline）时
  // 抑制每个请求失败触发的「获取…失败」toast 洪水，改由全局横幅统一提示（P0-002）。
  let suppressed = false;

  function notifyListeners(): void {
    listeners.forEach((fn) => {
      try {
        fn();
      } catch {
        // 单个 listener 失败不影响其他
      }
    });
  }

  function removeToast(id: number): void {
    const idx = toasts.findIndex((t) => t.id === id);
    if (idx === -1) return;
    const removed = toasts[idx];
    if (removed?.timerId) {
      clearTimeout(removed.timerId);
    }
    toasts = toasts.filter((t) => t.id !== id);
    notifyListeners();
  }

  function trimOldestIfExceeded(): void {
    if (toasts.length <= MAX_TOASTS) return;
    const oldest = toasts[0];
    if (oldest) removeToast(oldest.id);
  }

  return {
    add(message: string, type: ToastType, duration: number): number {
      if (suppressed) return -1;
      const id = ++nextId;
      const item: ToastItem = { id, message, type };
      item.timerId = setTimeout(() => removeToast(id), duration);
      toasts.push(item);
      trimOldestIfExceeded();
      notifyListeners();
      return id;
    },
    getAll(): ToastItem[] {
      return toasts;
    },
    setSuppressed(value: boolean): void {
      suppressed = value;
    },
    subscribe(fn: () => void): void {
      listeners.push(fn);
    },
    unsubscribe(fn: () => void): void {
      listeners = listeners.filter((l) => l !== fn);
    },
  };
})();

export function showToast(message: string, type: ToastType = 'info', duration: number = 3000): void {
  toastStore.add(message, type, duration);
}

/** 设置 toast 抑制开关：后端未就绪时置 true，抑制每个请求失败的 toast 洪水（P0-002）。 */
export function setSuppressed(value: boolean): void {
  toastStore.setSuppressed(value);
}

export function getToasts(): ToastItem[] {
  return toastStore.getAll();
}

export function addListener(fn: () => void): void {
  toastStore.subscribe(fn);
}

export function removeListener(fn: () => void): void {
  toastStore.unsubscribe(fn);
}
