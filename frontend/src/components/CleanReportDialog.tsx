import type { CleanResult } from "../types";
import { fmtSize } from "../types";

// 清理结果对话框：清理完成后自动弹出。展示统计 + 跳过/失败明细
// （十万级文件的逐文件列表过长，完整清单仅在启用审计时落盘，
// docs/07 §5；审计报告仍是权威记录）。
const ACTIONS: Record<string, { label: string; cls: string }> = {
  skip: { label: "跳过", cls: "dim" },
  fail: { label: "失败", cls: "err" },
};

const ORDER = ["skip", "fail"];

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
        {res.auditPath ? (
          <div style={{ fontSize: 11, color: "var(--text-dim)" }} title={res.auditPath}>
            审计报告已生成并打开（系统临时目录，可另存；路径见悬浮提示）
          </div>
        ) : (
          <div style={{ fontSize: 11, color: "var(--text-dim)" }}>
            未生成审计报告（本次未勾选「生成审计记录」）
          </div>
        )}
        <div className="clean-report">
          {items.length === 0 && (
            <div className="cond-empty">没有跳过或失败的文件。</div>
          )}
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
