import { createRoot } from "react-dom/client";
import App from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { applyTheme } from "./theme";
import "./styles.css";

// WebView 自带存储（localStorage/sessionStorage）整体弃用并清扫：这类
// 「浏览器数据」落在 WebView profile 目录（Windows 为 EBWebView），app
// 退出时无法可靠清理——所有持久化都走 Go 侧 config.yaml（主题/命名
// 筛选器/面板布局，见 App.tsx mergeConfig）。启动时清一次：旧版本的
// 遗留数据一并清除；App 的 beforeunload 是退出时的尽力清扫。
localStorage.clear();
sessionStorage.clear();

// config 到达前先按 auto 渲染，避免闪烁；App 播种配置后应用持久化主题。
applyTheme("auto");

// Windows 滚动条策略（用户指定，前端 UA 判定）：除照片墙外一律完全
// 隐藏（styles.css 的 html.windows 规则——Windows 的 Fluent 滚动条
// 常驻且不理会样式化，全隐藏是唯一干净的方案；照片墙保留滚动条供
// 虚拟滚动拖拽）。其它平台保持原生行为（macOS 原生覆盖式滚动条）。
if (navigator.userAgent.includes("Windows")) {
  document.documentElement.classList.add("windows");
}

// 把未捕获的运行时错误直接显示在屏幕上：黑屏排查神器，
// 生产环境同样保留（用户遇到问题时可拍照反馈）。渲染期异常由
// ErrorBoundary 兜底（红条 + 可选中复制 + 重新加载），这里是异步
// 错误（事件/promise）的补充通道。
function showOverlay(msg: string) {
  const el = document.createElement("div");
  el.style.cssText =
    "position:fixed;left:0;right:0;bottom:0;background:#7f1d1d;color:#fff;" +
    "font:12px monospace;padding:10px;z-index:99999;white-space:pre-wrap;" +
    "user-select:text;";
  el.textContent = msg;
  document.body.appendChild(el);
}
window.addEventListener("error", (e) => {
  showOverlay(`JS 错误: ${e.message}\n${e.filename}:${e.lineno}:${e.colno}`);
});
window.addEventListener("unhandledrejection", (e) => {
  showOverlay(`未处理的 Promise 拒绝: ${String(e.reason)}`);
});

createRoot(document.getElementById("root")!).render(
  <ErrorBoundary>
    <App />
  </ErrorBoundary>,
);
