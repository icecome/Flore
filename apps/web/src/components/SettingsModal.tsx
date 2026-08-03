import { useState, useEffect, useRef } from 'react';
import { useFocusTrap } from './ModalLayout';
import {
  Settings as SettingsIcon,
  X,
  Sun,
  Moon,
  Rss,
  Info,
  Filter,
  Monitor,
  Database,
  Palette,
  Bell,
  Globe,
  Keyboard,
} from './icons';
import { cn } from '../lib/cn';
import {
  type AppSettings,
  type MarkReadMode,
  type ListDensity,
  type ListSortOrder,
  type ListDateFormat,
  THEME_OPTIONS,
  LETTER_SPACING_OPTIONS,
  ACCENT_PRESETS,
  MARK_READ_OPTIONS,
  LIST_DENSITY_OPTIONS,
  LIST_SORT_OPTIONS,
  LIST_DATE_FORMAT_OPTIONS,
  applyAccentColor,
} from '../utils/settings';
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
import ConfirmDialog from './ConfirmDialog';
import { Section, Row, Toggle, Select, Slider } from './settings/SettingsShared';
import SettingsSourcesTab from './settings/SettingsSourcesTab';
import EditSourceModal from './EditSourceModal';
import SettingsRulesTab, { type RuleFormState } from './settings/SettingsRulesTab';
import SettingsAboutTab from './settings/SettingsAboutTab';
import SettingsDataTab from './settings/SettingsDataTab';
import SettingsNotifyTab from './settings/SettingsNotifyTab';
import SettingsNetworkTab from './settings/SettingsNetworkTab';
import SettingsShortcutsTab from './settings/SettingsShortcutsTab';

type TabId = 'general' | 'appearance' | 'sources' | 'rules' | 'data' | 'notify' | 'network' | 'shortcuts' | 'about';
type Theme = 'light' | 'dark' | 'system';

interface Props {
  settings: AppSettings;
  onSettingsChange: (settings: AppSettings) => void;
  onClose: () => void;
  onSourcesChanged?: () => void;
  onAddSource?: () => void;
  sources: Source[];
  folders: Folder[];
}

const TABS: { id: TabId; label: string; icon: React.ReactNode }[] = [
  { id: 'general', label: '通用', icon: <SettingsIcon size={17} /> },
  { id: 'appearance', label: '外观', icon: <Palette size={17} /> },
  { id: 'sources', label: '订阅源', icon: <Rss size={17} /> },
  { id: 'rules', label: '规则过滤', icon: <Filter size={17} /> },
  { id: 'data', label: '数据管理', icon: <Database size={17} /> },
  { id: 'notify', label: '通知与驻留', icon: <Bell size={17} /> },
  { id: 'network', label: '网络设置', icon: <Globe size={17} /> },
  { id: 'shortcuts', label: '快捷键', icon: <Keyboard size={17} /> },
  { id: 'about', label: '关于', icon: <Info size={17} /> },
];

const FONT_FAMILY_OPTIONS: { value: AppSettings['readerFontFamily']; label: string }[] = [
  { value: 'serif', label: '衬线体' },
  { value: 'sans', label: '无衬线体' },
  { value: 'mono', label: '等宽字体' },
];

const INTERVAL_OPTIONS: { value: number; label: string }[] = [
  { value: 30, label: '30 分钟' },
  { value: 60, label: '1 小时' },
  { value: 120, label: '2 小时' },
  { value: 180, label: '3 小时' },
  { value: 360, label: '6 小时' },
  { value: 720, label: '12 小时' },
  { value: 1440, label: '24 小时' },
];

const OPEN_MODE_OPTIONS: { value: AppSettings['openArticleMode']; label: string }[] = [
  { value: 'rss', label: '摘要模式' },
  { value: 'readability', label: '全文模式' },
  { value: 'iframe', label: '网页模式' },
  { value: 'browser', label: '浏览器打开' },
];

export default function SettingsModal({ settings, onSettingsChange, onClose, onSourcesChanged, onAddSource, sources, folders }: Props) {
  const [activeTab, setActiveTab] = useState<TabId>('general');
  const [theme, setTheme] = useState<Theme>(() => {
    // 用 AppSettings.appTheme 初始化，避免弹窗内状态与全局不一致
    return (settings.appTheme === 'light' || settings.appTheme === 'dark') ? settings.appTheme : 'system';
  });
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

  useEffect(() => {
    // 隐私模式下访问 localStorage 可能抛错，读失败时回退到跟随系统
    try {
      const saved = localStorage.getItem('theme');
      setTheme(saved === 'light' || saved === 'dark' ? saved : 'system');
    } catch (err) {
      console.error('Failed to read theme from localStorage:', err);
      setTheme('system');
    }
  }, []);

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
      showToast('获取过滤规则失败');
    }
  };

  const updateSetting = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    onSettingsChange({ ...settings, [key]: value });
  };

  const applyTheme = (value: Theme) => {
    setTheme(value);
    // 主题持久化失败不应阻断当前会话的视觉切换
    try {
      if (value === 'system') localStorage.removeItem('theme');
      else localStorage.setItem('theme', value);
    } catch (err) {
      console.error('Failed to persist theme to localStorage:', err);
    }
    if (value === 'system') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      document.documentElement.setAttribute('data-theme', prefersDark ? 'dark' : 'light');
      updateSetting('appTheme', 'system');
    } else {
      document.documentElement.setAttribute('data-theme', value);
      updateSetting('appTheme', value);
    }
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
      showToast('已删除订阅源');
    } catch (err) {
      console.error('Failed to delete sources:', err);
      showToast('删除失败');
    }
  };

  const notifyHideResult = (failedCount: number, total: number) => {
    if (failedCount === 0) {
      showToast('已在全部文章隐藏');
    } else if (failedCount === total) {
      showToast('设置失败');
    } else {
      showToast(`部分设置失败（${failedCount}/${total}）`);
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
      showToast(folderId == null ? '已移出文件夹' : '已移动到目标文件夹');
    } else if (failed === ids.length) {
      showToast('移动失败');
    } else {
      showToast(`部分移动失败（${failed}/${ids.length}）`);
    }
  };

  const toggleHideSource = async (source: Source) => {
    try {
      await updateSource(source.id, { hideInTimeline: !source.hideInTimeline });
      onSourcesChanged?.();
      showToast(source.hideInTimeline ? '已在全部文章显示' : '已在全部文章隐藏');
    } catch (err) {
      console.error('Failed to toggle hide source:', err);
      showToast('操作失败');
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
      showToast('订阅源已更新');
    } catch (err) {
      console.error('Failed to edit source:', err);
      showToast('编辑订阅源失败');
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

  const importOPML = async (fileOrXml: File | string) => {
    try {
      const text = typeof fileOrXml === 'string' ? fileOrXml : await fileOrXml.text();
      await importOPMLApi(text);
      showToast('OPML 导入成功');
      onSourcesChanged?.();
    } catch (err) {
      console.error('Failed to import OPML:', err);
      showToast(err instanceof Error && err.message ? `OPML 导入失败：${err.message}` : 'OPML 导入失败');
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
      showToast('导出失败');
    }
  };

  const importDatabase = async (file: File) => {
    try {
      await restoreDatabase(file);
      showToast('数据库恢复成功，请重启应用');
    } catch (err) {
      console.error('Failed to restore database:', err);
      showToast('数据库恢复失败');
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
      showToast('导出失败');
    }
  };

  const restartApp = async () => {
    const app = getDesktopApp();
    if (!app?.RestartApp) {
      showToast('当前环境不支持重启');
      return;
    }
    try {
      await app.RestartApp();
      showToast('正在重启应用...');
    } catch (err) {
      console.error('Failed to restart app:', err);
      showToast('重启失败');
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
    if (!name) { showToast('请填写规则名称'); return null; }
    const validConditions = ruleForm.conditions.filter((c) => c.value.trim());
    if (validConditions.length === 0) { showToast('请至少添加一个有效的匹配条件'); return null; }
    if (ruleForm.scope === 'source' && ruleForm.sourceId == null) { showToast('请选择订阅源'); return null; }
    if (ruleForm.scope === 'folder' && ruleForm.folderId == null) { showToast('请选择文件夹'); return null; }
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
      showToast('规则已保存');
    } catch (err) {
      console.error('Failed to save filter rule:', err);
      showToast('保存失败');
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
      showToast('状态已更新');
    } catch (err) {
      console.error('Failed to toggle filter rule:', err);
      showToast('更新失败');
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
      showToast('规则已删除');
    } catch (err) {
      console.error('Failed to delete filter rule:', err);
      showToast('删除失败');
    }
  };

  const renderTab = () => {
    switch (activeTab) {
      case 'general':
        return (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <Section title="时间线行为">
              <Row
                title="启动时仅显示未读"
                desc="应用启动时默认只显示未读内容。"
                control={
                  <Toggle
                    checked={settings.unreadOnStart}
                    onChange={(v) => updateSetting('unreadOnStart', v)}
                  />
                }
              />
              <Row
                title="淡化已读条目"
                desc="在时间线中降低已读条目的显示颜色。"
                control={
                  <Toggle checked={settings.dimRead} onChange={(v) => updateSetting('dimRead', v)} />
                }
              />
              <Row
                title="侧边栏隐藏私密订阅"
                desc="开启后，侧边栏隐藏标记为私密的订阅。"
                control={
                  <Toggle
                    checked={settings.hidePrivateInSidebar}
                    onChange={(v) => updateSetting('hidePrivateInSidebar', v)}
                  />
                }
              />
              <Row
                title="时间线隐藏私密文章"
                desc="开启后，时间线隐藏私密订阅的文章。"
                control={
                  <Toggle
                    checked={settings.hidePrivateInTimeline}
                    onChange={(v) => updateSetting('hidePrivateInTimeline', v)}
                  />
                }
              />
              <Row
                title="隐藏已读源"
                desc="在订阅列表中隐藏没有未读条目的订阅源。"
                control={
                  <Toggle checked={settings.hideRead} onChange={(v) => updateSetting('hideRead', v)} />
                }
              />
            </Section>

            <Section title="自动标记已读">
              <Row
                title="标记方式"
                desc="选择自动标记文章为已读的触发方式。"
                control={
                  <Select
                    value={settings.markReadMode}
                    options={MARK_READ_OPTIONS}
                    onChange={(v) => updateSetting('markReadMode', v as MarkReadMode)}
                  />
                }
              />
              {settings.markReadMode === 'hover' && (
                <Row
                  title="悬停延迟"
                  desc="鼠标悬停多久后标记为已读。"
                  control={
                    <Slider
                      value={settings.markReadHoverDelay}
                      min={200}
                      max={3000}
                      step={100}
                      unit="ms"
                      onChange={(v) => updateSetting('markReadHoverDelay', v)}
                    />
                  }
                />
              )}
            </Section>

            <Section title="文章列表偏好">
              <Row
                title="显示密度"
                desc="调整文章列表的行间距。"
                control={
                  <Select
                    value={settings.listDensity}
                    options={LIST_DENSITY_OPTIONS}
                    onChange={(v) => updateSetting('listDensity', v as ListDensity)}
                  />
                }
              />
              <Row
                title="排序方式"
                desc="文章列表的默认排序。"
                control={
                  <Select
                    value={settings.listSortOrder}
                    options={LIST_SORT_OPTIONS}
                    onChange={(v) => updateSetting('listSortOrder', v as ListSortOrder)}
                  />
                }
              />
              <Row
                title="显示摘要预览"
                desc="在列表中显示文章摘要。"
                control={
                  <Toggle
                    checked={settings.listShowPreview}
                    onChange={(v) => updateSetting('listShowPreview', v)}
                  />
                }
              />
              <Row
                title="日期格式"
                desc="文章时间的显示格式。"
                control={
                  <Select
                    value={settings.listDateFormat}
                    options={LIST_DATE_FORMAT_OPTIONS}
                    onChange={(v) => updateSetting('listDateFormat', v as ListDateFormat)}
                  />
                }
              />
            </Section>

            <Section title="抓取与同步">
              <Row
                title="默认抓取间隔"
                desc="订阅源将按此间隔自动抓取更新。"
                control={
                  <Select
                    value={settings.defaultInterval}
                    options={INTERVAL_OPTIONS}
                    onChange={(v) => updateSetting('defaultInterval', Number(v))}
                  />
                }
              />
              <Row
                title="启动时自动抓取"
                desc="应用启动后立即执行一次全量抓取。"
                control={
                  <Toggle
                    checked={settings.autoFetchOnStart}
                    onChange={(v) => updateSetting('autoFetchOnStart', v)}
                  />
                }
              />
            </Section>

            <Section title="文章打开方式">
              <Row
                title="默认打开方式"
                desc="点击文章时默认使用的打开方式。"
                control={
                  <Select
                    value={settings.openArticleMode}
                    options={OPEN_MODE_OPTIONS}
                    onChange={(v) =>
                      updateSetting('openArticleMode', v as AppSettings['openArticleMode'])
                    }
                  />
                }
              />
            </Section>

            <Section title="自动分组">
              <Row
                title="按域名自动分组"
                desc="创建新订阅源时自动按网站域名生成文件夹。"
                control={
                  <Toggle
                    checked={settings.autoGroup}
                    onChange={(v) => updateSetting('autoGroup', v)}
                  />
                }
              />
            </Section>
          </div>
        );

      case 'appearance':
        return (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <Section title="主题">
              <Row
                title="主题"
                desc="选择应用的整体外观。"
                control={<ThemeSelector theme={theme} onChange={applyTheme} />}
              />
            </Section>
            <Section title="强调色">
              <Row
                title="强调色"
                desc="自定义应用的强调色，影响按钮、选中态、链接等。"
                control={
                  <div className="flex items-center gap-1.5">
                    {ACCENT_PRESETS.map((preset) => (
                      <button
                        key={preset.color}
                        onClick={() => {
                          updateSetting('accentColor', preset.color);
                          applyAccentColor(preset.color);
                        }}
                        className={cn(
                          'h-7 w-7 rounded-md border-2 transition-transform hover:scale-110',
                          (settings.accentColor || '#7B68EE') === preset.color
                            ? 'border-text-primary scale-110'
                            : 'border-transparent'
                        )}
                        style={{ backgroundColor: preset.color }}
                        title={preset.name}
                      />
                    ))}
                    <label
                      className="relative flex h-7 w-7 cursor-pointer items-center justify-center rounded-md border-2 border-dashed border-border transition-colors hover:border-border-strong"
                      title="自定义颜色"
                    >
                      <input
                        type="color"
                        value={settings.accentColor || '#7B68EE'}
                        onInput={(e) => {
                          applyAccentColor((e.target as HTMLInputElement).value);
                        }}
                        onChange={(e) => {
                          updateSetting('accentColor', e.target.value);
                        }}
                        className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
                      />
                      <span className="text-[10px] text-muted">+</span>
                    </label>
                  </div>
                }
              />
            </Section>
            <Section title="排版设置">
              <Row
                title="默认字体"
                control={
                  <Select
                    value={settings.readerFontFamily}
                    options={FONT_FAMILY_OPTIONS}
                    onChange={(v) =>
                      updateSetting('readerFontFamily', v as AppSettings['readerFontFamily'])
                    }
                  />
                }
              />
              <Row
                title="默认字号"
                control={
                  <Slider
                    value={settings.readerFontSize}
                    min={14}
                    max={24}
                    step={1}
                    unit="px"
                    onChange={(v) => updateSetting('readerFontSize', v)}
                  />
                }
              />
              <Row
                title="默认行高"
                control={
                  <Slider
                    value={settings.readerLineHeight}
                    min={1.2}
                    max={2.4}
                    step={0.05}
                    onChange={(v) => updateSetting('readerLineHeight', v)}
                  />
                }
              />
              <Row
                title="最大宽度"
                control={
                  <Slider
                    value={settings.readerMaxWidth}
                    min={500}
                    max={1000}
                    step={10}
                    unit="px"
                    onChange={(v) => updateSetting('readerMaxWidth', v)}
                  />
                }
              />
              <Row
                title="字间距"
                control={
                  <Select
                    value={settings.readerLetterSpacing}
                    options={LETTER_SPACING_OPTIONS}
                    onChange={(v) =>
                      updateSetting('readerLetterSpacing', v as AppSettings['readerLetterSpacing'])
                    }
                  />
                }
              />
              <Row
                title="阅读主题"
                control={
                  <Select
                    value={settings.readerTheme}
                    options={THEME_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
                    onChange={(v) => updateSetting('readerTheme', v as AppSettings['readerTheme'])}
                  />
                }
              />
            </Section>
          </div>
        );

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

      case 'shortcuts':
        return <SettingsShortcutsTab />;

      case 'about':
        return <SettingsAboutTab />;

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
          className="flex h-[740px] max-h-[90vh] w-full max-w-[920px] overflow-hidden rounded-lg border border-border bg-elevated shadow-lg animate-modal-scale-in"
          onClick={(e) => e.stopPropagation()}
        >
          <nav className="flex w-[200px] shrink-0 flex-col border-r border-border bg-canvas p-3">
            <div className="mb-2 px-2 py-1.5 text-[15px] font-semibold text-primary">设置</div>
            {TABS.map((t) => (
              <button
                key={t.id}
                onClick={() => setActiveTab(t.id)}
                className={cn(
                  'mb-0.5 flex items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13.5px] transition-colors',
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
            <div className="flex h-[56px] shrink-0 items-center justify-between border-b border-border px-6">
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
            <div ref={contentRef} className="flex-1 min-h-0 flex flex-col overflow-hidden px-6 py-5">
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

function ThemeSelector({ theme, onChange }: { theme: Theme; onChange: (t: Theme) => void }) {
  const opts: { value: Theme; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: '浅色', icon: <Sun size={14} /> },
    { value: 'dark', label: '深色', icon: <Moon size={14} /> },
    { value: 'system', label: '跟随系统', icon: <Monitor size={14} /> },
  ];

  return (
    <div className="flex gap-1.5">
      {opts.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-[12.5px] transition-colors',
            theme === o.value
              ? 'border-primary bg-primary-subtle text-primary'
              : 'border-border text-secondary hover:border-border-strong hover:text-primary'
          )}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  );
}
