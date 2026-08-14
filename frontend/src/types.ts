// Mirrors the Go JSON contracts in internal/report + internal/app.
// Keep in sync when backend types change.

export interface AccountReport {
  hash: string;
  qqNum: string;
  ntData: string;
  latestMonth: string;
  totalFiles: number;
  totalSize: number;
  hashedFiles: number; // 参与内容哈希的文件数（二次扫描生效的直接证据）
}

export interface FileRow {
  id: number;
  md5: string; // 文件ID：QQ 取自原文件名的 md5（标识文件，不代表内容）
  biz: string;
  sub: string;
  month: string;
  size: number;
  mtime: number;
  ext: string;
  reason: string;
  thumbUrl: string;
  oriUrl: string;
  oriExt: string; // 配对原文件的扩展名（据此分派播放器），无原文件为空
  contentHash: string; // 文件字节的 SHA-256（仅同大小冲突组会计算；唯一大小为空）
  contentDupCount: number; // 字节级相同内容的副本总数（含自身；≥2 即重复出现）
}

// 条件叶子：{字段, 操作符, 值}。
export interface Condition {
  field: string;
  op: string;
  value: string;
}

// 布尔表达式树：AND/OR 组 + 条件叶子（括号 = 嵌套组）。
export interface Expr {
  and?: Expr[];
  or?: Expr[];
  c?: Condition | null;
}

// 管道 stage：与后端 internal/app/types.go 的 Stage JSON 契约一致。
// 管道按书写顺序从左到右组合，每个 stage 作用于前一 stage 的输出。
export interface Stage {
  kind: "select" | "order" | "drop" | "take";
  kinds?: string[]; // select: ori/thumb/dup（正交并集）
  field?: string; // order: size|mtime|month|md5
  desc?: boolean; // order: 降序
  n?: number; // drop/take: 条数
}

export interface Filter {
  account: string;
  expr?: Expr | null; // null/undefined = 全部
  stages?: Stage[]; // 管道（书写顺序执行）
}

export interface Stats {
  count: number;
  size: number;
}

export interface Sort {
  field: string; // size | mtime | month | md5
  desc: boolean;
}

export interface PageQuery {
  filter: Filter;
  sort: Sort;
  page: number;
  pageSize: number;
}

export interface PageResult {
  total: number;
  page: number;
  rows: FileRow[];
}

export interface GroupStat {
  key: string;
  count: number;
  size: number;
}

export interface ScanOptions {
  root: string;
  account: string;
  minAgeDays: number;
  minSize: number;
  onlyBizs: string[];
}

export interface ScanDone {
  root: string;
  accounts: AccountReport[];
  error: string;
}

export interface Progress {
  stage: string;
  done: number;
  total: number;
}

export interface CleanRequest {
  ids: number[];
  backupDir: string;
  force: boolean;
  confirmed: boolean;
  ignoreRunning?: boolean; // QQ 运行中仍清理（需二次确认）
}

// CleanItem：清理结果对话框中逐文件回显（审计日志仍是权威记录）。
export interface CleanItem {
  path: string;
  action: "move" | "remove" | "skip" | "fail";
  backupPath?: string;
  reason?: string; // skip/fail 的原因
  size: number;
}

export interface CleanResult {
  processed: number;
  moved: number;
  deleted: number;
  skipped: number;
  failed: number;
  bytesFreed: number;
  items: CleanItem[];
  errors: string[];
}

export interface Config {
  cleanTemp: boolean;
  cleanThumb: boolean;
  cleanOri: boolean;
  cleanBaseEmoji: boolean;
  cleanMarketface: boolean;
  cleanPersonalEmoji: boolean;
  cleanFile: boolean;
  cleanLog: boolean; // 高级 opt-in：运行日志
  cleanDatalineTmp: boolean; // 高级 opt-in：传输缓存（NFC 残留）
  cleanAvatar: boolean; // 高级 opt-in：头像缓存
  minFileSizeBytes: number;
  skipDirs: string[];
}

export const BIZ_LABEL: Record<string, string> = {
  pic: "图片",
  video: "视频",
  ptt: "语音消息",
  emoji: "表情",
  file: "文件",
  dataline: "设备互传",
  log: "运行日志",
  "log-cache": "日志缓存",
  avatar: "头像缓存",
};

export function fmtSize(n: number): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(1) + "G";
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + "M";
  if (n >= 1 << 10) return (n / (1 << 10)).toFixed(0) + "K";
  return n + "B";
}

export function fmtTime(unix: number): string {
  const d = new Date(unix * 1000);
  const pad = (x: number) => String(x).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export const emptyFilter = (): Filter => ({ account: "", expr: null });

// DupGroup：一份字节级相同的内容（SHA-256 分组）在全索引中有 ≥2 份时
// 的去重建言。
export interface DupGroup {
  hash: string;
  count: number;
  keepId: number;
  keepLabel: string;
  keepMtime: number;
  dupIds: number[];
  dupBytes: number;
  totalBytes: number;
}
