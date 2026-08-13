// Mirrors the Go JSON contracts in internal/report + internal/app.
// Keep in sync when backend types change.

export interface AccountReport {
  hash: string;
  qqNum: string;
  ntData: string;
  latestMonth: string;
  totalFiles: number;
  totalSize: number;
}

export interface FileRow {
  id: number;
  md5: string;
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

export interface OrderStage {
  field: string; // size|mtime|month|md5
  desc: boolean;
}

export interface Filter {
  account: string;
  expr?: Expr | null; // null/undefined = 全部
  select?: string[]; // select(ori|thumb|dup)：关联展开，可多个（正交并集）
  orders?: OrderStage[]; // order(field, asc|desc) 管道
  limit?: number; // take(n)：排序后取前 n 条
  offset?: number; // drop(n)：跳过前 n 条
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

export interface CleanResult {
  processed: number;
  moved: number;
  deleted: number;
  skipped: number;
  failed: number;
  bytesFreed: number;
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
