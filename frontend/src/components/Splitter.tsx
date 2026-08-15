import { useRef } from "react";

// 侧栏拖拽分隔条：mousedown 后跟随窗口级 mousemove，把**增量**（dx/dy）
// 回调给持有方（持有方做 clamp 与持久化）。与 FilterEditor 的拖拽排序
// 同一指针方案——WKWebView 不触发 HTML5 的 dragstart/drop 事件，拖拽
// 全部自行实现。拖动期间 body user-select:none，不会选中文本。
// axis="x"（默认）= 左右分栏；axis="y" = 上下分栏。
export function Splitter({ onDrag, axis = "x" }: { onDrag: (d: number) => void; axis?: "x" | "y" }) {
  const last = useRef<number | null>(null);
  // 兼容 React 合成事件与原生 MouseEvent：都带 clientX/clientY。
  const pick = (e: { clientX: number; clientY: number }) => (axis === "y" ? e.clientY : e.clientX);
  return (
    <div
      className={axis === "y" ? "splitter-v" : "splitter"}
      title={axis === "y" ? "拖动调整分栏高度" : "拖动调整栏宽"}
      onMouseDown={(e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        last.current = pick(e);
        const onMove = (ev: MouseEvent) => {
          if (last.current === null) return;
          const d = pick(ev) - last.current;
          last.current = pick(ev);
          onDrag(d);
        };
        const onUp = () => {
          last.current = null;
          window.removeEventListener("mousemove", onMove);
          window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
      }}
    />
  );
}
