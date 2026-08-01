import { AlertTriangle } from './icons';
import Button from './Button';
import ModalLayout from './ModalLayout';
import { cn } from '../lib/cn';

interface Props {
  title?: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmDialog({
  title = '确认操作',
  message,
  confirmText = '确定',
  cancelText = '取消',
  danger = false,
  onConfirm,
  onCancel,
}: Props) {
  return (
    <ModalLayout title={title} onClose={onCancel} width={400} closeOnBackdrop={false}>
      <div className="flex flex-col items-center gap-3 px-6 pt-6">
        <div
          className={cn(
            'flex h-11 w-11 items-center justify-center rounded-full',
            danger ? 'bg-danger-subtle text-danger' : 'bg-primary-subtle text-primary'
          )}
        >
          <AlertTriangle size={20} />
        </div>
        <p className="m-0 whitespace-pre-wrap text-center text-[13.5px] leading-relaxed text-secondary">
          {message}
        </p>
      </div>
      <div className="flex justify-end gap-2.5 p-5 pt-6">
        <Button variant="secondary" onClick={onCancel}>{cancelText}</Button>
        <Button variant={danger ? 'danger' : 'primary'} onClick={onConfirm}>{confirmText}</Button>
      </div>
    </ModalLayout>
  );
}
