import { useEffect, useMemo, useRef, useState } from "react";
import type { JSX } from "react";

export interface SelectOption {
  value: string;
  label: string;
}

// MultiSelect：tag + 下拉列表多选控件（自研，不引组件库——项目约束手写
// 语义 CSS）。交互模型综合 CoreUI MultiSelect / antd Select mode="multiple"
// / react-select 的惯例：
//   - 下拉分**固定区**（搜索框 + 全选/清空）与**滚动区**（选项列表），
//     与 CoreUI 的「下拉头部 + optionsMaxHeight 列表」结构一致
//   - 全选/清空作用于**当前列表**（CoreUI selectAllMode="filtered" 语义）：
//     无搜索词 = 全部候选，有搜索词 = 过滤结果；按钮带数量后缀提示范围
//   - 选项行 CoreUI 视觉：checkbox 内联紧贴文字、统一左对齐，间隔用
//     margin 不用 flex 拉伸
//   - tag 的 × 点击不开关下拉（react-select MultiValueRemove / antd tag
//     惯例：stopPropagation 阻断框体的切换 onClick）
//   - 外点（mousedown）/Esc 收起；Esc 走捕获阶段并 stopPropagation——
//     下拉开着时只关下拉，不让对话框的全局 Esc（关对话框）同时触发
// 值以 string[] 进出（条件树的 in 列表由调用方 join/split）。
export function MultiSelect({
  options,
  values,
  onChange,
  placeholder = "选择…",
  emptyHint = "暂无候选",
}: {
  options: SelectOption[];
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  emptyHint?: string;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const wrapRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!wrapRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation(); // 只收下拉，不连带关对话框
      setOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey, true); // 捕获：先于对话框的 Esc 处理
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [open]);

  // 搜索过滤：label 与 value 都参与（月份列表可上百项）。
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return options;
    return options.filter(
      (o) => o.label.toLowerCase().includes(s) || o.value.toLowerCase().includes(s),
    );
  }, [options, q]);

  const selected = useMemo(() => new Set(values), [values]);
  const labelOf = (v: string) => options.find((o) => o.value === v)?.label ?? v;
  const toggle = (v: string) =>
    onChange(selected.has(v) ? values.filter((x) => x !== v) : [...values, v]);

  // 全选/清空的目标 = 当前列表：无搜索词 = 全部候选，有搜索词 = 过滤结果。
  const target = q.trim() !== "" ? filtered : options;
  const inTarget = target.filter((o) => selected.has(o.value)).length;
  const selectAll = () =>
    onChange([...new Set([...values, ...target.map((o) => o.value)])]);
  const clearTarget = () =>
    onChange(values.filter((v) => !target.some((o) => o.value === v)));

  return (
    <div className="mselect" ref={wrapRef}>
      <div
        className={`mselect-box${open ? " open" : ""}`}
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        {values.length === 0 ? (
          <span className="mselect-placeholder">{placeholder}</span>
        ) : (
          values.map((v) => (
            <span key={v} className="mselect-tag">
              {labelOf(v)}
              <button
                type="button"
                className="mselect-x"
                title="移除"
                aria-label={`移除 ${labelOf(v)}`}
                onClick={(e) => {
                  e.stopPropagation(); // × 点击不开关下拉
                  onChange(values.filter((x) => x !== v));
                }}
              >
                ×
              </button>
            </span>
          ))
        )}
        <span className="mselect-caret">▾</span>
      </div>
      {open && (
        <div className="mselect-pop">
          {/* 固定区：搜索框 + 全选/清空（不随选项滚动） */}
          <div className="mselect-head">
            <input
              className="mselect-search"
              value={q}
              autoFocus
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索…"
            />
            <div className="mselect-actions">
              <button
                type="button"
                className="mini"
                disabled={target.length === 0}
                onClick={selectAll}
                title="把当前列表（含搜索过滤）全部加入选中"
              >
                全选{target.length > 0 ? `（${target.length}）` : ""}
              </button>
              <button
                type="button"
                className="mini"
                disabled={inTarget === 0}
                onClick={clearTarget}
                title="从选中中移除当前列表（含搜索过滤）"
              >
                清空{inTarget > 0 ? `（${inTarget}）` : ""}
              </button>
            </div>
          </div>
          {/* 滚动区：选项列表溢出滚动 */}
          <div className="mselect-list">
            {filtered.length === 0 && (
              <div className="mselect-empty">
                {q.trim() !== "" ? "无匹配项" : emptyHint}
              </div>
            )}
            {filtered.map((o) => {
              const on = selected.has(o.value);
              return (
                <label key={o.value} className={`mselect-opt${on ? " on" : ""}`}>
                  <input type="checkbox" checked={on} onChange={() => toggle(o.value)} />
                  <span>{o.label}</span>
                </label>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
