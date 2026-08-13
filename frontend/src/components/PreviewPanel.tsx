import { useState } from "react";
import { api } from "../api";
import { explainReason } from "../reasons";
import type { FileRow } from "../types";
import { BIZ_LABEL, fmtSize, fmtTime } from "../types";
import { Tooltip } from "./Tooltip";

interface Props {
  row: FileRow | null;
  rows: FileRow[];
  onNavigate: (row: FileRow) => void;
}

// Large original files are not loaded automatically (docs/07 §4.2);
// the user must click explicitly.
const BIG = 50 << 20;

export function PreviewPanel({ row, rows, onNavigate }: Props) {
  const [forceBig, setForceBig] = useState(false);
  if (!row) {
    return (
      <aside className="preview">
        <div className="wall-empty">点选照片墙中的缩略图，在这里预览媒体内容。</div>
      </aside>
    );
  }
  const idx = rows.findIndex((r) => r.id === row.id);
  const prev = idx > 0 ? rows[idx - 1] : null;
  const next = idx >= 0 && idx < rows.length - 1 ? rows[idx + 1] : null;
  const big = row.size > BIG;
  const showMedia = !big || forceBig;

  return (
    <aside className="preview">
      <div className="nav">
        <button disabled={!prev} onClick={() => prev && onNavigate(prev)}>
          ←
        </button>
        <button disabled={!next} onClick={() => next && onNavigate(next)}>
          →
        </button>
        <span className="stage" style={{ marginLeft: "auto" }}>
          {idx + 1} / {rows.length}
        </span>
      </div>

      <div className="media">
        {showMedia ? (
          row.biz === "video" && row.oriUrl ? (
            <video src={row.oriUrl} controls preload="metadata" />
          ) : row.biz === "ptt" ? (
            <audio src={row.oriUrl || row.thumbUrl} controls preload="metadata" />
          ) : (
            <img src={row.thumbUrl || row.oriUrl} draggable={false} alt={row.md5} />
          )
        ) : (
          <div className="big-warn">
            原图 {fmtSize(row.size)}，不自动加载。
            <br />
            <button onClick={() => setForceBig(true)}>仍然加载</button>
          </div>
        )}
      </div>

      <div className="detail">
        <div className="kv">
          <span className="k">说明</span>
          <span style={{ display: "flex", gap: 4, flexWrap: "wrap", justifyContent: "flex-end" }}>
            {explainReason(row.reason).map((r) => (
              <Tooltip key={r.label} content={`${r.label}：${r.explain}`}>
                <span className="reason-chip">{r.label}</span>
              </Tooltip>
            ))}
          </span>
        </div>
        <div className="kv">
          <span className="k">类型</span>
          <span>
            {BIZ_LABEL[row.biz] ?? row.biz} · {row.sub}
            {row.month ? ` · ${row.month}` : ""}
          </span>
        </div>
        <div className="kv">
          <span className="k">大小</span>
          <span>{fmtSize(row.size)}</span>
        </div>
        <div className="kv">
          <span className="k">修改时间</span>
          <span>{fmtTime(row.mtime)}</span>
        </div>
        <div className="kv">
          <span className="k">md5</span>
          <span>{row.md5 || "—"}</span>
        </div>
        <div className="kv">
          <span className="k">缩略图</span>
          <span>{row.thumbUrl ? "有" : "无"}</span>
        </div>
        <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
          <button onClick={() => void api.reveal(row.id)}>在文件夹中显示</button>
        </div>
      </div>
    </aside>
  );
}
