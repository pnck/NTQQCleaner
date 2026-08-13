import { useRef, useState } from "react";
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

// 大文件门限只针对「原文件」中的图片：图片无法流式，>50MB 需显式确认；
// 视频/音频可流式，点击播放即切换播放器。
const BIG_IMAGE = 50 << 20;

const IMG_EXTS = ["png", "jpg", "jpeg", "gif", "webp", "bmp", "heic", "avif", "ico"];
const VIDEO_EXTS = ["mp4", "mov", "m4v", "webm", "mkv"];
const AUDIO_EXTS = ["amr", "silk", "mp3", "wav", "m4a", "aac"];

type Kind = "img" | "video" | "audio" | "card";

// 依据「选中的那个源」判断渲染方式：缩略图一律是图片；
// 原文件按扩展名/业务类型分派。
function kindOf(row: FileRow, useOri: boolean): Kind {
  if (!useOri) return "img";
  const ext = row.ext.toLowerCase();
  if (row.biz === "ptt" || AUDIO_EXTS.includes(ext)) return "audio";
  if (VIDEO_EXTS.includes(ext)) return "video";
  if (IMG_EXTS.includes(ext)) return "img";
  return "card";
}

// 复制到剪贴板：优先 Clipboard API，降级 execCommand（老 WebView 兼容）。
function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text).catch(() => legacyCopy(text));
  } else {
    legacyCopy(text);
  }
}

function legacyCopy(text: string) {
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  document.execCommand("copy");
  ta.remove();
}

// 可「播放」的媒体（视频/语音/动图）叠 ▶；静态图片叠 ⤢（查看原文件）。
function playable(row: FileRow): boolean {
  const ext = row.ext.toLowerCase();
  return (
    row.biz === "video" ||
    row.biz === "ptt" ||
    VIDEO_EXTS.includes(ext) ||
    ["gif", "webp"].includes(ext)
  );
}

export function PreviewPanel({ row, rows, onNavigate }: Props) {
  // 初始态 = 缩略图 + 叠层图标；点击后切换为播放器/原文件（视频/音频即自动播放）。
  // 状态按 row.id 记录，切行时自动回到初始态。
  const [played, setPlayed] = useState<number | null>(null);
  const [forceBig, setForceBig] = useState<number | null>(null);
  const [copied, setCopied] = useState(false);
  const copyTimer = useRef<number | null>(null);

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
  const full = !hasThumb || played === row.id; // 无缩略图时直接原文件
  const kind = kindOf(row, full);
  const src = full ? row.oriUrl : row.thumbUrl;
  const bigImageGate = kind === "img" && full && row.size > BIG_IMAGE && forceBig !== row.id;
  const showOverlay = hasThumb && hasOri && !full;

  const onCopyMd5 = () => {
    copyText(row.md5);
    setCopied(true);
    if (copyTimer.current) window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopied(false), 1500);
  };

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
        {showOverlay && (
          <button
            className="media-overlay"
            onClick={() => setPlayed(row.id)}
            title={playable(row) ? "点击播放" : "点击查看原文件"}
          >
            <span className="play-badge">{playable(row) ? "▶" : "⤢"}</span>
          </button>
        )}
        {bigImageGate ? (
          <div className="big-warn">
            原文件 {fmtSize(row.size)}，不自动加载。
            <br />
            <button onClick={() => setForceBig(row.id)}>仍然加载</button>
          </div>
        ) : kind === "video" ? (
          <video key={src} src={src} controls autoPlay />
        ) : kind === "audio" ? (
          <audio key={src} src={src} controls autoPlay />
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
          {row.md5 ? (
            <span style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
              <span className="selectable">{row.md5}</span>
              <button className="mini" onClick={onCopyMd5} title="复制 md5">
                {copied ? "已复制" : "复制"}
              </button>
            </span>
          ) : (
            <span>—</span>
          )}
        </div>
        <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
          <button onClick={() => void api.reveal(row.id)}>在文件夹中显示</button>
        </div>
      </div>
    </aside>
  );
}
