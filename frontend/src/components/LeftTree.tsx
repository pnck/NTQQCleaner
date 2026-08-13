import type { GroupStat } from "../types";
import { BIZ_LABEL, fmtSize } from "../types";

// 左栏 = 多选筛选器（不是目录浏览器）：所有条目始终可见，
// 勾选状态即当前筛选条件；中栏照片墙按勾选过滤。

interface Props {
  bizGroups: GroupStat[];
  monthGroups: GroupStat[];
  activeBizs: string[];
  activeMonths: string[];
  onToggleBiz: (biz: string) => void;
  onToggleMonth: (month: string) => void;
  onSetBizs: (bizs: string[]) => void;
  onSetMonths: (months: string[]) => void;
}

function Section({
  title,
  groups,
  active,
  onToggle,
  onSet,
  labelOf,
}: {
  title: string;
  groups: GroupStat[];
  active: string[];
  onToggle: (key: string) => void;
  onSet: (keys: string[]) => void;
  labelOf: (key: string) => string;
}) {
  return (
    <div className="tree-section">
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
      {groups.map((g) => (
        <label key={g.key} className={`node${active.includes(g.key) ? " active" : ""}`}>
          <input
            type="checkbox"
            checked={active.includes(g.key)}
            onChange={() => onToggle(g.key)}
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
  bizGroups,
  monthGroups,
  activeBizs,
  activeMonths,
  onToggleBiz,
  onToggleMonth,
  onSetBizs,
  onSetMonths,
}: Props) {
  return (
    <aside className="lefttree">
      <Section
        title="业务类型"
        groups={bizGroups}
        active={activeBizs}
        onToggle={onToggleBiz}
        onSet={onSetBizs}
        labelOf={(k) => BIZ_LABEL[k] ?? k}
      />
      <Section
        title="月份"
        groups={monthGroups}
        active={activeMonths}
        onToggle={onToggleMonth}
        onSet={onSetMonths}
        labelOf={(k) => k}
      />
    </aside>
  );
}
