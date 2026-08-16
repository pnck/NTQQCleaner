// 主题：auto（跟随系统）/ light / dark。
// CSS 侧：styles.css 用 [data-theme="dark"] 覆盖暗色变量，
// auto 模式由 @media (prefers-color-scheme: dark) 接管。
//
// 持久化在 Go 侧 config.yaml（App 经 mergeConfig 写回）——WebView 自带
// 存储已弃用（localStorage 落在 WebView profile 目录，清理困难）。

export type Theme = "auto" | "light" | "dark";

export function validTheme(v: unknown): Theme {
  return v === "light" || v === "dark" || v === "auto" ? v : "auto";
}

export function applyTheme(t: Theme) {
  document.documentElement.dataset.theme = t;
}

export const THEME_LABEL: Record<Theme, string> = {
  auto: "跟随系统",
  light: "浅色",
  dark: "深色",
};

export const nextTheme = (t: Theme): Theme =>
  t === "auto" ? "light" : t === "light" ? "dark" : "auto";
