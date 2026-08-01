import { useState } from 'react';
import { loadSettings } from '../utils/settings';
import type { Folder } from '../types';
import Button from './Button';
import ModalLayout from './ModalLayout';

interface Props {
  onClose: () => void;
  onSubmit: (data: SourceFormData) => void;
  folders: Folder[];
}

export interface SourceFormData {
  name: string;
  url: string;
  folderId?: number | null;
  interval: number;
}

function getInputClasses(): string {
  return 'w-full px-3 py-2.5 border border-border rounded-sm text-base box-border bg-surface text-primary';
}

function getSourceFormFolders(folders: Folder[]): { label: string; value: string }[] {
  return folders.map((f) => ({ label: f.name, value: String(f.id) }));
}

function SourceForm({
  form,
  folders,
  onChange,
}: {
  form: SourceFormData;
  folders: Folder[];
  onChange: (field: keyof SourceFormData, value: SourceFormData[keyof SourceFormData]) => void;
}) {
  const folderOptions = getSourceFormFolders(folders);

  return (
    <div className="flex flex-col gap-3">
      <div>
        <label className="block text-sm font-medium text-secondary mb-1.5">名称</label>
        <input
          type="text"
          value={form.name}
          onChange={(e) => onChange('name', e.target.value)}
          className={getInputClasses()}
        />
      </div>

      <div>
        <label className="block text-sm font-medium text-secondary mb-1.5">RSS / Atom URL</label>
        <input
          type="url"
          required
          value={form.url}
          onChange={(e) => onChange('url', e.target.value)}
          className={getInputClasses()}
        />
      </div>

      <div>
        <label className="block text-sm font-medium text-secondary mb-1.5">所属文件夹</label>
        <select
          value={form.folderId ?? ''}
          onChange={(e) => onChange('folderId', e.target.value ? Number(e.target.value) : null)}
          className={getInputClasses()}
        >
          <option value="">无（不分组）</option>
          {folderOptions.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium text-secondary mb-1.5">抓取间隔（分钟）</label>
        <input
          type="number"
          min={5}
          value={form.interval}
          onChange={(e) => onChange('interval', Number(e.target.value))}
          className={getInputClasses()}
        />
      </div>
    </div>
  );
}

export default function AddSourceModal({ onClose, onSubmit, folders }: Props) {
  const [form, setForm] = useState<SourceFormData>(() => {
    const settings = loadSettings();
    return {
      name: '',
      url: '',
      folderId: null,
      interval: settings.defaultInterval,
    };
  });

  const handleChange = (field: keyof SourceFormData, value: SourceFormData[keyof SourceFormData]) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = form.name.trim();
    const trimmedUrl = form.url.trim();
    if (!trimmedUrl) return;
    if (form.interval < 5) return;
    onSubmit({ ...form, name: trimmedName, url: trimmedUrl });
  };

  return (
    <ModalLayout title="添加订阅源" onClose={onClose} width={520}>
      <form onSubmit={handleSubmit} className="p-6">
        <SourceForm form={form} folders={folders} onChange={handleChange} />
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={onClose}>取消</Button>
          <Button variant="primary" type="submit">添加</Button>
        </div>
      </form>
    </ModalLayout>
  );
}
