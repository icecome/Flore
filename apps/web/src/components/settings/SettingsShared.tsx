import { cn } from '../../lib/cn';

export function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-8">
      <h3 className="mb-1 border-b border-border-subtle pb-2 text-[14px] font-semibold text-primary">
        {title}
      </h3>
      <div className="flex flex-col">{children}</div>
    </section>
  );
}

export function SmallBtn({
  icon,
  label,
  disabled,
  danger,
  onClick,
}: {
  icon?: React.ReactNode;
  label: string;
  disabled?: boolean;
  danger?: boolean;
  onClick?: () => void;
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-md border border-border bg-surface px-2.5 py-1.5 text-[12.5px] transition-colors hover:border-border-strong hover:bg-hover disabled:pointer-events-none disabled:opacity-40',
        danger ? 'text-danger' : 'text-secondary'
      )}
    >
      {icon}
      {label}
    </button>
  );
}

export function IconBtn({
  icon,
  title,
  disabled,
  danger,
  onClick,
}: {
  icon: React.ReactNode;
  title: string;
  disabled?: boolean;
  danger?: boolean;
  onClick?: () => void;
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      title={title}
      className={cn(
        'inline-flex h-7 w-7 items-center justify-center rounded-md transition-colors disabled:pointer-events-none disabled:opacity-30',
        danger
          ? 'text-secondary hover:bg-danger/10 hover:text-danger'
          : 'text-secondary hover:bg-hover hover:text-primary'
      )}
    >
      {icon}
    </button>
  );
}

export function ShortcutRow({ keys, desc }: { keys: string; desc: string }) {
  return (
    <div className="flex items-center gap-3 border-b border-border-subtle py-2 last:border-0">
      <kbd className="rounded-md border border-border bg-elevated px-2 py-0.5 text-[12px] font-medium text-primary">
        {keys}
      </kbd>
      <span className="text-[13px] text-secondary">{desc}</span>
    </div>
  );
}

export function Row({ title, desc, control }: { title: string; desc?: string; control: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-6 border-b border-border-subtle py-3 last:border-0">
      <div className="min-w-0">
        <div className="text-[13.5px] font-medium text-primary">{title}</div>
        {desc && <div className="mt-0.5 text-[12px] leading-snug text-muted">{desc}</div>}
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}

export function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative flex h-[22px] w-[40px] shrink-0 items-center rounded-full p-0 transition-colors duration-200',
        checked ? 'bg-primary' : 'bg-border-strong'
      )}
    >
      <span
        className={cn(
          'absolute left-[3px] top-[3px] h-4 w-4 rounded-full bg-white shadow-sm transition-transform duration-200',
          checked ? 'translate-x-[18px]' : 'translate-x-0'
        )}
      />
    </button>
  );
}

export function Select({
  value,
  options,
  onChange,
}: {
  value: string | number;
  options: { value: string | number; label: string }[];
  onChange: (value: string | number) => void;
}) {
  const isNumber = typeof value === 'number';
  return (
    <select
      value={value}
      onChange={(e) => onChange(isNumber ? Number(e.target.value) : e.target.value)}
      className="rounded-md border border-border bg-surface px-2.5 py-1.5 text-[13px] text-primary outline-none focus:border-primary"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function Slider({
  value,
  min,
  max,
  step,
  unit,
  onChange,
}: {
  value: number;
  min: number;
  max: number;
  step: number;
  unit?: string;
  onChange: (value: number) => void;
}) {
  return (
    <div className="flex items-center gap-3">
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="h-1 w-32 cursor-pointer appearance-none rounded bg-border-strong"
      />
      <span className="min-w-[48px] text-right text-[13px] text-secondary">
        {value}
        {unit}
      </span>
    </div>
  );
}

// 数字输入框：用于需要精确数值的设置项（如保留天数、保留数量）
export function NumberInput({
  value,
  min,
  max,
  step = 1,
  unit,
  onChange,
}: {
  value: number;
  min?: number;
  max?: number;
  step?: number;
  unit?: string;
  onChange: (value: number) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <input
        type="number"
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(e) => {
          let n = Number(e.target.value);
          if (!Number.isFinite(n)) return;
          if (min !== undefined && n < min) n = min;
          if (max !== undefined && n > max) n = max;
          onChange(n);
        }}
        className="w-[72px] rounded-md border border-border bg-surface px-2 py-1 text-[13px] text-primary outline-none focus:border-primary"
      />
      {unit && <span className="text-[12px] text-muted">{unit}</span>}
    </div>
  );
}