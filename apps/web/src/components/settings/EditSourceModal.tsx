import { useState } from 'react';
import { Pencil } from '../icons';
import ModalLayout from '../ModalLayout';
import { showToast } from '../../utils/toast';
import type { Source } from '../../types';

interface Props {
  source: Source;
  onSave: (id: number, data: { name: string; url: string }) => Promise<void>;
  onClose: () => void;
}

export default function EditSourceModal({ source, onSave, onClose }: Props) {
  const [name, setName] = useState(source.name);
  const [url, setUrl] = useState(source.url);
  const [saving, setSaving] = useState(false);

  const canSave = name.trim().length > 0 && url.trim().length > 0 && !saving;

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    try {
      await onSave(source.id, { name: name.trim(), url: url.trim() });
      onClose();
    } catch (err) {
      console.error('Failed to save source:', err);
      showToast('保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <ModalLayout title="编辑订阅源" titleIcon={<Pencil size={16} />} onClose={onClose}>
      <div className="flex flex-col gap-4 px-4 py-4">
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-medium text-primary">名称</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSave();
            }}
            placeholder="订阅源名称"
            className="rounded-md border border-border bg-surface px-3 py-2 text-[13px] text-primary outline-none focus:border-primary"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <label className="text-[13px] font-medium text-primary">RSS 链接</label>
          <input
            type="text"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSave();
            }}
            placeholder="RSS 链接"
            className="rounded-md border border-border bg-surface px-3 py-2 text-[12px] text-muted outline-none focus:border-primary"
          />
        </div>
      </div>
      <div className="flex justify-end gap-2 border-t border-border px-4 py-3">
        <button
          type="button"
          onClick={onClose}
          className="rounded-md border border-border px-3 py-1.5 text-[13px] text-secondary transition-colors hover:bg-hover"
        >
          取消
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={!canSave}
          className="rounded-md bg-primary px-3 py-1.5 text-[13px] font-medium text-white transition-colors hover:bg-primary-hover disabled:pointer-events-none disabled:opacity-40"
        >
          {saving ? '保存中...' : '保存'}
        </button>
      </div>
    </ModalLayout>
  );
}
