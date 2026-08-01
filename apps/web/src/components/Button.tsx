import React from 'react';
import { cn } from '../lib/cn';

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
type Size = 'sm' | 'md' | 'lg';

interface Props extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

const variantClasses: Record<Variant, string> = {
  primary:
    'border-none bg-primary text-on-primary hover:bg-primary-hover active:bg-primary-active',
  secondary:
    'border border-border bg-surface text-secondary hover:bg-hover hover:text-primary active:bg-active',
  ghost:
    'border-none bg-transparent text-secondary hover:bg-hover hover:text-primary',
  danger:
    'border-none bg-danger text-on-primary hover:brightness-110 active:brightness-90',
};

const sizeClasses: Record<Size, string> = {
  sm:  'px-3 py-1.5 text-xs',
  md:  'px-[18px] py-2 text-sm',
  lg:  'px-6 py-2.5 text-base',
};

export default React.memo(function Button({
  variant = 'secondary',
  size = 'md',
  className,
  children,
  disabled,
  ...rest
}: Props) {
  return (
    <button
      type="button"
      disabled={disabled}
      className={cn(
        'inline-flex items-center justify-center rounded-sm font-medium cursor-pointer select-none',
        'transition-all duration-150 ease-out-expo active:scale-[0.97]',
        'disabled:opacity-40 disabled:pointer-events-none disabled:active:scale-100',
        variantClasses[variant],
        sizeClasses[size],
        className
      )}
      {...rest}
    >
      {children}
    </button>
  );
});