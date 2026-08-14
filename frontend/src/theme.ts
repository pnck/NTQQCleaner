// 主题：auto（跟随系统）/ light / dark。
// CSS 侧：styles.css 用 [data-theme="dark"] 覆盖暗色变量，
// auto 模式由 @media (prefers-color-scheme: dark) 接管。

export type Theme = "auto" | "light" | "dark";

const KEY = "ntqq-cleaner-theme";

export function getTheme(): Theme {
  const t = localStorage.getItem(KEY);
  return t === "light" || t === "dark" || t === "auto" ? t : "auto";
}

export function applyTheme(t: Theme) {
  document.documentElement.dataset.theme = t;
  localStorage.setItem(KEY, t);
}

export const THEME_LABEL: Record<Theme, string> = {
  auto: "跟随系统",
  light: "浅色",
  dark: "深色",
};

export const nextTheme = (t: Theme): Theme =>
  t === "auto" ? "light" : t === "light" ? "dark" : "auto";
