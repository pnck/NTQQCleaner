import { useState, type ReactNode } from "react";

// 悬浮 tooltip：hover 显示内容（解释标签含义等），不阻塞点击。
export function Tooltip({ content, children }: { content: ReactNode; children: ReactNode }) {
  const [show, setShow] = useState(false);
  return (
    <span
      className="tt-wrap"
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
      onMouseDown={() => setShow(false)}
    >
      {children}
      {show && <span className="tt-box">{content}</span>}
    </span>
  );
}
