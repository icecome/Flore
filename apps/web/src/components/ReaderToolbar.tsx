import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Star,
  Clock,
  Type,
  Globe,
  MailOpen,
  BookOpen,
  Minus,
  Plus,
  ArrowLeft,
  ExternalLink,
  FileText,
  ChevronRight,
  Check,
  Share2,
  MoreHorizontal,
  Copy,
  Image,
  FileDown,
} from './icons';
import type { Item } from '../types';
import { cn } from '../lib/cn';
import {
  type AppSettings,
  type ReaderTheme,
  type ReaderThemeVariant,
  THEME_OPTIONS,
} from '../utils/settings';
import { clipboardWrite } from '../utils/api';
import { showToast } from '../utils/toast';
import { useClickOutside } from '../hooks/useClickOutside';
import IconButton from './IconButton';

const FONT_STEPS = [14, 15, 16, 17, 18, 19, 20, 21, 22, 24];

const FONT_FAMILY_OPTIONS: { value: AppSettings['readerFontFamily']; label: string; family: string }[] = [
  { value: 'serif', label: '衬线', family: 'var(--font-serif)' },
  { value: 'sans', label: '无衬线', family: 'var(--font-sans)' },
  { value: 'mono', label: '等宽', family: 'var(--font-mono)' },
];

const LETTER_SPACING_OPTIONS: { value: AppSettings['readerLetterSpacing']; label: string }[] = [
  { value: 'normal', label: '默认' },
  { value: 'wide', label: '宽松' },
];

// 工具栏按钮定义
type ToolbarButtonId = 'star' | 'readLater' | 'readerMode' | 'viewOriginal' | 'openBrowser' | 'toggleRead' | 'typeSettings' | 'share' | 'more';

interface ToolbarButtonDef {
  id: ToolbarButtonId;
  icon: React.ReactNode;
  title: string;
  defaultVisible: boolean;
}

const ALL_BUTTONS: ToolbarButtonDef[] = [
  { id: 'star', icon: <Star size={16} />, title: '收藏', defaultVisible: true },
  { id: 'readLater', icon: <Clock size={16} />, title: '稍后阅读', defaultVisible: true },
  { id: 'readerMode', icon: <FileText size={16} />, title: '全文模式', defaultVisible: true },
  { id: 'viewOriginal', icon: <BookOpen size={16} />, title: '网页模式', defaultVisible: true },
  { id: 'openBrowser', icon: <Globe size={16} />, title: '浏览器打开', defaultVisible: true },
  { id: 'toggleRead', icon: <MailOpen size={16} />, title: '标为已读', defaultVisible: true },
  { id: 'typeSettings', icon: <Type size={16} />, title: '排版设置', defaultVisible: true },
  { id: 'share', icon: <Share2 size={16} />, title: '分享', defaultVisible: true },
  { id: 'more', icon: <MoreHorizontal size={16} />, title: '更多', defaultVisible: true },
];

interface ReaderToolbarProps {
  item: Item;
  settings: AppSettings;
  editable: boolean;
  resolvedReaderTheme: ReaderThemeVariant;
  displayMode: 'rss' | 'readability' | 'iframe';
  onBack?: () => void;
  onToggleRead: (id: number, read: boolean) => void;
  onToggleStar: (id: number, starred: boolean) => void;
  onToggleReadLater: (id: number, readLater: boolean) => void;
  onToggleReaderMode: () => void;
  onToggleViewOriginal: () => void;
  onOpenExternal: (url: string) => void;
  onSettingsChange?: (settings: AppSettings) => void;
  onExportPNG: () => Promise<boolean>;
  onExportPDF: () => Promise<boolean>;
}

export default function ReaderToolbar({
  item,
  settings,
  editable,
  resolvedReaderTheme,
  displayMode,
  onBack,
  onToggleRead,
  onToggleStar,
  onToggleReadLater,
  onToggleReaderMode,
  onToggleViewOriginal,
  onOpenExternal,
  onSettingsChange,
  onExportPNG,
  onExportPDF,
}: ReaderToolbarProps) {
  const [showType, setShowType] = useState(false);
  const [showMore, setShowMore] = useState(false);
  const [showShare, setShowShare] = useState(false);
  const [toolbarVisible, setToolbarVisible] = useState<ToolbarButtonId[]>(() =>
    ALL_BUTTONS.filter((b) => b.defaultVisible).map((b) => b.id)
  );
  const typeRef = useRef<HTMLDivElement>(null);
  const moreRef = useRef<HTMLDivElement>(null);
  const shareRef = useRef<HTMLDivElement>(null);

  // 切换文章时关闭更多面板
  useEffect(() => {
    setShowMore(false);
  }, [item.id]);

  const closeType = useCallback(() => setShowType(false), []);
  const closeMore = useCallback(() => setShowMore(false), []);
  const closeShare = useCallback(() => setShowShare(false), []);

  useClickOutside(typeRef, showType, closeType);
  useClickOutside(moreRef, showMore, closeMore);
  useClickOutside(shareRef, showShare, closeShare);

  const updateSetting = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    if (!onSettingsChange) return;
    onSettingsChange({ ...settings, [key]: value });
  };

  const stepFont = (dir: 1 | -1) => {
    if (!editable) return;
    const idx = FONT_STEPS.indexOf(settings.readerFontSize);
    const next = FONT_STEPS[Math.min(FONT_STEPS.length - 1, Math.max(0, idx + dir))];
    updateSetting('readerFontSize', next);
  };

  const toggleButtonVisibility = (id: ToolbarButtonId) => {
    if (id === 'more') return;
    setToolbarVisible((prev) => insertBeforeMore(id, prev));
  };

  function insertBeforeMore(id: ToolbarButtonId, prev: ToolbarButtonId[]): ToolbarButtonId[] {
    if (prev.includes(id)) return prev.filter((b) => b !== id);
    const moreIdx = prev.indexOf('more');
    if (moreIdx < 0) return [...prev, id];
    const newArr = [...prev];
    newArr.splice(moreIdx, 0, id);
    return newArr;
  }

  const renderButton = (id: ToolbarButtonId) => {
    if (!item) return null;
    switch (id) {
      case 'star':
        return (
          <IconButton
            key={id}
            onClick={() => onToggleStar(item.id, !item.isStarred)}
            active={item.isStarred}
            title={item.isStarred ? '取消收藏' : '收藏'}
          >
            <Star size={16} fill={item.isStarred ? 'currentColor' : 'none'} />
          </IconButton>
        );
      case 'readLater':
        return (
          <IconButton
            key={id}
            onClick={() => onToggleReadLater(item.id, !item.isReadLater)}
            active={item.isReadLater}
            title={item.isReadLater ? '取消稍后阅读' : '稍后阅读'}
          >
            <Clock size={16} fill={item.isReadLater ? 'currentColor' : 'none'} />
          </IconButton>
        );
      case 'readerMode':
        return (
          <IconButton
            key={id}
            onClick={() => onToggleReaderMode()}
            active={displayMode === 'readability'}
            title={displayMode === 'readability' ? '退出全文模式' : '全文模式'}
          >
            <FileText size={16} />
          </IconButton>
        );
      case 'viewOriginal':
        return (
          <IconButton
            key={id}
            onClick={() => onToggleViewOriginal()}
            active={displayMode === 'iframe'}
            title={displayMode === 'iframe' ? '退出网页模式' : '网页模式'}
          >
            <BookOpen size={16} />
          </IconButton>
        );
      case 'openBrowser':
        return (
          <IconButton
            key={id}
            onClick={() => {
              if (item.link) onOpenExternal(item.link);
            }}
            title="浏览器打开"
          >
            <Globe size={16} />
          </IconButton>
        );
      case 'toggleRead':
        return (
          <IconButton
            key={id}
            onClick={() => {
              onToggleRead(item.id, !item.isRead);
            }}
            title={item.isRead ? '标为未读' : '标为已读'}
          >
            <MailOpen size={16} />
          </IconButton>
        );
      case 'typeSettings':
        return (
          <div key={id} ref={typeRef} className="relative">
            <IconButton
              onClick={() => {
                setShowType((v) => !v);
                setShowMore(false);
              }}
              active={showType}
              title="排版设置"
            >
              <Type size={16} />
            </IconButton>
            {showType && (
              <TypePanel
                settings={settings}
                editable={editable}
                onStepFont={stepFont}
                onUpdateSetting={updateSetting}
              />
            )}
          </div>
        );
      case 'share':
        return (
          <div key={id} ref={shareRef} className="relative">
            <IconButton
              onClick={() => {
                setShowShare((v) => !v);
                setShowMore(false);
                setShowType(false);
              }}
              active={showShare}
              title="分享"
            >
              <Share2 size={16} />
            </IconButton>
            {showShare && (
              <SharePanel
                item={item}
                onClose={() => setShowShare(false)}
                onExportPNG={async () => {
                  const ok = await onExportPNG();
                  if (ok) setShowShare(false);
                }}
                onExportPDF={async () => {
                  const ok = await onExportPDF();
                  if (ok) setShowShare(false);
                }}
              />
            )}
          </div>
        );
      case 'more':
        return (
          <div key={id} ref={moreRef} className="relative">
            <IconButton
              onClick={() => {
                setShowMore((v) => !v);
                setShowType(false);
              }}
              active={showMore}
              title="更多"
            >
              <MoreHorizontal size={16} />
            </IconButton>
            {showMore && (
              <MorePanel
                allButtons={ALL_BUTTONS}
                toolbarVisible={toolbarVisible}
                onToggle={toggleButtonVisibility}
                onClose={() => setShowMore(false)}
              />
            )}
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <header
      className="relative z-10 flex h-[52px] shrink-0 items-center justify-between gap-2 border-b px-4 backdrop-blur"
      style={{
        background: `color-mix(in srgb, ${resolvedReaderTheme.bg} 88%, transparent)`,
        borderBottomColor: resolvedReaderTheme.border,
      }}
    >
      <div className="flex min-w-0 items-center gap-2">
        {onBack && (
          <IconButton onClick={onBack} title="返回">
            <ArrowLeft size={16} />
          </IconButton>
        )}
        <span className="truncate text-[13px] font-medium text-secondary">{item.sourceName}</span>
      </div>
      <div className="flex items-center gap-0.5">
        {toolbarVisible.map((id) => renderButton(id))}
      </div>
    </header>
  );
}

function TypePanel({
  settings,
  editable,
  onStepFont,
  onUpdateSetting,
}: {
  settings: AppSettings;
  editable: boolean;
  onStepFont: (dir: 1 | -1) => void;
  onUpdateSetting: <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => void;
}) {
  return (
    <div className="absolute right-0 top-[calc(100%+8px)] z-30 w-64 rounded-lg border border-border bg-surface p-4 shadow-lg animate-modal-scale-in">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-[13px] font-semibold text-primary">排版</span>
        {editable && (
          <button
            onClick={() => {
              onUpdateSetting('readerFontSize', 19);
              onUpdateSetting('readerLineHeight', 1.8);
              onUpdateSetting('readerFontFamily', 'serif');
              onUpdateSetting('readerLetterSpacing', 'normal');
              onUpdateSetting('readerMaxWidth', 720);
              onUpdateSetting('readerTheme', 'system');
            }}
            className="text-[12px] text-primary hover:underline"
          >
            重置
          </button>
        )}
      </div>
      <div className="flex flex-col gap-3">
        {/* 字号 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">字号</div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => onStepFont(-1)}
              disabled={!editable}
              className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border text-secondary transition-colors hover:bg-hover hover:text-primary disabled:opacity-40"
            >
              <Minus size={14} />
            </button>
            <span className="w-8 text-center text-[13px] tabular-nums text-primary">{settings.readerFontSize}</span>
            <button
              onClick={() => onStepFont(1)}
              disabled={!editable}
              className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border text-secondary transition-colors hover:bg-hover hover:text-primary disabled:opacity-40"
            >
              <Plus size={14} />
            </button>
          </div>
        </div>
        {/* 字体 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">字体</div>
          <div className="flex gap-1.5">
            {FONT_FAMILY_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                onClick={() => {
                  if (!editable) return;
                  onUpdateSetting('readerFontFamily', opt.value);
                }}
                disabled={!editable}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1.5 text-[12px] transition-colors',
                  settings.readerFontFamily === opt.value
                    ? 'border-primary bg-primary-subtle text-primary'
                    : 'border-border text-secondary hover:bg-hover'
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>
        {/* 行距 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">行距</div>
          <div className="flex gap-1.5">
            {[1.6, 1.8, 2.0, 2.2].map((v) => (
              <button
                key={v}
                onClick={() => {
                  if (!editable) return;
                  onUpdateSetting('readerLineHeight', v);
                }}
                disabled={!editable}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1.5 text-[12px] transition-colors',
                  settings.readerLineHeight === v
                    ? 'border-primary bg-primary-subtle text-primary'
                    : 'border-border text-secondary hover:bg-hover'
                )}
              >
                {v}
              </button>
            ))}
          </div>
        </div>
        {/* 字间距 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">字间距</div>
          <div className="flex gap-1.5">
            {LETTER_SPACING_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                onClick={() => {
                  if (!editable) return;
                  onUpdateSetting('readerLetterSpacing', opt.value);
                }}
                disabled={!editable}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1.5 text-[12px] transition-colors',
                  settings.readerLetterSpacing === opt.value
                    ? 'border-primary bg-primary-subtle text-primary'
                    : 'border-border text-secondary hover:bg-hover'
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>
        {/* 页面宽度 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">页面宽度</div>
          <div className="flex gap-1.5">
            {[
              { v: 600, label: '窄' },
              { v: 720, label: '中' },
              { v: 860, label: '宽' },
              { v: 1000, label: '全' },
            ].map((opt) => (
              <button
                key={opt.v}
                onClick={() => {
                  if (!editable) return;
                  onUpdateSetting('readerMaxWidth', opt.v);
                }}
                disabled={!editable}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1.5 text-[12px] transition-colors',
                  settings.readerMaxWidth === opt.v
                    ? 'border-primary bg-primary-subtle text-primary'
                    : 'border-border text-secondary hover:bg-hover'
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>
        {/* 主题 */}
        <div>
          <div className="mb-1.5 text-[12px] font-medium text-secondary">主题</div>
          <div className="flex gap-1.5">
            {THEME_OPTIONS.filter((t) => t.value !== 'system').map((opt) => (
              <button
                key={opt.value}
                onClick={() => {
                  if (!editable) return;
                  onUpdateSetting('readerTheme', opt.value as ReaderTheme);
                }}
                disabled={!editable}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1.5 text-[12px] transition-colors',
                  settings.readerTheme === opt.value
                    ? 'border-primary bg-primary-subtle text-primary'
                    : 'border-border text-secondary hover:bg-hover'
                )}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function MorePanel({
  allButtons,
  toolbarVisible,
  onToggle,
  onClose,
}: {
  allButtons: ToolbarButtonDef[];
  toolbarVisible: ToolbarButtonId[];
  onToggle: (id: ToolbarButtonId) => void;
  onClose: () => void;
}) {
  const visible = allButtons.filter((b) => b.id !== 'more' && toolbarVisible.includes(b.id));
  const hidden = allButtons.filter((b) => b.id !== 'more' && !toolbarVisible.includes(b.id));

  return (
    <div className="absolute right-0 top-[calc(100%+8px)] z-30 w-56 rounded-lg border border-border bg-surface p-3 shadow-lg animate-modal-scale-in">
      <div className="mb-2">
        <div className="mb-1.5 flex items-center gap-1 text-[11px] font-medium text-muted">
          <ChevronRight size={12} />
          工具栏按钮
        </div>
        <div className="flex flex-col gap-0.5">
          {visible.map((b) => (
            <button
              key={b.id}
              onClick={() => {
                onToggle(b.id);
                onClose();
              }}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] text-secondary transition-colors hover:bg-hover"
            >
              <span className="text-primary">{b.icon}</span>
              <span className="flex-1 text-left">{b.title}</span>
              <span className="text-muted">
                <Check size={14} />
              </span>
            </button>
          ))}
          {visible.length === 0 && (
            <span className="px-2 py-1 text-[12px] text-muted">无</span>
          )}
        </div>
      </div>
      <div className="h-px bg-border-subtle" />
      <div className="mt-2">
        <div className="mb-1.5 flex items-center gap-1 text-[11px] font-medium text-muted">
          <ChevronRight size={12} />
          折叠按钮
        </div>
        <div className="flex flex-col gap-0.5">
          {hidden.map((b) => (
            <button
              key={b.id}
              onClick={() => {
                onToggle(b.id);
                onClose();
              }}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] text-secondary transition-colors hover:bg-hover"
            >
              <span className="text-muted">{b.icon}</span>
              <span className="flex-1 text-left">{b.title}</span>
            </button>
          ))}
          {hidden.length === 0 && (
            <span className="px-2 py-1 text-[12px] text-muted">无</span>
          )}
        </div>
      </div>
    </div>
  );
}

/** 分享面板：展示文章标题和多种分享方式 */
function SharePanel({
  item,
  onClose,
  onExportPNG,
  onExportPDF,
}: {
  item: Item;
  onClose: () => void;
  onExportPNG: () => void;
  onExportPDF: () => void;
}) {
  const handleSystemShare = async () => {
    if (typeof navigator.share === 'function') {
      try {
        await navigator.share({
          title: item.title,
          url: item.link,
        });
        onClose();
        return;
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          onClose();
          return;
        }
      }
    }
    const ok = await clipboardWrite(item.link);
    showToast(ok ? '链接已复制' : '复制失败');
    onClose();
  };

  const copyToClipboard = async (text: string, toast: string) => {
    const ok = await clipboardWrite(text);
    if (ok) {
      showToast(toast);
      onClose();
    } else {
      showToast('复制失败');
    }
  };

  const displayTitle = item.title.length > 40 ? item.title.slice(0, 40) + '...' : item.title;

  const shareActions = [
    {
      icon: <Copy size={13} />,
      label: '复制链接',
      desc: item.link,
      action: () => copyToClipboard(item.link, '链接已复制'),
    },
    {
      icon: <FileText size={13} />,
      label: '复制标题+链接',
      desc: `${item.title}\n${item.link}`,
      action: () => copyToClipboard(`${item.title}\n${item.link}`, '标题和链接已复制'),
    },
    {
      icon: <ExternalLink size={13} />,
      label: '复制 Markdown',
      desc: `[${item.title}](${item.link})`,
      action: () => copyToClipboard(`[${item.title}](${item.link})`, 'Markdown 已复制'),
    },
    {
      icon: <Share2 size={13} />,
      label: '系统分享',
      desc: '调起系统分享面板',
      action: handleSystemShare,
    },
    {
      icon: <Image size={13} />,
      label: '导出 PNG',
      desc: '保存为图片',
      action: onExportPNG,
    },
    {
      icon: <FileDown size={13} />,
      label: '导出 PDF',
      desc: '保存为 PDF 文档',
      action: onExportPDF,
    },
  ];

  return (
    <div className="absolute right-0 top-[calc(100%+8px)] z-30 w-80 rounded-lg border border-border bg-surface p-4 shadow-lg animate-modal-scale-in">
      <div className="mb-3">
        <div className="mb-1 text-[11px] font-medium text-muted">分享内容</div>
        <div
          className="line-clamp-2 text-[13px] font-semibold leading-1.5 text-primary"
          title={item.title}
        >
          {displayTitle}
        </div>
        <div className="mt-0.5 break-all text-[11px] text-muted" title={item.link}>{item.link}</div>
      </div>

      <div className="h-px bg-border-subtle" />

      <div className="mt-3 flex flex-col gap-1">
        {shareActions.map((act) => (
          <button
            key={act.label}
            onClick={act.action}
            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-[13px] text-secondary transition-colors hover:bg-hover"
          >
            <span className="flex h-6 w-6 items-center justify-center rounded-md bg-primary-subtle text-primary">
              {act.icon}
            </span>
            <div className="flex flex-col items-start gap-0.5">
              <span className="text-[13px] text-primary">{act.label}</span>
              <span className="truncate text-[11px] text-muted max-w-[200px]">{act.desc}</span>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
