import { memo, useCallback, useEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { api } from "../api";
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
}

interface CellProps {
  row: FileRow;
  selected: boolean;
  checked: boolean;
  onSelect: () => void;
  onToggle: () => void; // Cell 内部闭包：onToggle(row.id, row.size)
}

// One wall cell: lazy <img> (only virtualized rows exist in the DOM, so
// images are naturally loaded on demand — docs/07 §4.3 #2).
// 角标 = 文件扩展名；reason 短标签悬浮显示含义解释。
const Cell = memo(function Cell({ row, selected, checked, onSelect, onToggle }: CellProps) {
  const reasons = explainReason(row.reason);
  return (
    <div className={`cell${selected ? " selected" : ""}`} onClick={onSelect}>
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

export function PhotoWall({ query, queryKey, selected, checked, onSelect, onToggle, onRowsChange }: Props) {
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

  // When the virtualizer catches up to the loaded rows (e.g. after a
  // resize), fetch the next page the same way onScroll does.
  useEffect(() => {
    const lastVirtual = items.length > 0 ? items[items.length - 1].index : -1;
    if (lastVirtual >= rowCount - 3 && !loading && rows.length < total) {
      void loadPage(nextPage.current);
    }
  }, [items, rowCount, loading, rows.length, total, loadPage]);

  return (
    <div className="wall-scroll" ref={scrollRef} onScroll={onScroll}>
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
                    selected={selected === row.id}
                    checked={checked.has(row.id)}
                    onSelect={() => onSelect(selected === row.id ? null : row.id)}
                    onToggle={() => onToggle(row.id, row.size)}
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
