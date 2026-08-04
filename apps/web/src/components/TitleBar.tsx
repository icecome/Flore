import { useState, useEffect, useCallback, useRef } from 'react';
import { isDesktop, getDesktopApp, getPlatform } from '../utils/api.js';
import { showToast } from '../utils/toast';
import { RssIcon, MinusIcon, MaximizeIcon, CopyIcon, X } from './icons';

type Runtime = {
  WindowGetPosition?: () => { x: number; y: number } | Promise<{ x: number; y: number }>;
  WindowSetPosition?: (x: number, y: number) => void;
};

function useRuntime(): Runtime | null {
  const [runtime, setRuntime] = useState<Runtime | null>(null);
  useEffect(() => {
    const r = (window as unknown as { runtime?: Record<string, (...args: number[]) => void | Promise<unknown>> }).runtime;
    if (r?.WindowGetPosition && r.WindowSetPosition) {
      setRuntime(r as unknown as Runtime);
    }
  }, []);
  return runtime;
}

function useWindowDrag(desktop: boolean, maximized: boolean, runtime: Runtime | null) {
  const dragging = useRef(false);
  const dragStartRef = useRef({ screenX: 0, screenY: 0, winX: 0, winY: 0 });

  const handleDragMouseDown = useCallback(async (e: React.MouseEvent) => {
    if (e.button !== 0 || maximized || !runtime?.WindowGetPosition) return;
    e.preventDefault();
    dragging.current = true;
    try {
      const pos = await runtime.WindowGetPosition();
      dragStartRef.current = {
        screenX: e.screenX,
        screenY: e.screenY,
        winX: pos.x,
        winY: pos.y,
      };
    } catch (err) {
      // 取不到窗口坐标只能放弃本次拖动，无需打扰用户
      console.error('TitleBar WindowGetPosition failed:', err);
      dragging.current = false;
    }
  }, [maximized, runtime]);

  useEffect(() => {
    if (!desktop || !runtime?.WindowSetPosition) return;
    let rafId: number | null = null;
    let lastEvent: MouseEvent | null = null;

    const setPos = runtime.WindowSetPosition;
    const handleMouseMove = (e: MouseEvent) => {
      if (!dragging.current) return;
      lastEvent = e;
      if (rafId !== null) return;
      rafId = requestAnimationFrame(() => {
        rafId = null;
        if (!lastEvent || !dragging.current) return;
        const dx = lastEvent.screenX - dragStartRef.current.screenX;
        const dy = lastEvent.screenY - dragStartRef.current.screenY;
        setPos(dragStartRef.current.winX + dx, dragStartRef.current.winY + dy);
      });
    };

    const handleMouseUp = () => {
      dragging.current = false;
      if (rafId !== null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
    };

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
      if (rafId !== null) cancelAnimationFrame(rafId);
    };
  }, [desktop, runtime]);

  return { handleDragMouseDown };
}

function useWindowState() {
  const [maximized, setMaximized] = useState(false);
  const maximizedRef = useRef(false);

  useEffect(() => {
    const app = getDesktopApp();
    if (!app) return;

    // 优先使用持久化的窗口状态，避免依赖 Wails runtime 启动期不可靠的查询
    const initMaximized = async () => {
      if (app.GetWindowState) {
        try {
          const state = await app.GetWindowState();
          setMaximized(!!state.maximised);
          maximizedRef.current = !!state.maximised;
          return;
        } catch {}
      }
      // 回退：WindowIsMaximised 也是 async，取返回值
      if (app.WindowIsMaximised) {
        try {
          const isMax = await app.WindowIsMaximised();
          setMaximized(!!isMax);
          maximizedRef.current = !!isMax;
        } catch {}
      }
    };
    initMaximized();
  }, []);

  const callWindow = useCallback(async (method: string) => {
    const app = getDesktopApp();
    if (!app) return;
    try {
      if (method === 'WindowToggleMaximise') {
        await app.WindowToggleMaximise?.();
        const isMax = app.WindowIsMaximised ? await app.WindowIsMaximised() : false;
        setMaximized(isMax);
        maximizedRef.current = isMax;
      } else if (method === 'WindowMaximise') {
        await app.WindowMaximise?.();
        setMaximized(true);
        maximizedRef.current = true;
      } else if (method === 'WindowUnmaximise') {
        await app.WindowUnmaximise?.();
        setMaximized(false);
        maximizedRef.current = false;
      } else {
        const fn = app[method as keyof typeof app] as unknown as (() => void | Promise<void>) | undefined;
        await fn?.();
      }
    } catch (err) {
      console.error(`TitleBar ${method} failed:`, err);
      showToast('窗口操作失败');
    }
  }, []);

  // 窗口状态变化时保存到 localStorage
  useEffect(() => {
    const app = getDesktopApp();
    if (app && app.SaveWindowState) {
      app.SaveWindowState(maximized);
    }
  }, [maximized]);

  const handleMaxRestore = useCallback(() => {
    callWindow(maximized ? 'WindowUnmaximise' : 'WindowMaximise');
  }, [maximized, callWindow]);

  return { maximized, callWindow, handleMaxRestore };
}

export default function TitleBar() {
  const [desktop, setDesktop] = useState(() => isDesktop());
  const [platform, setPlatform] = useState('');

  useEffect(() => {
    setDesktop(isDesktop());
    getPlatform().then(setPlatform).catch(() => setPlatform(''));
  }, []);

  const runtime = useRuntime();
  const { maximized, callWindow, handleMaxRestore } = useWindowState();
  const { handleDragMouseDown } = useWindowDrag(desktop, maximized, runtime);

  if (!desktop) return null;

  const isMac = platform === 'darwin';

  // macOS: 原生交通灯在左上角，标题区左移 76px 避开，不渲染自定义按钮
  // Windows/Linux: 左标题 + 右侧三按钮
  return (
    <div
      className={`flex h-[34px] min-h-[34px] bg-surface border-b border-border-subtle items-center select-none ${
        isMac ? 'pl-[76px]' : 'pl-3.5 justify-between'
      }`}
      data-wails-drag={isMac ? undefined : true}
    >
      <div
        className="flex items-center gap-2 flex-1 h-full cursor-default"
        data-wails-drag
        onMouseDown={isMac ? undefined : handleDragMouseDown}
      >
        <span className="text-primary flex items-center opacity-90">
          <RssIcon size={16} />
        </span>
        <span className="text-xs font-semibold text-secondary tracking-wide">
          Flore
        </span>
      </div>

      {!isMac && (
        <div className="flex items-center h-full" data-wails-no-drag>
          <button
            className="titlebar-control w-[42px] h-full border-0 bg-transparent text-secondary cursor-pointer flex items-center justify-center"
            onClick={() => callWindow('WindowMinimise')}
            title="最小化"
            aria-label="最小化窗口"
            data-wails-no-drag
          >
            <MinusIcon size={14} />
          </button>
          <button
            className="titlebar-control w-[42px] h-full border-0 bg-transparent text-secondary cursor-pointer flex items-center justify-center"
            onClick={handleMaxRestore}
            title={maximized ? '还原' : '最大化'}
            aria-label={maximized ? '还原窗口' : '最大化窗口'}
            data-wails-no-drag
          >
            {maximized ? <CopyIcon size={12} /> : <MaximizeIcon size={12} />}
          </button>
          <button
            className="titlebar-control close w-[42px] h-full border-0 bg-transparent text-secondary cursor-pointer flex items-center justify-center"
            onClick={() => callWindow('WindowClose')}
            title="关闭"
            aria-label="关闭窗口"
            data-wails-no-drag
          >
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  );
}