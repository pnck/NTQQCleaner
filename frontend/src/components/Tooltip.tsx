import { useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { createPortal } from "react-dom";

// 悬浮 tooltip：Portal 挂到 body + fixed 定位。
// 触发元素可能位于带 transform 的虚拟化行内（fixed 会被降级为 absolute 并被
// 滚动容器裁剪），portal 逃逸该限制；位置按视口钳制，上方空间不足时翻转到下方。
export function Tooltip({ content, children }: { content: ReactNode; children: ReactNode }) {
  const wrapRef = useRef<HTMLSpanElement>(null);
  const tipRef = useRef<HTMLDivElement>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number; h: number } | null>(null);
  const [style, setStyle] = useState<CSSProperties | null>(null);

  const show = () => {
    const r = wrapRef.current?.getBoundingClientRect();
    if (r) setAnchor({ x: r.left, y: r.top, h: r.height });
  };
  const hide = () => {
    setAnchor(null);
    setStyle(null);
  };

  useLayoutEffect(() => {
    if (!anchor || !tipRef.current) return;
    const t = tipRef.current.getBoundingClientRect();
    const vw = window.innerWidth;
    const above = anchor.y - t.height - 8 > 0;
    const x = Math.min(Math.max(8, anchor.x), Math.max(8, vw - t.width - 8));
    const y = above ? anchor.y - t.height - 6 : anchor.y + anchor.h + 6;
    setStyle({ left: x, top: y, position: "fixed", visibility: "visible" });
  }, [anchor, content]);

  return (
    <span
      className="tt-wrap"
      ref={wrapRef}
      onMouseEnter={show}
      onMouseLeave={hide}
      onMouseDown={hide}
    >
      {children}
      {anchor &&
        createPortal(
          <div
            ref={tipRef}
            className="tt-box"
            style={
              style ?? { position: "fixed", visibility: "hidden", left: -9999, top: -9999 }
            }
          >
            {content}
          </div>,
          document.body,
        )}
    </span>
  );
}
