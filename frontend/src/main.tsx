import { createRoot } from "react-dom/client";
import App from "./App";
import { applyTheme, getTheme } from "./theme";
import "./styles.css";

// 在 React 挂载前应用持久化主题，避免闪烁。
applyTheme(getTheme());

// 把未捕获的运行时错误直接显示在屏幕上：黑屏排查神器，
// 生产环境同样保留（用户遇到问题时可拍照反馈）。
function showOverlay(msg: string) {
  const el = document.createElement("div");
  el.style.cssText =
    "position:fixed;left:0;right:0;bottom:0;background:#7f1d1d;color:#fff;" +
    "font:12px monospace;padding:10px;z-index:99999;white-space:pre-wrap;";
  el.textContent = msg;
  document.body.appendChild(el);
}
window.addEventListener("error", (e) => {
  showOverlay(`JS 错误: ${e.message}\n${e.filename}:${e.lineno}:${e.colno}`);
});
window.addEventListener("unhandledrejection", (e) => {
  showOverlay(`未处理的 Promise 拒绝: ${String(e.reason)}`);
});

createRoot(document.getElementById("root")!).render(<App />);
