import { useRef, useEffect } from 'react';
import { RefreshCw, Plus, Upload, Download, EyeOff, Pencil, Trash2, Circle } from '../icons';
import { cn } from '../../lib/cn';
import type { Source } from '../../types';
import { IconBtn } from './SettingsShared';
import { showToast } from '../../utils/toast';

interface Props {
  sources: Source[];
  filteredSources: Source[];
  selectedSourceIds: number[];
  isChecking: boolean;
  checkingIds: Set<number>;
  editingSourceId: number | null;
  editingName: string;
  editingUrl: string;
  sourceFilter: 'all' | 'ok' | 'bad';
  onSourcesChanged?: () => void;
  onAddSource?: () => void;
  onToggleSelection: (id: number) => void;
  onToggleSelectAll: () => void;
  onDeleteSources: () => void;
  onHideSelected: () => Promise<void>;
  onStartRename: (source: Source) => void;
  onConfirmRename: () => Promise<void>;
  onCancelRename: () => void;
  onCheckAvailability: () => Promise<void>;
  onImportOPML: (fileOrXml: File | string) => Promise<void>;
  onExportOPML: () => Promise<void>;
  onSourceFilterChange: (filter: 'all' | 'ok' | 'bad') => void;
  onEditingNameChange: (name: string) => void;
  onEditingUrlChange: (url: string) => void;
}

const SOURCE_FILTER_OPTIONS: { value: 'all' | 'ok' | 'bad'; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'ok', label: '可用' },
  { value: 'bad', label: '超时' },
];

export default function SettingsSourcesTab(props: Props) {
  const {
    sources,
    filteredSources,
    selectedSourceIds,
    isChecking,
    checkingIds,
    editingSourceId,
    editingName,
    editingUrl,
    sourceFilter,
    onSourcesChanged,
    onAddSource,
    onToggleSelection,
    onToggleSelectAll,
    onDeleteSources,
    onHideSelected,
    onStartRename,
    onConfirmRename,
    onCancelRename,
    onCheckAvailability,
    onImportOPML,
    onExportOPML,
    onSourceFilterChange,
    onEditingNameChange,
    onEditingUrlChange,
  } = props;

  const editNameRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editingSourceId != null) editNameRef.current?.focus({ preventScroll: true });
  }, [editingSourceId]);

  const allSelected = filteredSources.length > 0 && selectedSourceIds.length === filteredSources.length;

  return (
    <div className="relative flex h-full flex-col">
      <div className="mb-5 flex items-center justify-end">
        <div className="flex items-center gap-2">
          <button
            onClick={onCheckAvailability}
            disabled={isChecking}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12.5px] transition-colors',
              'border border-border bg-surface text-secondary hover:border-border-strong hover:bg-hover disabled:opacity-40'
            )}
          >
            <span className={isChecking ? 'inline-flex spin-fixed' : 'inline-flex'}>
              <RefreshCw size={14} />
            </span>
            {isChecking ? '检测中...' : '检测可用性'}
          </button>
          <select
            value={sourceFilter}
            onChange={(e) => {
              onSourceFilterChange(e.target.value as 'all' | 'ok' | 'bad');
            }}
            className="rounded-md border border-border bg-surface px-2 py-1.5 text-[12.5px] text-primary outline-none focus:border-primary"
          >
            {SOURCE_FILTER_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <div className="mx-1 h-4 w-px bg-border" />
          <IconBtn icon={<Plus size={16} />} title="添加订阅源" onClick={() => onAddSource?.()} />
          <input
            ref={fileInputRef}
            type="file"
            accept=".opml,.xml"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) onImportOPML(file);
              if (e.target) e.target.value = '';
            }}
          />
          <IconBtn
            icon={<Upload size={16} />}
            title="导入 OPML"
            onClick={() => {
              const wailsApp = (window as unknown as { go?: { main?: { App?: { PickOPMLFile?: () => Promise<string> } } } }).go?.main?.App;
              if (wailsApp?.PickOPMLFile) {
                wailsApp.PickOPMLFile().then((xml: string) => {
                  if (xml) onImportOPML(xml);
                }).catch((err: unknown) => {
                  console.error('PickOPMLFile failed:', err);
                  showToast('选择 OPML 文件失败');
                });
              } else {
                fileInputRef.current?.click();
              }
            }}
          />
          <IconBtn icon={<Download size={16} />} title="导出 OPML" onClick={onExportOPML} />
        </div>
      </div>

      {filteredSources.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center text-muted">
          <p className="text-[13px]">暂无订阅源</p>
        </div>
      ) : (
        <div className="flex-1 overflow-hidden rounded-md border border-border">
          <div className="h-full overflow-y-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="bg-elevated text-left text-[12px] font-semibold text-muted">
                  <th className="w-10 px-3 py-2.5 text-center align-middle">
                    <input
                      type="checkbox"
                      checked={allSelected}
                      onChange={onToggleSelectAll}
                      className="h-3.5 w-3.5 cursor-pointer accent-primary align-middle"
                    />
                  </th>
                  <th className="px-3 py-2.5">名称</th>
                  <th className="px-3 py-2.5">订阅日期</th>
                  <th className="px-3 py-2.5 text-center">状态</th>
                </tr>
              </thead>
              <tbody>
                {filteredSources.map((source) => {
                  const neverFetched = source.lastFetchAt === null;
                  const bad = !neverFetched && source.fetchFailCount >= 3;
                  const selected = selectedSourceIds.includes(source.id);
                  const editing = editingSourceId === source.id;
                  return (
                    <tr
                      key={source.id}
                      className={cn(
                        'border-t border-border-subtle text-[13px] transition-colors hover:bg-hover',
                        selected && 'bg-primary-subtle'
                      )}
                    >
                      <td className="px-3 py-2.5 text-center align-middle">
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={() => onToggleSelection(source.id)}
                          className="h-3.5 w-3.5 cursor-pointer accent-primary align-middle"
                        />
                      </td>
                      <td className="px-3 py-2">
                        {editing ? (
                          <div className="flex flex-col gap-1.5">
                            <input
                              ref={editNameRef}
                              type="text"
                              value={editingName}
                              onChange={(e) => onEditingNameChange(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') onConfirmRename();
                                if (e.key === 'Escape') onCancelRename();
                              }}
                              placeholder="订阅源名称"
                              className="w-full rounded-md border border-border bg-surface px-2 py-1 text-[13px] text-primary outline-none focus:border-primary"
                            />
                            <input
                              type="text"
                              value={editingUrl}
                              onChange={(e) => onEditingUrlChange(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') onConfirmRename();
                                if (e.key === 'Escape') onCancelRename();
                              }}
                              placeholder="RSS 链接"
                              className="w-full rounded-md border border-border bg-surface px-2 py-1 text-[12px] text-muted outline-none focus:border-primary"
                            />
                            <div className="flex items-center gap-2">
                              <button
                                type="button"
                                onClick={onConfirmRename}
                                className="rounded-md bg-primary px-2.5 py-0.5 text-[11px] text-white hover:bg-primary-hover"
                              >
                                保存
                              </button>
                              <button
                                type="button"
                                onClick={onCancelRename}
                                className="rounded-md px-2.5 py-0.5 text-[11px] text-muted hover:bg-hover"
                              >
                                取消
                              </button>
                            </div>
                          </div>
                        ) : (
                          <>
                            <div
                              className={cn('truncate font-medium', bad ? 'text-danger' : 'text-primary')}
                              title={source.name}
                            >
                              {source.name}
                            </div>
                            <div className="truncate text-[11px] text-muted" title={source.url}>
                              {source.url}
                            </div>
                          </>
                        )}
                      </td>
                      <td className="px-3 py-2 text-[12px] text-secondary">
                        {source.createdAt ? new Date(source.createdAt).toLocaleDateString() : '-'}
                      </td>
                      <td className="px-3 py-2 text-center">
                        {checkingIds.has(source.id) ? (
                          <span className="inline-flex items-center gap-1 rounded-full bg-border/30 px-2 py-0.5 text-[11px] text-muted whitespace-nowrap">
                            <RefreshCw size={10} className="spin-fixed" />
                            检测中
                          </span>
                        ) : neverFetched ? (
                          <span className="inline-flex items-center gap-1 rounded-full bg-border/30 px-2 py-0.5 text-[11px] text-muted whitespace-nowrap">
                            <Circle size={7} fill="currentColor" />
                            未检测
                          </span>
                        ) : bad ? (
                          <span
                            className="inline-flex items-center gap-1 rounded-full bg-danger/10 px-2 py-0.5 text-[11px] font-medium text-danger whitespace-nowrap"
                            title={
                              source.lastError
                                ? `失败 ${source.fetchFailCount} 次: ${source.lastError}`
                                : `失败 ${source.fetchFailCount} 次`
                            }
                          >
                            <Circle size={7} fill="currentColor" />
                            超时
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1 rounded-full bg-success/10 px-2 py-0.5 text-[11px] font-medium text-success whitespace-nowrap">
                            <Circle size={7} fill="currentColor" />
                            正常
                          </span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
        <div className="flex items-center gap-1.5">
          <IconBtn
            icon={<EyeOff size={16} />}
            title="在全部文章隐藏"
            disabled={selectedSourceIds.length === 0}
            onClick={onHideSelected}
          />
          <IconBtn
            icon={<Pencil size={16} />}
            title="修改名称/链接"
            disabled={selectedSourceIds.length !== 1}
            onClick={() => {
              const s = sources.find((x) => x.id === selectedSourceIds[0]);
              if (s) onStartRename(s);
            }}
          />
          <IconBtn
            icon={<Trash2 size={16} />}
            title="取消订阅"
            danger
            disabled={selectedSourceIds.length === 0}
            onClick={onDeleteSources}
          />
          {selectedSourceIds.length > 0 && (
            <span className="ml-2 text-[12px] text-muted">{selectedSourceIds.length} 项已选中</span>
          )}
        </div>
      </div>
    </div>
  );
}