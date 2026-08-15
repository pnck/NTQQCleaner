import { useCallback, useEffect, useRef, useState } from "react";
import type { SyntheticEvent } from "react";
import { api } from "../api";
import { explainReason } from "../reasons";
import type { FileRow } from "../types";
import { BIZ_LABEL, SUB_LABEL, fmtSize, fmtTime } from "../types";
import { Splitter } from "./Splitter";
import { Tooltip } from "./Tooltip";

interface Props {
  width: number; // 拖拽分隔条调整的栏宽（App 持有并记忆）
  row: FileRow | null;
  rows: FileRow[];
  // 勾选模式：dups = 只勾副本（保留 keeper）；all = 勾选全部（含 keeper）。
  // 按钮文案 = 本次点击执行的动作（dups →「勾选副本」，all →「勾选全部」），
  // 点击后由 App 翻转到另一模式，反复点击来回切换。
  dupMode: "dups" | "all";
  onNavigate: (row: FileRow) => void;
  onToast: (msg: string) => void;
  onSelectDups: (row: FileRow) => void;
}

// 大文件门限只针对「原文件」中的图片：图片无法流式，>50MB 需显式确认；
// 视频/音频可流式，点击播放即切换播放器。
const BIG_IMAGE = 50 << 20;

// 媒体/详情上下分栏的持久化键与默认高度。
const MEDIA_H_KEY = "preview-media-h";
const DEFAULT_MEDIA_H = 340;

// ScrollEnd：可选中值（右对齐 + 溢出滚动）。内容变化时自动滚到行尾，
// 保证右对齐下可见的是值的尾部（文件名/哈希后缀）；用户可自由往回
// 滚动查看前段。滚动条完全隐藏（styles.css）——截断不常驻滚动条，
// 拖选/滚轮仍可滚动。
function ScrollEnd({ text, className }: { text: string; className: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const prevText = useRef<string | null>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || prevText.current === text) return;
    prevText.current = text;
    el.scrollLeft = el.scrollWidth;
  }, [text]);
  return (
    <span ref={ref} className={className}>
      {text}
    </span>
  );
}

const IMG_EXTS = ["png", "jpg", "jpeg", "gif", "webp", "bmp", "heic", "avif", "ico"];
const VIDEO_EXTS = ["mp4", "mov", "m4v", "webm", "mkv"];
const AUDIO_EXTS = ["amr", "silk", "mp3", "wav", "m4a", "aac"];

type Kind = "img" | "video" | "audio" | "card";

// 依据「选中的那个源」判断渲染方式：缩略图一律是图片；
// 原文件按其自身扩展名（OriExt，后端取自配对原文件，而非行自身 ext）
// 与业务类型分派。
function kindOf(row: FileRow, useOri: boolean): Kind {
  if (!useOri) return "img";
  const ext = (row.oriExt || row.ext).toLowerCase();
  if (row.biz === "ptt" || AUDIO_EXTS.includes(ext)) return "audio";
  if (VIDEO_EXTS.includes(ext)) return "video";
  if (IMG_EXTS.includes(ext)) return "img";
  return "card";
}

// 可「播放」的媒体（视频/语音/动图）叠 ▶；静态图片叠 ⤢（查看原文件）。
// 判断依据同样取原文件（OriExt），避免视频缩略图行被误判为静态图片。
function playable(row: FileRow): boolean {
  const ext = (row.oriExt || row.ext).toLowerCase();
  return (
    row.biz === "video" ||
    row.biz === "ptt" ||
    VIDEO_EXTS.includes(ext) ||
    ["gif", "webp"].includes(ext)
  );
}

// 音量/静音与自动播放是**全局单例**（模块级）：切换行/切换媒体后状态
// 保留；原生控件的改动经 volumechange 回流到单例。
const volumeState = { v: 1, muted: false };
const autoLoopState = { on: false };

// Player：**常驻播放器单例**。视频/音频两个元素各一个、跨行保活
// （display 切换，绝不重挂载），切换媒体只改 src——音量/静音/循环参数
// 随元素自持，原生音量滑块不会每次切换后「先出现再移动」（此前
// key={src} 每次切 src 都新建元素，原生控件从默认值重新初始化）。
// 挂载时在 ref 回调里（先于首次绘制）套用单例音量/静音。
// **起播必须显式门控（autoStart）**：src 装载后无条件 play() 在 Windows
// WebView2 上会无视「自动播放」开关直接起播（WebView2 的自动播放策略比
// WebKit 宽松，不拦非手势 play）。autoStart = 自动播放开关 ∨ 显式播放
// 意图（▶ 叠层点击/空格）；为 false 时只装载不播放，由原生控件/空格
// 启动。自动起播发生在 effect 期、不在用户手势上下文——先静音起播、
// 'playing' 后恢复元素既有静音态。
function Player({
  kind,
  src,
  autoStart,
  autoLoop,
  onActiveEl,
}: {
  kind: "video" | "audio" | null; // null = 隐藏待命（当前行非媒体/未进入播放器）
  src: string;
  autoStart: boolean; // 起播门控（开关 ∨ 显式播放意图）；与 loop 属性独立
  autoLoop: boolean; // 自动循环 = 常驻元素 loop 属性
  onActiveEl: (el: HTMLVideoElement | HTMLAudioElement | null) => void;
}) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const prev = useRef<{ kind: string; src: string; autoStart: boolean }>({
    kind: "",
    src: "",
    autoStart: false,
  });

  // 循环开关：两个元素同步（自动循环 = 常驻元素 loop 属性）。
  useEffect(() => {
    if (videoRef.current) videoRef.current.loop = autoLoop;
    if (audioRef.current) audioRef.current.loop = autoLoop;
  }, [autoLoop]);

  // 活动元素切换时套用单例音量/静音（元素隐藏时设置无可见跳变），并向
  // 父级暴露活动元素（空格播放/暂停用）。
  useEffect(() => {
    const el =
      kind === "video" ? videoRef.current : kind === "audio" ? audioRef.current : null;
    if (el) {
      el.volume = volumeState.v;
      el.muted = volumeState.muted;
    }
    onActiveEl(el);
  }, [kind, onActiveEl]);

  // src 装载与起播。prev 守卫：StrictMode 下 effect 双跑时第二次直接
  // 返回——否则两轮 muted 恢复监听叠加，静音态会被还原错。autoStart
  // 翻转同样触发（开关勾上时当前媒体立即起播），此时 src 未变、不重设
  // （重设会让播放中的媒体从头开始）。
  useEffect(() => {
    if (kind !== "video" && kind !== "audio") {
      // 隐藏待命：停掉仍在播放的旧媒体——display:none 不会暂停播放，
      // 切行后旧视频在后台继续输出音频（严格单例的一致性缺口：先自动
      // 播放 A、取消开关、切到 B，A 的音频仍在响）。
      videoRef.current?.pause();
      audioRef.current?.pause();
      // 重新进入同一媒体时视为新的起播决策：否则 prev 守卫会把恢复
      // 播放误判为「已处理」直接返回，▶ 点击后媒体停在暂停态。
      prev.current.autoStart = false;
      return;
    }
    const el = kind === "video" ? videoRef.current : audioRef.current;
    if (!el || !src) return;
    const same = prev.current.kind === kind && prev.current.src === src;
    if (same && prev.current.autoStart === autoStart) return;
    if (!same) el.src = src;
    prev.current = { kind, src, autoStart };
    if (!autoStart) return;
    const wasMuted = el.muted;
    el.muted = true;
    const restore = () => {
      el.removeEventListener("playing", restore);
      el.muted = wasMuted;
    };
    el.addEventListener("playing", restore);
    void el.play().catch(() => {});
  }, [kind, src, autoStart]);

  const applySingleton = (el: HTMLVideoElement | HTMLAudioElement | null) => {
    if (!el) return;
    el.volume = volumeState.v;
    el.muted = volumeState.muted;
  };
  const volumeSync = (e: SyntheticEvent<HTMLVideoElement | HTMLAudioElement>) => {
    volumeState.v = e.currentTarget.volume;
    volumeState.muted = e.currentTarget.muted;
  };

  return (
    <div className="player-host" style={{ display: kind ? "flex" : "none" }}>
      <video
        ref={(el) => {
          videoRef.current = el;
          applySingleton(el);
        }}
        controls
        style={{ display: kind === "video" ? "" : "none", width: "100%", height: "100%", objectFit: "contain" }}
        onVolumeChange={volumeSync}
      />
      <audio
        ref={(el) => {
          audioRef.current = el;
          applySingleton(el);
        }}
        controls
        style={{ display: kind === "audio" ? "" : "none", width: "100%" }}
        onVolumeChange={volumeSync}
      />
    </div>
  );
}

export function PreviewPanel({ width, row, rows, dupMode, onNavigate, onToast, onSelectDups }: Props) {
  // 初始态 = 缩略图 + 叠层图标；点击后切换为播放器/原文件（起播由
  // autoStart 门控：开关开或用户显式点击 ▶/空格）。状态按 row.id 记录——
  // 切行后旧行的 played 自然失效（full 按当前行判定），无需随行重置。
  // 播放器元素常驻（Player），不随行重挂载。
  const [played, setPlayed] = useState<number | null>(null);
  const [forceBig, setForceBig] = useState<number | null>(null);
  const mediaRef = useRef<HTMLVideoElement | HTMLAudioElement | null>(null);
  const [autoLoop, setAutoLoop] = useState(autoLoopState.on);
  // 显式播放意图（▶ 叠层点击 / 空格起播）：与自动播放开关取或后作为
  // Player 的 autoStart。切行时复位——无缩略图的行直接进入播放器视图
  // 不得自动起播（Windows WebView2 自动播放策略宽松，装 src 即播）。
  const playIntentRef = useRef(false);
  // 媒体/详情上下分栏：媒体区高度由分隔条拖动调整（localStorage 记忆，
  // 与左右栏宽同一模式），详情区占剩余高度并内部滚动。
  const panelRef = useRef<HTMLElement | null>(null);
  const [mediaH, setMediaH] = useState(() => {
    const v = Number(localStorage.getItem(MEDIA_H_KEY));
    return Number.isFinite(v) && v > 0 ? v : DEFAULT_MEDIA_H;
  });
  useEffect(() => {
    localStorage.setItem(MEDIA_H_KEY, String(mediaH));
  }, [mediaH]);

  // 活动媒体元素回写（空格播放/暂停用；Player 在 kind 变化时回调）。
  const setMediaRef = useCallback(
    (el: HTMLVideoElement | HTMLAudioElement | null) => {
      mediaRef.current = el;
    },
    [],
  );

  // 切行回到初始态（缩略图叠层）：面板不再随行重挂载（播放器常驻），
  // played/forceBig 按 row.id 记录——重扫后行 id 会复用，行变化时必须
  // 清掉旧行状态。ref 守卫只在行 id 真正变化时执行（autoLoop 开关翻转
  // 不打断正在播放的媒体）；自动播放开时让位给下方 effect 直接进播放器。
  const lastRowId = useRef<number | null>(null);
  useEffect(() => {
    if (!row) {
      lastRowId.current = null;
      return;
    }
    if (lastRowId.current === row.id) return;
    lastRowId.current = row.id;
    if (autoLoop) return;
    playIntentRef.current = false;
    setPlayed(null);
    setForceBig(null);
  }, [row, autoLoop]);

  // 自动播放开：聚焦行变化/勾选时自动进入播放器并起播（常驻播放器只换
  // src）。勾选语义 = 「循环自动播放当前聚焦的媒体」——切换到下一个媒体
  // 同样自动开始。无缩略图的行本就直接原文件（full 恒真）。**不做
  // playable 预判**：扩展名与 MIME 常不一致（jpg 实为 webp 等），WebView
  // 按内容嗅探可以播放/渲染的文件会被 playable() 误拒——有原文件就进
  // 播放器视图，交给元素自身尽力渲染/播放。
  useEffect(() => {
    if (!autoLoop || !row) return;
    const hasThumb = row.thumbUrl !== "";
    const fullNow = !hasThumb || played === row.id;
    if (hasThumb && row.oriUrl !== "" && !fullNow) {
      setPlayed(row.id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row, autoLoop]);

  // 空格 = 播放/暂停当前媒体（键盘操作无说明文案）；焦点在输入控件时
  // 不接管。媒体尚未切换（仍是缩略图叠层）时直接开始播放。
  useEffect(() => {
    if (!row) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "Space") return;
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select") || t.isContentEditable) return;
      e.preventDefault();
      const el = mediaRef.current;
      if (el && "paused" in el) {
        if (el.paused) void el.play();
        else el.pause();
        return;
      }
      // 尚未切换到播放器（缩略图 + 叠层状态）：空格直接开始播放
      const hasThumb = row.thumbUrl !== "";
      const full = !hasThumb || played === row.id;
      if (hasThumb && row.oriUrl !== "" && !full && playable(row)) {
        playIntentRef.current = true;
        setPlayed(row.id);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [row, played]);

  if (!row) {
    return (
      <aside className="preview" style={{ width }}>
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
  const hashText = row.contentHash
    ? row.contentHash + (row.contentDupCount < 2 ? " · 无重复（同大小但内容不同）" : "")
    : "未计算（大小唯一）";
  const bigImageGate = kind === "img" && full && row.size > BIG_IMAGE && forceBig !== row.id;
  // 缩略图叠层（播放/查看切换控件）只在**未启用自动播放**时存在：
  // 自动播放开 = 直接进入原文件视图（叠层完全消失），交给元素按内容
  // 嗅探尽力播放/渲染（扩展名搞错的可播放媒体不再被 playable 误拒）。
  const showOverlay = hasThumb && hasOri && !full && !autoLoop;

  const reveal = () =>
    void api.reveal(row.id).catch((e) => onToast(`无法在文件夹中显示：${e}`));

  return (
    <aside className="preview" style={{ width }} ref={panelRef}>
      <div className="nav">
        <button disabled={!prev} onClick={() => prev && onNavigate(prev)}>
          ←
        </button>
        <button disabled={!next} onClick={() => next && onNavigate(next)}>
          →
        </button>
        <label className="autoplay-toggle" title="勾选后循环自动播放：当前媒体立即开始，切换到下一个媒体同样自动开始">
          <input
            type="checkbox"
            checked={autoLoop}
            onChange={(e) => {
              // 只切开关：起播由自动播放 effect 统一驱动（勾选/切行同一
              // 条路径）。不做「可播放性」预判——扩展名与 MIME 常不一致
              // （如 jpg 实为 webp），交给元素自身尽力播放即可。
              autoLoopState.on = e.target.checked;
              setAutoLoop(e.target.checked);
            }}
          />
          自动播放
        </label>
        <span className="stage" style={{ marginLeft: "auto" }}>
          {idx + 1} / {rows.length}
        </span>
      </div>

      <div className="media" style={{ height: mediaH }}>
        {showOverlay && (
          <button
            className="media-overlay"
            onClick={() => {
              playIntentRef.current = true;
              setPlayed(row.id);
            }}
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
        ) : kind === "card" ? (
          <div className="big-warn">
            此类型不支持内嵌预览
            <br />
            <button onClick={reveal}>在文件夹中显示</button>
          </div>
        ) : kind === "img" ? (
          <img src={src} draggable={false} alt={row.md5} />
        ) : null}
        {/* 常驻播放器：媒体行以外/未进入播放器时隐藏待命（元素不卸载，
            音量等参数保留）；src 只在进入播放器后装载 */}
        <Player
          kind={kind === "video" || kind === "audio" ? kind : null}
          src={full ? row.oriUrl : ""}
          autoStart={autoLoop || playIntentRef.current}
          autoLoop={autoLoop}
          onActiveEl={setMediaRef}
        />
      </div>

      <Splitter
        axis="y"
        onDrag={(dy) => {
          const max = (panelRef.current?.clientHeight ?? 600) - 160;
          setMediaH((h) => Math.min(max, Math.max(160, h + dy)));
        }}
      />

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
          <span className="k">路径</span>
          {/* 绝对路径（docs/07 §4.4）：仅展示；预览/删除仍按 id 走 Go 侧
              白名单，路径字符串不给前端任何文件访问能力 */}
          <ScrollEnd text={row.path} className="selectable path" />
        </div>
        <div className="kv">
          <span className="k">类型</span>
          <span>
            {BIZ_LABEL[row.biz] ?? row.biz} · {SUB_LABEL[row.sub] ?? row.sub}
            {row.month ? ` · ${row.month}` : ""}
          </span>
        </div>
        <div className="kv">
          <span className="k">大小</span>
          <span>{fmtSize(row.size)}</span>
        </div>
        <div className="kv">
          <span className="k">修改时间</span>
          <ScrollEnd text={fmtTime(row.mtime)} className="selectable" />
        </div>
        <div className="kv">
          <Tooltip content="QQ 的文件识别方式：原文件名的 md5 用作文件 ID（标识文件，不代表内容；与下方内容哈希无关）">
            <span className="k">文件ID</span>
          </Tooltip>
          <ScrollEnd text={row.md5 || "—"} className="selectable" />
        </div>
        <div className="kv">
          <span className="k">内容哈希</span>
          {/* 完整 64 位哈希（可选中复制到筛选器，contentHash ~ 前缀 匹配）； */}
          <ScrollEnd text={hashText} className="selectable hash" />
        </div>
        <div className="detail-actions">
          <button onClick={reveal}>在文件夹中显示</button>
          {row.contentDupCount >= 2 && (
            <button
              onClick={() => onSelectDups(row)}
              title={
                dupMode === "all"
                  ? "当前已勾选全部：点击回到只勾副本（保留一份不勾）"
                  : "勾选全部副本（保留一份不勾）：点击后再点一次可切换为勾选全部"
              }
            >
              同内容 {row.contentDupCount} 份 · {dupMode === "all" ? "勾选全部" : "勾选副本"}
            </button>
          )}
        </div>
      </div>
    </aside>
  );
}
