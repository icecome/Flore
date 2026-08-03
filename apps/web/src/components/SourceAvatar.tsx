import React, { useEffect, useState } from 'react';
import { cn } from '../lib/cn';
import { sourceColorAuto } from '../utils/sourceColor';
import { getCachedSettings } from '../utils/settings';
import { getFaviconProxyBase, getFaviconDirectBase } from '../utils/api';

interface Props {
  name: string;
  url?: string;
  color?: string;
  size?: number;
  className?: string;
}

function getDomain(url?: string): string | null {
  if (!url) return null;
  try {
    return new URL(url).hostname;
  } catch {
    return null;
  }
}

function SourceAvatarBase({ name, url, color, size = 20, className }: Props) {
  const [imgError, setImgError] = useState(false);
  // 读取带缓存的设置，避免列表中每个头像挂载都重新 JSON.parse localStorage
  const [faviconMode] = useState(() => {
    const s = getCachedSettings();
    return s.faviconMode ?? 'off';
  });
  // url 变化时重置 imgError，避免切换源后仍显示字母头像
  useEffect(() => {
    setImgError(false);
  }, [url]);
  const letter = name.trim().charAt(0).toUpperCase() || '?';
  const background = color ?? sourceColorAuto(name);
  const domain = getDomain(url);

  // 尝试加载站点图标，失败则回退到字母头像
  if (domain && faviconMode !== 'off' && !imgError) {
    const isDirect = faviconMode === 'direct';
    const src = isDirect
      ? `${getFaviconDirectBase()}?domain=${encodeURIComponent(domain)}`
      : `${getFaviconProxyBase()}?domain=${encodeURIComponent(domain)}`;
    return (
      <span
        className={cn('inline-flex shrink-0 items-center justify-center rounded', className)}
        style={{ width: size, height: size }}
        aria-hidden="true"
        title={name}
      >
        <img
          src={src}
          alt=""
          width={size}
          height={size}
          loading="lazy"
          decoding="async"
          className="rounded"
          style={{ objectFit: 'contain' }}
          onError={() => setImgError(true)}
        />
      </span>
    );
  }

  return (
    <span
      className={cn('inline-flex shrink-0 items-center justify-center rounded font-semibold text-on-primary', className)}
      style={{
        width: size,
        height: size,
        background,
        fontSize: size * 0.5,
        lineHeight: 1,
      }}
      aria-hidden="true"
      title={name}
    >
      {letter}
    </span>
  );
}

const SourceAvatar = React.memo(SourceAvatarBase);
export default SourceAvatar;