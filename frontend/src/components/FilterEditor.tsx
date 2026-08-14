import { useEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import { filterToText, group, leaf, parseExpr, updateTree } from "../expression";
import type { Condition, Expr, Sort } from "../types";
import { FILTER_FIELDS, OPS_LABEL, fieldDef, type NamedFilter } from "../filters";

interface Props {
  open: boolean;
  expr: Expr | null;
  onChangeExpr: (e: Expr | null) => void;
  limit?: number;
  offset?: number;
  orders?: { field: string; desc: boolean }[];
  select?: string[];
  onStagesChange: (s: {
    limit?: number;
    offset?: number;
    orders?: { field: string; desc: boolean }[];
    select?: string[];
  }) => void;
  sort: Sort;
  onSortChange: (s: Sort) => void;
  filters: NamedFilter[];
  onSaveFilters: (list: NamedFilter[]) => void;
  onApply: (f: NamedFilter) => void;
  onClose: () => void;
}

// select() 管道的三个正交维度（可多选，并集展开）：把结果集替换为其中
// 文件关联的另一组文件。
const SELECT_KINDS = [
  { value: "ori", label: "原文件" },
  { value: "thumb", label: "缩略图" },
  { value: "dup", label: "重复副本" },
] as const;

// 语法帮助的字段取值参考（与 filters.ts 的 FILTER_FIELDS 语义一致）。
const HELP_FIELDS: { field: string; note: string }[] = [
  { field: "biz", note: "业务类型：pic / video / ptt / emoji / file / dataline" },
  { field: "sub", note: "子目录：Ori / Thumb / OriTemp / ThumbTemp / file_assistant" },
  { field: "category", note: "分类路径片段，如 marketface / personal-emoji" },
  { field: "month", note: "月份，形如 2024-09（字符串比较即时间序）" },
  { field: "age", note: "修改于 N 天前（数值）" },
  { field: "size", note: "字节（编辑器中按 MB 输入）" },
  { field: "md5", note: "文件名 md5；~ 可按前缀/片段匹配" },
  { field: "contentHash", note: "SHA-256 内容哈希（仅同大小冲突组计算）；~ 可按前缀匹配" },
  { field: "reason", note: "说明标签（缩略图/原图仍在/有缩略图/重复出现…）" },
  { field: "thumb", note: "是否缩略图：true / false" },
  { field: "temp", note: "是否 *Temp 下载残留：true / false" },
];

// ---- 条件叶子行 ----

function ConditionRow({
  cond,
  onChange,
  onRemove,
}: {
  cond: Condition;
  onChange: (c: Condition) => void;
  onRemove: () => void;
}) {
  const def = fieldDef(cond.field) ?? FILTER_FIELDS[0];
  const displayValue =
    def.toBytes && cond.value !== ""
      ? String(Math.round(Number(cond.value) / (1024 * 1024)))
      : cond.value;

  const setValue = (v: string) => {
    if (def.toBytes && v !== "") {
      const mb = Number(v);
      if (!Number.isFinite(mb)) return;
      onChange({ ...cond, value: String(Math.round(mb * 1024 * 1024)) });
      return;
    }
    onChange({ ...cond, value: v });
  };

  return (
    <div className="cond-row">
      <select
        value={cond.field}
        onChange={(e) => {
          const nd = fieldDef(e.target.value);
          onChange({
            field: e.target.value,
            op: nd?.ops.includes(cond.op) ? cond.op : (nd?.ops[0] ?? "eq"),
            value: "",
          });
        }}
      >
        {FILTER_FIELDS.map((f) => (
          <option key={f.field} value={f.field}>
            {f.label}
          </option>
        ))}
      </select>
      <select value={cond.op} onChange={(e) => onChange({ ...cond, op: e.target.value })}>
        {def.ops.map((op) => (
          <option key={op} value={op}>
            {OPS_LABEL[op]}
          </option>
        ))}
      </select>
      {def.kind === "bool" ? (
        <select value={cond.value} onChange={(e) => onChange({ ...cond, value: e.target.value })}>
          <option value="true">是</option>
          <option value="false">否</option>
        </select>
      ) : (
        <input
          type={def.kind === "number" ? "number" : "text"}
          placeholder={def.kind === "number" ? "数值" : def.field === "month" ? "如 2024-09" : "值"}
          value={displayValue}
          onChange={(e) => setValue(e.target.value)}
        />
      )}
      {def.unit && <span className="cond-unit">{def.unit}</span>}
      <button className="mini" onClick={onRemove} title="删除此条件">
        ✕
      </button>
    </div>
  );
}

// ---- 列表视图（简易模式）：嵌套且/或组 ----

function GroupBlock({
  root,
  path,
  depth,
  onChangeRoot,
}: {
  root: Expr; // 整棵树根（updateTree 需要）
  path: number[]; // 本组在树中的位置
  depth: number;
  onChangeRoot: (e: Expr | null) => void;
}) {
  let node = root;
  for (const i of path) node = node.or?.[i] ?? node.and?.[i] ?? group("and", []);
  const isOr = Array.isArray(node.or);
  const kids = node.or ?? node.and ?? [];

  const apply = (upd: (n: Expr) => Expr | null) => onChangeRoot(updateTree(root, path, upd));
  const setKind = (or: boolean) => apply((n) => group(or ? "or" : "and", n.or ?? n.and ?? []));
  const add = (child: Expr) =>
    apply((n) => {
      const arr = [...(n.or ?? n.and ?? [])];
      arr.push(child);
      return n.or ? { or: arr } : { and: arr };
    });
  const removeSelf = () => onChangeRoot(updateTree(root, path, () => null));

  return (
    <div className="expr-group" style={{ marginLeft: depth > 0 ? 12 : 0 }}>
      <div className="expr-group-head">
        <select
          value={isOr ? "or" : "and"}
          onChange={(e) => setKind(e.target.value === "or")}
          title="组内逻辑：且 = 全部满足；或 = 任一满足"
        >
          <option value="and">且（全部满足）</option>
          <option value="or">或（任一满足）</option>
        </select>
        <button className="mini" onClick={() => add(leaf("age", "gte", "90"))}>
          ＋条件
        </button>
        <button className="mini" onClick={() => add(group("and", [leaf("age", "gte", "90")]))}>
          ＋且组
        </button>
        <button className="mini" onClick={() => add(group("or", [leaf("age", "gte", "90")]))}>
          ＋或组
        </button>
        {depth > 0 && (
          <button className="mini" onClick={removeSelf}>
            删除组
          </button>
        )}
      </div>
      {kids.length === 0 && <div className="cond-empty">空组 = 匹配全部</div>}
      {kids.map((child, i) => {
        const childPath = [...path, i];
        if (child.c) {
          return (
            <ConditionRow
              key={i}
              cond={child.c}
              onChange={(nc) => onChangeRoot(updateTree(root, childPath, () => ({ c: nc })))}
              onRemove={() => onChangeRoot(updateTree(root, childPath, () => null))}
            />
          );
        }
        return (
          <GroupBlock
            key={i}
            root={root}
            path={childPath}
            depth={depth + 1}
            onChangeRoot={onChangeRoot}
          />
        );
      })}
    </div>
  );
}

// ---- 主对话框：列表（简易）/ 表达式（高级）双视图 + 筛选器列表 ----

export function FilterEditor({
  open,
  expr,
  onChangeExpr,
  limit,
  offset,
  orders,
  select,
  onStagesChange,
  sort,
  onSortChange,
  filters,
  onSaveFilters,
  onApply,
  onClose,
}: Props) {
  const [view, setView] = useState<"list" | "text">("list");
  const [text, setText] = useState("");
  const [parseErr, setParseErr] = useState("");
  const [applied, setApplied] = useState(false);
  const [filterName, setFilterName] = useState("");
  const [selected, setSelected] = useState("");
  const [helpOpen, setHelpOpen] = useState(false);
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [overIdx, setOverIdx] = useState<number | null>(null);
  const dragFrom = useRef<number | null>(null);
  const overRef = useRef<number | null>(null);
  const listRef = useRef<HTMLDivElement | null>(null);

  // 表达式文本同步：打开/切视图，或外部 expr/管道变化（如在筛选器列表
  // 点击「应用」）时重置为当前筛选的规范文本——列表应用随动到表达式视图。
  useEffect(() => {
    if (open && view === "text") {
      setText(filterToText(expr, limit, offset, orders, select));
      setParseErr("");
      setApplied(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, view, expr, limit, offset, orders, select]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const root = expr ?? group("and", []);

  // 管道含 order/take/drop = 高级函数 → 简易模式只读
  // （select 是独立下拉，两个视图都可编辑，不参与 advanced 判定）
  const advanced = (orders?.length ?? 0) > 0 || limit !== undefined || offset !== undefined;

  // 表达式视图：按钮验证并应用
  const applyText = (): boolean => {
    const res = parseExpr(text);
    if (res.error) {
      setParseErr(res.error);
      setApplied(false);
      return false;
    }
    setParseErr("");
    onChangeExpr(res.expr ?? null);
    onStagesChange({ limit: res.limit, offset: res.offset, orders: res.orders, select: res.select });
    setText(filterToText(res.expr ?? null, res.limit, res.offset, res.orders, res.select));
    setApplied(true);
    return true;
  };

  const saveFilter = () => {
    const name = filterName.trim();
    if (!name) return;
    const next = [
      ...filters.filter((f) => f.name !== name),
      { name, expr, sort, limit, offset, orders, select },
    ];
    onSaveFilters(next);
    setSelected(name);
    setFilterName("");
  };

  const deleteFilter = (name: string) => {
    onSaveFilters(filters.filter((f) => f.name !== name));
    if (selected === name) setSelected("");
  };

  const togglePin = (name: string) => {
    onSaveFilters(filters.map((f) => (f.name === name ? { ...f, pinned: !f.pinned } : f)));
  };

  const finish = () => {
    if (view === "text" && !applyText()) return; // 无效表达式：停留显示错误
    onClose();
  };

  // 指针拖拽排序：WKWebView 不触发 HTML5 的 dragstart/drop 事件，
  // draggable 方案在 macOS GUI 里静默失效，改用 mousedown/mousemove/
  // mouseup 自行实现（handle 按下开始，mouseup 落点插入）。
  const startDrag = (i: number, e: ReactMouseEvent) => {
    e.preventDefault();
    dragFrom.current = i;
    overRef.current = i;
    setDragIdx(i);
    setOverIdx(i);
    const onMove = (ev: MouseEvent) => {
      const items = listRef.current?.querySelectorAll<HTMLElement>("[data-fi]");
      if (!items) return;
      let best = -1;
      let bestD = Infinity;
      items.forEach((el, j) => {
        const r = el.getBoundingClientRect();
        const d = Math.abs(ev.clientY - (r.top + r.height / 2));
        if (d < bestD) {
          bestD = d;
          best = j;
        }
      });
      if (best >= 0 && best !== overRef.current) {
        overRef.current = best;
        setOverIdx(best);
      }
    };
    const onUp = () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      const from = dragFrom.current;
      const to = overRef.current;
      dragFrom.current = null;
      overRef.current = null;
      setDragIdx(null);
      setOverIdx(null);
      if (from !== null && to !== null && from !== to) {
        const next = [...filters];
        const [moved] = next.splice(from, 1);
        next.splice(to, 0, moved);
        onSaveFilters(next);
      }
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  };

  return (
    <div className="dialog-mask" onClick={onClose}>
      <div className="dialog dialog-wide" onClick={(e) => e.stopPropagation()}>
        <h2>编辑筛选器</h2>

        <div className="row">
          <label>视图</label>
          <span style={{ display: "flex", gap: 6 }}>
            <button className={view === "list" ? "primary" : ""} onClick={() => setView("list")}>
              列表（简易）
            </button>
            <button className={view === "text" ? "primary" : ""} onClick={() => setView("text")}>
              表达式（高级）
            </button>
          </span>
        </div>

        {view === "list" ? (
          <>
            {advanced && (
              <div className="advanced-banner">
                当前筛选含有高级函数（order/take/drop 管道）——请在「表达式」模式下编辑
              </div>
            )}
            <div
              className="cond-list"
              style={advanced ? { opacity: 0.5, pointerEvents: "none" } : undefined}
            >
              <GroupBlock root={root} path={[]} depth={0} onChangeRoot={onChangeExpr} />
              {expr === null && <div className="cond-empty">无条件 = 显示全部</div>}
              <div className="row">
                <label>排序</label>
                <select
                  disabled={advanced}
                  value={`${sort.field}:${sort.desc}`}
                  onChange={(e) => {
                    const [field, desc] = e.target.value.split(":");
                    onSortChange({ field, desc: desc === "true" });
                  }}
                >
                  <option value="size:true">大小（大→小）</option>
                  <option value="size:false">大小（小→大）</option>
                  <option value="mtime:true">时间（新→旧）</option>
                  <option value="mtime:false">时间（旧→新）</option>
                  <option value="month:true">月份（新→旧）</option>
                  <option value="month:false">月份（旧→新）</option>
                </select>
              </div>
              <div className="row">
                <label>关联展开</label>
                <span
                  style={{ display: "flex", gap: 12, alignItems: "center", fontSize: 12 }}
                  title="select() 管道：把当前结果替换为其中文件关联的其它文件（可多选，正交并集）"
                >
                  {SELECT_KINDS.map((k) => {
                    const on = (select ?? []).includes(k.value);
                    return (
                      <label
                        key={k.value}
                        style={{ display: "flex", gap: 4, alignItems: "center" }}
                      >
                        <input
                          type="checkbox"
                          checked={on}
                          onChange={(e) => {
                            const next = new Set(select ?? []);
                            if (e.target.checked) next.add(k.value);
                            else next.delete(k.value);
                            onStagesChange({ select: next.size > 0 ? [...next] : undefined });
                          }}
                        />
                        {k.label}
                      </label>
                    );
                  })}
                  {(select?.length ?? 0) === 0 && (
                    <span style={{ color: "var(--text-dim)" }}>（不展开）</span>
                  )}
                </span>
              </div>
            </div>
          </>
        ) : (
          <div className="expr-text-view">
            <textarea
              value={text}
              onChange={(e) => {
                setText(e.target.value);
                setApplied(false);
              }}
              placeholder="输入表达式，如：biz in (pic, video) AND size > 1048576"
              rows={5}
            />
            {/* 可折叠语法参考：默认收起（常驻占位太大），需要时展开 */}
            <button
              className="mini help-toggle"
              onClick={() => setHelpOpen((o) => !o)}
              title="语法参考：字段取值 / 操作符 / 管道函数"
            >
              {helpOpen ? "▾" : "▸"} 语法帮助
            </button>
            {helpOpen && (
              <div className="expr-help">
                <div className="help-sec">
                  <b>结构</b>
                  <div>
                    条件用 AND（且）/ OR（或）连接，( ) 嵌套分组；含空格的值加引号
                    "…"。管道函数接在表达式末尾，执行顺序固定 select → order → drop →
                    take。
                  </div>
                </div>
                <div className="help-sec">
                  <b>操作符</b>
                  <div>= 等于 · != 不等于</div>
                  <div>~ 包含：子串匹配（LIKE %值%）；contentHash ~ 9f2e 可按哈希前缀/片段筛选</div>
                  <div>in 属于列表：列表写在括号内，如 biz in (pic, video)</div>
                  <div>&gt; &gt;= &lt; &lt;= 比较：size/age 按数值，month 按字符串序</div>
                </div>
                <div className="help-sec">
                  <b>字段与取值</b>
                  {HELP_FIELDS.map((f) => (
                    <div key={f.field}>
                      <code>{f.field}</code> {f.note}
                    </div>
                  ))}
                </div>
                <div className="help-sec">
                  <b>管道函数</b>
                  <div>
                    select(ori|thumb|dup, 可多个)：把结果替换为其中文件关联的原文件/缩略图/同内容副本（并集展开）
                  </div>
                  <div>
                    order(size|mtime|month|md5, asc|desc)：排序，可链多个
                  </div>
                  <div>take(n)：取前 n 条 · drop(n)：跳过前 n 条</div>
                </div>
                <div className="help-sec">
                  <b>示例</b>
                  <div>thumb = true AND age &gt;= 90 | take(100)</div>
                  <div>size &gt; 104857600 | order(size, desc) | drop(10)</div>
                  <div>biz in (pic, video) OR category ~ marketface</div>
                  <div>(biz = pic AND size &gt; 1048576) OR month &lt; 2025-01</div>
                  <div>contentHash ~ 9f2e | select(dup)</div>
                </div>
              </div>
            )}
            {parseErr ? (
              <div className="parse-err">✗ {parseErr}</div>
            ) : applied ? (
              <div className="parse-ok">✓ 已应用</div>
            ) : null}
            <div className="row">
              <button className="primary" onClick={applyText}>
                验证并应用
              </button>
              <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
                管道函数接在表达式末尾；select → order → drop → take 依固定顺序执行
              </span>
            </div>
          </div>
        )}

        <div className="row">
          <label>保存当前为筛选器</label>
          <input
            value={filterName}
            onChange={(e) => setFilterName(e.target.value)}
            placeholder="名称（条件与排序一并保存）"
            style={{ flex: 1 }}
          />
          <button onClick={saveFilter}>保存</button>
        </div>

        <div className="filter-list" ref={listRef}>
          <h3>筛选器列表（拖拽排序；置顶的按此顺序出现在工具栏）</h3>
          {filters.length === 0 && (
            <div className="filter-item" style={{ color: "var(--text-dim)", cursor: "default" }}>
              暂无筛选器（保存一个筛选器后出现）
            </div>
          )}
          {filters.map((f, i) => (
            <div
              key={f.name}
              data-fi={i}
              className={`filter-item${dragIdx === i ? " dragging" : ""}${
                dragIdx !== null && overIdx === i && overIdx !== dragIdx ? " drop-target" : ""
              }`}
            >
              <span
                className="drag-handle"
                title="拖拽排序"
                onMouseDown={(e) => startDrag(i, e)}
              >
                ⠿
              </span>
              <span style={{ flex: 1 }}>
                {f.name}
                {f.pinned ? " ★" : ""}
              </span>
              <button className="mini" onClick={() => onApply(f)}>
                应用
              </button>
              <button className="mini" onClick={() => togglePin(f.name)}>
                {f.pinned ? "取消置顶" : "置顶"}
              </button>
              <button className="mini" onClick={() => deleteFilter(f.name)}>
                删除
              </button>
            </div>
          ))}
        </div>

        <div className="actions">
          <button onClick={() => onChangeExpr(null)}>清空条件</button>
          <button onClick={onClose}>取消</button>
          <button className="primary" onClick={finish}>
            完成
          </button>
        </div>
      </div>
    </div>
  );
}
