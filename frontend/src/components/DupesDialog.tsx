import type { DupGroup } from "../types";
import { fmtSize, fmtTime } from "../types";

// 去重建议：同一 md5 在多处各存了一份时，每 md5 只留一份
// （原图优先，其次最新），其余副本可勾选清理。
interface Props {
  open: boolean;
  groups: DupGroup[];
  onSelectGroup: (ids: number[]) => void;
  onSelectAll: (ids: number[]) => void;
  onClose: () => void;
}

export function DupesDialog({ open, groups, onSelectGroup, onSelectAll, onClose }: Props) {
  if (!open) return null;
  const allDupIDs = groups.flatMap((g) => g.dupIds);
  const totalBytes = groups.reduce((a, g) => a + g.dupBytes, 0);

  return (
    <div className="dialog-mask" onClick={onClose}>
      <div className="dialog dialog-wide" onClick={(e) => e.stopPropagation()}>
        <h2>去重建议</h2>
        <div style={{ color: "var(--text-dim)", fontSize: 12 }}>
          同一内容（同 md5）被 QQ 存在多个目录里。每组保留一份（原图优先，其次最新），
          其余副本可勾选后走正常清理流程（备份/审计红线不变）。
          当前筛选内共 {allDupIDs.length} 个多余副本，可释放 {fmtSize(totalBytes)}。
        </div>
        <div className="filter-list">
          {groups.length === 0 && <div className="cond-empty">当前筛选内没有重复项。</div>}
          {groups.map((g) => (
            <div key={g.md5} className="dupe-row">
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: "ui-monospace, monospace", fontSize: 11 }}>
                  {g.md5.slice(0, 16)}…（共 {g.count} 份）
                </div>
                <div style={{ color: "var(--text-dim)", fontSize: 11 }}>
                  保留：{g.keepLabel}（{fmtTime(g.keepMtime)}）
                </div>
                <div style={{ fontSize: 11 }}>
                  当前筛选内可删 {g.dupIds.length} 份 · {fmtSize(g.dupBytes)}
                </div>
              </div>
              <button className="mini" onClick={() => onSelectGroup(g.dupIds)}>
                勾选此组副本
              </button>
            </div>
          ))}
        </div>
        <div className="actions">
          <button onClick={onClose}>关闭</button>
          <button
            className="primary"
            disabled={allDupIDs.length === 0}
            onClick={() => onSelectAll(allDupIDs)}
            title="勾选全部多余副本（每 md5 仍保留一份），随后点底栏「清理」"
          >
            全选全部多余副本
          </button>
        </div>
      </div>
    </div>
  );
}
