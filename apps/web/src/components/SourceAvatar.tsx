import React, { useEffect, useState } from 'react';
import { cn } from '../lib/cn';
import { sourceColorAuto } from '../utils/sourceColor';
import { loadSettings } from '../utils/settings';

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
  // 缓存 loadOnlineAvatar 到 state，避免每次渲染都读 localStorage
  // 挂载时读取一次即可，设置变更会在应用整体重新渲染时通过重新挂载生效
  const [allowOnline] = useState(() => loadSettings().loadOnlineAvatar);
  // url 变化时重置 imgError，避免切换源后仍显示字母头像
  useEffect(() => {
    setImgError(false);
  }, [url]);
  const letter = name.trim().charAt(0).toUpperCase() || '?';
  const background = color ?? sourceColorAuto(name);
  const domain = getDomain(url);

  // 尝试加载站点图标，失败则回退到字母头像
  if (domain && !imgError && allowOnline) {
    return (
      <span
        className={cn('inline-flex shrink-0 items-center justify-center rounded', className)}
        style={{ width: size, height: size }}
        aria-hidden="true"
        title={name}
      >
        <img
          src={`https://icons.duckduckgo.com/ip3/${domain}.ico`}
          alt=""
          width={size}
          height={size}
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