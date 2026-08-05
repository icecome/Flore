import { useRef, useState } from 'react';
import { UploadIcon } from './icons';
import Button from './Button';
import ContextMenu from './ContextMenu';
import ModalLayout from './ModalLayout';
import { useContextMenu } from '../hooks/useContextMenu';
import { buildInputMenu } from '../utils/contextMenu';

interface Props {
  onClose: () => void;
  onImport: (xml: string) => Promise<void>;
}

function readSelectedFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = (ev) => resolve(String(ev.target?.result || ''));
    reader.onerror = () => reject(new Error('读取文件失败'));
    reader.readAsText(file);
  });
}

async function executeImport({
  opmlContent,
  setLoading,
  setError,
  onImport,
  onClose,
}: {
  opmlContent: string;
  setLoading: (v: boolean) => void;
  setError: (v: string) => void;
  onImport: (xml: string) => Promise<void>;
  onClose: () => void;
}) {
  if (!opmlContent.trim()) {
    setError('请选择或粘贴 OPML 内容');
    return;
  }
  setLoading(true);
  setError('');
  try {
    await onImport(opmlContent);
    onClose();
  } catch (err: unknown) {
    setError((err as Error).message || '导入失败');
  } finally {
    setLoading(false);
  }
}

export default function ImportOPMLModal({ onClose, onImport }: Props) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [opmlContent, setOpmlContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const { menuProps, showMenu } = useContextMenu();

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const content = await readSelectedFileAsText(file);
      setOpmlContent(content);
    } catch {
      setError('读取文件失败');
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await executeImport({ opmlContent, setLoading, setError, onImport, onClose });
  };

  return (
    <>
    <ModalLayout title="导入 OPML" onClose={onClose} width={520}>
      <form onSubmit={handleSubmit} className="p-5 px-6 pb-6">
        <input
          ref={fileInputRef}
          type="file"
          accept=".opml,.xml,text/xml"
          onChange={handleFileChange}
          className="hidden"
        />

        <div className="flex items-center gap-3 mb-4">
          <Button variant="ghost" onClick={() => fileInputRef.current?.click()}>
            <UploadIcon />
            <span>选择文件</span>
          </Button>
          <span className="text-xs text-muted">支持 .opml / .xml</span>
        </div>

        <div className="mb-4">
          <label className="block text-sm font-medium text-secondary mb-1.5">或粘贴 OPML 内容</label>
          <textarea
            ref={textareaRef}
            value={opmlContent}
            onChange={(e) => setOpmlContent(e.target.value)}
            onContextMenu={(e) => {
              const el = textareaRef.current;
              if (!el) return;
              showMenu(e, buildInputMenu(
                { hasSelection: el.selectionStart !== null && el.selectionEnd !== null && el.selectionStart !== el.selectionEnd, hasValue: opmlContent.length > 0, readOnly: false },
                { onClear: () => setOpmlContent('') },
              ));
            }}
            className="w-full px-3 py-[10px] border border-border rounded-sm text-sm font-mono box-border resize-vertical bg-surface text-primary"
            placeholder="粘贴 OPML XML 内容..."
            rows={10}
          />
        </div>

        {error && <div className="text-danger text-sm mb-3">{error}</div>}

        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={onClose}>取消</Button>
          <Button variant="primary" type="submit" disabled={loading}>{loading ? '导入中...' : '导入'}</Button>
        </div>
      </form>
    </ModalLayout>
      {menuProps && <ContextMenu {...menuProps} />}
    </>
  );
}
