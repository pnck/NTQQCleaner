import type { CleanResult } from "../types";
import { fmtSize } from "../types";

// 清理结果对话框：清理完成后自动弹出，逐文件回显本次动作
// （移动到备份/已删除/跳过/失败 + 原因）。审计日志仍是权威记录。
const ACTIONS: Record<string, { label: string; cls: string }> = {
  move: { label: "移动到备份", cls: "ok" },
  remove: { label: "已删除", cls: "warn" },
  skip: { label: "跳过", cls: "dim" },
  fail: { label: "失败", cls: "err" },
};

const ORDER = ["move", "remove", "skip", "fail"];

interface Props {
  res: CleanResult;
  onClose: () => void;
}

export function CleanReportDialog({ res, onClose }: Props) {
  const items = [...res.items].sort(
    (a, b) => ORDER.indexOf(a.action) - ORDER.indexOf(b.action),
  );
  return (
    <div className="dialog-mask" onClick={onClose}>
      <div className="dialog dialog-wide" onClick={(e) => e.stopPropagation()}>
        <h2>清理结果</h2>
        <div style={{ fontSize: 12, color: "var(--text-dim)" }}>
          处理 {res.processed}：移动到备份 {res.moved} · 已删除 {res.deleted} · 跳过{" "}
          {res.skipped} · 失败 {res.failed} · 释放 {fmtSize(res.bytesFreed)}
        </div>
        <div className="clean-report">
          {items.length === 0 && <div className="cond-empty">无逐文件记录</div>}
          {items.map((it, i) => {
            const a = ACTIONS[it.action] ?? { label: it.action, cls: "dim" };
            return (
              <div key={i} className="clean-item">
                <span className={`clean-badge ${a.cls}`}>{a.label}</span>
                <span className="clean-path" title={it.backupPath ? `${it.path} → ${it.backupPath}` : it.path}>
                  {it.path}
                </span>
                <span className="clean-size">{fmtSize(it.size)}</span>
                {it.reason && <span className="clean-reason">{it.reason}</span>}
              </div>
            );
          })}
        </div>
        <div className="actions">
          <button className="primary" onClick={onClose}>
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}
