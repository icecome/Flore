import { useCallback, useState } from 'react';
import type { ContextMenuItem, MenuState } from '../components/ContextMenu';

export interface MenuProps {
  x: number;
  y: number;
  items: ContextMenuItem[];
  onClose: () => void;
}

export function useContextMenu() {
  const [menu, setMenu] = useState<MenuState | null>(null);

  const showMenu = useCallback((e: React.MouseEvent, items: ContextMenuItem[]) => {
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, items });
  }, []);

  const closeMenu = useCallback(() => setMenu(null), []);

  const menuProps: MenuProps | null = menu
    ? { x: menu.x, y: menu.y, items: menu.items, onClose: closeMenu }
    : null;

  return { menuProps, showMenu, closeMenu };
}
