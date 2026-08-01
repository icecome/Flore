import React, { useCallback, useEffect, useRef } from 'react';
import { X } from './icons';
import { cn } from '../lib/cn';

interface Props {
  title?: React.ReactNode;
  titleIcon?: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  width?: number | string;
  showHeader?: boolean;
  className?: string;
  closeOnBackdrop?: boolean;
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

function focusFirstElement(container: HTMLDivElement) {
  const focusable = container.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
  if (focusable) {
    requestAnimationFrame(() => {
      if (document.activeElement === null || !container.contains(document.activeElement)) {
        focusable.focus();
      }
    });
  } else {
    container.tabIndex = -1;
    container.focus();
  }
}

function shouldShiftFocusBack(active: HTMLElement | null, first: HTMLElement, last: HTMLElement, container: HTMLDivElement, isShift: boolean) {
  if (isShift) return active === first || !container.contains(active);
  return active === last || !container.contains(active);
}

function trapFocus(e: KeyboardEvent, container: HTMLDivElement) {
  const focusables = container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
  if (focusables.length === 0) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  const active = document.activeElement as HTMLElement | null;

  if (!shouldShiftFocusBack(active, first, last, container, e.shiftKey)) return;

  e.preventDefault();
  (e.shiftKey ? last : first).focus();
}

function useFocusTrap(onClose: () => void) {
  const containerRef = useRef<HTMLDivElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);

  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);

  useEffect(() => {
    previouslyFocusedRef.current = document.activeElement as HTMLElement | null;
    const container = containerRef.current;
    if (container) focusFirstElement(container);

    const handleKeydown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.stopPropagation(); onCloseRef.current(); return; }
      if (e.key === 'Tab' && container) trapFocus(e, container);
    };

    document.addEventListener('keydown', handleKeydown, true);
    return () => {
      document.removeEventListener('keydown', handleKeydown, true);
      const prev = previouslyFocusedRef.current;
      if (!prev || typeof prev.focus !== 'function') return;
      prev.focus();
    };
  }, []);

  return containerRef;
}

function ModalHeader({ title, titleIcon, onClose }: { title?: React.ReactNode; titleIcon?: React.ReactNode; onClose: () => void }) {
  return (
    <div className="flex min-h-[52px] shrink-0 items-center justify-between border-b border-border px-4 py-3">
      <h3 className="m-0 flex items-center gap-2 text-[15px] font-semibold text-primary">
        {titleIcon && <span className="inline-flex text-secondary">{titleIcon}</span>}
        {title}
      </h3>
      <button
        type="button"
        className="inline-flex h-7 w-7 items-center justify-center rounded-sm bg-transparent text-muted transition-colors duration-150 hover:bg-hover hover:text-primary active:scale-95"
        onClick={onClose}
        aria-label="关闭"
      >
        <X size={18} />
      </button>
    </div>
  );
}

export default function ModalLayout({
  title,
  titleIcon,
  onClose,
  children,
  width = 480,
  showHeader = true,
  className = '',
  closeOnBackdrop = true,
}: Props) {
  const containerRef = useFocusTrap(onClose);

  const handleBackdrop = useCallback((e: React.MouseEvent) => {
    if (!closeOnBackdrop) return;
    if (e.target === e.currentTarget) onClose();
  }, [closeOnBackdrop, onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--overlay)] p-4 backdrop-blur-sm animate-modal-fade-in"
      onClick={handleBackdrop}
      role="dialog"
      aria-modal="true"
    >
      <div
        ref={containerRef}
        className={cn(
          'flex max-h-[90vh] w-full flex-col overflow-hidden rounded-lg border border-border bg-elevated shadow-lg animate-modal-scale-in',
          className
        )}
        style={{ maxWidth: width }}
        onClick={(e) => e.stopPropagation()}
      >
        {showHeader && <ModalHeader title={title} titleIcon={titleIcon} onClose={onClose} />}
        {children}
      </div>
    </div>
  );
}
