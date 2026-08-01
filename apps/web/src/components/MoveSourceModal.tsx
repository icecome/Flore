import { useState } from 'react';
import type { Folder, Source } from '../types';
import { FolderIcon } from './icons';
import Button from './Button';
import ModalLayout from './ModalLayout';

interface Props {
  source: Source;
  folders: Folder[];
  onClose: () => void;
  onMove: (sourceId: number, folderId: number | null) => void;
}

export default function MoveSourceModal({ source, folders, onClose, onMove }: Props) {
  const [targetFolderId, setTargetFolderId] = useState<string>(
    source.folderId === null ? 'none' : String(source.folderId)
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const folderId = targetFolderId === 'none' ? null : Number(targetFolderId);
    onMove(source.id, folderId);
    onClose();
  };

  return (
    <ModalLayout title="移动订阅源" onClose={onClose} width={400}>
      <form onSubmit={handleSubmit} className="pt-5 px-6 pb-6">
          <div className="flex flex-col gap-1 text-base font-medium text-primary mb-4 px-3 py-2.5 bg-surface rounded-sm border border-border">
            <span className="text-[11px] font-medium text-muted uppercase tracking-[0.03em]">当前订阅</span>
            <span>{source.name}</span>
          </div>

          <div className="mb-5">
            <label className="block text-sm font-medium text-secondary mb-1.5">目标文件夹</label>
            <select
              value={targetFolderId}
              onChange={(e) => setTargetFolderId(e.target.value)}
              className="w-full px-3 py-2.5 border border-border rounded-sm text-base bg-surface text-primary box-border"
            >
              <option value="none">未分类</option>
              {folders.map((folder) => (
                <option key={folder.id} value={String(folder.id)}>
                  {folder.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex justify-end gap-3">
            <Button variant="secondary" onClick={onClose}>取消</Button>
            <Button variant="primary" type="submit">移动</Button>
          </div>
        </form>
      </ModalLayout>
    );
  }
