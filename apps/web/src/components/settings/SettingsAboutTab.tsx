import { useEffect, useState } from 'react';
import { getVersion, isDesktop, openExternal, generateDiagnosticPackage } from '../../utils/api';
import useUpdate from '../../hooks/useUpdate';
import type { AppSettings } from '../../utils/settings';
import { Section, SmallBtn, Row, Toggle } from './SettingsShared';
import { showToast } from '../../utils/toast';
import { translateBackendError } from '../../utils/backendError';

// 项目仓库地址（开源地址，非敏感配置）
const REPO_URL = 'https://github.com/icecome/Flore';

const desktop = isDesktop();

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}

export default function SettingsAboutTab({ settings, updateSetting }: Props) {
  const [version, setVersion] = useState('...');
  // 更新状态统一走全局仓库：状态属主在桌面壳，弹窗开合/卸载不影响下载与进度复原
  const { snapshot, check, download, restart } = useUpdate();
  const [checking, setChecking] = useState(false);
  // 手动下载地址（下载失败时展示）与最近一次错误
  const [manualUrl, setManualUrl] = useState<string | null>(null);

  // 挂载时读取版本号
  useEffect(() => {
    let cancelled = false;
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
    setManualUrl(null);
    try {
      await check();
      // 结果经事件/快照回写，无需本地保存；这里只提示"无更新"由 error/available 兜底
      if (snapshot.status === 'idle') showToast('已是最新版本', 'success');
    } catch (err) {
      console.error('检查更新失败:', err);
      showToast('检查更新失败，请稍后重试', 'error');
    } finally {
      setChecking(false);
    }
  };

  const handleDownload = async () => {
    setManualUrl(null);
    try {
      await download();
    } catch (err) {
      console.error('更新失败:', err);
      const url = snapshot.info?.urls?.[0] ?? REPO_URL;
      setManualUrl(url);
      showToast('更新失败：' + (translateBackendError(err instanceof Error ? err.message : String(err)) ?? '未知原因'), 'error');
    }
  };

  const handleRestart = async () => {
    try {
      await restart();
    } catch (err) {
      console.error('重启应用失败:', err);
      showToast('重启应用以完成更新失败', 'error');
    }
  };

  const pct = Math.min(100, Math.max(0, Math.round(snapshot.progress * 100)));
  const { status } = snapshot;
  const busy = status === 'checking' || status === 'downloading';

  // 诊断包生成（面向所有用户，P0/decision3）：打包数据库快照、脱敏日志与系统信息，
  // 便于用户反馈问题时附带可诊断材料。
  const [generatingDiagnostic, setGeneratingDiagnostic] = useState(false);
  const handleGenerateDiagnostic = async () => {
    setGeneratingDiagnostic(true);
    try {
      const { path } = await generateDiagnosticPackage();
      showToast('诊断包已生成', 'success');
      // 桌面端可直接打开所在目录；Web 端提示路径。
      if (desktop) {
        const dir = path.replace(/[^/\\]*$/, '');
        openExternal(dir);
      } else {
        showToast(`诊断包已保存到：${path}`, 'info', 6000);
      }
    } catch (err) {
      console.error('生成诊断包失败:', err);
      showToast(translateBackendError(err instanceof Error ? err.message : String(err)) || '生成诊断包失败', 'error');
    } finally {
      setGeneratingDiagnostic(false);
    }
  };

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="版本">
        <div className="flex items-center justify-between gap-3">
          <span className="text-[15px] font-medium text-primary">Flore v{version}</span>
          {desktop && (
            <SmallBtn
              label={status === 'checking' ? '检查中…' : '检查更新'}
              disabled={busy}
              onClick={handleCheck}
            />
          )}
        </div>

        {desktop && (
          <Row
            title="启动时检查更新"
            desc="每 3 天最多检查一次，仅在有新版本时于右下角轻提示"
            control={
              <Toggle
                checked={settings.updateCheckOnStartup}
                onChange={(v) => updateSetting('updateCheckOnStartup', v)}
              />
            }
          />
        )}
        {desktop && (
          <Row
            title="检测到新版本后自动下载"
            desc="后台静默下载安装包，下载完成后仍需确认重启才会应用更新"
            control={
              <Toggle
                checked={settings.updateAutoDownload}
                onChange={(v) => updateSetting('updateAutoDownload', v)}
              />
            }
          />
        )}

        {/* 发现新版本 → 发起下载 */}
        {status === 'available' && snapshot.info && !busy && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <div className="text-[13.5px] font-medium text-primary">
              发现新版本 v{snapshot.info.latestVersion}
              {snapshot.info.forced && (
                <span className="ml-2 text-[12px] font-normal text-danger">（当前版本过低，建议尽快更新）</span>
              )}
            </div>
            {snapshot.info.notes && (
              <p className="mt-1 whitespace-pre-wrap text-[12.5px] leading-snug text-secondary">
                {snapshot.info.notes}
              </p>
            )}
            <div className="mt-3">
              <SmallBtn label="立即更新" onClick={handleDownload} />
            </div>
          </div>
        )}

        {/* 下载进行中 → 进度条 */}
        {status === 'downloading' && (
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

        {/* 下载完成 → 确认重启应用 */}
        {status === 'ready' && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <div className="text-[13.5px] font-medium text-primary">
              更新已下载完成
            </div>
            <p className="mt-1 text-[12.5px] text-secondary">
              重启应用以完成更新。
            </p>
            <div className="mt-3">
              <SmallBtn label="立即重启" onClick={handleRestart} />
            </div>
          </div>
        )}

        {/* 下载/检查失败 → 手动下载提示 */}
        {status === 'error' && (
          <div className="mt-3 rounded-md border border-border-subtle p-3">
            <p className="text-[13px] text-danger">
              {snapshot.info && !manualUrl
                ? `自动更新失败（${snapshot.error || '未知错误'}）。请手动下载最新版本：`
                : `自动更新失败，请手动下载最新版本：`}
            </p>
            {manualUrl && (
              <button
                type="button"
                className="mt-1 block w-full break-all text-left text-[13px] text-primary underline-offset-2 hover:underline focus:outline-none"
                onClick={() => openExternal(manualUrl)}
              >
                {manualUrl}
              </button>
            )}
            {!manualUrl && (
              <button
                type="button"
                className="mt-1 block w-full break-all text-left text-[13px] text-primary underline-offset-2 hover:underline focus:outline-none"
                onClick={() => openExternal(REPO_URL)}
              >
                {REPO_URL}
              </button>
            )}
          </div>
        )}
      </Section>

      <Section title="诊断">
        <Row
          title="生成诊断包"
          desc="打包数据库快照、脱敏日志与系统信息为一个压缩文件，便于反馈问题时附带。"
          control={
            <SmallBtn
              label={generatingDiagnostic ? '生成中…' : '生成诊断包'}
              disabled={generatingDiagnostic}
              onClick={handleGenerateDiagnostic}
            />
          }
        />
      </Section>

      <Section title="关于">
        <p className="m-0 text-[14px] leading-relaxed text-secondary">
          又一个 RSS 阅读器，支持订阅源管理、文件夹分组、阅读模式、OPML 导入导出等功能。
        </p>
        <p className="mt-3 m-0 text-[13px] text-secondary">
          项目开源地址：
          <button
            type="button"
            className="ml-1 break-all text-primary underline-offset-2 hover:underline focus:outline-none"
            onClick={() => openExternal(REPO_URL)}
          >
            {REPO_URL}
          </button>
        </p>
      </Section>
    </div>
  );
}