import { useRef } from "react";

// 侧栏拖拽分隔条：mousedown 后跟随窗口级 mousemove，把**增量**（dx/dy）
// 回调给持有方（持有方做 clamp 与持久化）。与 FilterEditor 的拖拽排序
// 同一指针方案——WKWebView 不触发 HTML5 的 dragstart/drop 事件，拖拽
// 全部自行实现。拖动期间 body user-select:none，不会选中文本。
// axis="x"（默认）= 左右分栏；axis="y" = 上下分栏。onDragEnd 在指针
// 释放时回调（布局持久化用——拖拽全程高频 onDrag，只在结束时写配置）。
//
// **契约（唯一的机制，横向/纵向一致）**：onDrag 只报**增量**，持有方
// 必须用函数式更新把它应用到状态（如 setH((h) => clamp(h + dy))）——
// mousedown 注册的 window 监听捕获的是当次渲染的回调闭包，读 props
// 旧值做增量会在重渲染后持续回弹到拖拽起点。
export function Splitter({
  onDrag,
  onDragEnd,
  axis = "x",
}: {
  onDrag: (d: number) => void;
  onDragEnd?: () => void;
  axis?: "x" | "y";
}) {
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
          onDragEnd?.();
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
      }}
    />
  );
}
