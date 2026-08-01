import { useState, useMemo } from 'react';
import type { Item, Source, Folder } from '../types';
import { cn } from '../lib/cn';
import { FileArchive, FileJson } from './icons';
import Button from './Button';
import ModalLayout from './ModalLayout';

export interface ExportScope {
  ids?: number[];
  sourceId?: number;
  folderId?: number;
  starred?: boolean;
  unread?: boolean;
  readLater?: boolean;
  hidePrivate?: boolean;
}

interface Props {
  isOpen: boolean;
  onClose: () => void;
  items: Item[];
  selectedIds: number[];
  currentSource: Source | null;
  currentFolder: Folder | null;
  filter: 'all' | 'unread' | 'starred' | 'readLater';
  settings: { hidePrivateInTimeline: boolean };
  onExport: (scope: ExportScope, format: 'markdown' | 'json') => void;
}

type ScopeKey = 'selected' | 'currentView' | 'currentSource' | 'currentFolder' | 'starred' | 'readLater';

function getCurrentViewLabel(currentSource: Source | null, currentFolder: Folder | null, filter: string): string {
  if (currentSource) return `当前列表：${currentSource.name}`;
  if (currentFolder) return `当前文件夹：${currentFolder.name}`;
  if (filter === 'starred') return '全部收藏';
  if (filter === 'readLater') return '稍后阅读';
  if (filter === 'unread') return '全部未读';
  return '当前时间线';
}

interface ScopeBuildConfig {
  scopeKey: ScopeKey;
  selectedIds: number[];
  currentSource: Source | null;
  currentFolder: Folder | null;
  filter: string;
  hidePrivate: boolean;
}

function buildScopeByKey(config: ScopeBuildConfig): ExportScope {
  const { scopeKey, selectedIds, currentSource, currentFolder, filter, hidePrivate } = config;
  switch (scopeKey) {
    case 'selected':
      return { ids: selectedIds };
    case 'currentSource':
      return currentSource ? { sourceId: currentSource.id, hidePrivate } : { hidePrivate };
    case 'currentFolder':
      return currentFolder ? { folderId: currentFolder.id, hidePrivate } : { hidePrivate };
    case 'starred':
      return { starred: true, hidePrivate };
    case 'readLater':
      return { readLater: true, hidePrivate };
    case 'currentView':
    default:
      return buildCurrentViewScope(currentSource, currentFolder, filter, hidePrivate);
  }
}

function buildCurrentViewScope(currentSource: Source | null, currentFolder: Folder | null, filter: string, hidePrivate: boolean): ExportScope {
  if (currentSource) return { sourceId: currentSource.id, hidePrivate };
  if (currentFolder) return { folderId: currentFolder.id, hidePrivate };
  if (filter === 'starred') return { starred: true, hidePrivate };
  if (filter === 'readLater') return { readLater: true, hidePrivate };
  if (filter === 'unread') return { unread: true, hidePrivate };
  return { hidePrivate };
}

export default function ExportArticlesModal({
  isOpen,
  onClose,
  items,
  selectedIds,
  currentSource,
  currentFolder,
  filter,
  settings,
  onExport,
}: Props) {
  const [scopeKey, setScopeKey] = useState<ScopeKey>(selectedIds.length > 0 ? 'selected' : 'currentView');
  const [format, setFormat] = useState<'markdown' | 'json'>('markdown');

  const scopeOptions = useMemo(() => {
    const options: { key: ScopeKey; label: string; count: number; disabled?: boolean }[] = [
      {
        key: 'selected',
        label: '已选中的文章',
        count: selectedIds.length,
        disabled: selectedIds.length === 0,
      },
      {
        key: 'currentView',
        label: getCurrentViewLabel(currentSource, currentFolder, filter),
        count: items.length,
      },
    ];

    if (currentSource) {
      options.push({ key: 'currentSource', label: `订阅源：${currentSource.name}`, count: items.length });
    }
    if (currentFolder) {
      options.push({ key: 'currentFolder', label: `文件夹：${currentFolder.name}`, count: items.length });
    }
    if (filter !== 'starred') {
      options.push({ key: 'starred', label: '全部收藏', count: -1 });
    }
    if (filter !== 'readLater') {
      options.push({ key: 'readLater', label: '稍后阅读', count: -1 });
    }

    return options;
  }, [items.length, selectedIds.length, currentSource, currentFolder, filter]);

  const selectedCount = useMemo(() => {
    if (scopeKey === 'selected') return selectedIds.length;
    if (scopeKey === 'currentView' || scopeKey === 'currentSource' || scopeKey === 'currentFolder') return items.length;
    return -1;
  }, [scopeKey, selectedIds.length, items.length]);

  const handleExport = () => {
    onExport(buildScopeByKey({
      scopeKey,
      selectedIds,
      currentSource,
      currentFolder,
      filter,
      hidePrivate: settings.hidePrivateInTimeline,
    }), format);
    onClose();
  };

  if (!isOpen) return null;

  return (
    <ModalLayout title="导出文章" onClose={onClose} width={480}>
      <div className="flex-1 overflow-y-auto p-5 flex flex-col gap-6">
          <section className="flex flex-col gap-3">
            <h4 className="m-0 text-xs font-semibold text-muted uppercase tracking-[0.5px]">导出范围</h4>
            <div className="flex flex-col gap-2">
              {scopeOptions.map((opt) => (
                <label
                  key={opt.key}
                  className={cn(
                    'flex items-center gap-2.5 px-3 py-[10px] rounded-sm border',
                    opt.disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
                    scopeKey === opt.key ? 'border-primary bg-primary-subtle' : 'border-border bg-surface'
                  )}
                >
                  <input
                    type="radio"
                    name="export-scope"
                    value={opt.key}
                    checked={scopeKey === opt.key}
                    disabled={opt.disabled}
                    onChange={() => setScopeKey(opt.key)}
                    className="m-0 accent-primary"
                  />
                  <span className="flex-1 text-sm text-primary">{opt.label}</span>
                  {opt.count >= 0 && <span className="text-xs text-muted">{opt.count} 篇</span>}
                </label>
              ))}
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h4 className="m-0 text-xs font-semibold text-muted uppercase tracking-[0.5px]">导出格式</h4>
            <div className="flex gap-3">
              <button
                onClick={() => setFormat('markdown')}
                className={cn(
                  'flex-1 flex flex-col items-center gap-1.5 px-3 py-4 rounded-md border cursor-pointer text-secondary',
                  format === 'markdown' ? 'border-primary bg-primary-subtle' : 'border-border bg-surface'
                )}
              >
                <FileArchive size={20} />
                <span className="text-sm font-medium text-primary">Markdown 压缩包</span>
                <span className="text-[11px] text-muted">每篇文章一个 .md 文件</span>
              </button>
              <button
                onClick={() => setFormat('json')}
                className={cn(
                  'flex-1 flex flex-col items-center gap-1.5 px-3 py-4 rounded-md border cursor-pointer text-secondary',
                  format === 'json' ? 'border-primary bg-primary-subtle' : 'border-border bg-surface'
                )}
              >
                <FileJson size={20} />
                <span className="text-sm font-medium text-primary">JSON 数据包</span>
                <span className="text-[11px] text-muted">结构化 JSON 文件</span>
              </button>
            </div>
          </section>

          <div className="text-xs text-muted leading-relaxed px-3 py-[10px] bg-hover rounded-sm">
            提示：导出内容为文章摘要 HTML，不下载原文中的图片。
          </div>
        </div>

        <div className="flex justify-end gap-2.5 px-[18px] py-[14px] border-t border-border">
          <Button variant="secondary" onClick={onClose}>取消</Button>
          <Button
            variant="primary"
            onClick={handleExport}
            disabled={selectedCount === 0}
            className={cn(selectedCount === 0 ? 'opacity-50' : '')}
          >
            导出
          </Button>
        </div>
      </ModalLayout>
    );
  }
