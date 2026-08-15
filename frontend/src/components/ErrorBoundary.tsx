import { Component, type ErrorInfo, type ReactNode } from "react";

// ErrorBoundary：渲染兜底——任何 React 渲染异常都**不能**把整个 DOM 打没
// （React 未捕获的渲染错误会卸载整棵树 → 白屏，比报错本身更糟）。捕获后
// 显示红色错误条：错误信息可选中复制（含组件栈），并提供重新加载。
// 异步错误（事件/promise）不走渲染边界，由 main.tsx 的 window error/
// unhandledrejection 底部红条兜底。
interface State {
  error: Error | null;
  info: string;
}

export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null, info: "" };

  static getDerivedStateFromError(error: Error): State {
    return { error, info: "" };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("ErrorBoundary:", error, info);
    this.setState({ info: info.componentStack ?? "" });
  }

  render() {
    if (!this.state.error) return this.props.children;
    const msg = String(this.state.error?.message ?? this.state.error);
    return (
      <div className="fatal-bar">
        <b>界面发生错误——已拦截兜底，页面其余部分可能无法继续使用。</b>
        <div className="fatal-msg">
          {msg}
          {this.state.info ? `\n\n${this.state.info.trim()}` : ""}
        </div>
        <div>
          <button onClick={() => window.location.reload()}>重新加载界面</button>
        </div>
      </div>
    );
  }
}
