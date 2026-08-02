import { useState } from 'react';
import { Filter, Check, Pencil, Trash2, Plus, X, Search, RefreshCw, ExternalLink } from '../icons';
import { cn } from '../../lib/cn';
import type { Source, Folder, FilterRule, FilterCondition, Item } from '../../types';
import { testFilterRule } from '../../utils/api';
import { showToast } from '../../utils/toast';
import ModalLayout from '../ModalLayout';
import { Section, SmallBtn } from './SettingsShared';

export interface RuleCondition {
  field: FilterCondition['field'];
  operator: FilterCondition['operator'];
  value: string;
}

export interface RuleFormState {
  name: string;
  conditions: RuleCondition[];
  action: FilterRule['action'];
  scope: FilterRule['scope'];
  sourceId: number | null;
  folderId: number | null;
  enabled: boolean;
}

interface Props {
  rules: FilterRule[];
  ruleForm: RuleFormState;
  editingRuleId: number | null;
  sources: Source[];
  folders: Folder[];
  onSaveRule: () => Promise<void>;
  onResetRuleForm: () => void;
  onStartEditRule: (rule: FilterRule) => void;
  onToggleRule: (rule: FilterRule) => Promise<void>;
  onDeleteRule: (id: number) => void;
  onRuleFormChange: (form: RuleFormState) => void;
  onDeleteConfirm: (confirm: { type: 'sources' | 'rule'; id?: number } | null) => void;
}

const ACTION_LABELS: Record<FilterRule['action'], string> = {
  markRead: '标记为已读',
  star: '添加收藏',
  readLater: '加入稍后阅读',
};

const SCOPE_LABELS: Record<FilterRule['scope'], string> = {
  global: '全部订阅',
  source: '指定订阅源',
  folder: '指定文件夹',
};

const FIELD_LABELS: Record<FilterCondition['field'], string> = {
  title: '标题',
  desc: '摘要',
  author: '作者',
  link: '链接',
};

const OPERATOR_LABELS: Record<FilterCondition['operator'], string> = {
  contains: '包含',
  notContains: '不包含',
  equals: '等于',
  notEquals: '不等于',
};

const FIELD_OPTIONS = Object.entries(FIELD_LABELS).map(([value, label]) => ({ value, label }));
const OPERATOR_OPTIONS = Object.entries(OPERATOR_LABELS).map(([value, label]) => ({ value, label }));

// isSafeLink 校验 URL 协议为 http/https，防止 javascript:/data: XSS
function isSafeLink(url: string): boolean {
  try {
    const u = new URL(url);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
}

// 规则测试结果单行
function TestResultItem({ item }: { item: Item }) {
  const date = item.pubDate ? new Date(item.pubDate) : null;
  const dateStr = date && !isNaN(date.getTime())
    ? date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
    : '未知时间';
  const safe = isSafeLink(item.link);
  return (
    <a
      href={safe ? item.link : undefined}
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-start gap-2.5 rounded-md border border-border-subtle px-3 py-2 transition-colors hover:border-border-strong hover:bg-hover"
      onClick={(e) => { if (!safe) e.preventDefault(); }}
    >
      <ExternalLink size={13} className="mt-0.5 shrink-0 text-muted" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] font-medium text-primary">{item.title}</div>
        <div className="mt-0.5 flex items-center gap-2 text-[11.5px] text-muted">
          <span className="truncate">{item.sourceName}</span>
          <span>·</span>
          <span className="shrink-0">{dateStr}</span>
        </div>
      </div>
    </a>
  );
}

export default function SettingsRulesTab(props: Props) {
  const { rules, ruleForm, editingRuleId, sources, folders, onSaveRule, onResetRuleForm, onStartEditRule, onToggleRule, onDeleteRule, onRuleFormChange } = props;

  const [testingId, setTestingId] = useState<number | null>(null);
  const [testResults, setTestResults] = useState<{ ruleName: string; items: Item[] } | null>(null);

  const updateCondition = (index: number, patch: Partial<RuleCondition>) => {
    const conditions = ruleForm.conditions.map((c, i) => (i === index ? { ...c, ...patch } : c));
    onRuleFormChange({ ...ruleForm, conditions });
  };

  const addCondition = () => {
    onRuleFormChange({
      ...ruleForm,
      conditions: [...ruleForm.conditions, { field: 'title', operator: 'contains', value: '' }],
    });
  };

  const removeCondition = (index: number) => {
    onRuleFormChange({
      ...ruleForm,
      conditions: ruleForm.conditions.filter((_, i) => i !== index),
    });
  };

  const ruleScopeTarget = (rule: FilterRule) => {
    if (rule.scope === 'source' && rule.sourceId) {
      const source = sources.find((s) => s.id === rule.sourceId);
      return source ? source.name : `源 #${rule.sourceId}`;
    }
    if (rule.scope === 'folder' && rule.folderId) {
      const folder = folders.find((f) => f.id === rule.folderId);
      return folder ? folder.name : `文件夹 #${rule.folderId}`;
    }
    return '';
  };

  const formatRuleDescription = (rule: FilterRule) => {
    const parts = rule.conditions.map((c) => {
      const val = c.value.length > 20 ? c.value.slice(0, 20) + '...' : c.value;
      return `${FIELD_LABELS[c.field]} ${OPERATOR_LABELS[c.operator]} "${val}"`;
    });
    const conditionText = parts.length > 0 ? parts.join(' 且 ') : '无条件';
    const target = ruleScopeTarget(rule);
    return `当 ${conditionText}，则 ${ACTION_LABELS[rule.action]} · 范围：${SCOPE_LABELS[rule.scope]}${target ? ` · ${target}` : ''}`;
  };

  const handleTestRule = async (rule: FilterRule) => {
    setTestingId(rule.id);
    try {
      const items = await testFilterRule(rule.id);
      setTestResults({ ruleName: rule.name, items: Array.isArray(items) ? items : [] });
    } catch (err) {
      console.error('Failed to test filter rule:', err);
      showToast('规则测试失败');
    } finally {
      setTestingId(null);
    }
  };

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title={editingRuleId ? '编辑规则' : '添加规则'}>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <label className="text-[12px] text-secondary">规则名称</label>
            <input
              type="text"
              value={ruleForm.name}
              onChange={(e) => onRuleFormChange({ ...ruleForm, name: e.target.value })}
              placeholder="例如：屏蔽广告文章"
              className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
            />
          </div>

          {/* 多条件表单 */}
          <div className="flex flex-col gap-1">
            <label className="text-[12px] text-secondary">匹配条件（AND 逻辑）</label>
            <div className="flex flex-col gap-2">
              {ruleForm.conditions.map((cond, index) => (
                <div key={index} className="flex items-center gap-2">
                  <select
                    value={cond.field}
                    onChange={(e) => updateCondition(index, { field: e.target.value as FilterCondition['field'] })}
                    className="rounded-md border border-border bg-surface px-2 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
                  >
                    {FIELD_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                  <select
                    value={cond.operator}
                    onChange={(e) => updateCondition(index, { operator: e.target.value as FilterCondition['operator'] })}
                    className="rounded-md border border-border bg-surface px-2 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
                  >
                    {OPERATOR_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                  <input
                    type="text"
                    value={cond.value}
                    onChange={(e) => updateCondition(index, { value: e.target.value })}
                    placeholder="关键词"
                    className="min-w-0 flex-1 rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
                  />
                  {ruleForm.conditions.length > 1 && (
                    <button
                      onClick={() => removeCondition(index)}
                      className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-secondary transition-colors hover:bg-hover hover:text-danger"
                      title="移除条件"
                    >
                      <X size={14} />
                    </button>
                  )}
                </div>
              ))}
            </div>
            <button
              onClick={addCondition}
              className="inline-flex items-center gap-1.5 self-start rounded-md border border-dashed border-border px-2.5 py-1.5 text-[12.5px] text-secondary transition-colors hover:border-border-strong hover:text-primary"
            >
              <Plus size={14} />
              添加条件
            </button>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="flex flex-col gap-1">
              <label className="text-[12px] text-secondary">执行操作</label>
              <select
                value={ruleForm.action}
                onChange={(e) => onRuleFormChange({ ...ruleForm, action: e.target.value as FilterRule['action'] })}
                className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
              >
                {(['markRead', 'star', 'readLater'] as FilterRule['action'][]).map((a) => (
                  <option key={a} value={a}>{ACTION_LABELS[a]}</option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-[12px] text-secondary">作用范围</label>
              <select
                value={ruleForm.scope}
                onChange={(e) => {
                  const scope = e.target.value as FilterRule['scope'];
                  onRuleFormChange({
                    ...ruleForm,
                    scope,
                    sourceId: scope === 'source' ? ruleForm.sourceId : null,
                    folderId: scope === 'folder' ? ruleForm.folderId : null,
                  });
                }}
                className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
              >
                {(['global', 'source', 'folder'] as FilterRule['scope'][]).map((s) => (
                  <option key={s} value={s}>{SCOPE_LABELS[s]}</option>
                ))}
              </select>
            </div>
            {ruleForm.scope === 'source' && (
              <div className="flex flex-col gap-1">
                <label className="text-[12px] text-secondary">订阅源</label>
                <select
                  value={ruleForm.sourceId ?? ''}
                  onChange={(e) => onRuleFormChange({ ...ruleForm, sourceId: e.target.value ? Number(e.target.value) : null })}
                  className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
                >
                  <option value="">请选择</option>
                  {sources.map((s) => (
                    <option key={s.id} value={s.id}>{s.name}</option>
                  ))}
                </select>
              </div>
            )}
            {ruleForm.scope === 'folder' && (
              <div className="flex flex-col gap-1">
                <label className="text-[12px] text-secondary">文件夹</label>
                <select
                  value={ruleForm.folderId ?? ''}
                  onChange={(e) => onRuleFormChange({ ...ruleForm, folderId: e.target.value ? Number(e.target.value) : null })}
                  className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
                >
                  <option value="">请选择</option>
                  {folders.map((f) => (
                    <option key={f.id} value={f.id}>{f.name}</option>
                  ))}
                </select>
              </div>
            )}
          </div>

          <label className="flex items-center gap-2 text-[13px] text-secondary">
            <input
              type="checkbox"
              checked={ruleForm.enabled}
              onChange={(e) => onRuleFormChange({ ...ruleForm, enabled: e.target.checked })}
              className="h-3.5 w-3.5 accent-primary"
            />
            启用规则
          </label>

          <div className="flex items-center gap-2">
            <SmallBtn icon={<Check size={14} />} label={editingRuleId ? '保存' : '添加'} onClick={onSaveRule} />
            {editingRuleId && <SmallBtn label="取消" onClick={onResetRuleForm} />}
          </div>
        </div>
      </Section>

      {rules.length > 0 && (
        <Section title={`已配置的规则 (${rules.length})`}>
          <div className="flex flex-col gap-2">
            {rules.map((rule) => (
              <div
                key={rule.id}
                className={cn(
                  'flex items-center gap-3 rounded-md border border-border bg-surface px-3.5 py-3',
                  !rule.enabled && 'opacity-55'
                )}
              >
                <Filter size={15} className="shrink-0 text-primary" />
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-medium text-primary">{rule.name}</div>
                  <div className="mt-0.5 text-[12px] text-muted">{formatRuleDescription(rule)}</div>
                </div>
                <button
                  onClick={() => handleTestRule(rule)}
                  disabled={testingId !== null}
                  className="inline-flex h-7 w-7 items-center justify-center rounded-md text-secondary transition-colors hover:bg-hover hover:text-primary disabled:pointer-events-none disabled:opacity-40"
                  title="测试规则"
                >
                  {testingId === rule.id ? (
                    <RefreshCw size={14} className="animate-spin" />
                  ) : (
                    <Search size={14} />
                  )}
                </button>
                <button
                  onClick={() => onToggleRule(rule)}
                  className={cn(
                    'inline-flex h-7 w-7 items-center justify-center rounded-md transition-colors',
                    rule.enabled
                      ? 'bg-primary text-on-primary hover:bg-primary-hover'
                      : 'bg-hover text-muted hover:text-primary'
                  )}
                  title={rule.enabled ? '禁用' : '启用'}
                >
                  <Check size={14} />
                </button>
                <button
                  onClick={() => onStartEditRule(rule)}
                  className="inline-flex h-7 w-7 items-center justify-center rounded-md text-secondary transition-colors hover:bg-hover hover:text-primary"
                  title="编辑"
                >
                  <Pencil size={14} />
                </button>
                <button
                  onClick={() => onDeleteRule(rule.id)}
                  className="inline-flex h-7 w-7 items-center justify-center rounded-md text-secondary transition-colors hover:bg-hover hover:text-danger"
                  title="删除"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
        </Section>
      )}

      {testResults && (
        <ModalLayout
          title={`规则测试：${testResults.ruleName}`}
          titleIcon={<Search size={16} />}
          onClose={() => setTestResults(null)}
          width={560}
        >
          <div className="px-5 pt-4 pb-5">
            <div className="mb-3 text-[12.5px] text-muted">
              在最近 200 篇文章中，命中 {testResults.items.length} 篇
              {testResults.items.length >= 20 && '（仅显示前 20 条）'}
            </div>
            {testResults.items.length === 0 ? (
              <div className="py-8 text-center text-[13px] text-muted">未匹配到文章</div>
            ) : (
              <div className="flex max-h-[50vh] flex-col gap-1.5 overflow-y-auto">
                {testResults.items.map((item) => (
                  <TestResultItem key={item.id} item={item} />
                ))}
              </div>
            )}
          </div>
        </ModalLayout>
      )}
    </div>
  );
}
