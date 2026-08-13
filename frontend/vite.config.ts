import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 34115,
    strictPort: true,
  },
  build: {
    // Safari 14（macOS 11 Big Sur）底线：vite 默认 target 需要 Safari 16+，
    // 在较旧的 WKWebView 上会导致 JS 模块加载失败（黑屏）。
    target: "safari14",
  },
});
