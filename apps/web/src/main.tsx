import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import { initApiBase } from './utils/api';

// 桌面端：等待 Wails 注入 window.go 并获取后端动态端口；
// Web 端：isDesktop() 返回 false，函数立即返回，不影响默认地址。
// 无论 initApiBase 成功或失败都应渲染应用：失败时回退到默认 API 地址，
// 由 ErrorBoundary 兜底后续的网络错误，避免页面永久白屏。
const rootEl = document.getElementById('root');
if (!rootEl) {
  // 极端情况下 root 容器缺失，直接抛错比静默失败更易排查
  throw new Error('Root container #root not found in document');
}

initApiBase()
  .catch((err) => {
    // 桌面端注入失败不阻塞渲染：App 内部会按默认地址尝试连接
    console.error('[main] initApiBase failed, falling back to default API base', err);
  })
  .finally(() => {
    createRoot(rootEl).render(
      <StrictMode>
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
      </StrictMode>
    );
  });