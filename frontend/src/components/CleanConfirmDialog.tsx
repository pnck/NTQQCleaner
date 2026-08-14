import { useState } from "react";
import { fmtSize } from "../types";

// 清理确认对话框（点底栏「清理」弹出）：收集两个显式 opt-in——
// ① 生成审计记录（默认关，docs/06 §3：十万级文件的逐文件清单过长）
// ② 以移动代替删除（默认关；未设备份目录时拒绝确认并引导去设置）。
// 最终确认仍在 Go 侧原生对话框（红线：确认在 Go 侧，前端不可信）。
interface Props {
  open: boolean;
  count: number;
  bytes: number;
  hasBackup: boolean;
  onConfirm: (audit: boolean, move: boolean) => void;
  onCancel: () => void;
  onOpenSettings: () => void;
}

export function CleanConfirmDialog({
  open,
  count,
  bytes,
  hasBackup,
  onConfirm,
  onCancel,
  onOpenSettings,
}: Props) {
  const [audit, setAudit] = useState(false);
  const [move, setMove] = useState(false);
  if (!open) return null;
  const blocked = move && !hasBackup;

  return (
    <div className="dialog-mask" onClick={onCancel}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <h2>清理确认</h2>
        <div style={{ fontSize: 12 }}>
          将清理 <b>{count}</b> 个文件 · 可释放 <b>{fmtSize(bytes)}</b>
        </div>
        <label className="confirm-opt">
          <input type="checkbox" checked={audit} onChange={(e) => setAudit(e.target.checked)} />
          <span>
            <b>生成审计记录</b>
            <span className="confirm-note">
              逐文件 JSONL 清单（系统临时目录，完成后自动打开）。文件量大时生成耗时，默认关闭。
            </span>
          </span>
        </label>
        <label className="confirm-opt">
          <input type="checkbox" checked={move} onChange={(e) => setMove(e.target.checked)} />
          <span>
            <b>以移动代替删除</b>
            <span className="confirm-note">文件移动到备份目录，可恢复；不勾选则直接删除。</span>
          </span>
        </label>
        {blocked && (
          <div className="confirm-warn">
            <span className="confirm-warn-text">尚未设置备份目录，请先到设置中指定。</span>
            <button className="mini" onClick={onOpenSettings}>
              去设置
            </button>
          </div>
        )}
        <div className="actions">
          <button onClick={onCancel}>取消</button>
          <button className="primary" disabled={blocked} onClick={() => onConfirm(audit, move)}>
            确认清理
          </button>
        </div>
      </div>
    </div>
  );
}
