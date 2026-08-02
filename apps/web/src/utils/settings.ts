import { getDesktopApp, getServerSettings, putServerSettings } from './api';

export type ReaderTheme = 'system' | 'paper' | 'sepia' | 'green';

// 阅读主题变体：每个主题内置浅色+深色，随应用主题自动切换
export interface ReaderThemeVariant {
  bg: string;
  color: string;
  muted: string;
  link: string;
  border: string;
}

export const READER_THEME_PRESETS: Record<Exclude<ReaderTheme, 'system'>, {
  label: string;
  light: ReaderThemeVariant;
  dark: ReaderThemeVariant;
}> = {
  paper: {
    label: '纸白',
    light: {
      bg: '#FFFFFF',
      color: '#1A1D24',
      muted: '#68707A',
      link: '#7B68EE',
      border: '#E5E8ED',
    },
    dark: {
      bg: '#1E1F23',
      color: '#E6E8EC',
      muted: '#A8ACB4',
      link: '#9B8AFB',
      border: '#2C2E33',
    },
  },
  sepia: {
    label: '护眼',
    light: {
      bg: '#F5F1EA',
      color: '#5C5043',
      muted: '#8A7E6E',
      link: '#9B6B3E',
      border: '#E0D9CC',
    },
    dark: {
      bg: '#28241F',
      color: '#D4CCBE',
      muted: '#A89A86',
      link: '#D4A574',
      border: '#3A352E',
    },
  },
  green: {
    label: '绿意',
    light: {
      bg: '#ECF3EE',
      color: '#2B4A32',
      muted: '#5A7560',
      link: '#3D7A45',
      border: '#D5E2D9',
    },
    dark: {
      bg: '#1E251F',
      color: '#BED4C4',
      muted: '#8AA088',
      link: '#7DBE7C',
      border: '#2C352E',
    },
  },
};

// 根据阅读主题与应用主题解析出对应变体
// system 主题跟随应用主题使用 paper 变体
export function resolveReaderTheme(
  readerTheme: ReaderTheme,
  appTheme: 'light' | 'dark'
): ReaderThemeVariant {
  const mood = readerTheme === 'system' ? 'paper' : readerTheme;
  const preset = READER_THEME_PRESETS[mood];
  return appTheme === 'dark' ? preset.dark : preset.light;
}

// 标记已读模式（单选）
export type MarkReadMode = 'none' | 'scroll' | 'hover' | 'view';

// 文章列表密度
export type ListDensity = 'compact' | 'normal' | 'comfortable';

// 文章列表排序
export type ListSortOrder = 'newest' | 'oldest';

// 日期格式
export type ListDateFormat = 'relative' | 'absolute';

// 窗口关闭行为
export type CloseBehavior = 'quit' | 'tray';

// 窗口最小化行为
export type MinimizeBehavior = 'taskbar' | 'tray';

export interface AppSettings {
  // === 通用：时间线行为 ===
  autoGroup: boolean;
  hideRead: boolean;
  hidePrivateInSidebar: boolean;
  hidePrivateInTimeline: boolean;
  unreadOnStart: boolean;
  dimRead: boolean;

  // === 通用：自动标记已读（单选） ===
  markReadMode: MarkReadMode;
  markReadHoverDelay: number;

  // === 通用：抓取与同步 ===
  defaultInterval: number;
  autoFetchOnStart: boolean;

  // === 通用：文章打开方式 ===
  openArticleMode: 'rss' | 'readability' | 'iframe' | 'browser';

  // === 通用：文章列表偏好 ===
  listDensity: ListDensity;
  listSortOrder: ListSortOrder;
  listShowPreview: boolean;
  listDateFormat: ListDateFormat;

  // === 外观 ===
  appTheme: 'light' | 'dark' | 'system';
  readerTheme: ReaderTheme;
  readerFontSize: number;
  readerLineHeight: number;
  readerFontFamily: 'serif' | 'sans' | 'mono';
  readerLetterSpacing: 'normal' | 'wide';
  readerMaxWidth: number;
  accentColor: string;

  // === 数据管理：备份策略 ===
  backupMaxKeep: number;
  backupMaxDays: number;
  backupAutoEnabled: boolean;
  backupAutoInterval: number;

  // === 数据管理：文章留存 ===
  articleRetentionDays: number;
  articleRetentionMax: number;
  retentionExcludeStarred: boolean;
  retentionExcludeReadLater: boolean;

  // === 通知与驻留 ===
  closeBehavior: CloseBehavior;
  minimizeBehavior: MinimizeBehavior;
  trayEnabled: boolean;
  notifyEnabled: boolean;
  notifyBatchMin: number;

  // === 网络 ===
  fetchTimeout: number;
  fetchConcurrency: number;
  proxyEnabled: boolean;
  proxyUrl: string;

  // === 隐私 ===
  loadOnlineAvatar: boolean;
}

const STORAGE_KEY = 'flore-settings';
const MIGRATION_KEY = 'flore-settings-migrated';

export const DEFAULT_SETTINGS: AppSettings = {
  // 通用：时间线行为
  autoGroup: false,
  hideRead: false,
  hidePrivateInSidebar: false,
  hidePrivateInTimeline: false,
  unreadOnStart: false,
  dimRead: false,

  // 通用：标记已读
  markReadMode: 'view',
  markReadHoverDelay: 800,

  // 通用：抓取与同步
  defaultInterval: 120,
  autoFetchOnStart: false,

  // 通用：文章打开方式
  openArticleMode: 'rss',

  // 通用：文章列表偏好
  listDensity: 'normal',
  listSortOrder: 'newest',
  listShowPreview: true,
  listDateFormat: 'relative',

  // 外观
  appTheme: 'system',
  readerTheme: 'system',
  readerFontSize: 19,
  readerLineHeight: 1.8,
  readerFontFamily: 'serif',
  readerLetterSpacing: 'normal',
  readerMaxWidth: 720,
  accentColor: '#7B68EE',

  // 数据管理：备份策略
  backupMaxKeep: 10,
  backupMaxDays: 30,
  backupAutoEnabled: false,
  backupAutoInterval: 1440,

  // 数据管理：文章留存
  articleRetentionDays: 0,
  articleRetentionMax: 0,
  retentionExcludeStarred: true,
  retentionExcludeReadLater: true,

  // 通知与驻留
  closeBehavior: 'quit',
  minimizeBehavior: 'taskbar',
  trayEnabled: false,
  notifyEnabled: false,
  notifyBatchMin: 5,

  // 网络
  fetchTimeout: 30,
  fetchConcurrency: 5,
  proxyEnabled: false,
  proxyUrl: '',

  // 隐私
  loadOnlineAvatar: true,
};

// 迁移旧版 mark 已读配置到新字段（优先级：hover > scroll > view）
function migrateMarkReadMode(
  raw: Record<string, unknown>
): { markReadMode: MarkReadMode } | undefined {
  const has = (k: string) => k in raw && raw[k] !== undefined;
  if (has('markReadMode')) {
    return { markReadMode: raw.markReadMode as MarkReadMode };
  }
  if (has('markOnHover') || has('markOnScroll') || has('markOnView')) {
    if (raw.markOnHover) return { markReadMode: 'hover' };
    if (raw.markOnScroll) return { markReadMode: 'scroll' };
    if (raw.markOnView) return { markReadMode: 'view' };
    return { markReadMode: 'none' };
  }
  return undefined;
}

// 迁移旧版 hidePrivate 配置到新字段
function migrateHidePrivate(
  raw: Record<string, unknown>
): { hidePrivateInSidebar: boolean; hidePrivateInTimeline: boolean } | undefined {
  const has = (k: string) => k in raw && raw[k] !== undefined;
  if (!has('hidePrivate') || has('hidePrivateInSidebar')) return undefined;
  return {
    hidePrivateInSidebar: raw.hidePrivate as boolean,
    hidePrivateInTimeline: raw.hidePrivate as boolean,
  };
}

// 迁移旧版 readerTheme 旧值（'light'/'dark' -> 'paper'）
function migrateReaderTheme(
  raw: Record<string, unknown>
): { readerTheme: ReaderTheme } | undefined {
  const has = (k: string) => k in raw && raw[k] !== undefined;
  if (!has('readerTheme')) return undefined;
  const oldTheme = raw.readerTheme as string;
  if (oldTheme === 'light' || oldTheme === 'dark') return { readerTheme: 'paper' };
  return undefined;
}

// 旧字段兼容转换
function migrateLegacySettings(raw: Record<string, unknown>): Partial<AppSettings> {
  const migrated: Partial<AppSettings> = {};
  const markMode = migrateMarkReadMode(raw);
  if (markMode) Object.assign(migrated, markMode);
  const hide = migrateHidePrivate(raw);
  if (hide) Object.assign(migrated, hide);
  const theme = migrateReaderTheme(raw);
  if (theme) Object.assign(migrated, theme);
  return migrated;
}

// 从 localStorage 同步加载设置（用于 useState 初始化，保证首屏不闪烁）
export function loadSettings(): AppSettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULT_SETTINGS };
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const migrated = migrateLegacySettings(parsed);
    // 合并：默认值 < 旧数据（去掉已废弃字段）< 迁移后的新字段
    const { markOnScroll, markOnHover, markOnView, hidePrivate, ...rest } = parsed as Record<string, unknown>;
    void markOnScroll; void markOnHover; void markOnView; void hidePrivate;
    // 对每个字段做合法性校验，避免损坏的数据导致 UI 异常
    const coerced: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(rest)) {
      if (key in DEFAULT_SETTINGS && value !== undefined && value !== null) {
        coerced[key] = coerceSettingValue(key as keyof AppSettings, value);
      }
    }
    return { ...DEFAULT_SETTINGS, ...coerced, ...migrated } as AppSettings;
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

// loadSettings 的进程内缓存：列表中大量组件挂载时会反复读取同一份设置，
// 每次都 JSON.parse localStorage 代价过高。saveSettings 与跨标签页 storage 事件会刷新缓存。
let _cachedSettings: AppSettings | null = null;

/** 读取设置（带缓存），适用于高频挂载的展示型组件 */
export function getCachedSettings(): AppSettings {
  if (!_cachedSettings) _cachedSettings = loadSettings();
  return _cachedSettings;
}

if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => {
    if (e.key === STORAGE_KEY || e.key === null) _cachedSettings = null;
  });
}

// 从后端数据库异步加载设置（启动后合并到 state，实现跨设备同步）
// 后端返回空对象时返回 null，由调用方决定是否触发迁移
export async function loadSettingsFromServer(): Promise<AppSettings | null> {
  try {
    const raw = await getServerSettings();
    if (!raw || Object.keys(raw).length === 0) return null;
    return parseServerSettings(raw);
  } catch (err) {
    // 后端不可用时回退到本地设置，属预期路径，不打扰用户
    console.error('Failed to load settings from server:', err);
    return null;
  }
}

// 枚举字段合法值集合，用于校验从 localStorage/后端加载的值
const ENUM_VALIDATORS: Partial<Record<keyof AppSettings, readonly string[]>> = {
  appTheme: ['light', 'dark', 'system'],
  markReadMode: ['none', 'scroll', 'hover', 'view'],
  listDensity: ['compact', 'normal', 'comfortable'],
  listSortOrder: ['newest', 'oldest'],
  listDateFormat: ['relative', 'absolute'],
  readerTheme: ['system', 'paper', 'sepia', 'green'],
  readerFontFamily: ['serif', 'sans', 'mono'],
  readerLetterSpacing: ['normal', 'wide'],
  openArticleMode: ['rss', 'readability', 'iframe', 'browser'],
  closeBehavior: ['quit', 'tray'],
  minimizeBehavior: ['taskbar', 'tray'],
};

/** 校验单个字段值是否合法，不合法则返回 undefined（由调用方回退到默认值） */
function coerceSettingValue(key: keyof AppSettings, value: unknown): unknown {
  const defaultVal = DEFAULT_SETTINGS[key];
  if (typeof defaultVal === 'boolean') {
    return value === 'true' || value === true;
  }
  if (typeof defaultVal === 'number') {
    const n = typeof value === 'number' ? value : Number(value);
    return Number.isFinite(n) ? n : defaultVal;
  }
  // 字符串枚举校验
  const allowed = ENUM_VALIDATORS[key];
  if (allowed) {
    return allowed.includes(value as string) ? value : defaultVal;
  }
  // 非枚举字符串字段（如 accentColor、proxyUrl）直接保留
  return typeof value === 'string' ? value : defaultVal;
}

// 解析后端 string->string 映射为 AppSettings
function parseServerSettings(raw: Record<string, string>): AppSettings {
  const parsed: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(raw)) {
    if (!(key in DEFAULT_SETTINGS)) continue;
    parsed[key] = coerceSettingValue(key as keyof AppSettings, value);
  }
  return { ...DEFAULT_SETTINGS, ...parsed } as AppSettings;
}

// 序列化 AppSettings 为 string->string，供后端存储
function serializeSettings(settings: AppSettings): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(settings)) {
    result[key] = String(value);
  }
  return result;
}

// 保存设置：同步写入 localStorage（UI 不阻塞），异步上传后端
// 保留同步签名以兼容现有调用方；后端写入失败不影响本地体验
export function saveSettings(settings: AppSettings): void {
  _cachedSettings = settings;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch (err) {
    // 隐私模式或配额耗尽时写入会抛错，此时仅依赖后端持久化
    console.error('Failed to persist settings to localStorage:', err);
  }
  void saveSettingsToServer(settings);
}

// 异步写入后端数据库
export async function saveSettingsToServer(settings: AppSettings): Promise<void> {
  try {
    await putServerSettings(serializeSettings(settings));
  } catch (err) {
    // 网络失败静默，本地 localStorage 已是最新
    console.error('Failed to save settings to server:', err);
  }
  // 通知桌面壳刷新窗口行为设置缓存（closeBehavior/minimizeBehavior）
  try {
    getDesktopApp()?.RefreshWindowSettings?.();
  } catch (err) {
    console.error('Failed to refresh desktop window settings:', err);
  }
}

// 一次性迁移：将 localStorage 中的配置上传到后端
// 仅在未标记 MIGRATION_KEY 时执行，避免重复迁移
export async function migrateLegacySettingsIfNeeded(): Promise<void> {
  if (localStorage.getItem(MIGRATION_KEY)) return;
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    localStorage.setItem(MIGRATION_KEY, '1');
    return;
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const migrated = migrateLegacySettings(parsed);
    const { markOnScroll, markOnHover, markOnView, hidePrivate, ...rest } = parsed as Record<string, unknown>;
    void markOnScroll; void markOnHover; void markOnView; void hidePrivate;
    const settings = { ...DEFAULT_SETTINGS, ...rest, ...migrated } as AppSettings;
    await saveSettingsToServer(settings);
    localStorage.setItem(MIGRATION_KEY, '1');
  } catch {
    // 迁移失败下次重试
  }
}

// 阅读器主题选项（只选色彩倾向，明暗由应用主题决定）
export const THEME_OPTIONS: { value: string; label: string }[] = [
  { value: 'system', label: '跟随应用' },
  { value: 'paper', label: '纸白' },
  { value: 'sepia', label: '护眼' },
  { value: 'green', label: '绿意' },
];

// 字间距选项
export type ReaderLetterSpacing = 'normal' | 'wide';

export const LETTER_SPACING_OPTIONS: { value: string; label: string }[] = [
  { value: 'normal', label: '正常' },
  { value: 'wide', label: '宽松' },
];

// 强调色预设
export const ACCENT_PRESETS: { name: string; color: string }[] = [
  { name: '默认紫', color: '#7B68EE' },
  { name: '靛蓝', color: '#6366F1' },
  { name: '天蓝', color: '#3B82F6' },
  { name: '青绿', color: '#14B8A6' },
  { name: '翠绿', color: '#22C55E' },
  { name: '琥珀', color: '#F59E0B' },
  { name: '橙红', color: '#F97316' },
  { name: '玫瑰', color: '#F43F5E' },
  { name: '品红', color: '#EC4899' },
];

// 标记已读选项
export const MARK_READ_OPTIONS: { value: MarkReadMode; label: string }[] = [
  { value: 'none', label: '不自动标记' },
  { value: 'scroll', label: '滚动浏览文章时' },
  { value: 'view', label: '内容进入视图时' },
  { value: 'hover', label: '悬停在文章上时' },
];

// 文章列表密度选项
export const LIST_DENSITY_OPTIONS: { value: ListDensity; label: string }[] = [
  { value: 'compact', label: '紧凑' },
  { value: 'normal', label: '正常' },
  { value: 'comfortable', label: '宽松' },
];

// 排序选项
export const LIST_SORT_OPTIONS: { value: ListSortOrder; label: string }[] = [
  { value: 'newest', label: '最新优先' },
  { value: 'oldest', label: '最旧优先' },
];

// 日期格式选项
export const LIST_DATE_FORMAT_OPTIONS: { value: ListDateFormat; label: string }[] = [
  { value: 'relative', label: '相对时间' },
  { value: 'absolute', label: '绝对时间' },
];

let _rafId = 0;
let _pendingColor: string | null = null;

/** 将待设置的强调色写入 CSS 变量 */
function setAccentColorInline(color: string): void {
  document.documentElement.style.setProperty('--primary', color);
  _pendingColor = null;
}

/** 从 requestAnimationFrame 回调中处理待设置的强调色 */
function tickAccentColor(): void {
  _rafId = 0;
  if (!_pendingColor) return;
  setAccentColorInline(_pendingColor);
}

/**
 * 将强调色应用到 CSS 变量
 * 使用 requestAnimationFrame 节流，避免拖动时频繁重绘
 */
export function applyAccentColor(color: string): void {
  _pendingColor = color;
  if (_rafId) return;
  _rafId = requestAnimationFrame(tickAccentColor);
}

/** 导入配置文件的顶层结构 */
interface ExportedConfig {
  settings?: Record<string, unknown>;
  theme?: string;
  serverSettings?: Record<string, string>;
}

/** 从选中的 JSON 文件中读取并导入配置 */
async function applyImportedConfig(file: File): Promise<boolean> {
  try {
    const text = await file.text();
    const parsed: unknown = JSON.parse(text);
    if (typeof parsed !== 'object' || parsed === null) return false;
    const config = parsed as ExportedConfig;
    if (config.settings && typeof config.settings === 'object') {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(config.settings));
    }
    if (typeof config.theme === 'string') {
      localStorage.setItem('theme', config.theme);
    }
    // 后端设置（备份策略、留存策略、代理等）一并写回，确保配置备份可完整迁移（M-A1）
    if (config.serverSettings && typeof config.serverSettings === 'object') {
      try {
        await putServerSettings(config.serverSettings);
      } catch (err) {
        // 后端写入失败忽略，前端配置已落地
        console.error('Failed to import server settings:', err);
      }
    }
    return true;
  } catch (err) {
    console.error('Failed to import config file:', err);
    return false;
  }
}

/** 窗口重新获得焦点后，等待 change 事件的宽限期 */
const FILE_PICK_GRACE_MS = 800;
/** 用户长时间未操作时释放 Promise，避免调用方永久挂起 */
const FILE_PICK_TIMEOUT_MS = 120000;

/**
 * 弹出文件选择框导入配置。
 * 注意：窗口 focus 事件早于 input 的 change 事件触发，
 * 若在 focus 时立即以「取消」结算 Promise，用户真实选择的文件会被静默丢弃。
 * 因此 focus 只启动宽限计时，超时且确实没有选中文件才判定为取消。
 */
export function importConfig(): Promise<boolean> {
  return new Promise((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.style.display = 'none';

    let settled = false;
    let graceTimer: ReturnType<typeof setTimeout> | null = null;
    let guardTimer: ReturnType<typeof setTimeout> | null = null;

    const clearTimers = () => {
      if (graceTimer) { clearTimeout(graceTimer); graceTimer = null; }
      if (guardTimer) { clearTimeout(guardTimer); guardTimer = null; }
    };

    const finish = (result: boolean) => {
      if (settled) return;
      settled = true;
      clearTimers();
      window.removeEventListener('focus', onFocus);
      input.remove();
      resolve(result);
    };

    const onFocus = () => {
      if (graceTimer) clearTimeout(graceTimer);
      graceTimer = setTimeout(() => {
        if (!input.files || input.files.length === 0) finish(false);
      }, FILE_PICK_GRACE_MS);
    };

    // 现代浏览器在用户点「取消」时会派发 cancel 事件，可立即结算
    input.addEventListener('cancel', () => finish(false));

    input.onchange = async () => {
      // 已选中文件：先停掉取消判定，再执行异步导入，避免宽限期内被误判
      clearTimers();
      window.removeEventListener('focus', onFocus);
      const file = input.files?.[0];
      if (!file) {
        finish(false);
        return;
      }
      finish(await applyImportedConfig(file));
    };

    document.body.appendChild(input);
    window.addEventListener('focus', onFocus);
    guardTimer = setTimeout(() => finish(false), FILE_PICK_TIMEOUT_MS);
    input.click();
  });
}
