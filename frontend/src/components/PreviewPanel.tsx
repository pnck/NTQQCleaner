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

// 大文件门限只针对「原图」：图片无法流式，>50MB 需显式确认；
// 视频/音频用 preload="none"，播放器立即渲染、点击播放才拉流。
const BIG_IMAGE = 50 << 20;

const IMG_EXTS = ["png", "jpg", "jpeg", "gif", "webp", "bmp", "heic", "avif", "ico"];
const VIDEO_EXTS = ["mp4", "mov", "m4v", "webm", "mkv"];
const AUDIO_EXTS = ["amr", "silk", "mp3", "wav", "m4a", "aac"];

type Kind = "img" | "video" | "audio" | "card";

// 依据「选中的那个源」判断渲染方式：缩略图一律是图片；
// 原图按扩展名/业务类型分派。
function kindOf(row: FileRow, useOri: boolean): Kind {
  if (!useOri) return "img";
  const ext = row.ext.toLowerCase();
  if (row.biz === "ptt" || AUDIO_EXTS.includes(ext)) return "audio";
  if (VIDEO_EXTS.includes(ext)) return "video";
  if (IMG_EXTS.includes(ext)) return "img";
  return "card";
}

export function PreviewPanel({ row, rows, onNavigate }: Props) {
  const [useOri, setUseOri] = useState(false);
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

  const hasThumb = row.thumbUrl !== "";
  const hasOri = row.oriUrl !== "";
  // 视频/语音/动图（gif/webp）默认直接给原文件（preload=none 不产生流量）；
  // 普通图片默认缩略图（秒开），可切换原图
  const defaultOri =
    row.biz === "video" ||
    row.biz === "ptt" ||
    ["gif", "webp"].includes(row.ext.toLowerCase());
  const effectiveOri = hasOri && (useOri || !hasThumb || defaultOri);
  const kind = kindOf(row, effectiveOri);
  const src = effectiveOri ? row.oriUrl : row.thumbUrl;
  const bigImageGate = kind === "img" && effectiveOri && row.size > BIG_IMAGE && !forceBig;

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

      {hasThumb && hasOri && (
        <div className="nav">
          <button
            className={!effectiveOri ? "primary" : ""}
            onClick={() => setUseOri(false)}
            title="查看缩略图（秒开）"
          >
            缩略图
          </button>
          <button
            className={effectiveOri ? "primary" : ""}
            onClick={() => setUseOri(true)}
            title="查看原文件（动图/视频/原图）"
          >
            原图
          </button>
        </div>
      )}

      <div className="media">
        {bigImageGate ? (
          <div className="big-warn">
            原图 {fmtSize(row.size)}，不自动加载。
            <br />
            <button onClick={() => setForceBig(true)}>仍然加载</button>
          </div>
        ) : kind === "video" ? (
          <video src={src} controls preload="none" />
        ) : kind === "audio" ? (
          <audio src={src} controls preload="none" />
        ) : kind === "card" ? (
          <div className="big-warn">
            此类型不支持内嵌预览
            <br />
            <button onClick={() => void api.reveal(row.id)}>在文件夹中显示</button>
          </div>
        ) : (
          <img src={src} draggable={false} alt={row.md5} />
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
        <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
          <button onClick={() => void api.reveal(row.id)}>在文件夹中显示</button>
        </div>
      </div>
    </aside>
  );
}
