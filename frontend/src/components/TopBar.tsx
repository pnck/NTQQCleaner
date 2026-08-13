import type { AccountReport, Progress } from "../types";
import type { Theme } from "../theme";
import { THEME_LABEL } from "../theme";

// 主题图标：内联 SVG（太阳/月亮/自动半圆），不依赖图标库。
function ThemeIcon({ theme }: { theme: Theme }) {
  const common = {
    width: 15,
    height: 15,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 2,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };
  if (theme === "light") {
    return (
      <svg {...common}>
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
      </svg>
    );
  }
  if (theme === "dark") {
    return (
      <svg {...common}>
        <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
      </svg>
    );
  }
  return (
    <svg {...common}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" stroke="none" />
    </svg>
  );
}

interface Props {
  roots: string[];
  root: string;
  accounts: AccountReport[];
  account: string;
  scanning: boolean;
  progress: Progress;
  theme: Theme;
  onRootChange: (root: string) => void;
  onAccountChange: (account: string) => void;
  onScan: () => void;
  onStop: () => void;
  onPickRoot: () => void;
  onOpenSettings: () => void;
  onCycleTheme: () => void;
}

export function TopBar({
  roots,
  root,
  accounts,
  account,
  scanning,
  progress,
  theme,
  onRootChange,
  onAccountChange,
  onScan,
  onStop,
  onPickRoot,
  onOpenSettings,
  onCycleTheme,
}: Props) {
  const pct = progress.total > 0 ? Math.min(100, (progress.done / progress.total) * 100) : 0;
  return (
    <div className="topbar">
      <span style={{ fontWeight: 600 }}>QQ Cleaner</span>
      <select value={root} onChange={(e) => onRootChange(e.target.value)}>
        <option value="">选择数据根…</option>
        {roots.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      <button onClick={onPickRoot} title="浏览选择 QQ 数据根目录">
        浏览…
      </button>
      <select
        value={account}
        onChange={(e) => onAccountChange(e.target.value)}
        disabled={accounts.length === 0}
      >
        <option value="">全部账号</option>
        {accounts.map((a) => (
          <option key={a.hash} value={a.hash}>
            QQ {a.qqNum || "unknown"}（{a.latestMonth || "无活动"}）
          </option>
        ))}
      </select>
      {scanning ? (
        <>
          <button onClick={onStop}>停止</button>
          <div className="progress-track">
            <div className="progress-fill" style={{ width: `${pct}%` }} />
          </div>
          <span className="stage">
            {progress.stage} · {progress.done}
            {progress.total > 0 ? ` / ${progress.total}` : ""}
          </span>
        </>
      ) : (
        <>
          <button className="primary" onClick={onScan} disabled={!root}>
            扫描
          </button>
        </>
      )}
      <div className="grow" />
      <button
        className="icon-btn"
        onClick={onCycleTheme}
        title={`主题：${THEME_LABEL[theme]}（点击切换）`}
      >
        <ThemeIcon theme={theme} />
      </button>
      <button onClick={onOpenSettings}>设置</button>
    </div>
  );
}
