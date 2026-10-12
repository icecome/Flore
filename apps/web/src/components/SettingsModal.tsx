import { useState, useEffect, useRef } from 'react';
import { useFocusTrap } from './ModalLayout';
import {
  Settings as SettingsIcon,
  X,
  Rss,
  Info,
  Filter,
  Database,
  Palette,
  Bell,
  Globe,
  Keyboard,
  RefreshCw,
} from './icons';
import { cn } from '../lib/cn';
import type { AppSettings } from '../utils/settings';
import type { Source, Folder, FilterRule, FilterCondition } from '../types';
import {
  getDesktopApp,
  downloadBlob,
  deleteSourcesBatch,
  updateSource,
  triggerFetch,
  importOPML as importOPMLApi,
  restoreDatabase,
  getDatabaseExportUrl,
  listFilterRules,
  saveFilterRule,
  deleteFilterRule,
} from '../utils/api';
import { showToast } from '../utils/toast';
import { translateBackendError } from '../utils/backendError';
import ConfirmDialog from './ConfirmDialog';
import EditSourceModal from './EditSourceModal';
import SettingsGeneralTab from './settings/SettingsGeneralTab';
import SettingsAppearanceTab from './settings/SettingsAppearanceTab';
import SettingsSourcesTab from './settings/SettingsSourcesTab';
import SettingsRulesTab, { type RuleFormState } from './settings/SettingsRulesTab';
import SettingsAboutTab from './settings/SettingsAboutTab';
import SettingsDataTab from './settings/SettingsDataTab';
import SettingsNotifyTab from './settings/SettingsNotifyTab';
import SettingsNetworkTab from './settings/SettingsNetworkTab';
import SettingsSyncTab from './settings/SettingsSyncTab';
import SettingsShortcutsTab from './settings/SettingsShortcutsTab';

export type TabId = 'general' | 'appearance' | 'sources' | 'rules' | 'data' | 'notify' | 'network' | 'sync' | 'shortcuts' | 'about';

interface Props {
  settings: AppSettings;
  onSettingsChange: (settings: AppSettings) => void;
  onClose: () => void;
  onSourcesChanged?: () => void;
  onAddSource?: () => void;
  sources: Source[];
  folders: Folder[];
  /** 打开时定位到的 Tab（同步指示器跳转用） */
  initialTab?: TabId;
}

const TABS: { id: TabId; label: string; icon: React.ReactNode }[] = [
  { id: 'general', label: '通用', icon: <SettingsIcon size={17} /> },
  { id: 'appearance', label: '外观', icon: <Palette size={17} /> },
  { id: 'sources', label: '订阅源', icon: <Rss size={17} /> },
  { id: 'rules', label: '规则过滤', icon: <Filter size={17} /> },
  { id: 'data', label: '数据管理', icon: <Database size={17} /> },
  { id: 'notify', label: '通知与驻留', icon: <Bell size={17} /> },
  { id: 'network', label: '网络设置', icon: <Globe size={17} /> },
  { id: 'sync', label: '同步', icon: <RefreshCw size={17} /> },
  { id: 'shortcuts', label: '快捷键', icon: <Keyboard size={17} /> },
  { id: 'about', label: '关于', icon: <Info size={17} /> },
];

export default function SettingsModal({ settings, onSettingsChange, onClose, onSourcesChanged, onAddSource, sources, folders, initialTab }: Props) {
  const [activeTab, setActiveTab] = useState<TabId>(initialTab ?? 'general');
  const [sourceFilter, setSourceFilter] = useState<'all' | 'ok' | 'bad'>('all');
  const [selectedSourceIds, setSelectedSourceIds] = useState<number[]>([]);
  const [isChecking, setIsChecking] = useState(false);
  const [checkingIds, setCheckingIds] = useState<Set<number>>(new Set());
  const [editTarget, setEditTarget] = useState<Source | null>(null);
  const [rules, setRules] = useState<FilterRule[]>([]);
  const [ruleForm, setRuleForm] = useState<RuleFormState>({
    name: '',
    conditions: [{ field: 'title', operator: 'contains', value: '' }],
    action: 'markRead',
    scope: 'global',
    sourceId: null,
    folderId: null,
    enabled: true,
  });
  const [editingRuleId, setEditingRuleId] = useState<number | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ type: 'sources' | 'rule' | 'source'; id?: number } | null>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  // 焦点陷阱 + Esc 关闭（与 ModalLayout 同源，保证可访问性一致）
  const dialogRef = useFocusTrap(onClose);

  useEffect(() => { fetchRules(); }, []);

  // 单一数据源：sources/folders 直接消费 App 全局 prop，不再持有本地副本。
  // 所有源结构变更（增/删/改/移动/导入）统一通过 onSourcesChanged 刷新 App 全局，
  // prop 随之更新并反映到本面板，避免双数据源分叉与重开面板数据复现。

  useEffect(() => {
    if (contentRef.current) contentRef.current.scrollTop = 0;
  }, [activeTab]);

  const fetchRules = async () => {
    try {
      const data = await listFilterRules();
      setRules(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch filter rules:', err);
      showToast('获取过滤规则失败', 'error');
    }
  };

  const updateSetting = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    onSettingsChange({ ...settings, [key]: value });
  };

  const filteredSources = sources.filter((s) => {
    if (sourceFilter === 'all') return true;
    const neverFetched = s.lastFetchAt === null;
    if (neverFetched) return false;
    const bad = s.fetchFailCount >= 3;
    if (sourceFilter === 'ok') return !bad;
    if (sourceFilter === 'bad') return bad;
    return true;
  });

  const toggleSourceSelection = (id: number) => {
    setSelectedSourceIds((prev) =>
      prev.includes(id) ? prev.filter((i) => i !== id) : [...prev, id]
    );
  };

  const toggleSelectAll = () => {
    if (selectedSourceIds.length === filteredSources.length) {
      setSelectedSourceIds([]);
    } else {
      setSelectedSourceIds(filteredSources.map((s) => s.id));
    }
  };

  const deleteSelectedSources = () => {
    if (selectedSourceIds.length === 0) return;
    setDeleteConfirm({ type: 'sources' });
  };

  const deleteSingleSource = (source: Source) => {
    setDeleteConfirm({ type: 'source', id: source.id });
  };

  const confirmDeleteSources = async () => {
    if (deleteConfirm?.type !== 'sources' && deleteConfirm?.type !== 'source') return;
    const ids =
      deleteConfirm.type === 'source' && deleteConfirm.id != null
        ? [deleteConfirm.id]
        : [...selectedSourceIds];
    setDeleteConfirm(null);
    if (ids.length === 0) return;
    try {
      await deleteSourcesBatch(ids);
      setSelectedSourceIds([]);
      onSourcesChanged?.();
      showToast('已删除订阅源', 'success');
    } catch (err) {
      console.error('Failed to delete sources:', err);
      showToast('删除失败', 'error');
    }
  };

  const notifyHideResult = (failedCount: number, total: number) => {
    if (failedCount === 0) {
      showToast('已从时间线隐藏该订阅源', 'success');
    } else if (failedCount === total) {
      showToast('设置失败', 'error');
    } else {
      showToast(`部分设置失败（${failedCount}/${total}）`, 'error');
    }
  };

  const hideSelectedSources = async () => {
    const ids = selectedSourceIds;
    if (ids.length === 0) return;

    const results = await Promise.allSettled(
      ids.map((id) => updateSource(id, { hideInTimeline: true }))
    );

    const succeededResults = results.filter((r) => r.status === 'fulfilled');
    const succeededCount = succeededResults.length;
    setSelectedSourceIds([]);
    onSourcesChanged?.();
    notifyHideResult(results.length - succeededCount, ids.length);
  };

  const moveSelectedSourcesToFolder = async (folderId: number | null) => {
    const ids = [...selectedSourceIds];
    if (ids.length === 0) return;
    const results = await Promise.allSettled(ids.map((id) => updateSource(id, { folderId })));
    const failed = results.filter((r) => r.status === 'rejected').length;
    setSelectedSourceIds([]);
    onSourcesChanged?.();
    if (failed === 0) {
      showToast(folderId == null ? '已移出文件夹' : '已移动到目标文件夹', 'success');
    } else if (failed === ids.length) {
      showToast('移动失败', 'error');
    } else {
      showToast(`部分移动失败（${failed}/${ids.length}）`, 'error');
    }
  };

  const toggleHideSource = async (source: Source) => {
    try {
      await updateSource(source.id, { hideInTimeline: !source.hideInTimeline });
      onSourcesChanged?.();
      showToast(source.hideInTimeline ? '已恢复在时间线显示' : '已从时间线隐藏', 'success');
    } catch (err) {
      console.error('Failed to toggle hide source:', err);
      showToast('操作失败', 'error');
    }
  };

  const saveSourceEdit = async (params: {
    sourceId: number;
    name: string;
    url: string;
    folderId: number | null;
    isPrivate: boolean;
    hideInTimeline: boolean;
  }) => {
    try {
      await updateSource(params.sourceId, {
        name: params.name.trim(),
        url: params.url.trim(),
        folderId: params.folderId,
        isPrivate: params.isPrivate,
        hideInTimeline: params.hideInTimeline,
      });
      setEditTarget(null);
      onSourcesChanged?.();
      showToast('订阅源已更新', 'success');
    } catch (err) {
      console.error('Failed to edit source:', err);
      showToast('编辑订阅源失败', 'error');
    }
  };

  const refreshSourcesAfterCheck = async () => {
    try {
      await onSourcesChanged?.();
    } finally {
      setIsChecking(false);
      setCheckingIds(new Set());
    }
  };

  const checkAvailability = async () => {
    const ids = sources.map((s) => s.id);
    setIsChecking(true);
    setCheckingIds(new Set(ids));

    const queue = [...ids];
    // 单源检测失败不中断整体流程，失败计数由后端记录并在列表中呈现
    const checkOne = async (id: number) => {
      try {
        await triggerFetch({ sourceId: id });
      } catch (err) {
        console.error(`Failed to check source ${id}:`, err);
      } finally {
        setCheckingIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      }
    };

    const checkBatch = async () => {
      for (let id = queue.shift(); id !== undefined; id = queue.shift()) {
        await checkOne(id);
      }
    };

    const CONCURRENCY = 5;
    const workers = Array.from(
      { length: Math.min(CONCURRENCY, ids.length) },
      () => checkBatch()
    );
    await Promise.all(workers);
    await refreshSourcesAfterCheck();
  };

  // 坏源「立即重试」（T-v03-04）：手动提交不经调度退避，立即抓取一次
  const retrySource = async (sourceId: number) => {
    try {
      await triggerFetch({ sourceId });
      showToast('已提交重试，稍后自动刷新状态', 'success');
      // 抓取为后台异步执行，延迟刷新以呈现最新健康状态
      setTimeout(() => { onSourcesChanged?.(); }, 3000);
    } catch (err) {
      console.error('Failed to retry source:', err);
      showToast('重试失败', 'error');
    }
  };

  const importOPML = async (fileOrXml: File | string) => {
    try {
      const text = typeof fileOrXml === 'string' ? fileOrXml : await fileOrXml.text();
      await importOPMLApi(text);
      showToast('OPML 导入成功', 'success');
      onSourcesChanged?.();
    } catch (err) {
      console.error('Failed to import OPML:', err);
      showToast('OPML 导入失败：' + (translateBackendError(err instanceof Error ? err.message : '') ?? '未知原因'), 'error');
    }
  };

  const exportOPML = async () => {
    try {
      const app = getDesktopApp();
      if (app?.SaveOPMLFile) {
        await app.SaveOPMLFile();
        return;
      }
      await downloadBlob('/opml/export', 'subscriptions.opml');
    } catch (err) {
      console.error('Failed to export OPML:', err);
      showToast('导出失败', 'error');
    }
  };

  const importDatabase = async (file: File) => {
    try {
      await restoreDatabase(file);
      showToast('数据库恢复成功，请重启应用', 'success');
    } catch (err) {
      console.error('Failed to restore database:', err);
      showToast('数据库恢复失败', 'error');
    }
  };

  const exportDatabase = async () => {
    try {
      const app = getDesktopApp();
      if (app?.SaveDatabaseFile) {
        await app.SaveDatabaseFile();
        return;
      }
      window.location.href = getDatabaseExportUrl();
    } catch (err) {
      console.error('Failed to export database:', err);
      showToast('导出失败', 'error');
    }
  };

  const restartApp = async () => {
    const app = getDesktopApp();
    if (!app?.RestartApp) {
      showToast('当前环境不支持重启', 'error');
      return;
    }
    try {
      await app.RestartApp();
      showToast('正在重启应用……');
    } catch (err) {
      console.error('Failed to restart app:', err);
      showToast('重启失败', 'error');
    }
  };

  const resetRuleForm = () => {
    setRuleForm({
      name: '',
      conditions: [{ field: 'title', operator: 'contains', value: '' }],
      action: 'markRead',
      scope: 'global',
      sourceId: null,
      folderId: null,
      enabled: true,
    });
    setEditingRuleId(null);
  };

  const startEditRule = (rule: FilterRule) => {
    const conditions = rule.conditions.length > 0
      ? rule.conditions.map((c) => ({ field: c.field, operator: c.operator, value: c.value }))
      : [{ field: 'title' as const, operator: 'contains' as const, value: '' }];
    setRuleForm({
      name: rule.name,
      conditions,
      action: rule.action,
      scope: rule.scope,
      sourceId: rule.sourceId,
      folderId: rule.folderId,
      enabled: rule.enabled,
    });
    setEditingRuleId(rule.id);
  };

  const validateRuleForm = (): FilterRule['conditions'] | null => {
    const name = ruleForm.name.trim();
    if (!name) { showToast('请填写规则名称', 'error'); return null; }
    const validConditions = ruleForm.conditions.filter((c) => c.value.trim());
    if (validConditions.length === 0) { showToast('请至少添加一个有效的匹配条件', 'error'); return null; }
    if (ruleForm.scope === 'source' && ruleForm.sourceId == null) { showToast('请选择订阅源', 'error'); return null; }
    if (ruleForm.scope === 'folder' && ruleForm.folderId == null) { showToast('请选择文件夹', 'error'); return null; }
    return validConditions as FilterCondition[];
  };

  const buildRulePayload = (name: string, conditions: FilterCondition[]): Record<string, unknown> => ({
    name,
    enabled: ruleForm.enabled,
    priority: 0,
    scope: ruleForm.scope,
    sourceId: ruleForm.scope === 'source' ? ruleForm.sourceId : null,
    folderId: ruleForm.scope === 'folder' ? ruleForm.folderId : null,
    conditions,
    action: ruleForm.action,
  });

  const saveRule = async () => {
    const conditions = validateRuleForm();
    if (conditions == null) return;

    const body = buildRulePayload(ruleForm.name.trim(), conditions);
    try {
      await saveFilterRule(editingRuleId, body);
      await fetchRules();
      resetRuleForm();
      showToast('规则已保存', 'success');
    } catch (err) {
      console.error('Failed to save filter rule:', err);
      showToast('保存失败', 'error');
    }
  };

  const handleToggleRule = async (rule: FilterRule) => {
    const body = {
      name: rule.name,
      enabled: !rule.enabled,
      priority: rule.priority,
      scope: rule.scope,
      sourceId: rule.sourceId,
      folderId: rule.folderId,
      conditions: rule.conditions,
      action: rule.action,
    };
    try {
      await saveFilterRule(rule.id, body);
      await fetchRules();
      showToast('状态已更新', 'success');
    } catch (err) {
      console.error('Failed to toggle filter rule:', err);
      showToast('更新失败', 'error');
    }
  };

  const handleDeleteRule = (id: number) => {
    setDeleteConfirm({ type: 'rule', id });
  };

  const confirmDeleteRule = async () => {
    if (deleteConfirm?.type !== 'rule' || deleteConfirm.id == null) return;
    const id = deleteConfirm.id;
    setDeleteConfirm(null);
    try {
      await deleteFilterRule(id);
      await fetchRules();
      showToast('规则已删除', 'success');
    } catch (err) {
      console.error('Failed to delete filter rule:', err);
      showToast('删除失败', 'error');
    }
  };

  const renderTab = () => {
    switch (activeTab) {
      case 'general':
        return <SettingsGeneralTab settings={settings} updateSetting={updateSetting} />;

      case 'appearance':
        return <SettingsAppearanceTab settings={settings} updateSetting={updateSetting} />;

      case 'sources':
        return (
          <SettingsSourcesTab
            folders={folders}
            filteredSources={filteredSources}
            selectedSourceIds={selectedSourceIds}
            isChecking={isChecking}
            checkingIds={checkingIds}
            sourceFilter={sourceFilter}
            onSourcesChanged={onSourcesChanged}
            onAddSource={onAddSource}
            onToggleSelection={toggleSourceSelection}
            onToggleSelectAll={toggleSelectAll}
            onDeleteSources={deleteSelectedSources}
            onDeleteSource={deleteSingleSource}
            onHideSelected={hideSelectedSources}
            onHideSource={toggleHideSource}
            onMoveSelectedToFolder={moveSelectedSourcesToFolder}
            onEditSource={(s) => setEditTarget(s)}
            onCheckAvailability={checkAvailability}
            onRetrySource={retrySource}
            onImportOPML={importOPML}
            onExportOPML={exportOPML}
            onSourceFilterChange={(v) => { setSourceFilter(v); setSelectedSourceIds([]); }}
          />
        );

      case 'rules':
        return (
          <SettingsRulesTab
            rules={rules}
            ruleForm={ruleForm}
            editingRuleId={editingRuleId}
            sources={sources}
            folders={folders}
            onSaveRule={saveRule}
            onResetRuleForm={resetRuleForm}
            onStartEditRule={startEditRule}
            onToggleRule={handleToggleRule}
            onDeleteRule={handleDeleteRule}
            onRuleFormChange={setRuleForm}
            onDeleteConfirm={setDeleteConfirm}
          />
        );

      case 'data':
        return (
          <SettingsDataTab
            settings={settings}
            updateSetting={updateSetting}
            onRestartApp={restartApp}
          />
        );

      case 'notify':
        return (
          <SettingsNotifyTab settings={settings} updateSetting={updateSetting} />
        );

      case 'network':
        return (
          <SettingsNetworkTab settings={settings} updateSetting={updateSetting} />
        );

      case 'sync':
        // 同步会新增源/文章，完成后回调 onSourcesChanged 刷新 App 全局数据
        return <SettingsSyncTab onSourcesChanged={onSourcesChanged} />;

      case 'shortcuts':
        return <SettingsShortcutsTab />;

      case 'about':
        return <SettingsAboutTab settings={settings} updateSetting={updateSetting} />;

      default:
        return null;
    }
  };

  return (
    <>
      <div
        className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--overlay)] p-4 backdrop-blur-sm animate-modal-fade-in"
        onClick={(e) => e.target === e.currentTarget && onClose()}
        role="dialog"
        aria-modal="true"
      >
        <div
          ref={dialogRef}
          className="flex h-[740px] max-h-[90vh] w-full max-w-[920px] flex-col overflow-hidden rounded-lg border border-border bg-elevated shadow-lg animate-modal-scale-in md:flex-row"
          onClick={(e) => e.stopPropagation()}
        >
          {/* 标签导航：桌面为左侧 200px 竖排；<768px 变为顶部横向可滚动条，
              避免 375px 下竖排 rail 与内容互相挤压（P1-006）。 */}
          <nav className="flex shrink-0 flex-row gap-1 overflow-x-auto border-b border-border bg-canvas p-2 md:mb-0 md:w-[200px] md:flex-col md:overflow-visible md:border-b-0 md:border-r md:p-3">
            <div className="hidden px-2 py-1.5 text-[15px] font-semibold text-primary md:mb-2 md:block">设置</div>
            {TABS.map((t) => (
              <button
                key={t.id}
                onClick={() => setActiveTab(t.id)}
                className={cn(
                  'mb-0.5 flex shrink-0 items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13.5px] transition-colors md:w-full',
                  activeTab === t.id ? 'bg-active font-medium text-primary' : 'text-secondary hover:bg-hover'
                )}
              >
                <span className={cn(activeTab === t.id ? 'text-primary' : 'text-muted')}>
                  {t.icon}
                </span>
                {t.label}
              </button>
            ))}
          </nav>

          <div className="flex min-w-0 flex-1 flex-col bg-surface">
            <div className="flex h-[56px] shrink-0 items-center justify-between border-b border-border px-4 md:px-6">
              <h2 className="text-[16px] font-semibold text-primary">
                {TABS.find((t) => t.id === activeTab)?.label}
              </h2>
              <button
                onClick={onClose}
                className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted transition-colors hover:bg-hover hover:text-primary"
                aria-label="关闭"
              >
                <X size={18} />
              </button>
            </div>
            <div ref={contentRef} className="flex-1 min-h-0 flex flex-col overflow-hidden px-4 py-4 md:px-6 md:py-5">
              {renderTab()}
            </div>
          </div>
        </div>
      </div>

      {deleteConfirm?.type === 'sources' && (
        <ConfirmDialog
          message={`确定要删除选中的 ${selectedSourceIds.length} 个订阅源吗？`}
          danger
          onConfirm={confirmDeleteSources}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}

      {deleteConfirm?.type === 'source' && (
        <ConfirmDialog
          message="确定要取消订阅该订阅源吗？"
          danger
          onConfirm={confirmDeleteSources}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}

      {editTarget && (
        <EditSourceModal
          source={editTarget}
          folders={folders}
          onClose={() => setEditTarget(null)}
          onSubmit={saveSourceEdit}
        />
      )}

      {deleteConfirm?.type === 'rule' && (
        <ConfirmDialog
          message="确定要删除这条规则吗？"
          danger
          onConfirm={confirmDeleteRule}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}
    </>
  );
}

