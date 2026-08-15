import type { DupGroup } from "../types";
import { fmtSize, fmtTime } from "../types";

// 去重建议：字节级内容完全相同（SHA-256 分组）的文件被 QQ 存到了
// 多个目录/月份（QQ 只按目录去重，不同名不代表不同内容）。每组保留
// 一份（原图优先，其次最新，可能在当前筛选之外），可去重项 = 组内
// 副本 ∩ 当前筛选（交集语义：面板只出现筛选覆盖的文件）。
interface Props {
  open: boolean;
  groups: DupGroup[];
  onSelectGroup: (g: DupGroup) => void;
  onSelectAll: (groups: DupGroup[]) => void;
  onClose: () => void;
  checked: Set<number>; // 全局勾选集（墙的手动勾选 + 本对话框）
  checkedCount: number;
}

export function DupesDialog({
  open,
  groups,
  onSelectGroup,
  onSelectAll,
  onClose,
  checked,
  checkedCount,
}: Props) {
  if (!open) return null;
  const totalBytes = groups.reduce((a, g) => a + g.dupBytes, 0);
  const allDupCount = groups.reduce((a, g) => a + g.dupIds.length, 0);

  return (
    <div className="dialog-mask" onClick={onClose}>
      {/* dupes-dialog：标题与底部操作行固定，仅中间列表滚动（回归修复——
          共用 .filter-list 的 max-height 在筛选器编辑器「单一滚动上下文」
          改造中被移除，整个对话框一起滚、标题按钮滚出视口） */}
      <div className="dialog dialog-wide dupes-dialog" onClick={(e) => e.stopPropagation()}>
        <h2>去重建议</h2>
        <div style={{ color: "var(--text-dim)", fontSize: 12 }}>
          当前筛选内 {allDupCount} 个多余副本 · 可释放 {fmtSize(totalBytes)}
        </div>
        <div className="dupe-list">
          {groups.length === 0 && <div className="cond-empty">当前筛选内没有重复项。</div>}
          {groups.map((g) => {
            const done = g.dupIds.every((id) => checked.has(id));
            return (
              <div key={g.hash} className="dupe-row">
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="dupe-hash" title="内容哈希（SHA-256）：字节级相同内容的签名">
                    {g.hash}（共 {g.count} 份）
                  </div>
                  <div style={{ color: "var(--text-dim)", fontSize: 11 }}>
                    保留 1 份：{g.keepLabel}（{fmtTime(g.keepMtime)}）
                    {!g.keepInFilter ? " · 筛选外" : ""}
                    {checked.has(g.keepId) ? " · ⚠已勾选" : ""}
                  </div>
                  <div style={{ fontSize: 11 }}>
                    可删 {g.dupIds.length} 份 · {fmtSize(g.dupBytes)}
                  </div>
                </div>
                {/* 可逆开关：文案 = 本次点击执行的动作（与预览面板
                    「勾选副本/勾选全部」同一交互模式）。已勾选组再次点击
                    = 取消勾选该组副本——对话框内即可撤销建议应用 */}
                <button
                  className="mini"
                  onClick={() => onSelectGroup(g)}
                  title={done ? "取消勾选该组副本（保留份维持现状）" : "勾选该组全部副本（保留一份不勾）"}
                >
                  {done ? "取消勾选" : "勾选此组副本"}
                </button>
              </div>
            );
          })}
        </div>
        <div style={{ fontSize: 11, color: "var(--text-dim)" }}>
          已勾选 {checkedCount} 个文件（含手动勾选与预览面板操作）
        </div>
        <div className="actions">
          <button onClick={onClose}>关闭</button>
          <button
            className="primary"
            disabled={allDupCount === 0}
            onClick={() => onSelectAll(groups)}
            title="勾选当前筛选内全部多余副本、取消勾选各组的保留份（每组内容仍保留一份），随后点底栏「清理」"
          >
            勾选筛选内全部多余副本
          </button>
        </div>
      </div>
    </div>
  );
}
