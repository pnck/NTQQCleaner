import { useEffect, useState } from "react";
import { api } from "../api";
import { THEME_LABEL, type Theme } from "../theme";
import type { Config } from "../types";

// 设置：主题 + 备份目录 + 高级扫描门控。
// 普通类别门控（clean_*）已不再是用户概念 —— 选什么清理由筛选器决定；
// 高级区只放「默认不覆盖」的 opt-in 类别（传输缓存/日志/头像），勾选后
// 扫描与清理才会覆盖它们（默认关，与 Go 侧 config 共用一份）。
interface Props {
  open: boolean;
  onClose: () => void;
  theme: Theme;
  onThemeChange: (t: Theme) => void;
  config: Config | null;
  onConfigSave: (c: Config) => void;
}

// 高级 opt-in 门控（与后端 Config.cleanLog/cleanDatalineTmp/cleanAvatar 对应）。
// 语义是「扫描与清理的覆盖范围」：默认不覆盖，勾选 = 参与扫描并参与清理。
const ADVANCED: { key: keyof Config; label: string; note: string }[] = [
  { key: "cleanDatalineTmp", label: "传输残留", note: "数据线传输中断留下的未完成文件" },
  { key: "cleanLog", label: "运行日志", note: "QQ 的运行日志，删除后自动重建" },
  { key: "cleanAvatar", label: "头像缓存", note: "好友头像的本地缓存，重新查看时自动下载" },
];

export function SettingsDialog({ open, onClose, theme, onThemeChange, config, onConfigSave }: Props) {
  const [backup, setBackup] = useState("");
  const [advanced, setAdvanced] = useState<Record<string, boolean>>({});

  // 打开时同步后端 config（备份目录 + 高级门控的暂存值，tmp 下跨启动复用）。
  useEffect(() => {
    if (!open || !config) return;
    setBackup(config.backupDir ?? "");
    setAdvanced({
      cleanDatalineTmp: Boolean(config.cleanDatalineTmp),
      cleanLog: Boolean(config.cleanLog),
      cleanAvatar: Boolean(config.cleanAvatar),
    });
  }, [open, config]);

  if (!open) return null;

  const finish = () => {
    if (config) {
      onConfigSave({
        ...config,
        cleanDatalineTmp: Boolean(advanced.cleanDatalineTmp),
        cleanLog: Boolean(advanced.cleanLog),
        cleanAvatar: Boolean(advanced.cleanAvatar),
        backupDir: backup,
      });
    }
    onClose();
  };

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
          <label title="清理时把文件移动到该目录（可恢复）；留空则直接删除，仅写审计日志（路径/大小/时间）">
            备份目录
          </label>
          <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
            {backup || "未设置（删除后不可恢复，仅记审计日志）"}
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
        {/* 扫描/清理范围：默认不覆盖这些类别；勾选 = 参与扫描并参与清理 */}
        <div className="settings-advanced">
          <div className="adv-head">
            <b>高级</b>
            <span style={{ color: "var(--text-dim)" }}>
              以下类别默认不扫描、不清理：勾选启用后才会出现在扫描结果中，删除仍需手动勾选并确认。保存后自动重新扫描
            </span>
          </div>
          {ADVANCED.map((a) => (
            <label key={a.key} className="adv-item">
              <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  type="checkbox"
                  checked={Boolean(advanced[a.key])}
                  onChange={(e) => setAdvanced((prev) => ({ ...prev, [a.key]: e.target.checked }))}
                />
                <span>{a.label}</span>
              </span>
              <span className="adv-note">{a.note}</span>
            </label>
          ))}
        </div>
        <div className="actions">
          <button className="primary" onClick={finish}>
            完成
          </button>
        </div>
      </div>
    </div>
  );
}
