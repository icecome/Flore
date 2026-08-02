import { useEffect, useState } from 'react';
import { getVersion, checkForUpdate, startUpdate, isDesktop } from '../../utils/api';
import type { UpdateInfo } from '../../utils/api';
import { Section, SmallBtn } from './SettingsShared';
import { showToast } from '../../utils/toast';

const desktop = isDesktop();

export default function SettingsAboutTab() {
  const [version, setVersion] = useState('...');
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [checkError, setCheckError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // 版本号获取失败仅降级展示"未知"，不打扰用户
    getVersion()
      .then((data) => { if (!cancelled) setVersion(data.version || '未知'); })
      .catch((err) => {
        console.error('Failed to fetch version:', err);
        if (!cancelled) setVersion('未知');
      });
    return () => { cancelled = true; };
  }, []);

  const handleCheck = async () => {
    setChecking(true);
    setCheckError(null);
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
    try {
      await startUpdate();
      // 进程将重启，此处通常不再返回
    } catch (err) {
      console.error('更新失败:', err);
      showToast('更新失败：' + (err instanceof Error ? err.message : String(err)));
      setUpdating(false);
    }
  };

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
        {update && (
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
      </Section>

      <Section title="关于">
        <p className="m-0 text-[14px] leading-relaxed text-secondary">
          又一个 RSS 阅读器，支持订阅源管理、文件夹分组、阅读模式、OPML 导入导出等功能。
        </p>
      </Section>
    </div>
  );
}
