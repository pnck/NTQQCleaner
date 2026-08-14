import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { api } from "../api";
import { setFocusArea } from "../focus";
import { explainReason } from "../reasons";
import type { FileRow, PageQuery } from "../types";
import { fmtSize } from "../types";
import { Tooltip } from "./Tooltip";

// Cell geometry: fixed row height keeps virtualization cheap and the wall
// perfectly smooth at 140k+ rows (docs/07 §4.3 #1).
const CELL_H = 112;
const CELL_W = 132;
const PAGE_SIZE = 200; // backend page size (docs/07 §4.3 #7)

interface Props {
  query: PageQuery;
  queryKey: string; // bump to reset the wall (filter/sort change)
  selected: number | null;
  checked: Set<number>;
  onSelect: (id: number | null) => void;
  onToggle: (id: number, size: number) => void;
  onRowsChange: (rows: FileRow[]) => void; // App mirrors the loaded rows for preview
  onToggleRange: (from: number, to: number, want: boolean) => void; // Shift 点击/划选范围
}

interface CellProps {
  row: FileRow;
  dataIdx: number;
  selected: boolean;
  checked: boolean;
  onSelect: (shift: boolean) => void;
  onToggle: () => void; // Cell 内部闭包：onToggle(row.id, row.size)
  onCellMouseDown: (e: ReactMouseEvent) => void; // 划选手势起点
}

// One wall cell: lazy <img> (only virtualized rows exist in the DOM, so
// images are naturally loaded on demand — docs/07 §4.3 #2).
// 角标 = 文件扩展名；reason 短标签悬浮显示含义解释。
const Cell = memo(function Cell({ row, dataIdx, selected, checked, onSelect, onToggle, onCellMouseDown }: CellProps) {
  const reasons = explainReason(row.reason);
  return (
    <div
      className={`cell${selected ? " selected" : ""}`}
      data-idx={dataIdx}
      onClick={(e) => onSelect(e.shiftKey)}
      onMouseDown={onCellMouseDown}
    >
      <input
        className="cell-check"
        type="checkbox"
        checked={checked}
        onClick={(e) => e.stopPropagation()}
        onChange={onToggle}
      />
      <span className="cell-biz">{row.ext ? row.ext.toUpperCase() : row.biz.toUpperCase()}</span>
      {row.thumbUrl ? (
        <img src={row.thumbUrl} loading="lazy" draggable={false} alt={row.md5} />
      ) : (
        <div style={{ flex: 1 }} />
      )}
      <div className="cell-info">
        <Tooltip
          content={
            <span>
              {reasons.map((r) => (
                <span key={r.label} style={{ display: "block" }}>
                  <b>{r.label}</b>：{r.explain}
                </span>
              ))}
            </span>
          }
        >
          {/* 全部标签并排（如「缩略图 · 原图仍在 · 重复出现」）：重复/配对
              标识直接在卡片可见，不只藏在 tooltip 里 */}
          <span className="reason-label" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {reasons.map((r) => r.label).join(" · ")}
          </span>
        </Tooltip>
        {row.month && <span style={{ color: "var(--text-dim)", flex: "none" }}>{row.month}</span>}
        <span>{fmtSize(row.size)}</span>
      </div>
    </div>
  );
});

export function PhotoWall({ query, queryKey, selected, checked, onSelect, onToggle, onRowsChange, onToggleRange }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [rows, setRows] = useState<FileRow[]>([]);
  const [total, setTotal] = useState(0);
  const [cols, setCols] = useState(4);
  const [loading, setLoading] = useState(false);
  const nextPage = useRef(1);
  // 初始值 null：挂载即触发首次加载。此前用 useRef(queryKey) 初值，
  // 挂载时与当前 queryKey 相等导致首屏跳过加载——扫描完成/清理报错后的
  // 重挂载都会出现空白墙，直到筛选器变化才恢复。
  const key = useRef<string | null>(null);

  // Reset and reload the first page whenever the query changes.
  useEffect(() => {
    if (key.current === queryKey) return;
    key.current = queryKey;
    nextPage.current = 1;
    setRows([]);
    setTotal(0);
    setLoading(false);
    void loadPage(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey]);

  const loadPage = useCallback(
    async (page: number) => {
      setLoading(true);
      try {
        const res = await api.queryRows({ ...query, page, pageSize: PAGE_SIZE });
        setTotal(res.total);
        setRows((prev) => (page === 1 ? res.rows : [...prev, ...res.rows]));
        nextPage.current = page + 1;
      } catch (e) {
        console.error("queryRows:", e);
      } finally {
        setLoading(false);
      }
    },
    [query],
  );

  // Mirror rows up to App for preview / checked sums.
  useEffect(() => onRowsChange(rows), [rows, onRowsChange]);

  // Column count follows the container width.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      setCols(Math.max(1, Math.floor(entry.contentRect.width / CELL_W)));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const rowCount = Math.ceil(rows.length / cols);
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => CELL_H,
    overscan: 8, // rows of buffer above/below the viewport
  });

  // Infinite scroll: fetch the next page before hitting the bottom
  // (docs/07 §4.3 #7).
  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el || loading) return;
    if (rows.length >= total) return;
    if (el.scrollTop + el.clientHeight > el.scrollHeight - 2400) {
      void loadPage(nextPage.current);
    }
  }, [loading, rows.length, total, loadPage]);

  const items = virtualizer.getVirtualItems();
  const lastIndex = rows.length - 1;

  // 键盘/Shift 的锚点：最近一次单击或方向键导航到的行下标。
  const anchorRef = useRef<number>(-1);

  // 单击 = 聚焦预览；Shift+单击 = 从锚点连续勾选到当前行。
  const handleCellSelect = useCallback(
    (i: number, id: number, shift: boolean) => {
      // 划选结束后的 click 不再触发聚焦（移动阈值内除外）
      if (suppressClick.current) {
        suppressClick.current = false;
        return;
      }
      if (shift && anchorRef.current >= 0) {
        onToggleRange(anchorRef.current, i, true);
        anchorRef.current = i;
        onSelect(id);
        return;
      }
      anchorRef.current = i;
      onSelect(selected === id ? null : id);
    },
    [onToggleRange, onSelect, selected],
  );

  // 划选期间每次移动都取**最新**的范围回调（onToggleRange 随 App 的
  // checked 变化重建，闭包捕获旧值会丢失中途的状态）。
  const rangeRef = useRef(onToggleRange);
  rangeRef.current = onToggleRange;

  // 鼠标划选（手势无说明文案）：在 **check 角标**按下并拖动 = 从起点
  // 连续勾选到松开位置；若起点已勾选，则整段连续取消勾选（方向由按下
  // 位置的初始状态决定）。触发区与勾选一致（角标）；图片本体保留聚焦
  // 语义，不参与划选。
  const dragRef = useRef<{ mode: boolean; start: number; last: number; moved: boolean } | null>(null);
  const dragXY = useRef({ x: 0, y: 0 });
  const suppressClick = useRef(false);

  const onCellMouseDown = useCallback(
    (e: ReactMouseEvent, i: number, id: number) => {
      if (e.button !== 0) return;
      if (!(e.target as HTMLElement).closest(".cell-check")) return;
      e.preventDefault();
      dragXY.current = { x: e.clientX, y: e.clientY };
      dragRef.current = { mode: !checked.has(id), start: i, last: i, moved: false };
      const onMove = (ev: MouseEvent) => {
        const d = dragRef.current;
        if (!d) return;
        if (
          !d.moved &&
          Math.abs(ev.clientX - dragXY.current.x) + Math.abs(ev.clientY - dragXY.current.y) > 4
        ) {
          d.moved = true;
        }
        const el = document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.("[data-idx]");
        if (!el) return;
        const j = Number(el.getAttribute("data-idx"));
        if (Number.isFinite(j) && j !== d.last) {
          d.last = j;
          rangeRef.current(d.start, j, d.mode);
        }
      };
      const onUp = () => {
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
        const d = dragRef.current;
        if (d?.moved) suppressClick.current = true;
        dragRef.current = null;
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [checked],
  );

  // 方向键在照片墙内移动聚焦（键盘操作无说明文案）；未聚焦时从第一项
  // 开始。移动后滚动到可视区。
  const onKeyDown = useCallback(
    (e: ReactKeyboardEvent<HTMLDivElement>) => {
      const t = e.target as HTMLElement;
      // 文本输入控件照常；单元格勾选框上没有方向键语义，继续接管。
      if (t.closest("input, textarea, select") && !t.closest(".cell-check")) return;
      if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(e.key)) return;
      if (rows.length === 0) return;
      e.preventDefault();
      let idx = selected != null ? rows.findIndex((r) => r.id === selected) : -1;
      if (idx < 0) idx = 0;
      switch (e.key) {
        case "ArrowRight":
          idx = Math.min(idx + 1, lastIndex);
          break;
        case "ArrowLeft":
          idx = Math.max(idx - 1, 0);
          break;
        case "ArrowDown":
          idx = Math.min(idx + cols, lastIndex);
          break;
        case "ArrowUp":
          idx = Math.max(idx - cols, 0);
          break;
      }
      anchorRef.current = idx;
      onSelect(rows[idx].id);
      virtualizer.scrollToIndex(Math.floor(idx / cols), { align: "auto" });
    },
    [rows, selected, cols, lastIndex, onSelect, virtualizer],
  );

  // When the virtualizer catches up to the loaded rows (e.g. after a
  // resize), fetch the next page the same way onScroll does.
  useEffect(() => {
    const lastVirtual = items.length > 0 ? items[items.length - 1].index : -1;
    if (lastVirtual >= rowCount - 3 && !loading && rows.length < total) {
      void loadPage(nextPage.current);
    }
  }, [items, rowCount, loading, rows.length, total, loadPage]);

  return (
    <div
      className="wall-scroll"
      ref={scrollRef}
      onScroll={onScroll}
      tabIndex={0}
      onKeyDown={onKeyDown}
      onMouseDownCapture={() => setFocusArea("wall")}
    >
      {rows.length === 0 && !loading && (
        <div className="wall-empty">
          还没有扫描结果 —— 选择数据根和账号后点击「扫描」。
        </div>
      )}
      <div className="wall-inner" style={{ height: virtualizer.getTotalSize() }}>
        {items.map((vrow) => {
          const base = vrow.index * cols;
          return (
            <div
              key={vrow.key}
              className="wall-row"
              style={{ transform: `translateY(${vrow.start}px)`, height: CELL_H }}
            >
              {Array.from({ length: cols }, (_, c) => {
                const i = base + c;
                if (i > lastIndex) return <div key={c} style={{ flex: 1 }} />;
                const row = rows[i];
                return (
                  <Cell
                    key={row.id}
                    row={row}
                    dataIdx={i}
                    selected={selected === row.id}
                    checked={checked.has(row.id)}
                    onSelect={(shift) => handleCellSelect(i, row.id, shift)}
                    onToggle={() => {
                      // 划选结束后的 click 不再触发单格勾选（移动阈值内除外）
                      if (suppressClick.current) {
                        suppressClick.current = false;
                        return;
                      }
                      onToggle(row.id, row.size);
                    }}
                    onCellMouseDown={(e) => onCellMouseDown(e, i, row.id)}
                  />
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
