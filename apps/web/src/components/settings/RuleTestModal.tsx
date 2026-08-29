import { Search, ExternalLink } from '../icons';
import type { Item } from '../../types';
import ModalLayout from '../ModalLayout';

// isSafeLink 校验 URL 协议为 http/https，防止 javascript:/data: XSS
function isSafeLink(url: string): boolean {
  try {
    const u = new URL(url);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
}

// 规则测试结果单行
function TestResultItem({ item }: { item: Item }) {
  const date = item.pubDate ? new Date(item.pubDate) : null;
  const dateStr = date && !isNaN(date.getTime())
    ? date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
    : '未知时间';
  const safe = isSafeLink(item.link);
  return (
    <a
      href={safe ? item.link : undefined}
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-start gap-2.5 rounded-md border border-border-subtle px-3 py-2 transition-colors hover:border-border-strong hover:bg-hover"
      onClick={(e) => { if (!safe) e.preventDefault(); }}
    >
      <ExternalLink size={13} className="mt-0.5 shrink-0 text-muted" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] font-medium text-primary">{item.title}</div>
        <div className="mt-0.5 flex items-center gap-2 text-[11.5px] text-muted">
          <span className="truncate">{item.sourceName}</span>
          <span>·</span>
          <span className="shrink-0">{dateStr}</span>
        </div>
      </div>
    </a>
  );
}

export interface RuleTestResult {
  ruleName: string;
  items: Item[];
}

interface Props {
  result: RuleTestResult | null;
  onClose: () => void;
}

// 规则测试结果弹窗
export default function RuleTestModal({ result, onClose }: Props) {
  if (!result) return null;
  return (
    <ModalLayout
      title={`规则测试：${result.ruleName}`}
      titleIcon={<Search size={16} />}
      onClose={onClose}
      width={560}
    >
      <div className="px-5 pt-4 pb-5">
        <div className="mb-3 text-[12.5px] text-muted">
          在最近 200 篇文章中，命中 {result.items.length} 篇
          {result.items.length >= 20 && '（仅显示前 20 条）'}
        </div>
        {result.items.length === 0 ? (
          <div className="py-8 text-center text-[13px] text-muted">未匹配到文章</div>
        ) : (
          <div className="flex max-h-[50vh] flex-col gap-1.5 overflow-y-auto">
            {result.items.map((item) => (
              <TestResultItem key={item.id} item={item} />
            ))}
          </div>
        )}
      </div>
    </ModalLayout>
  );
}