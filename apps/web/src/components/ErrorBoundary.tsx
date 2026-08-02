import { Component, Fragment, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  // 用于在"重置"时强制重新挂载子树，丢弃可能已损坏的内部状态
  resetKey: number;
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, resetKey: 0 };
  }

  // 只更新错误相关字段：resetKey 必须保留，否则再次出错会把重置计数清零，
  // 导致下一次"重置应用"生成重复的 key、子树无法真正重新挂载
  static getDerivedStateFromError(error: Error): Pick<State, 'hasError' | 'error'> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // 仅 console.error，不涉及第三方上报
    console.error('[ErrorBoundary]', error, info.componentStack);
  }

  // 重置错误边界：尝试在不刷新整个页面的前提下恢复子树。
  // 通过递增 resetKey 强制子树重新挂载，丢弃已损坏的状态。
  handleReset = () => {
    this.setState((prev) => ({ hasError: false, error: null, resetKey: prev.resetKey + 1 }));
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex h-screen items-center justify-center bg-canvas p-8">
          <div className="max-w-md text-center">
            <h1 className="mb-2 text-[20px] font-bold text-primary">应用出现异常</h1>
            <p className="mb-4 text-[14px] text-secondary">
              请尝试重置应用状态，若问题持续请刷新页面。
            </p>
            <div className="flex items-center justify-center gap-2">
              <button
                onClick={this.handleReset}
                className="rounded-md bg-primary px-4 py-2 text-[13px] text-on-primary transition-colors hover:opacity-90"
              >
                重置应用
              </button>
              <button
                onClick={() => window.location.reload()}
                className="rounded-md border border-border-subtle px-4 py-2 text-[13px] text-secondary transition-colors hover:bg-hover"
              >
                刷新页面
              </button>
            </div>
            {this.state.error && (
              <details className="mt-4 text-left">
                <summary className="cursor-pointer text-[12px] text-muted">错误详情</summary>
                <pre className="mt-2 max-h-40 overflow-auto rounded bg-hover p-2 text-[11px] text-muted">
                  {this.state.error.message}
                </pre>
              </details>
            )}
          </div>
        </div>
      );
    }

    // Fragment + key：调用 handleReset 后会重新挂载子树，且不引入额外 DOM 节点
    return <Fragment key={this.state.resetKey}>{this.props.children}</Fragment>;
  }
}