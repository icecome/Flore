import { useState } from 'react';
import type { Folder, Source } from '../types';
import { cn } from '../lib/cn';
import Button from './Button';
import ModalLayout from './ModalLayout';

interface EditSourceParams {
  sourceId: number;
  name: string;
  url: string;
  folderId: number | null;
  isPrivate: boolean;
  hideInTimeline: boolean;
}

interface Props {
  source: Source;
  folders: Folder[];
  onClose: () => void;
  onSubmit: (params: EditSourceParams) => void;
}

export default function EditSourceModal({ source, folders, onClose, onSubmit }: Props) {
  const [name, setName] = useState(source.name);
  const [url, setUrl] = useState(source.url);
  const [selectedFolder, setSelectedFolder] = useState<string>(
    source.folderId === null ? 'none' : String(source.folderId)
  );
  const [isPrivate, setIsPrivate] = useState(source.isPrivate ?? false);
  const [hideInTimeline, setHideInTimeline] = useState(source.hideInTimeline ?? false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    const folderId = selectedFolder === 'none' ? null : Number(selectedFolder);
    onSubmit({ sourceId: source.id, name: trimmed, url: url.trim(), folderId, isPrivate, hideInTimeline });
  };

  return (
    <ModalLayout title="编辑订阅" onClose={onClose} width={480}>
      <form onSubmit={handleSubmit} className="p-5 px-6 pb-6">
        <div className="mb-5">
          <label className="block text-base font-semibold text-primary mb-1">标题</label>
            <div className="text-xs text-muted mb-2">此订阅源的自定义标题，留空则使用默认标题。</div>
            <div className="flex gap-2">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full px-3 py-[10px] border border-border rounded-sm text-base box-border bg-surface text-primary"
                placeholder="订阅源名称"
              />
              <Button variant="ghost" size="sm" onClick={() => setName(source.name)}>填充</Button>
            </div>
          </div>

          <div className="mb-5">
            <label className="block text-base font-semibold text-primary mb-1">订阅地址</label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              className="w-full px-3 py-[10px] border border-border rounded-sm text-base box-border bg-surface text-primary"
              placeholder="https://example.com/feed.xml"
            />
          </div>

          <div className="mb-5">
            <label className="block text-base font-semibold text-primary mb-1">文件夹</label>
            <div className="text-xs text-muted mb-2">默认情况下，你的订阅将按网站域名分组。</div>
            <select
              value={selectedFolder}
              onChange={(e) => setSelectedFolder(e.target.value)}
              className="w-full px-3 py-[10px] border border-border rounded-sm text-base bg-surface text-primary box-border"
            >
              <option value="none">未分类</option>
              {folders.map((folder) => (
                <option key={folder.id} value={String(folder.id)}>
                  {folder.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex items-center justify-between mb-5 gap-4">
            <div className="flex-1 min-w-0">
              <div className="text-base font-semibold text-primary mb-0.5">私密订阅</div>
              <div className="text-xs text-muted">开启后，此订阅不再显示在个人资料页面。</div>
            </div>
            <label className="relative inline-block w-10 h-[22px] cursor-pointer shrink-0">
              <input
                type="checkbox"
                checked={isPrivate}
                onChange={(e) => setIsPrivate(e.target.checked)}
                className="opacity-0 w-0 h-0 absolute"
              />
              <span className={cn('absolute inset-0 rounded-full transition-colors duration-200', isPrivate ? 'bg-primary' : 'bg-border')}>
                <span className={cn('absolute top-0.5 left-0.5 w-[18px] h-[18px] rounded-full bg-white shadow-sm transition-transform duration-200', isPrivate ? 'translate-x-[18px]' : 'translate-x-0')} />
              </span>
            </label>
          </div>

          <div className="flex items-center justify-between mb-5 gap-4">
            <div className="flex-1 min-w-0">
              <div className="text-base font-semibold text-primary mb-0.5">在时间线上隐藏</div>
              <div className="text-xs text-muted">开启后，此订阅将不再显示在主时间线中。</div>
            </div>
            <label className="relative inline-block w-10 h-[22px] cursor-pointer shrink-0">
              <input
                type="checkbox"
                checked={hideInTimeline}
                onChange={(e) => setHideInTimeline(e.target.checked)}
                className="opacity-0 w-0 h-0 absolute"
              />
              <span className={cn('absolute inset-0 rounded-full transition-colors duration-200', hideInTimeline ? 'bg-primary' : 'bg-border')}>
                <span className={cn('absolute top-0.5 left-0.5 w-[18px] h-[18px] rounded-full bg-white shadow-sm transition-transform duration-200', hideInTimeline ? 'translate-x-[18px]' : 'translate-x-0')} />
              </span>
            </label>
          </div>

          <div className="flex justify-end gap-3 mt-2">
            <Button variant="secondary" onClick={onClose}>取消</Button>
            <Button variant="primary" type="submit">更新</Button>
          </div>
        </form>
      </ModalLayout>
    );
  }
