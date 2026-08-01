import { useEffect, useRef, useState } from 'react';
import Button from './Button';
import ModalLayout from './ModalLayout';

interface Props {
  title: string;
  initialValue: string;
  onClose: () => void;
  onSubmit: (newName: string) => void;
}

export default function RenameModal({ title, initialValue, onClose, onSubmit }: Props) {
  const [newName, setNewName] = useState(initialValue);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = newName.trim();
    if (trimmed) {
      onSubmit(trimmed);
      onClose();
    }
  };

  return (
    <ModalLayout title={title} onClose={onClose} width={360}>
      <form onSubmit={handleSubmit} className="pt-5 px-6 pb-6">
        <input
          ref={inputRef}
          type="text"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          className="w-full px-3 py-2.5 border border-border rounded-sm text-base box-border mb-4 bg-surface text-primary"
        />
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={onClose}>取消</Button>
          <Button variant="primary" type="submit">保存</Button>
        </div>
      </form>
    </ModalLayout>
  );
}
