import { useEffect, useState } from 'react';
import { getVersion, checkForUpdate, startUpdate, getCachedUpdate, getUpdateProgress, isDesktop, openExternal } from '../../utils/api';

// 项目仓库地址（开源地址，非敏感配置）
const REPO_URL = 'https://github.com/icecome/Flore';
import type { UpdateInfo } from '../../utils/api';
import { Section, SmallBtn } from './SettingsShared';
import { showToast } from '../../utils/toast';

const desktop = isDesktop();

export default function SettingsAboutTab() {
  const [version, setVersion] = useState('...');
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [progress, setProgress] = useState(0);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [updateManualUrl, setUpdateManualUrl] = useState<string | null>(null);

  // 挂载时读取版本号 + 后台缓存更新结果
  useEffect(() => {
    let cancelled = false;

    // 版本号获取失败仅降级展示"未知"，不打扰用户
    getVersion()
      .then((data) => { if (!cancelled) setVersion(data.version || '未知'); })
      .catch((err) => {
        console.error('Failed to fetch version:', err);
        if (!cancelled) setVersion('未知');
      });

    // 桌面端读取后台缓存的更新结果；缓存为空时静默补一次检查（不弹 toast）
    if (desktop) {
      (async () => {
        const cached = await getCachedUpdate();
        if (cancelled) return;
        if (cached) {
          setUpdate(cached);
          return;
        }
        try {
          const info = await checkForUpdate();
          if (!cancelled && info) setUpdate(info);
        } catch { /* 静默 */ }
      })();
    }

    return () => { cancelled = true; };
  }, []);

  // 更新下载中时轮询进度
  useEffect(() => {
    if (!desktop || !updating) return;
    const timer = setInterval(async () => {
      try {
        const p = await getUpdateProgress();
        setProgress(p);
      } catch { /* 静默 */ }
    }, 400);
    return () => clearInterval(timer);
  }, [updating]);

  const handleCheck = async () => {
    setChecking(true);
    setCheckError(null);
    setUpdateError(null);
    setUpdate(null);
    try {
      const info = await checkForUpdate();
      if (!info) {
        showToast('已是最新版本');
      } else {
        setUpdate(info);
      }
    } catch (err) {
      console.error('检查更新失败:', err);
      setCheckError('检查更新失败，请稍后重试');
    } finally {
      setChecking(false);
    }
  };

  const handleUpdate = async () => {
    setUpdating(true);
    setUpdateError(null);
    setProgress(0);
    try {
      await startUpdate();
      // 进程将重启，此处通常不再返回
    } catch (err) {
      console.error('更新失败:', err);
      // 所有下载源均不可用时，提示用户手动下载
      const manualUrl = update?.urls?.[0] ?? REPO_URL;
      setUpdateError(`自动更新失败，请手动下载最新版本。下载地址：`);
      setUpdateManualUrl(manualUrl);
      showToast('更新失败：' + (err instanceof Error ? err.message : String(err)));
      setUpdating(false);
    }
  };

  // 进度条百分比
  const pct = Math.min(100, Math.max(0, Math.round(progress * 100)));

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="版本">
        <div className="flex items-center justify-between gap-3">
          <span className="text-[15px] font-medium text-primary">Flore v{version}</span>
          {desktop && (
            <SmallBtn
              label={checking ? '检查中…' : '检查更新'}
              disabled={checking || updating}
              onClick={handleCheck}
            />
          )}
        </div>
        {checkError && <p className="mt-2 text-[13px] text-danger">{checkError}</p>}
        {update && !updating && !updateError && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <div className="text-[13.5px] font-medium text-primary">
              发现新版本 v{update.latestVersion}
            </div>
            {update.notes && (
              <p className="mt-1 whitespace-pre-wrap text-[12.5px] leading-snug text-secondary">
                {update.notes}
              </p>
            )}
            <div className="mt-3">
              <SmallBtn
                label={updating ? '更新中…' : '立即更新'}
                disabled={updating}
                onClick={handleUpdate}
              />
            </div>
          </div>
        )}
        {/* 下载进度条 */}
        {updating && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <div className="mb-1.5 flex items-center justify-between text-[12.5px] text-secondary">
              <span>正在下载更新…</span>
              <span>{pct}%</span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-border-subtle">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300 ease-out"
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        )}
        {/* 下载失败 → 手动下载提示 */}
        {updateError && updateManualUrl && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <p className="text-[13px] text-danger">{updateError}</p>
            <button
              type="button"
              className="mt-1 text-[13px] text-primary underline-offset-2 hover:underline focus:outline-none"
              onClick={() => openExternal(updateManualUrl)}
            >
              {updateManualUrl.replace(/^https?:\/\//, '')}
            </button>
          </div>
        )}
      </Section>

      <Section title="关于">
        <p className="m-0 text-[14px] leading-relaxed text-secondary">
          又一个 RSS 阅读器，支持订阅源管理、文件夹分组、阅读模式、OPML 导入导出等功能。
        </p>
        <p className="mt-3 m-0 text-[13px] text-secondary">
          项目开源地址：
          <button
            type="button"
            className="ml-1 text-primary underline-offset-2 hover:underline focus:outline-none"
            onClick={() => openExternal(REPO_URL)}
          >
            {REPO_URL.replace(/^https?:\/\//, '')}
          </button>
        </p>
      </Section>
    </div>
  );
}
