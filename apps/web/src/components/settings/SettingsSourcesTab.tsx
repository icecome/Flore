import { useRef } from 'react';
import { RefreshCw, Plus, Upload, Download, EyeOff, Pencil, Trash2, Circle } from '../icons';
import { cn } from '../../lib/cn';
import type { Source, Folder } from '../../types';
import { SmallBtn, IconBtn } from './SettingsShared';
import { getDesktopApp } from '../../utils/api';
import { showToast } from '../../utils/toast';

interface Props {
  folders: Folder[];
  filteredSources: Source[];
  selectedSourceIds: number[];
  isChecking: boolean;
  checkingIds: Set<number>;
  sourceFilter: 'all' | 'ok' | 'bad';
  onSourcesChanged?: () => void;
  onAddSource?: () => void;
  onToggleSelection: (id: number) => void;
  onToggleSelectAll: () => void;
  onDeleteSources: () => void;
  onDeleteSource: (source: Source) => void;
  onHideSelected: () => Promise<void>;
  onHideSource: (source: Source) => void;
  onEditSource: (source: Source) => void;
  onCheckAvailability: () => Promise<void>;
  onImportOPML: (fileOrXml: File | string) => Promise<void>;
  onExportOPML: () => Promise<void>;
  onSourceFilterChange: (filter: 'all' | 'ok' | 'bad') => void;
}

const SOURCE_FILTER_OPTIONS: { value: 'all' | 'ok' | 'bad'; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'ok', label: '可用' },
  { value: 'bad', label: '超时' },
];

export default function SettingsSourcesTab(props: Props) {
  const {
    folders,
    filteredSources,
    selectedSourceIds,
    isChecking,
    checkingIds,
    sourceFilter,
    onAddSource,
    onToggleSelection,
    onToggleSelectAll,
    onDeleteSources,
    onDeleteSource,
    onHideSelected,
    onHideSource,
    onEditSource,
    onCheckAvailability,
    onImportOPML,
    onExportOPML,
    onSourceFilterChange,
  } = props;

  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderMap = new Map(folders.map((f) => [f.id, f.name]));
  const allSelected = filteredSources.length > 0 && selectedSourceIds.length === filteredSources.length;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
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
              const wailsApp = getDesktopApp();
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
          <IconBtn icon={<Plus size={16} />} title="添加订阅源" onClick={() => onAddSource?.()} />
        </div>
        <SmallBtn
          icon={
            <RefreshCw size={14} className={isChecking ? 'spin-fixed' : undefined} />
          }
          label={isChecking ? '检测中...' : '检测可用性'}
          disabled={isChecking}
          onClick={onCheckAvailability}
        />
      </div>

      {filteredSources.length === 0 ? (
        <div className="flex h-[200px] flex-col items-center justify-center rounded-lg border border-border text-muted">
          <p className="text-[13px]">暂无订阅源，点击添加订阅源开始使用</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border">
          <div className="grid grid-cols-12 bg-elevated px-3 py-2.5 text-[11.5px] font-semibold text-muted">
            <div className="col-span-1 flex items-center justify-center">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={onToggleSelectAll}
                className="h-3.5 w-3.5 cursor-pointer accent-primary"
                aria-label="全选"
              />
            </div>
            <div className="col-span-4">名称</div>
            <div className="col-span-2">文件夹</div>
            <div className="col-span-2">订阅日期</div>
            <div className="col-span-1">状态</div>
            <div className="col-span-2 text-right">操作</div>
          </div>
          <div className="flex-1 min-h-0 overflow-y-auto">
            {filteredSources.map((source) => {
              const neverFetched = source.lastFetchAt === null;
              const bad = !neverFetched && source.fetchFailCount >= 3;
              const selected = selectedSourceIds.includes(source.id);
              const folderName = source.folderId != null ? folderMap.get(source.folderId) ?? '—' : '—';
              return (
                <div
                  key={source.id}
                  className={cn(
                    'grid grid-cols-12 items-center border-t border-border-subtle px-3 py-2.5 text-[13px] transition-colors hover:bg-hover',
                    selected && 'bg-primary-subtle'
                  )}
                >
                  <div className="col-span-1 flex items-center justify-center">
                    <input
                      type="checkbox"
                      checked={selected}
                      onChange={() => onToggleSelection(source.id)}
                      className="h-3.5 w-3.5 cursor-pointer accent-primary"
                      aria-label={`选择 ${source.name}`}
                    />
                  </div>
                  <div className="col-span-4 min-w-0">
                    <div
                      className={cn('truncate font-medium', bad ? 'text-danger' : 'text-primary')}
                      title={source.name}
                    >
                      {source.name}
                    </div>
                    <div className="truncate text-[11px] text-muted" title={source.url}>
                      {source.url}
                    </div>
                  </div>
                  <div className="col-span-2 truncate text-[12px] text-secondary" title={folderName}>
                    {folderName}
                  </div>
                  <div className="col-span-2 text-[12px] text-secondary">
                    {source.createdAt ? new Date(source.createdAt).toLocaleDateString() : '-'}
                  </div>
                  <div className="col-span-1">
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
                  </div>
                  <div className="col-span-2 flex items-center justify-end gap-1">
                    <IconBtn icon={<Pencil size={15} />} title="编辑" onClick={() => onEditSource(source)} />
                    <IconBtn
                      icon={<EyeOff size={15} />}
                      title={source.hideInTimeline ? '在全部文章显示' : '在全部文章隐藏'}
                      onClick={() => onHideSource(source)}
                    />
                    <IconBtn icon={<Trash2 size={15} />} title="取消订阅" danger onClick={() => onDeleteSource(source)} />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {selectedSourceIds.length > 0 && (
        <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
          <div className="flex items-center gap-1.5">
            <IconBtn icon={<EyeOff size={16} />} title="在全部文章隐藏" onClick={onHideSelected} />
            <IconBtn
              icon={<Trash2 size={16} />}
              title="取消订阅"
              danger
              onClick={onDeleteSources}
            />
            <span className="ml-2 text-[12px] text-muted">{selectedSourceIds.length} 项已选中</span>
          </div>
        </div>
      )}
    </div>
  );
}
