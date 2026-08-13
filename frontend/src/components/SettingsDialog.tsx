import { useState } from "react";
import { api } from "../api";
import { THEME_LABEL, type Theme } from "../theme";

// 设置：主题 + 备份目录。
// 分类门控（clean_*）已不再是用户概念 —— 选什么清理由筛选器决定，
// Go 侧只保留结构性红线（黑名单/白名单/进程保护/审计）。
interface Props {
  open: boolean;
  onClose: () => void;
  theme: Theme;
  onThemeChange: (t: Theme) => void;
}

export function SettingsDialog({ open, onClose, theme, onThemeChange }: Props) {
  const [backup, setBackup] = useState(getBackupDir());

  if (!open) return null;

  return (
    <div className="dialog-mask" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2>设置</h2>
        <div className="row">
          <label>主题</label>
          <span style={{ display: "flex", gap: 10 }}>
            {(["auto", "light", "dark"] as Theme[]).map((t) => (
              <label
                key={t}
                style={{ display: "flex", alignItems: "center", gap: 4, color: "var(--text)" }}
              >
                <input
                  type="radio"
                  name="theme"
                  checked={theme === t}
                  onChange={() => onThemeChange(t)}
                />
                {THEME_LABEL[t]}
              </label>
            ))}
          </span>
        </div>
        <div className="row">
          <label title="清理时把文件移动到该目录（可恢复）；留空则先计算 SHA-256 再删除">
            备份目录
          </label>
          <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
            {backup || "未设置（SHA-256 审计）"}
          </span>
          <button
            onClick={() =>
              void api.pickDirectory("选择备份目录").then((d) => {
                if (d) setBackup(d);
              })
            }
          >
            选择…
          </button>
        </div>
        <div className="actions">
          <button
            className="primary"
            onClick={() => {
              setBackupDir(backup);
              onClose();
            }}
          >
            完成
          </button>
        </div>
      </div>
    </div>
  );
}

// 备份目录暂存于模块级（进入 clean 请求时读取）。
let backupDir = "";
export function getBackupDir() {
  return backupDir;
}
export function setBackupDir(d: string) {
  backupDir = d;
}
