import { useEffect, type RefObject } from 'react';

/**
 * 点击外部时触发回调的 hook
 * @param ref 目标元素的 ref
 * @param active 是否启用监听（false 时不注册 listener）
 * @param onClickOutside 点击外部时的回调
 */
export function useClickOutside<T extends HTMLElement>(
  ref: RefObject<T | null>,
  active: boolean,
  onClickOutside: () => void
): void {
  useEffect(() => {
    if (!active) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClickOutside();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [active, onClickOutside, ref]);
}
