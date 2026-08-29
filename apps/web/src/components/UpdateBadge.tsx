import { useEffect, useRef, useState } from 'react';
import { X, DownloadIcon } from './icons';
import { startUpdate, getUpdateProgress, type UpdateInfo } from '../utils/api';
import { showToast } from '../utils/toast';

interface Props {
  update: UpdateInfo;
  onDismiss: () => void;
}

// 新版本角落轻提示（T-v03-02）：不打断阅读，点击直接更新，下载进度内联展示。
// 更新失败走 toast 提示，引导用户到设置-关于手动下载。
export default function UpdateBadge({ update, onDismiss }: Props) {
  const [updating, setUpdating] = useState(false);
  const [progress, setProgress] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  const handleUpdate = async () => {
    if (updating) return;
    setUpdating(true);
    setProgress(0);
    try {
      await startUpdate();
      // 下载进行中：轮询进度直至进程重启
      timerRef.current = setInterval(async () => {
        try {
          setProgress(await getUpdateProgress());
        } catch { /* 静默 */ }
      }, 400);
    } catch (err) {
      setUpdating(false);
      console.error('Update failed:', err);
      showToast('自动更新失败，可到 设置-关于 手动下载');
    }
  };

  const pct = Math.min(100, Math.max(0, Math.round(progress * 100)));

  return (
    <div className="fixed bottom-4 right-4 z-[1200] flex items-center gap-2.5 rounded-full border border-border bg-surface py-1.5 pl-3.5 pr-2 shadow-md animate-modal-scale-in">
      {updating ? (
        <span className="text-[12.5px] text-secondary" role="status">
          正在下载更新… {pct}%
        </span>
      ) : (
        <button
          type="button"
          onClick={handleUpdate}
          className="flex items-center gap-1.5 text-[12.5px] text-primary underline-offset-2 transition-colors hover:underline focus:outline-none"
        >
          <DownloadIcon size={14} />
          <span>发现新版本 v{update.latestVersion}，点击更新</span>
        </button>
      )}
      {!updating && (
        <button
          type="button"
          onClick={onDismiss}
          title="本次启动内不再提示"
          aria-label="关闭更新提示"
          className="flex h-5 w-5 items-center justify-center rounded-full text-muted transition-colors hover:bg-hover hover:text-primary focus:outline-none"
        >
          <X size={13} />
        </button>
      )}
    </div>
  );
}
