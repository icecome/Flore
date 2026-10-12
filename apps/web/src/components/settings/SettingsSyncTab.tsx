import { useEffect, useState } from 'react';
import type { SyncConfig, SyncStatus } from '../../types';
import {
  getSyncConfig,
  updateSyncConfig,
  testSyncConnection,
  testFileConnection as testFileConnectionApi,
  triggerSync,
  getSyncStatus,
  alignSubscriptions,
  listFileBackups,
  createFileBackup,
  downloadFileBackup,
  type FileBackup,
} from '../../utils/api';
import { showToast } from '../../utils/toast';
import { translateBackendError } from '../../utils/backendError';
import { Section, Row, Toggle, Select, SmallBtn } from './SettingsShared';

const INPUT_CLASS =
  'w-[240px] rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary';

const API_PROVIDER_OPTIONS = [
  { value: 'miniflux', label: 'Miniflux' },
  { value: 'greader', label: 'FreshRSS / GReader' },
];

const WINDOW_DAY_OPTIONS = [
  { value: 7, label: '最近 7 天' },
  { value: 30, label: '最近 30 天' },
  { value: 90, label: '最近 90 天' },
  { value: 0, label: '全部历史' },
];

const PULL_INTERVAL_OPTIONS = [
  { value: 5, label: '5 分钟' },
  { value: 15, label: '15 分钟' },
  { value: 30, label: '30 分钟' },
  { value: 60, label: '1 小时' },
  { value: 0, label: '仅手动' },
];

/** 敏感字段掩码前缀：以该前缀提交时后端保留原值 */
const MASK_PREFIX = '****';

const isMasked = (value: string) => value.startsWith(MASK_PREFIX);

interface FormState {
  apiEnabled: boolean;
  apiProvider: string;
  apiEndpoint: string;
  apiUsername: string;
  apiPassword: string;
  apiToken: string;
  fileEnabled: boolean;
  fileProvider: string;
  fileUrl: string;
  fileUsername: string;
  filePassword: string;
  filePrefix: string;
  windowDays: number;
  pullInterval: number;
}

const toFormState = (config: SyncConfig): FormState => ({
  apiEnabled: config.apiEnabled,
  apiProvider: config.apiProvider || 'miniflux',
  apiEndpoint: config.apiEndpoint ?? '',
  apiUsername: config.apiUsername ?? '',
  apiPassword: config.apiPassword ?? '',
  apiToken: config.apiToken ?? '',
  fileEnabled: config.fileEnabled,
  fileProvider: config.fileProvider || 'webdav',
  fileUrl: config.fileUrl ?? '',
  fileUsername: config.fileUsername ?? '',
  filePassword: config.filePassword ?? '',
  filePrefix: config.filePrefix ?? '',
  windowDays: config.windowDays ?? 30,
  // 后端以秒存储，表单以分钟展示；0 表示仅手动
  pullInterval: toPullMinutes(config.pullInterval),
});

const toPullMinutes = (seconds?: number): number => {
  if (!seconds || seconds <= 0) return 0;
  if (seconds % 60 !== 0) return 30;
  return seconds / 60;
};

interface SettingsSyncTabProps {
  /** 同步新增源/文章后通知 App 刷新全局数据 */
  onSourcesChanged?: () => void;
}

export default function SettingsSyncTab({ onSourcesChanged }: SettingsSyncTabProps) {
  const [form, setForm] = useState<FormState | null>(null);
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [fileTesting, setFileTesting] = useState(false);
  const [syncing, setSyncing] = useState(false);
  // 对齐向导：上次同步发现的本地独有源
  const [alignUrls, setAlignUrls] = useState<string[]>([]);
  const [alignChecked, setAlignChecked] = useState<Set<string>>(new Set());
  const [aligning, setAligning] = useState(false);
  // WebDAV 云端备份
  const [fileBackups, setFileBackups] = useState<FileBackup[] | null>(null);
  const [backupLoading, setBackupLoading] = useState(false);
  const [backupUploading, setBackupUploading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [config, syncStatus] = await Promise.all([getSyncConfig(), getSyncStatus()]);
        if (cancelled) return;
        setForm(toFormState(config));
        setStatus(syncStatus);
      } catch (err) {
        console.error('Failed to load sync config:', err);
        showToast('获取同步配置失败', 'error');
      }
    };
    load();
    return () => { cancelled = true; };
  }, []);

  if (!form) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto text-[13px] text-muted">加载中...</div>
    );
  }

  const patch = (partial: Partial<FormState>) => setForm((prev) => (prev ? { ...prev, ...partial } : prev));

  // 键名与后端 setting 键一一对应；pullInterval 分钟转秒，0 表示仅手动
  const buildPayload = (): Record<string, string> => ({
    sync_api_enabled: String(form.apiEnabled),
    sync_api_provider: form.apiProvider,
    sync_api_endpoint: form.apiEndpoint,
    sync_api_username: form.apiUsername,
    // 掩码值原样提交，后端识别后保留原值
    sync_api_password: form.apiPassword,
    sync_api_token: form.apiToken,
    sync_file_enabled: String(form.fileEnabled),
    sync_file_provider: form.fileProvider,
    sync_file_url: form.fileUrl,
    sync_file_username: form.fileUsername,
    sync_file_password: form.filePassword,
    sync_file_prefix: form.filePrefix,
    sync_window_days: String(form.windowDays),
    sync_pull_interval: String(form.pullInterval * 60),
  });

  const save = async (): Promise<boolean> => {
    setSaving(true);
    try {
      const config = await updateSyncConfig(buildPayload());
      setForm(toFormState(config));
      return true;
    } catch (err) {
      console.error('Failed to save sync config:', err);
      showToast('保存同步配置失败', 'error');
      return false;
    } finally {
      setSaving(false);
    }
  };

  const testConnection = async () => {
    // 测试使用后端已存储配置，需先保存当前编辑值
    const saved = await save();
    if (!saved) return;
    setTesting(true);
    try {
      await testSyncConnection();
      showToast('连接成功', 'success');
    } catch (err) {
      // 透传后端错误文案（认证被拒/地址不可达等），便于用户定位
      const message = translateBackendError(err instanceof Error ? err.message : '') ?? '';
      showToast(message || '连接失败，请检查地址与凭证', 'error');
    } finally {
      setTesting(false);
    }
  };

  const runFileTest = async () => {
    // 测试使用后端已存储配置，需先保存当前编辑值
    const saved = await save();
    if (!saved) return;
    setFileTesting(true);
    try {
      await testFileConnectionApi();
      showToast('连接成功', 'success');
    } catch (err) {
      const message = translateBackendError(err instanceof Error ? err.message : '') ?? '';
      showToast(message || '连接失败，请检查地址与凭证', 'error');
    } finally {
      setFileTesting(false);
    }
  };

  const refreshFileBackups = async () => {
    setBackupLoading(true);
    try {
      setFileBackups(await listFileBackups());
    } catch (err) {
      setFileBackups([]);
      showToast(translateBackendError(err instanceof Error ? err.message : '') || '获取远端备份列表失败', 'error');
    } finally {
      setBackupLoading(false);
    }
  };

  const runFileBackup = async () => {
    const saved = await save();
    if (!saved) return;
    setBackupUploading(true);
    try {
      await createFileBackup();
      showToast('已备份到 WebDAV', 'success');
      await refreshFileBackups();
    } catch (err) {
      console.error('Failed to backup to webdav:', err);
      showToast(translateBackendError(err instanceof Error ? err.message : '') || '备份上传失败', 'error');
    } finally {
      setBackupUploading(false);
    }
  };

  const runFileBackupDownload = async (name: string) => {
    try {
      const blob = await downloadFileBackup(name);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      showToast(translateBackendError(err instanceof Error ? err.message : '') || '下载失败', 'error');
    }
  };

  const formatSize = (bytes: number): string => {
    if (bytes <= 0) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const refreshStatus = async () => {
    try {
      setStatus(await getSyncStatus());
    } catch (err) {
      console.error('Failed to refresh sync status:', err);
    }
  };

  const runSync = async () => {
    const saved = await save();
    if (!saved) return;
    setSyncing(true);
    try {
      const result = await triggerSync();
      const parts = [`导入 ${result.feedsImported} 个源`];
      if (result.itemsUpserted > 0) parts.push(`更新 ${result.itemsUpserted} 篇文章`);
      if (result.localOnlyUrls.length > 0) parts.push(`${result.localOnlyUrls.length} 个源仅在本地`);
      showToast(`同步完成：${parts.join('，')}`, 'success');
      setAlignUrls(result.localOnlyUrls);
      setAlignChecked(new Set(result.localOnlyUrls));
      onSourcesChanged?.();
      await refreshStatus();
    } catch (err) {
      console.error('Sync failed:', err);
      showToast(translateBackendError(err instanceof Error ? err.message : '') || '同步失败', 'error');
    } finally {
      setSyncing(false);
    }
  };

  // 对齐向导：推送勾选的本地独有源到服务端
  const pushAlign = async () => {
    const urls = alignUrls.filter((url) => alignChecked.has(url));
    if (urls.length === 0) return;
    setAligning(true);
    try {
      const result = await alignSubscriptions(urls);
      showToast(`已推送 ${result.pushed} 个订阅到服务端`, 'success');
      const pushed = new Set(urls);
      setAlignUrls((prev) => prev.filter((url) => !pushed.has(url)));
      // 同步清理勾选，避免残留导致按钮计数虚高
      setAlignChecked((prev) => {
        const next = new Set(prev);
        for (const url of pushed) next.delete(url);
        return next;
      });
      onSourcesChanged?.();
      await refreshStatus();
    } catch (err) {
      console.error('Align subscriptions failed:', err);
      showToast(translateBackendError(err instanceof Error ? err.message : '') || '推送订阅失败', 'error');
    } finally {
      setAligning(false);
    }
  };

  const toggleAlignUrl = (url: string) => {
    setAlignChecked((prev) => {
      const next = new Set(prev);
      if (next.has(url)) next.delete(url);
      else next.add(url);
      return next;
    });
  };

  const passwordHint = (value: string) =>
    isMasked(value) ? '已保存，输入新值可覆盖' : undefined;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="服务端同步">
        <Row
          title="启用服务端同步"
          desc="连接自建 RSS 服务端，同步订阅与阅读状态。本地数据始终保留。"
          control={<Toggle checked={form.apiEnabled} onChange={(v) => patch({ apiEnabled: v })} />}
        />
        {form.apiEnabled && (
          <>
            <Row
              title="服务类型"
              control={
                <Select
                  value={form.apiProvider}
                  options={API_PROVIDER_OPTIONS}
                  onChange={(v) => patch({ apiProvider: String(v) })}
                />
              }
            />
            <Row
              title="服务器地址"
              desc={form.apiProvider === 'greader' ? '服务根地址，如 https://freshrss.example.com' : '如 https://miniflux.example.com'}
              control={
                <input
                  type="text"
                  value={form.apiEndpoint}
                  onChange={(e) => patch({ apiEndpoint: e.target.value })}
                  placeholder={form.apiProvider === 'greader' ? 'https://freshrss.example.com' : 'https://miniflux.example.com'}
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="用户名"
              desc={form.apiProvider === 'miniflux' ? 'Token 认证时可留空' : undefined}
              control={
                <input
                  type="text"
                  value={form.apiUsername}
                  onChange={(e) => patch({ apiUsername: e.target.value })}
                  placeholder="账号"
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="密码"
              desc={passwordHint(form.apiPassword)}
              control={
                <input
                  type="password"
                  value={form.apiPassword}
                  onChange={(e) => patch({ apiPassword: e.target.value })}
                  placeholder="密码"
                  className={INPUT_CLASS}
                />
              }
            />
            {form.apiProvider === 'miniflux' && (
              <Row
                title="API Token"
                desc={passwordHint(form.apiToken)}
                control={
                  <input
                    type="password"
                    value={form.apiToken}
                    onChange={(e) => patch({ apiToken: e.target.value })}
                    placeholder="可选，用于 Token 认证"
                    className={INPUT_CLASS}
                  />
                }
              />
            )}
            <Row
              title="拉取窗口"
              desc="首次同步时拉取多长时间内的文章。"
              control={
                <Select
                  value={form.windowDays}
                  options={WINDOW_DAY_OPTIONS}
                  onChange={(v) => patch({ windowDays: Number(v) })}
                />
              }
            />
            <Row
              title="自动同步间隔"
              control={
                <Select
                  value={form.pullInterval}
                  options={PULL_INTERVAL_OPTIONS}
                  onChange={(v) => patch({ pullInterval: Number(v) })}
                />
              }
            />
          </>
        )}
      </Section>

      <Section title="云端备份">
        <Row
          title="启用云端备份"
          desc="将本地数据库整包备份到自有 WebDAV 存储。"
          control={<Toggle checked={form.fileEnabled} onChange={(v) => patch({ fileEnabled: v })} />}
        />
        {form.fileEnabled && (
          <>
            <Row
              title="WebDAV 地址"
              control={
                <input
                  type="text"
                  value={form.fileUrl}
                  onChange={(e) => patch({ fileUrl: e.target.value })}
                  placeholder="https://cloud.example.com/dav"
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="用户名"
              control={
                <input
                  type="text"
                  value={form.fileUsername}
                  onChange={(e) => patch({ fileUsername: e.target.value })}
                  placeholder="账号"
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="密码"
              desc={passwordHint(form.filePassword)}
              control={
                <input
                  type="password"
                  value={form.filePassword}
                  onChange={(e) => patch({ filePassword: e.target.value })}
                  placeholder="密码或应用密码"
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="备份路径前缀"
              control={
                <input
                  type="text"
                  value={form.filePrefix}
                  onChange={(e) => patch({ filePrefix: e.target.value })}
                  placeholder="flore/"
                  className={INPUT_CLASS}
                />
              }
            />
            <Row
              title="测试连接"
              desc="验证 WebDAV 地址与凭证是否有效。"
              control={
                <SmallBtn
                  label={fileTesting ? '测试中……' : '测试连接'}
                  disabled={fileTesting || saving}
                  onClick={runFileTest}
                />
              }
            />
            <Row
              title="立即备份到 WebDAV"
              desc="创建一份本地压缩备份并上传到该地址。"
              control={
                <SmallBtn
                  label={backupUploading ? '备份中……' : '备份'}
                  disabled={backupUploading || saving}
                  onClick={runFileBackup}
                />
              }
            />
            <Row
              title="远端备份"
              desc="列出该地址下的备份文件，可下载到本地。"
              control={
                <div className="flex gap-2">
                  <SmallBtn
                    label={backupLoading ? '加载中……' : '刷新列表'}
                    disabled={backupLoading || saving}
                    onClick={refreshFileBackups}
                  />
                  <SmallBtn label="下载" disabled={!fileBackups || fileBackups.length === 0} onClick={() => fileBackups && runFileBackupDownload(fileBackups[0].name)} />
                </div>
              }
            />
            {fileBackups && fileBackups.length > 0 && (
              <div className="border-b border-border-subtle py-2">
                <div className="text-[12px] text-muted">最近备份（下载到本地文件）：</div>
                <div className="mt-1 flex flex-col gap-1">
                  {fileBackups.slice(0, 10).map((b) => (
                    <div key={b.name} className="flex items-center justify-between gap-2 text-[12.5px]">
                      <span className="truncate text-secondary" title={b.name}>{b.name}</span>
                      <div className="flex shrink-0 items-center gap-2">
                        <span className="text-[11px] text-muted">{formatSize(b.size)}</span>
                        <button
                          type="button"
                          onClick={() => runFileBackupDownload(b.name)}
                          className="text-[11px] text-primary hover:underline"
                        >
                          下载
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </Section>

      <Section title="同步操作">
        <Row
          title={status?.lastError ? '上次同步异常' : '立即同步'}
          desc={status?.lastError || status?.lastResult || '保存配置并执行一次完整同步。'}
          control={
            <div className="flex gap-2">
              <SmallBtn
                label="测试连接"
                disabled={testing || saving || !form.apiEnabled}
                onClick={testConnection}
              />
              <SmallBtn label={syncing ? '同步中……' : '立即同步'} disabled={syncing || saving || !form.apiEnabled} onClick={runSync} />
            </div>
          }
        />
        <Row
          title="保存配置"
          desc="保存当前同步设置。点击「测试连接」或「立即同步」时会先自动保存。"
          control={
            <SmallBtn label={saving ? '保存中……' : '保存'} disabled={saving} onClick={() => { void save(); }} />
          }
        />
      </Section>

      {alignUrls.length > 0 && (
        <Section title="同步订阅">
          <div className="border-b border-border-subtle py-3">
            <div className="text-[13.5px] font-medium text-primary">
              以下 {alignUrls.length} 个订阅源仅存在于本地
            </div>
            <div className="mt-0.5 text-[12px] leading-snug text-muted">
              勾选后推送到服务端，两端订阅保持一致。
            </div>
            <div className="mt-2 flex max-h-[200px] flex-col gap-0.5 overflow-y-auto">
              {alignUrls.map((url) => (
                <label
                  key={url}
                  className="flex items-center gap-2 rounded-md px-1 py-1 text-[12.5px] text-secondary transition-colors hover:bg-hover"
                >
                  <input
                    type="checkbox"
                    checked={alignChecked.has(url)}
                    onChange={() => toggleAlignUrl(url)}
                    className="shrink-0 accent-[var(--primary)]"
                  />
                  <span className="truncate" title={url}>{url}</span>
                </label>
              ))}
            </div>
          </div>
          <Row
            title="推送订阅"
            desc="仅在本地存在的订阅源，推送到服务端后两端订阅一致。"
            control={
              <SmallBtn
                label={aligning ? '推送中……' : `推送 ${alignChecked.size} 个源`}
                disabled={aligning || alignChecked.size === 0}
                onClick={pushAlign}
              />
            }
          />
        </Section>
      )}
    </div>
  );
}
