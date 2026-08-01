import React, { forwardRef } from 'react';
import { cn } from '../lib/cn';

interface Props extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  active?: boolean;
  size?: 'sm' | 'md';
}

const IconButton = React.memo(forwardRef<HTMLButtonElement, Props>(function IconButton(
  { active, size = 'md', className, children, ...rest },
  ref
) {
  return (
    <button
      ref={ref}
      type="button"
      className={cn(
        'inline-flex items-center justify-center rounded-md border border-transparent',
        'transition-all duration-150 ease-out-expo active:scale-[0.94]',
        'disabled:opacity-40 disabled:pointer-events-none',
        size === 'md' ? 'h-8 w-8' : 'h-7 w-7',
        active
          ? 'bg-primary-subtle text-primary'
          : 'text-secondary hover:bg-hover hover:text-primary',
        className
      )}
      {...rest}
    >
      {children}
    </button>
  );
}));

export default IconButton;
