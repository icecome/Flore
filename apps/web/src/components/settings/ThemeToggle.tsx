import React from 'react';
import { Sun, Moon, Monitor } from '../icons';

interface ThemeToggleProps {
  current: 'light' | 'dark' | 'system';
  onToggle: () => void;
}

export default function ThemeToggle({ current, onToggle }: ThemeToggleProps) {
  // 'system' 时根据系统偏好决定显示效果
  const isDark = current === 'dark' || (current === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  return (
    <button
      type="button"
      onClick={onToggle}
      title={isDark ? '切换到浅色模式' : '切换到深色模式'}
      className="group w-full flex items-center gap-3 px-2.5 py-2 rounded-md text-sm cursor-pointer transition-colors duration-150 hover:bg-hover active:bg-pressed text-secondary"
      aria-label={isDark ? '深色模式已启用，点击切换浅色' : '浅色模式已启用，点击切换深色'}
    >
      <div className="w-5 h-5 flex items-center justify-center text-muted group-hover:text-secondary transition-colors">
        {isDark ? <Moon size={16} strokeWidth={2} /> : current === 'system' ? <Monitor size={16} strokeWidth={2} /> : <Sun size={16} strokeWidth={2} />}
      </div>
      <span className="flex-1 text-left">{isDark ? '深色模式' : current === 'system' ? '跟随系统' : '浅色模式'}</span>
      <div className={`w-9 h-5 rounded-full relative transition-colors duration-200 ${isDark ? 'bg-primary' : 'bg-border'}`}>
        <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform duration-200 ${isDark ? 'translate-x-4' : 'translate-x-0.5'}`} />
      </div>
    </button>
  );
}
