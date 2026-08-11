import { useState } from 'react';
import { Rss, Plus, Upload, Globe, Check, Loader2 } from './icons';
import { RECOMMENDED_SOURCES, type RecommendedSource } from '../data/recommended-sources';
import { createSource } from '../utils/api';
import { showToast } from '../utils/toast';

interface Props {
  onImport: () => void;
  onAddSource: () => void;
  onSourcesChanged: () => void;
}

export default function OnboardingWelcome({ onImport, onAddSource, onSourcesChanged }: Props) {
  const [subscribing, setSubscribing] = useState<string | null>(null);

  const handleSubscribe = async (item: RecommendedSource) => {
    if (subscribing) return;
    setSubscribing(item.url);
    try {
      await createSource({
        name: item.name,
        url: item.url,
        folderId: null,
        interval: 30,
        listRule: 'show',
      });
      showToast('已订阅「' + item.name + '」');
      onSourcesChanged();
    } catch (err) {
      console.error('Failed to subscribe:', err);
      showToast('订阅失败，请检查链接或稍后重试');
    } finally {
      setSubscribing(null);
    }
  };

  return (
    <div className="flex h-full w-full items-center justify-center bg-canvas p-8">
      <div className="flex max-w-2xl flex-col items-center gap-10">
        {/* 顶部品牌区 */}
        <div className="flex flex-col items-center gap-4">
          <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Rss size={32} />
          </div>
          <h1 className="text-[22px] font-bold text-primary">欢迎使用 Flore</h1>
          <p className="max-w-md text-center text-[14px] leading-relaxed text-muted">
            一款本地优先的 RSS 阅读器。你还没有订阅任何源，选一种方式开始吧。
          </p>
        </div>

        {/* 三个入口卡片 */}
        <div className="grid w-full grid-cols-1 gap-4 sm:grid-cols-3">
          <button
            onClick={onAddSource}
            className="group flex flex-col items-center gap-3 rounded-xl border border-border-subtle bg-surface p-6 text-center transition-all hover:border-primary/30 hover:shadow-sm"
          >
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Plus size={20} />
            </div>
            <span className="text-[14px] font-semibold text-primary">手动添加</span>
            <span className="text-[12px] text-muted">输入订阅源地址，从零开始</span>
          </button>

          <button
            onClick={onImport}
            className="group flex flex-col items-center gap-3 rounded-xl border border-border-subtle bg-surface p-6 text-center transition-all hover:border-primary/30 hover:shadow-sm"
          >
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Upload size={20} />
            </div>
            <span className="text-[14px] font-semibold text-primary">导入 OPML</span>
            <span className="text-[12px] text-muted">从其他阅读器导入订阅列表</span>
          </button>

          <div className="flex flex-col items-center gap-3 rounded-xl border border-border-subtle bg-surface p-6 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Globe size={20} />
            </div>
            <span className="text-[14px] font-semibold text-primary">浏览推荐</span>
            <span className="text-[12px] text-muted">从内置精选列表一键订阅</span>
          </div>
        </div>

        {/* 推荐源列表 */}
        <div className="w-full">
          <h2 className="mb-3 text-[13px] font-semibold text-muted uppercase tracking-wider">
            推荐订阅源
          </h2>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {RECOMMENDED_SOURCES.map((item) => (
              <div
                key={item.url}
                className="flex items-center justify-between rounded-lg border border-border-subtle bg-surface px-4 py-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[13px] font-medium text-primary">
                    {item.name}
                  </div>
                  <div className="truncate text-[11px] text-muted">{item.hint}</div>
                </div>
                <button
                  onClick={() => handleSubscribe(item)}
                  disabled={subscribing === item.url}
                  className="ml-3 flex shrink-0 items-center gap-1 rounded-md bg-primary/10 px-3 py-1.5 text-[12px] font-medium text-primary transition-colors hover:bg-primary/20 disabled:opacity-50"
                >
                  {subscribing === item.url ? (
                    <Loader2 size={12} className="animate-spin" />
                  ) : (
                    <Plus size={12} />
                  )}
                  <span>订阅</span>
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* 页脚提示 */}
        <p className="text-[12px] text-muted">
          已有订阅源？从侧边栏右上角 + 号或设置页导入 OPML 文件。
        </p>
      </div>
    </div>
  );
}