import { useEffect, useState } from 'react';
import { X, DownloadIcon, RestartIcon } from './icons';
import useUpdate from '../hooks/useUpdate';
import { showToast } from '../utils/toast';

interface Props {
  update: { latestVersion: string };
  onDismiss: () => void;
}

// 新版本角落轻提示（T-v03-02）：不打断阅读，点击直接更新，下载进度内联展示。
// 状态统一来自全局 useUpdate 仓库：available（提示）→ downloading（进度）→
// ready（确认重启）→ error（指引手动下载）。弹窗关闭不影响后台下载与进度展示。
export default function UpdateBadge({ update, onDismiss }: Props) {
  const { snapshot, download, restart } = useUpdate();
  const [dismissed, setDismissed] = useState(false);

  // 状态回归 available（如下载失败后复位）且本组件已展示过下载视图时，不再重复闪现
  useEffect(() => {
    if (snapshot.status === 'downloading' || snapshot.status === 'ready') {
      setDismissed(false);
    }
  }, [snapshot.status]);

  const handleUpdate = async () => {
    try {
      await download();
    } catch (err) {
      showToast('自动更新失败，可到 设置-关于 手动下载', 'error');
    }
  };

  const handleRestart = async () => {
    try {
      await restart();
    } catch (err) {
      showToast('重启应用以完成更新失败', 'error');
    }
  };

  // 已关闭且当前不是下载进行中/待重启，则隐藏
  if (dismissed && snapshot.status !== 'downloading' && snapshot.status !== 'ready') {
    return null;
  }

  const pct = Math.min(100, Math.max(0, Math.round(snapshot.progress * 100)));

  return (
    <div className="fixed bottom-4 right-4 z-[1200] flex items-center gap-2.5 rounded-full border border-border bg-surface py-1.5 pl-3.5 pr-2 shadow-md animate-modal-scale-in">
      {snapshot.status === 'downloading' && (
        <span className="text-[12.5px] text-secondary" role="status">
          正在下载更新… {pct}%
        </span>
      )}
      {snapshot.status === 'ready' && (
        <button
          type="button"
          onClick={handleRestart}
          className="flex items-center gap-1.5 text-[12.5px] text-primary underline-offset-2 transition-colors hover:underline focus:outline-none"
        >
          <RestartIcon size={14} />
          <span>更新已就绪，点击重启应用</span>
        </button>
      )}
      {snapshot.status === 'error' && (
        <span className="text-[12.5px] text-danger">更新失败，可到 设置-关于 手动下载</span>
      )}
      {(snapshot.status === 'available' || snapshot.status === 'idle') && !dismissed && (
        <button
          type="button"
          onClick={handleUpdate}
          className="flex items-center gap-1.5 text-[12.5px] text-primary underline-offset-2 transition-colors hover:underline focus:outline-none"
        >
          <DownloadIcon size={14} />
          <span>
            发现新版本 v{update.latestVersion}
            {snapshot.info?.forced ? '（版本过低，请尽快更新）' : ''}，点击更新
          </span>
        </button>
      )}
      {snapshot.status !== 'downloading' && snapshot.status !== 'ready' && !dismissed && (
        <button
          type="button"
          onClick={() => {
            setDismissed(true);
            onDismiss();
          }}
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