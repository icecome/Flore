import { useEffect, useReducer } from 'react';
import { Check, AlertTriangle, Info } from './icons';
import { getToasts, addListener, removeListener } from '../utils/toast';
import { cn } from '../lib/cn';

const typeConfig = {
  success: { icon: Check, accent: 'var(--success)' },
  error: { icon: AlertTriangle, accent: 'var(--danger)' },
  info: { icon: Info, accent: 'var(--primary)' },
} as const;

export default function ToastContainer() {
  const [, forceUpdate] = useReducer((x) => x + 1, 0);

  useEffect(() => {
    addListener(forceUpdate);
    return () => removeListener(forceUpdate);
  }, []);

  const toasts = getToasts();

  if (toasts.length === 0) return null;

  return (
    <div
      className="fixed bottom-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none"
      role="status"
      aria-live="polite"
      aria-atomic="false"
    >
      {toasts.map((t) => {
        const cfg = typeConfig[t.type];
        const Icon = cfg.icon;
        return (
          <div
            key={t.id}
            className={cn(
              'pointer-events-auto flex items-center gap-2.5 pl-3.5 pr-4 py-3 rounded-lg border shadow-lg',
              'animate-list-item-in backdrop-blur-md'
            )}
            style={{
              background: 'var(--glass-bg)',
              borderColor: 'var(--glass-border)',
              borderLeft: `3px solid ${cfg.accent}`,
            }}
          >
            <Icon size={16} className="shrink-0" style={{ color: cfg.accent }} />
            <span className="text-sm text-primary">{t.message}</span>
          </div>
        );
      })}
    </div>
  );
}
