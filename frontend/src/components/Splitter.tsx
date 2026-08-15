import { useRef } from "react";

// 侧栏拖拽分隔条：mousedown 后跟随窗口级 mousemove，把**增量**（dx）
// 回调给 App（App 做 clamp 与持久化）。与 FilterEditor 的拖拽排序同一
// 指针方案——WKWebView 不触发 HTML5 的 dragstart/drop 事件，拖拽全部
// 自行实现。拖动期间 body user-select:none，不会选中文本。
export function Splitter({ onDrag }: { onDrag: (dx: number) => void }) {
  const lastX = useRef<number | null>(null);
  return (
    <div
      className="splitter"
      title="拖动调整栏宽"
      onMouseDown={(e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        lastX.current = e.clientX;
        const onMove = (ev: MouseEvent) => {
          if (lastX.current === null) return;
          const dx = ev.clientX - lastX.current;
          lastX.current = ev.clientX;
          onDrag(dx);
        };
        const onUp = () => {
          lastX.current = null;
          window.removeEventListener("mousemove", onMove);
          window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
      }}
    />
  );
}
