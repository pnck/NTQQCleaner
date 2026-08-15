import { setFocusArea } from "../focus";
import type { GroupStat } from "../types";
import { BIZ_LABEL, fmtSize } from "../types";

// 左栏 = 多选筛选器（不是目录浏览器）：所有条目始终可见，
// 勾选状态即当前筛选条件；中栏照片墙按勾选过滤。

interface Props {
  width: number; // 拖拽分隔条调整的栏宽（App 持有并记忆）
  bizGroups: GroupStat[];
  monthGroups: GroupStat[];
  activeBizs: string[];
  activeMonths: string[];
  onToggleBiz: (biz: string, idx: number) => void;
  onToggleMonth: (month: string, idx: number) => void;
  onShiftBiz: (idx: number) => void;
  onShiftMonth: (idx: number) => void;
  onSetBizs: (bizs: string[]) => void;
  onSetMonths: (months: string[]) => void;
}

function Section({
  sid,
  title,
  groups,
  active,
  onToggle,
  onShift,
  onSet,
  labelOf,
}: {
  sid: string; // data-section：全局 ⌘/Ctrl+A 按聚焦分区选中全部
  title: string;
  groups: GroupStat[];
  active: string[];
  onToggle: (key: string, idx: number) => void;
  onShift: (idx: number) => void;
  onSet: (keys: string[]) => void;
  labelOf: (key: string) => string;
}) {
  return (
    <div
      className="tree-section"
      data-section={sid}
      onMouseDownCapture={() => setFocusArea(sid === "biz" ? "tree-biz" : "tree-month")}
    >
      <div className="tree-header">
        <h3>{title}</h3>
        <span className="tree-actions">
          <button className="mini" onClick={() => onSet(groups.map((g) => g.key))}>
            全选
          </button>
          <button className="mini" onClick={() => onSet([])}>
            清空
          </button>
        </span>
      </div>
      {groups.length === 0 && <div className="node dim">扫描后显示</div>}
      {groups.map((g, i) => (
        <label
          key={g.key}
          className={`node${active.includes(g.key) ? " active" : ""}`}
          onClick={(e) => {
            // Shift+点击 = 从上次点击位置连续选中到本项（键盘操作无说明）。
            if (e.shiftKey) {
              e.preventDefault();
              onShift(i);
            }
          }}
        >
          <input
            type="checkbox"
            checked={active.includes(g.key)}
            onChange={() => onToggle(g.key, i)}
          />
          <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {labelOf(g.key)}
          </span>
          <span className="meta">{g.count} · {fmtSize(g.size)}</span>
        </label>
      ))}
    </div>
  );
}

export function LeftTree({
  width,
  bizGroups,
  monthGroups,
  activeBizs,
  activeMonths,
  onToggleBiz,
  onToggleMonth,
  onShiftBiz,
  onShiftMonth,
  onSetBizs,
  onSetMonths,
}: Props) {
  return (
    <aside className="lefttree" style={{ width }}>
      <Section
        sid="biz"
        title="业务类型"
        groups={bizGroups}
        active={activeBizs}
        onToggle={onToggleBiz}
        onShift={onShiftBiz}
        onSet={onSetBizs}
        labelOf={(k) => BIZ_LABEL[k] ?? k}
      />
      <Section
        sid="month"
        title="月份"
        groups={monthGroups}
        active={activeMonths}
        onToggle={onToggleMonth}
        onShift={onShiftMonth}
        onSet={onSetMonths}
        labelOf={(k) => k}
      />
    </aside>
  );
}
