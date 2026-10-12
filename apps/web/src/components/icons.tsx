/**
 * Flore 图标系统
 * 基于开源 Lucide 图标库（MIT 许可证）
 * https://lucide.dev
 *
 * 统一通过 lucide-react 引入，保持 16px 默认尺寸、1.5px 描边、圆角端点，
 * 与项目整体低视觉噪音风格一致。
 *
 * 命名规范：
 * - 直接 re-export 使用原名（如 Star、Settings）
 * - 需要区分语义时用显式别名（如 StarIcon 表示未填充星标、StarFilledIcon 表示已填充）
 * - 不为「可能以后用到」的图标预留导出：未被引用的图标按需删除，需要时再补
 */

export {
  Settings,
  Plus,
  RefreshCw,
  RefreshCw as RestartIcon,
  Star,
  Circle,
  Check,
  Keyboard,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  FileText,
  Upload,
  Folder,
  FolderInput,
  BookOpen,
  EyeOff,
  MailOpen,
  Sun,
  Moon,
  Trash2,
  Pencil,
  Loader2,
  Globe,
  Rss,
  Inbox,
  CheckCheck,
  X,
  Search,
  SearchX,
  Clock,
  Type,
  Download,
  Square,
  CheckSquare,
  FileArchive,
  FileJson,
  ArrowLeft,
  Menu,
  AlertTriangle,
  Minus,
  ArchiveRestore,
  Copy,
  Share2,
  MoreHorizontal,
  Info,
  Monitor,
  Filter,
  Image,
  FileDown,
  Database,
  Palette,
  Bell,
  RotateCw,
  Cog,
  Pause,
  PauseCircle,
} from 'lucide-react';

import type { LucideProps } from 'lucide-react';
import { Star } from 'lucide-react';

export type IconProps = LucideProps;

// 语义别名：仅保留有明确语义差异的星标（描边 = 未收藏，填充 = 已收藏）
export const StarIcon = (props: IconProps) => <Star {...props} strokeWidth={1.5} />;

export const StarFilledIcon = (props: IconProps) => (
  <Star {...props} fill="currentColor" strokeWidth={1.5} />
);

// 组件内引用的稳定别名（原名在 lucide-react 升级时可能变化，此处收口）
export {
  Settings as SettingsIcon,
  Upload as UploadIcon,
  Folder as FolderIcon,
  Rss as RssIcon,
  Download as DownloadIcon,
  Menu as MenuIcon,
  FolderOpen as FolderOpenIcon,
  Minus as MinusIcon,
  Maximize as MaximizeIcon,
  Copy as CopyIcon,
} from 'lucide-react';
