import { useEffect, useRef } from 'react';
import { cn } from '../lib/cn';
import { showToast } from '../utils/toast';

export interface ContextMenuItem {
  id: string;
  label: string;
  disabled?: boolean;
  danger?: boolean;
  shortcut?: string;
  onClick?: () => void;
  separator?: boolean;
}

export interface MenuState {
  x: number;
  y: number;
  items: ContextMenuItem[];
}

interface Props {
  x: number;
  y: number;
  items: ContextMenuItem[];
  onClose: () => void;
}

export default function ContextMenu({ x, y, items, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleBackdropClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', handleBackdropClick);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleBackdropClick);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  const adjustedX = Math.min(x, window.innerWidth - 180);
  const itemHeight = 32;
  const separatorHeight = 9;
  const totalHeight = items.reduce(
    (sum, item) => sum + (item.separator ? separatorHeight : itemHeight),
    0
  );
  const adjustedY = Math.min(y, window.innerHeight - totalHeight - 8);

  return (
    <div
      ref={ref}
      className="fixed z-[2000] min-w-[160px] bg-elevated border border-border rounded-md shadow-lg py-1.5 flex flex-col animate-modal-fade-in"
      style={{ left: adjustedX, top: adjustedY }}
      role="menu"
      aria-orientation="vertical"
    >
      {items.map((item) =>
        item.separator ? (
          <div key={item.id} className="h-px bg-border my-1 mx-2.5" role="separator" />
        ) : (
          <button
            key={item.id}
            type="button"
            disabled={item.disabled}
            onClick={async () => {
              try {
                await item.onClick?.();
              } catch (err) {
                // 菜单项自身未捕获的异常在此兜底，必须给出反馈否则点击像"没反应"
                console.error('ContextMenu item click error:', err);
                showToast('操作失败，请重试');
              } finally {
                onClose();
              }
            }}
            className={cn(
              'flex justify-between items-center px-3.5 py-2 border-none bg-transparent text-sm text-left cursor-pointer',
              'transition-colors duration-150',
              item.danger
                ? 'text-danger hover:bg-danger-subtle'
                : 'text-primary hover:bg-hover',
              item.disabled && 'opacity-40 cursor-not-allowed'
            )}
            role="menuitem"
            aria-disabled={item.disabled}
          >
            <span>{item.label}</span>
            {item.shortcut && <span className="text-[11px] text-muted ml-4">{item.shortcut}</span>}
          </button>
        )
      )}
    </div>
  );
}