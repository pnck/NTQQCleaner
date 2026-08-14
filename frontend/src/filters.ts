import type { Condition, Expr, Sort, Stage } from "./types";
import { group, leaf } from "./exprbase";

// 筛选器领域：字段定义 + 命名筛选（= 名称 + 表达式 + 排序 + 是否置顶工具栏）。

export interface FilterFieldDef {
  field: string;
  label: string;
  kind: "enum" | "bool" | "text" | "number";
  options?: { value: string; label: string }[]; // enum
  unit?: string; // number 单位（显示用）
  toBytes?: boolean; // size：输入 MB，存储字节
  ops: string[];
}

export const OPS_LABEL: Record<string, string> = {
  eq: "等于",
  ne: "不等于",
  in: "属于",
  gt: "大于",
  gte: "大于等于",
  lt: "小于",
  lte: "小于等于",
  contains: "包含",
};

export const FILTER_FIELDS: FilterFieldDef[] = [
  {
    field: "biz",
    label: "业务类型",
    kind: "enum",
    ops: ["in", "ne"],
    options: [
      { value: "pic", label: "图片" },
      { value: "video", label: "视频" },
      { value: "ptt", label: "语音消息" },
      { value: "emoji", label: "表情" },
      { value: "file", label: "文件" },
      { value: "dataline", label: "设备互传" },
    ],
  },
  {
    field: "sub",
    label: "子类型",
    kind: "enum",
    ops: ["in", "ne"],
    options: [
      { value: "Ori", label: "Ori（原图）" },
      { value: "Thumb", label: "Thumb（缩略图）" },
      { value: "OriTemp", label: "OriTemp（下载残留）" },
      { value: "ThumbTemp", label: "ThumbTemp（下载残留）" },
      { value: "file_assistant", label: "file_assistant" },
    ],
  },
  {
    field: "category",
    label: "分类",
    kind: "text",
    ops: ["contains", "eq"],
  },
  {
    field: "month",
    label: "月份",
    kind: "text",
    ops: ["eq", "ne", "gte", "lte", "gt", "lt", "in"],
  },
  {
    field: "age",
    label: "修改于",
    kind: "number",
    unit: "天前",
    ops: ["gte", "lte", "gt", "lt", "in"],
  },
  {
    field: "size",
    label: "大小",
    kind: "number",
    unit: "MB",
    toBytes: true,
    ops: ["gte", "lte", "gt", "lt"],
  },
  {
    field: "fileId",
    label: "文件ID",
    kind: "text",
    ops: ["contains", "eq"],
  },
  {
    field: "contentHash",
    label: "内容哈希",
    kind: "text",
    ops: ["contains", "eq"],
  },
  {
    field: "reason",
    label: "说明",
    kind: "text",
    ops: ["contains", "eq"],
  },
  { field: "thumb", label: "是缩略图", kind: "bool", ops: ["eq", "ne"] },
  { field: "temp", label: "是 *Temp 残留", kind: "bool", ops: ["eq", "ne"] },
];

export function fieldDef(field: string): FilterFieldDef | undefined {
  return FILTER_FIELDS.find((f) => f.field === field);
}

// ---- 命名筛选（就叫「筛选器」）----

export interface NamedFilter {
  name: string;
  expr: Expr | null;
  sort: Sort;
  stages?: Stage[]; // 管道（书写顺序执行）
  pinned?: boolean; // 固定到工具栏直选
}

const MB = 1024 * 1024;
export const DEFAULT_SORT: Sort = { field: "size", desc: true };

// SEED_FILTERS 仅是「首次启动」写入用户筛选器列表的种子：写入后它们就是
// 普通筛选器，与用户自建的完全同权（可重排/修改/置顶/删除）。之后每次
// 启动都以存储为准，删除即永久删除。
export const SEED_FILTERS: NamedFilter[] = [
  { name: "全部", expr: null, sort: DEFAULT_SORT, pinned: true },
  {
    name: "缩略图 · 90天前",
    expr: group("and", [leaf("thumb", "eq", "true"), leaf("age", "gte", "90")]),
    sort: { field: "size", desc: true },
    pinned: true,
  },
  {
    name: "下载残留（*Temp）",
    expr: leaf("temp", "eq", "true"),
    sort: { field: "size", desc: true },
    pinned: true,
  },
  {
    name: "一年前",
    expr: leaf("age", "gte", "365"),
    sort: { field: "mtime", desc: false },
    pinned: true,
  },
  {
    name: "大文件（>100MB）",
    expr: leaf("size", "gt", String(100 * MB)),
    sort: { field: "size", desc: true },
    pinned: true,
  },
  {
    name: "表情包",
    expr: leaf("category", "contains", "marketface"),
    sort: { field: "size", desc: true },
    pinned: true,
  },
];

const KEY = "ntqq-cleaner-named-filters";
// 种子合并标记：旧版本（内置筛选器硬编码在代码里）升级时只合并一次，
// 之后存储就是唯一权威——用户删除内置筛选器后不会在下次启动被复活。
const SEEDED_KEY = "ntqq-cleaner-named-filters-seeded";

// legacyStages 迁移旧存储的 select/orders/limit/offset 字段（旧规范顺序
// select → order → drop → take）为统一管道 stages。
function legacyStages(f: Record<string, unknown>): Stage[] | undefined {
  const out: Stage[] = [];
  const sel = f.select as string[] | undefined;
  if (sel && sel.length > 0) out.push({ kind: "select", kinds: sel });
  for (const o of (f.orders as { field: string; desc: boolean }[] | undefined) ?? []) {
    out.push({ kind: "order", field: o.field, desc: o.desc });
  }
  if (typeof f.offset === "number" && f.offset > 0) out.push({ kind: "drop", n: f.offset });
  if (typeof f.limit === "number" && f.limit > 0) out.push({ kind: "take", n: f.limit });
  return out.length > 0 ? out : undefined;
}

// normalizeLegacyNames 把已存筛选器里的旧语言词一次性映射为现行名
// （字段 md5 → fileId；select 维度 ori → origin；order 字段跟随字段名）。
function normalizeExpr(e: Expr): Expr {
  if (e.c) {
    return { c: e.c.field === "md5" ? { ...e.c, field: "fileId" } : e.c };
  }
  return { and: e.and?.map(normalizeExpr), or: e.or?.map(normalizeExpr) };
}

function normalizeStages(stages?: Stage[]): Stage[] | undefined {
  const out = (stages ?? []).map((s) => {
    if (s.kind === "select") {
      return { ...s, kinds: (s.kinds ?? []).map((k) => (k === "ori" ? "origin" : k)) };
    }
    if (s.kind === "order" && s.field === "md5") return { ...s, field: "fileId" };
    return s;
  });
  return out.length > 0 ? out : undefined;
}

// readStored 读取用户筛选器；兼容旧存储（conditions 数组迁移为 AND 组，
// select/orders/limit/offset 迁移为 stages）。
function readStored(): NamedFilter[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as Array<Record<string, unknown>>;
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((f): NamedFilter => {
        const conds = f.conditions as Condition[] | undefined;
        const expr = (f.expr as Expr | null) ?? (conds?.length ? { and: conds.map((c) => ({ c })) } : null);
        return {
          name: String(f.name ?? ""),
          expr: expr ? normalizeExpr(expr) : null,
          sort: (f.sort as Sort) ?? DEFAULT_SORT,
          stages: normalizeStages((f.stages as Stage[] | undefined) ?? legacyStages(f)),
          pinned: Boolean(f.pinned),
        };
      })
      .filter((f) => f.name !== "");
  } catch {
    return [];
  }
}

// loadFilters 返回完整筛选器列表。首次使用（或旧版本首次升级）时把内置
// 种子写入存储——从此种子与自定义筛选器地位完全相同。同名条目保留用户
// 已有的（用户版本优先），避免旧版本自定义筛选器与内置重名时产生重复。
export function loadFilters(): NamedFilter[] {
  const stored = readStored();
  if (localStorage.getItem(SEEDED_KEY) === "1") return stored;
  const names = new Set(stored.map((f) => f.name));
  const merged = [...SEED_FILTERS.filter((s) => !names.has(s.name)).map((s) => ({ ...s })), ...stored];
  localStorage.setItem(KEY, JSON.stringify(merged));
  localStorage.setItem(SEEDED_KEY, "1");
  return merged;
}

export function saveFilters(list: NamedFilter[]) {
  localStorage.setItem(KEY, JSON.stringify(list));
}
