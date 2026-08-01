import React from 'react';

interface Props {
  icon: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

export default function EmptyState({ icon, title, description, action }: Props) {
  return (
    <div className="flex max-w-sm flex-col items-center gap-3 px-8 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl border border-border-subtle bg-surface text-border-strong">
        {icon}
      </div>
      <h3 className="text-[16px] font-semibold text-primary text-balance">{title}</h3>
      {description && <p className="text-[13px] leading-relaxed text-muted text-pretty">{description}</p>}
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}
