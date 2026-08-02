import { useEffect, useState } from 'react';
import { getVersion } from '../../utils/api';
import { Section } from './SettingsShared';

export default function SettingsAboutTab() {
  const [version, setVersion] = useState('...');

  useEffect(() => {
    let cancelled = false;
    // 版本号获取失败仅降级展示"未知"，不打扰用户
    getVersion()
      .then((data) => { if (!cancelled) setVersion(data.version || '未知'); })
      .catch((err) => {
        console.error('Failed to fetch version:', err);
        if (!cancelled) setVersion('未知');
      });
    return () => { cancelled = true; };
  }, []);

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Section title="版本">
        <div className="flex items-center justify-between">
          <span className="text-[15px] font-medium text-primary">Flore v{version}</span>
        </div>
      </Section>

      <Section title="关于">
        <p className="m-0 text-[14px] leading-relaxed text-secondary">
          本地优先的 RSS 阅读器，支持订阅源管理、文件夹分组、阅读模式、OPML 导入导出等功能。
        </p>
      </Section>
    </div>
  );
}
