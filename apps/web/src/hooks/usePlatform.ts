import { useEffect, useState } from 'react';
import { getPlatform } from '../utils/api';

// 平台标识：'windows' | 'darwin' | 'linux' | ''（Web 或未就绪）
export type Platform = 'windows' | 'darwin' | 'linux' | '';

/**
 * 统一平台判断入口。组件挂载时平台未就绪（ready=false），
 * 依赖平台的渲染必须先判 ready，避免错版式闪烁。
 */
export function usePlatform() {
  const [platform, setPlatform] = useState<Platform>('');
  useEffect(() => {
    let cancelled = false;
    getPlatform()
      .then((p) => {
        if (!cancelled) setPlatform((p as Platform) || '');
      })
      .catch(() => {
        if (!cancelled) setPlatform('');
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return {
    platform,
    isMac: platform === 'darwin',
    isWindows: platform === 'windows',
    isLinux: platform === 'linux',
    ready: platform !== '',
  };
}
