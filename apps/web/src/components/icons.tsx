/**
 * Typestream 图标系统
 * 基于开源 Lucide 图标库（MIT 许可证）
 * https://lucide.dev
 *
 * 统一通过 lucide-react 引入，保持 16px 默认尺寸、
 * 1.5px 描边、圆角端点，与项目整体低视觉噪音风格一致。
 *
 * 命名规范：
 * - 直接 re-export 使用原名（如 Star、Settings）
 * - 业务语义版本使用后缀 Icon（如 StarIcon 表示未填充星标）
 * - 别名仅保留语义差异显著的情况（如 FolderOpen → FolderOpenIcon）
 */

export {
  Settings,
  Plus,
  RefreshCw,
  Star,
  Circle,
  Dot,
  Check,
  Keyboard,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  ArrowUpRight,
  FileText,
  Upload,
  Folder,
  BookOpen,
  Eye,
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
  ListFilter,
  CheckCheck,
  X,
  Search,
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
  AlertCircle,
  FolderOpen,
  Minus,
  Maximize,
  Copy,
  Share2,
  MoreHorizontal,
  Info,
  Monitor,
  Filter,
  Image,
  FileDown,
  Database,
  Trash,
  Archive,
  FolderSync,
  Palette,
  Bell,
  RotateCw,
} from 'lucide-react';

import type { LucideProps } from 'lucide-react';
import { Star } from 'lucide-react';

export type IconProps = LucideProps;

// 业务语义别名：保留与原项目命名兼容
// 收藏星标：未填充（描边）
export const StarIcon = (props: IconProps) => <Star {...props} strokeWidth={1.5} />;

// 收藏星标：已填充
export const StarFilledIcon = (props: IconProps) => (
  <Star {...props} fill="currentColor" strokeWidth={1.5} />
);

// 其他 *Icon 别名：统一通过 re-export 简化，避免与原名重复定义
export {
  Settings as SettingsIcon,
  Plus as PlusIcon,
  RefreshCw as RefreshIcon,
  Circle as CircleIcon,
  Dot as DotIcon,
  Check as CheckIcon,
  ChevronDown as ChevronDownIcon,
  ChevronRight as ChevronRightIcon,
  ExternalLink as ExternalLinkIcon,
  FileText as DocumentIcon,
  Upload as UploadIcon,
  Folder as FolderIcon,
  BookOpen as ReadabilityIcon,
  Eye as EyeIcon,
  EyeOff as EyeOffIcon,
  MailOpen as MailOpenIcon,
  Sun as SunIcon,
  Moon as MoonIcon,
  Trash2 as TrashIcon,
  Pencil as PencilIcon,
  Loader2 as LoaderIcon,
  Globe as GlobeIcon,
  Rss as RssIcon,
  Inbox as InboxIcon,
  ListFilter as FilterIcon,
  CheckCheck as CheckAllIcon,
  Type as TypeIcon,
  Download as DownloadIcon,
  Square as SquareIcon,
  CheckSquare as CheckSquareIcon,
  ArrowLeft as ArrowLeftIcon,
  Menu as MenuIcon,
  FolderOpen as FolderOpenIcon,
  Minus as MinusIcon,
  Maximize as MaximizeIcon,
  Copy as CopyIcon,
} from 'lucide-react';
