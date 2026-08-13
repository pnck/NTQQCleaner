import type { Condition, Expr, Sort } from "./types";
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
    ops: ["eq", "ne", "gte", "lte"],
  },
  {
    field: "age",
    label: "修改于",
    kind: "number",
    unit: "天前",
    ops: ["gte", "lte", "gt", "lt"],
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
    field: "md5",
    label: "md5",
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
  orders?: { field: string; desc: boolean }[]; // 表达式 order() 管道
  limit?: number; // take(n)
  offset?: number; // drop(n)
  builtin?: boolean;
  pinned?: boolean; // 固定到工具栏直选
}

const MB = 1024 * 1024;
const DEFAULT_SORT: Sort = { field: "size", desc: true };

export const BUILTIN_FILTERS: NamedFilter[] = [
  { name: "全部", expr: null, sort: DEFAULT_SORT, builtin: true },
  {
    name: "缩略图 · 90天前",
    expr: group("and", [leaf("thumb", "eq", "true"), leaf("age", "gte", "90")]),
    sort: { field: "size", desc: true },
    builtin: true,
  },
  {
    name: "下载残留（*Temp）",
    expr: leaf("temp", "eq", "true"),
    sort: { field: "size", desc: true },
    builtin: true,
  },
  {
    name: "一年前",
    expr: leaf("age", "gte", "365"),
    sort: { field: "mtime", desc: false },
    builtin: true,
  },
  {
    name: "大文件（>100MB）",
    expr: leaf("size", "gt", String(100 * MB)),
    sort: { field: "size", desc: true },
    builtin: true,
  },
  {
    name: "表情包",
    expr: leaf("category", "contains", "marketface"),
    sort: { field: "size", desc: true },
    builtin: true,
  },
];

const KEY = "qq-cleaner-named-filters";

// loadUserFilters 读取用户筛选器；兼容旧存储（conditions 数组迁移为 AND 组）。
export function loadUserFilters(): NamedFilter[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as Array<Record<string, unknown>>;
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((f): NamedFilter => {
        const conds = f.conditions as Condition[] | undefined;
        return {
          name: String(f.name ?? ""),
          expr: (f.expr as Expr | null) ?? (conds?.length ? { and: conds.map((c) => ({ c })) } : null),
          sort: (f.sort as Sort) ?? DEFAULT_SORT,
          pinned: Boolean(f.pinned),
        };
      })
      .filter((f) => f.name !== "");
  } catch {
    return [];
  }
}

export function saveUserFilters(list: NamedFilter[]) {
  localStorage.setItem(KEY, JSON.stringify(list));
}

export function allFilters(user: NamedFilter[]): NamedFilter[] {
  return [...BUILTIN_FILTERS, ...user];
}
