import { useState, useEffect, useRef, useCallback } from 'react';
import type { JSX } from 'react';
import { Upload, Download, Trash2, RefreshCw, Archive, Database, FolderSync, RotateCw } from '../icons';
import { showToast } from '../../utils/toast';
import { getApi, getDesktopApp, isDesktop } from '../../utils/api';
import { Section, SmallBtn, Row, Toggle, NumberInput, Select } from './SettingsShared';
import type { AppSettings } from '../../utils/settings';
import { importConfig } from '../../utils/settings';

interface CacheStats {
  readabilityCount: number;
  readabilitySize: number;
  ftsItemCount: number;
  totalItems: number;
  totalSources: number;
}

interface DatabaseInfo {
  path: string;
  size: number;
  backupDir: string;
  backupDirExists: boolean;
  journalMode?: string;
}

interface BackupEntry {
  name: string;
  size: number;
  modTime: string;
}

interface Props {
  settings: AppSettings;
  updateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
  importDatabase: (file: File) => Promise<void>;
  exportDatabase: () => Promise<void>;
  onRestartApp: () => Promise<void>;
}

const BACKUP_INTERVAL_OPTIONS = [
  { value: '360', label: '每 6 小时' },
  { value: '720', label: '每 12 小时' },
  { value: '1440', label: '每天' },
  { value: '2880', label: '每 2 天' },
  { value: '10080', label: '每周' },
];

function formatBytes(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

async function doClearReadability(setLoadingState: (v: string) => void, refresh: () => Promise<void>): Promise<void> {
  setLoadingState('readability');
  try {
    const res = await fetch(`${getApi()}/cache/clear-readability`, { method: 'POST' });
    if (!res.ok) throw new Error('清理失败');
    const data = (await res.json()) as { deleted: number };
    showToast(`已清理 ${data.deleted} 条全文缓存`);
    await refresh();
  } catch {
    showToast('清理失败');
  } finally {
    setLoadingState('');
  }
}

async function doRebuildSearch(setLoadingState: (v: string) => void, refresh: () => Promise<void>): Promise<void> {
  setLoadingState('search');
  try {
    const res = await fetch(`${getApi()}/cache/rebuild-search`, { method: 'POST' });
    if (!res.ok) throw new Error('重建失败');
    showToast('搜索索引已重建');
    await refresh();
  } catch {
    showToast('重建失败');
  } finally {
    setLoadingState('');
  }
}

async function doVacuum(setLoadingState: (v: string) => void, refresh: () => Promise<void>): Promise<void> {
  setLoadingState('vacuum');
  try {
    const res = await fetch(`${getApi()}/database/vacuum`, { method: 'POST' });
    if (!res.ok) throw new Error('压缩失败');
    showToast('数据库已压缩');
    await refresh();
  } catch {
    showToast('压缩失败');
  } finally {
    setLoadingState('');
  }
}

async function doCreateBackup(setLoadingState: (v: string) => void, refresh: () => Promise<void>): Promise<void> {
  setLoadingState('backup');
  try {
    const res = await fetch(`${getApi()}/backups/create`, { method: 'POST' });
    if (!res.ok) throw new Error('备份失败');
    const data = (await res.json()) as { name: string };
    showToast(`备份已创建: ${data.name}`);
    await refresh();
  } catch {
    showToast('备份失败');
  } finally {
    setLoadingState('');
  }
}

async function doDeleteBackup(name: string, refresh: () => Promise<void>): Promise<void> {
  try {
    const res = await fetch(`${getApi()}/backups/${encodeURIComponent(name)}`, { method: 'DELETE' });
    if (!res.ok) throw new Error('删除失败');
    showToast('备份已删除');
    await refresh();
  } catch {
    showToast('删除失败');
  }
}

async function doDownloadBackup(name: string): Promise<void> {
  try {
    const res = await fetch(`${getApi()}/backups/${encodeURIComponent(name)}/download`);
    if (!res.ok) throw new Error('下载失败');
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch {
    showToast('下载失败');
  }
}

async function doRestoreBackup(
  name: string,
  setLoadingState: (v: string) => void,
  refresh: () => Promise<void>,
): Promise<void> {
  if (!window.confirm(`确定要从备份「${name}」恢复吗？\n当前数据将先自动备份，再被该备份替换。恢复后建议重启应用生效。`)) {
    return;
  }
  setLoadingState('restore');
  try {
    const res = await fetch(`${getApi()}/backups/${encodeURIComponent(name)}/restore`, { method: 'POST' });
    if (!res.ok) throw new Error('恢复失败');
    showToast('已从备份恢复，建议重启应用生效');
    await refresh();
  } catch {
    showToast('恢复失败');
  } finally {
    setLoadingState('');
  }
}

async function doCleanupExpiredBackups(
  setLoadingState: (v: string) => void,
  settings: AppSettings,
  refresh: () => Promise<void>,
): Promise<void> {
  setLoadingState('cleanup-backup');
  try {
    const res = await fetch(`${getApi()}/backups/cleanup`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ maxKeep: settings.backupMaxKeep, maxDays: settings.backupMaxDays }),
    });
    if (!res.ok) throw new Error('清理失败');
    const data = (await res.json()) as { deleted: number };
    showToast(`已清理 ${data.deleted} 个过期备份`);
    await refresh();
  } catch {
    showToast('清理失败');
  } finally {
    setLoadingState('');
  }
}

async function doCleanupArticles(
  setLoadingState: (v: string) => void,
  settings: AppSettings,
  refresh: () => Promise<void>,
): Promise<void> {
  if (settings.articleRetentionDays <= 0 && settings.articleRetentionMax <= 0) {
    showToast('请先设置保留天数或最大文章数');
    return;
  }
  setLoadingState('cleanup-articles');
  try {
    const res = await fetch(`${getApi()}/articles/cleanup`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        retentionDays: settings.articleRetentionDays,
        retentionMax: settings.articleRetentionMax,
        excludeStarred: settings.retentionExcludeStarred,
        excludeReadLater: settings.retentionExcludeReadLater,
      }),
    });
    if (!res.ok) throw new Error('清理失败');
    const data = (await res.json()) as { deleted: number };
    showToast(`已清理 ${data.deleted} 篇文章`);
    await refresh();
  } catch {
    showToast('清理失败');
  } finally {
    setLoadingState('');
  }
}

function SectionConfigManagement(
  onImportConfig: () => Promise<void>,
  onExportConfig: () => Promise<void>,
): JSX.Element {
  return (
    <Section title="配置管理">
      <div className="flex items-center gap-2">
        <SmallBtn icon={<Upload size={14} />} label="导入配置" onClick={onImportConfig} />
        <SmallBtn icon={<Download size={14} />} label="导出配置" onClick={onExportConfig} />
      </div>
      <p className="mt-2 text-[12px] text-muted">配置为 JSON 格式，可跨设备迁移。</p>
    </Section>
  );
}

function SectionDatabaseManagement(
  dbInputRef: React.RefObject<HTMLInputElement | null>,
  onImportDatabase: (file: File) => Promise<void>,
  onExportDatabase: () => Promise<void>,
  onVacuum: () => Promise<void>,
  onRestartApp: () => Promise<void>,
  loading: string,
  dbInfo: DatabaseInfo | null,
): JSX.Element {
  return (
    <Section title="数据库管理">
      <input
        ref={dbInputRef}
        type="file"
        accept=".db"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) onImportDatabase(file);
          if (e.target) e.target.value = '';
        }}
      />
      <div className="flex items-center gap-2">
        <SmallBtn icon={<Upload size={14} />} label="导入数据库" onClick={() => dbInputRef.current?.click()} />
        <SmallBtn icon={<Download size={14} />} label="导出数据库" onClick={onExportDatabase} />
        <SmallBtn
          icon={<Archive size={14} />}
          label="压缩数据库"
          onClick={onVacuum}
          disabled={loading === 'vacuum'}
        />
        {isDesktop() && (
          <SmallBtn
            icon={<RotateCw size={14} />}
            label="重启应用"
            onClick={onRestartApp}
            disabled={loading === 'restart'}
          />
        )}
      </div>
      <p className="mt-2 text-[12px] text-muted">导入会覆盖当前数据，请谨慎操作。压缩可减小数据库文件体积。导入数据后需重启应用生效。</p>
      {dbInfo && (
        <div className="mt-3 space-y-1 text-[12px] text-muted">
          <p>数据库大小: {formatBytes(dbInfo.size)}</p>
          <p className="truncate" title={dbInfo.path}>路径: {dbInfo.path}</p>
          {dbInfo.journalMode && (
            <p>
              日志模式:{' '}
              {dbInfo.journalMode.toLowerCase() === 'wal' ? 'WAL（预写日志，已启用）' : dbInfo.journalMode}
            </p>
          )}
        </div>
      )}
    </Section>
  );
}

function SectionCacheCleanup(
  cacheStats: CacheStats | null,
  onClearReadability: () => Promise<void>,
  onRebuildSearch: () => Promise<void>,
  loading: string,
): JSX.Element {
  return (
    <Section title="缓存清理">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div>
            <span className="text-[13px] text-primary">阅读缓存</span>
            {cacheStats && (
              <span className="ml-2 text-[12px] text-muted">
                {cacheStats.readabilityCount} 条 / {formatBytes(cacheStats.readabilitySize)}
              </span>
            )}
          </div>
          <SmallBtn
            icon={<Trash2 size={14} />}
            label="清理"
            onClick={onClearReadability}
            disabled={loading === 'readability'}
            danger
          />
        </div>
        <div className="flex items-center justify-between">
          <div>
            <span className="text-[13px] text-primary">搜索索引</span>
            {cacheStats && (
              <span className="ml-2 text-[12px] text-muted">
                {cacheStats.ftsItemCount} / {cacheStats.totalItems} 条
              </span>
            )}
          </div>
          <SmallBtn
            icon={<FolderSync size={14} />}
            label="重建"
            onClick={onRebuildSearch}
            disabled={loading === 'search'}
          />
        </div>
      </div>
      <p className="mt-2 text-[12px] text-muted">清理缓存后下次访问时自动重新生成。</p>
    </Section>
  );
}

function SectionBackupManagement(
  backups: BackupEntry[],
  dbInfo: DatabaseInfo | null,
  onCreateBackup: () => Promise<void>,
  onDeleteBackup: (name: string) => Promise<void>,
  onRestoreBackup: (name: string) => Promise<void>,
  onDownloadBackup: (name: string) => Promise<void>,
  onRefresh: () => Promise<void>,
  loading: string,
): JSX.Element {
  const totalBackupSize = backups.reduce((sum, b) => sum + b.size, 0);

  return (
    <Section title="备份管理">
      <div className="flex items-center gap-2">
        <SmallBtn
          icon={<Database size={14} />}
          label="立即备份"
          onClick={onCreateBackup}
          disabled={loading === 'backup'}
        />
        <SmallBtn
          icon={<RefreshCw size={14} />}
          label="刷新"
          onClick={onRefresh}
        />
      </div>
      {backups.length > 0 && (
        <div className="mt-3">
          <p className="mb-2 text-[12px] text-muted">
            共 {backups.length} 个备份 ({formatBytes(totalBackupSize)})
          </p>
          <div className="max-h-[200px] overflow-y-auto rounded-md border border-border">
            {backups.map((b) => (
              <div
                key={b.name}
                className="flex items-center justify-between border-b border-border-subtle px-3 py-2 last:border-0"
              >
                <div className="min-w-0">
                  <p className="truncate text-[12.5px] text-primary">{b.name}</p>
                  <p className="text-[11px] text-muted">{b.modTime} &middot; {formatBytes(b.size)}</p>
                </div>
                <div className="ml-2 flex shrink-0 items-center gap-1">
                  <button
                    onClick={() => onDownloadBackup(b.name)}
                    className="rounded-md p-1 text-muted transition-colors hover:bg-primary/10 hover:text-primary"
                    title="下载"
                  >
                    <Download size={14} />
                  </button>
                  <button
                    onClick={() => onRestoreBackup(b.name)}
                    className="rounded-md p-1 text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:opacity-40"
                    title="恢复到此备份"
                    disabled={loading === 'restore'}
                  >
                    <RotateCw size={14} />
                  </button>
                  <button
                    onClick={() => onDeleteBackup(b.name)}
                    className="rounded-md p-1 text-muted transition-colors hover:bg-danger/10 hover:text-danger"
                    title="删除"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      {backups.length === 0 && dbInfo && (
        <p className="mt-2 text-[12px] text-muted">暂无备份。</p>
      )}
      {dbInfo && (
        <p className="mt-2 text-[12px] text-muted truncate" title={dbInfo.backupDir}>
          备份目录: {dbInfo.backupDir}
        </p>
      )}
    </Section>
  );
}

function SectionBackupStrategy(
  settings: AppSettings,
  onUpdateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void,
  onCleanupExpired: () => Promise<void>,
  loading: string,
): JSX.Element {
  return (
    <Section title="备份策略">
      <Row
        title="最大保留数量"
        desc="超过此数量的旧备份将被自动清理。"
        control={
          <NumberInput
            value={settings.backupMaxKeep}
            min={1}
            max={100}
            unit="个"
            onChange={(v) => onUpdateSetting('backupMaxKeep', v)}
          />
        }
      />
      <Row
        title="最大保留天数"
        desc="超过此天数的备份将被自动清理。"
        control={
          <NumberInput
            value={settings.backupMaxDays}
            min={1}
            max={365}
            unit="天"
            onChange={(v) => onUpdateSetting('backupMaxDays', v)}
          />
        }
      />
      <Row
        title="启用自动备份"
        desc="按固定间隔自动创建压缩备份。"
        control={
          <Toggle
            checked={settings.backupAutoEnabled}
            onChange={(v) => onUpdateSetting('backupAutoEnabled', v)}
          />
        }
      />
      {settings.backupAutoEnabled && (
        <Row
          title="自动备份间隔"
          control={
            <Select
              value={settings.backupAutoInterval}
              options={BACKUP_INTERVAL_OPTIONS}
              onChange={(v) => onUpdateSetting('backupAutoInterval', Number(v))}
            />
          }
        />
      )}
      <div className="pt-2">
        <SmallBtn
          icon={<Trash2 size={14} />}
          label="立即清理过期备份"
          onClick={onCleanupExpired}
          disabled={loading === 'cleanup-backup'}
        />
      </div>
    </Section>
  );
}

function SectionArticleRetention(
  settings: AppSettings,
  onUpdateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void,
  onCleanupArticles: () => Promise<void>,
  loading: string,
): JSX.Element {
  return (
    <Section title="文章留存策略">
      <Row
        title="保留天数"
        desc="已读文章保留天数，0 表示不限制。"
        control={
          <NumberInput
            value={settings.articleRetentionDays}
            min={0}
            max={3650}
            unit="天"
            onChange={(v) => onUpdateSetting('articleRetentionDays', v)}
          />
        }
      />
      <Row
        title="最大文章数"
        desc="已读文章最大保留数量，0 表示不限制。"
        control={
          <NumberInput
            value={settings.articleRetentionMax}
            min={0}
            max={100000}
            step={100}
            unit="篇"
            onChange={(v) => onUpdateSetting('articleRetentionMax', v)}
          />
        }
      />
      <Row
        title="收藏文章不清理"
        control={
          <Toggle
            checked={settings.retentionExcludeStarred}
            onChange={(v) => onUpdateSetting('retentionExcludeStarred', v)}
          />
        }
      />
      <Row
        title="稍后阅读文章不清理"
        control={
          <Toggle
            checked={settings.retentionExcludeReadLater}
            onChange={(v) => onUpdateSetting('retentionExcludeReadLater', v)}
          />
        }
      />
      <div className="pt-2">
        <SmallBtn
          icon={<Trash2 size={14} />}
          label="立即清理"
          onClick={onCleanupArticles}
          disabled={loading === 'cleanup-articles'}
          danger
        />
      </div>
    </Section>
  );
}

function SectionStatistics(cacheStats: CacheStats | null): JSX.Element {
  return (
    <Section title="数据统计">
      {cacheStats && (
        <div className="space-y-1 text-[12.5px] text-secondary">
          <p>订阅源: {cacheStats.totalSources} 个</p>
          <p>文章总数: {cacheStats.totalItems} 篇</p>
          <p>搜索索引: {cacheStats.ftsItemCount} 条</p>
        </div>
      )}
    </Section>
  );
}

export default function SettingsDataTab({ settings, updateSetting, importDatabase, exportDatabase, onRestartApp }: Props) {
  const [cacheStats, setCacheStats] = useState<CacheStats | null>(null);
  const [dbInfo, setDbInfo] = useState<DatabaseInfo | null>(null);
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [loading, setLoading] = useState('');
  const dbInputRef = useRef<HTMLInputElement>(null);

  const refreshAll = useCallback(async () => {
    try {
      const [statsRes, infoRes, backupsRes] = await Promise.all([
        fetch(`${getApi()}/cache/stats`),
        fetch(`${getApi()}/database/info`),
        fetch(`${getApi()}/backups`),
      ]);
      if (statsRes.ok) setCacheStats(await statsRes.json());
      if (infoRes.ok) setDbInfo(await infoRes.json());
      if (backupsRes.ok) setBackups(await backupsRes.json());
    } catch {
      // 静默失败
    }
  }, []);

  useEffect(() => { refreshAll(); }, [refreshAll]);

  const handleClearReadability = useCallback(() => doClearReadability(setLoading, refreshAll), [refreshAll]);
  const handleRebuildSearch = useCallback(() => doRebuildSearch(setLoading, refreshAll), [refreshAll]);
  const handleVacuum = useCallback(() => doVacuum(setLoading, refreshAll), [refreshAll]);
  const handleCreateBackup = useCallback(() => doCreateBackup(setLoading, refreshAll), [refreshAll]);
  const handleDeleteBackup = useCallback((name: string) => doDeleteBackup(name, refreshAll), [refreshAll]);
  const handleRestoreBackup = useCallback((name: string) => doRestoreBackup(name, setLoading, refreshAll), [refreshAll]);
  const handleDownloadBackup = useCallback((name: string) => doDownloadBackup(name), []);
  const handleCleanupExpiredBackups = useCallback(() => doCleanupExpiredBackups(setLoading, settings, refreshAll), [settings, refreshAll]);
  const handleCleanupArticles = useCallback(() => doCleanupArticles(setLoading, settings, refreshAll), [settings, refreshAll]);

  const handleExportConfig = async () => {
    try {
      const settingsRaw = localStorage.getItem('flore-settings');
      const theme = localStorage.getItem('theme');
      // 一并导出后端 Setting 表（含备份策略、留存策略、代理等），确保配置备份可完整迁移（M-A1）
      let serverSettings: Record<string, string> = {};
      try {
        const res = await fetch(`${getApi()}/settings`);
        if (res.ok) serverSettings = (await res.json()) as Record<string, string>;
      } catch {
        // 后端不可达时仅导出前端配置
      }
      const config = {
        _version: 1,
        _exportedAt: new Date().toISOString(),
        settings: settingsRaw ? JSON.parse(settingsRaw) : {},
        theme: theme || 'system',
        serverSettings,
      };
      const configJSON = JSON.stringify(config, null, 2);

      const app = getDesktopApp();
      if (app?.SaveConfigFile) {
        await app.SaveConfigFile(configJSON);
        showToast('配置已导出');
        return;
      }

      const blob = new Blob([configJSON], { type: 'application/json' });
      const handle = await window.showSaveFilePicker({
        suggestedName: `flore-config-${new Date().toISOString().slice(0, 10)}.json`,
        types: [{ description: 'JSON 配置文件', accept: { 'application/json': ['.json'] } }],
      });
      const writable = await handle.createWritable();
      await writable.write(blob);
      await writable.close();
      showToast('配置已导出');
    } catch (err) {
      // 用户取消文件选择时不报错
      if (err instanceof Error && err.name === 'AbortError') return;
      showToast('导出失败');
    }
  };

  const handleImportConfig = async () => {
    const ok = await importConfig();
    if (ok) {
      showToast('配置已导入，请重启应用生效');
    } else {
      showToast('导入失败：无效的配置文件');
    }
  };

  return (
    <div>
      {SectionConfigManagement(handleImportConfig, handleExportConfig)}
      {SectionDatabaseManagement(dbInputRef, importDatabase, exportDatabase, handleVacuum, onRestartApp, loading, dbInfo)}
      {SectionCacheCleanup(cacheStats, handleClearReadability, handleRebuildSearch, loading)}
      {SectionBackupManagement(backups, dbInfo, handleCreateBackup, handleDeleteBackup, handleRestoreBackup, handleDownloadBackup, refreshAll, loading)}
      {SectionBackupStrategy(settings, updateSetting, handleCleanupExpiredBackups, loading)}
      {SectionArticleRetention(settings, updateSetting, handleCleanupArticles, loading)}
      {SectionStatistics(cacheStats)}
    </div>
  );
}
