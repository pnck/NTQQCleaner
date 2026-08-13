import type { Stats } from "../types";
import { fmtSize } from "../types";

interface Props {
  stats: Stats;
  checkedCount: number;
  checkedBytes: number;
  busy: boolean;
  onSelectAll: () => void;
  onClean: () => void;
}

export function BottomBar({ stats, checkedCount, checkedBytes, busy, onSelectAll, onClean }: Props) {
  return (
    <div className="bottombar">
      <span className="tier-stat">
        当前筛选 <b>{stats.count}</b> 个文件 · <b>{fmtSize(stats.size)}</b>
      </span>
      <div className="grow" />
      <button onClick={onSelectAll} title="勾选当前筛选结果中的全部文件">
        全选当前筛选
      </button>
      <span className="release">
        已勾选 {checkedCount} 项 · 可释放 {fmtSize(checkedBytes)}
      </span>
      <button
        className="primary"
        disabled={busy || checkedCount === 0}
        onClick={onClean}
        title="勾选后可执行清理（执行前会再次确认）"
      >
        清理
      </button>
    </div>
  );
}
