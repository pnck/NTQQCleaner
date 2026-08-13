import type { Expr } from "./types";
import { FILTER_FIELDS } from "./filters";
import { leaf, group } from "./exprbase";

export { leaf, group, isGroup } from "./exprbase";

// 表达式（JQL 风格）：解析 / 序列化 / 树操作。
// 语法：字段 操作符 值 [AND|OR 字段 操作符 值 ...]，支持括号与引号；
// 尾部可接最小函数管道：| take(n)（取前 n）/ drop(n)（跳过前 n）。
// 不设 first/tail 别名（= take(1)/drop(1)，避免功能重复）。
// 操作符：= != ~（包含） in > >= < <=

export const OPS_SYM: Record<string, string> = {
  eq: "=",
  ne: "!=",
  contains: "~",
  in: "in",
  gt: ">",
  gte: ">=",
  lt: "<",
  lte: "<=",
};

const SYM_OPS: Record<string, string> = {
  "=": "eq",
  "!=": "ne",
  "~": "contains",
  in: "in",
  ">": "gt",
  ">=": "gte",
  "<": "lt",
  "<=": "lte",
};

// ---- 序列化 ----

function quoteValue(v: string): string {
  return /[\s()]/.test(v) ? `"${v}"` : v;
}

export function exprToText(e: Expr | null | undefined): string {
  if (!e) return "";
  if (e.c) return `${e.c.field} ${OPS_SYM[e.c.op] ?? e.c.op} ${quoteValue(e.c.value)}`;
  const isOr = Array.isArray(e.or);
  const parts = (e.or ?? e.and ?? []).map((sub) => {
    const inner = exprToText(sub);
    const kids = sub.or ?? sub.and;
    return kids && kids.length > 1 ? `(${inner})` : inner;
  });
  return parts.join(isOr ? " OR " : " AND ");
}

// ---- 解析 ----

type Tok =
  | { t: "word"; v: string }
  | { t: "str"; v: string }
  | { t: "op"; v: string }
  | { t: "lparen" }
  | { t: "rparen" };

function tokenize(text: string): Tok[] {
  const toks: Tok[] = [];
  let i = 0;
  while (i < text.length) {
    const ch = text[i];
    if (/\s/.test(ch)) {
      i++;
      continue;
    }
    if (ch === "(") {
      toks.push({ t: "lparen" });
      i++;
      continue;
    }
    if (ch === ")") {
      toks.push({ t: "rparen" });
      i++;
      continue;
    }
    if (ch === '"' || ch === "'") {
      const quote = ch;
      let j = i + 1;
      let v = "";
      while (j < text.length && text[j] !== quote) {
        v += text[j];
        j++;
      }
      if (j >= text.length) return toks; // 未闭合引号 → 解析器报错
      toks.push({ t: "str", v });
      i = j + 1;
      continue;
    }
    // 操作符（长优先）
    const two = text.slice(i, i + 2);
    if (two === "!=" || two === ">=" || two === "<=") {
      toks.push({ t: "op", v: two });
      i += 2;
      continue;
    }
    if ("=><~".includes(ch)) {
      toks.push({ t: "op", v: ch });
      i++;
      continue;
    }
    // 单词（逗号是 order(size, desc) 的参数分隔符，剥离不参与匹配）
    let j = i;
    while (j < text.length && !/[\s()]/.test(text[j])) {
      const c2 = text[j];
      if ("=><~".includes(c2) && j > i) break; // 值紧跟操作符时截断
      j++;
    }
    const w = text.slice(i, j).replace(/,+$/, "");
    if (w) toks.push({ t: "word", v: w });
    i = j;
  }
  return toks;
}

// order() 可用的排序字段（与后端 sortIDs 一致）
export const ORDERABLE_FIELDS = ["size", "mtime", "month", "md5", "tier"];

export interface ParseResult {
  expr: Expr | null;
  error?: string;
  limit?: number;
  offset?: number;
  orders?: { field: string; desc: boolean }[];
}

export function parseExpr(text: string): ParseResult {
  const trimmed = text.trim();
  if (trimmed === "") return { expr: null };
  const toks = tokenize(trimmed);
  let pos = 0;

  const peek = (): Tok | undefined => toks[pos];
  const next = (): Tok | undefined => toks[pos++];

  const isKeyword = (v: string) => ["AND", "OR"].includes(v.toUpperCase());

  const parseCondition = (): ParseResult => {
    const f = next();
    if (!f || f.t !== "word") return { expr: null, error: `第 ${pos + 1} 个记号处应有字段名` };
    if (!FILTER_FIELDS.some((d) => d.field === f.v)) {
      return {
        expr: null,
        error: `未知字段「${f.v}」（可用：${FILTER_FIELDS.map((d) => d.field).join(" / ")}）`,
      };
    }
    const o = next();
    if (!o || o.t !== "op") {
      return { expr: null, error: `字段「${f.v}」后缺少操作符（= != ~ in > >= < <=）` };
    }
    const v = next();
    if (!v || (v.t !== "word" && v.t !== "str")) {
      return { expr: null, error: `操作符后缺少值（含空格的值请加引号）` };
    }
    return { expr: leaf(f.v, SYM_OPS[o.v] ?? o.v, v.v) };
  };

  // 递归下降：orExpr := andExpr (OR andExpr)*
  const parseOr = (): ParseResult => {
    const first = parseAnd();
    if (first.error) return first;
    const items: Expr[] = [first.expr!];
    let t = peek();
    while (t && t.t === "word" && t.v.toUpperCase() === "OR") {
      pos++;
      const r = parseAnd();
      if (r.error) return r;
      items.push(r.expr!);
      t = peek();
    }
    return { expr: items.length === 1 ? items[0] : group("or", items) };
  };

  const parseAnd = (): ParseResult => {
    const first = parseUnary();
    if (first.error) return first;
    const items: Expr[] = [first.expr!];
    let t = peek();
    while (t && t.t === "word" && t.v.toUpperCase() === "AND") {
      pos++;
      const r = parseUnary();
      if (r.error) return r;
      items.push(r.expr!);
      t = peek();
    }
    return { expr: items.length === 1 ? items[0] : group("and", items) };
  };

  const parseUnary = (): ParseResult => {
    const t = peek();
    if (t?.t === "lparen") {
      pos++;
      const inner = parseOr();
      if (inner.error) return inner;
      if (peek()?.t !== "rparen") return { expr: null, error: "括号未闭合" };
      pos++;
      return { expr: inner.expr };
    }
    if (t && t.t === "word" && isKeyword(t.v)) {
      return { expr: null, error: `「${t.v}」前缺少表达式` };
    }
    return parseCondition();
  };

  const res = parseOr();
  if (res.error) return res;

  // 尾部最小函数管道：| order(field, asc|desc) / take(n) / drop(n)
  let limit: number | undefined;
  let offset: number | undefined;
  const orders: { field: string; desc: boolean }[] = [];
  let t = peek();
  while (t && t.t === "word") {
    const fn = t.v.toLowerCase();
    if (fn === "order") {
      pos++;
      if (peek()?.t !== "lparen") {
        return { expr: null, error: `order 需要括号参数，如 order(size, desc)` };
      }
      pos++;
      const fieldTok = next();
      const field = fieldTok && fieldTok.t !== "lparen" && fieldTok.t !== "rparen" ? fieldTok.v : "";
      if (!ORDERABLE_FIELDS.includes(field)) {
        return {
          expr: null,
          error: `order() 的字段必须是 ${ORDERABLE_FIELDS.join(" / ")} 之一`,
        };
      }
      const dirTok = next();
      const dir = dirTok && dirTok.t !== "lparen" && dirTok.t !== "rparen" ? dirTok.v.toLowerCase() : "desc";
      if (dir !== "asc" && dir !== "desc") {
        return { expr: null, error: `order() 的方向只能是 asc 或 desc` };
      }
      if (peek()?.t !== "rparen") return { expr: null, error: `order() 括号未闭合` };
      pos++;
      orders.push({ field, desc: dir === "desc" });
      t = peek();
      continue;
    }
    if (fn !== "take" && fn !== "drop") break;
    pos++;
    if (peek()?.t !== "lparen") {
      return { expr: null, error: `函数 ${fn} 需要括号参数，如 ${fn}(10)` };
    }
    pos++;
    const numTok = next();
    if (!numTok || (numTok.t !== "word" && numTok.t !== "str") || !/^\d+$/.test(numTok.v)) {
      return { expr: null, error: `${fn}() 的参数必须是非负整数` };
    }
    if (peek()?.t !== "rparen") return { expr: null, error: `${fn}() 括号未闭合` };
    pos++;
    const n = Number(numTok.v);
    if (fn === "take") limit = n;
    else offset = n;
    t = peek();
  }

  if (pos < toks.length) {
    const t = toks[pos];
    const v =
      t.t === "str"
        ? `"${t.v}"`
        : t.t === "word" || t.t === "op"
          ? t.v
          : t.t === "lparen"
            ? "("
            : ")";
    return { expr: null, error: `无法解析「${v}」：两个条件之间需要 AND 或 OR 连接` };
  }
  return { expr: res.expr ?? null, limit, offset, orders };
}

// filterToText 序列化完整筛选（表达式 + order/drop/take 管道，规范顺序）。
export function filterToText(
  expr: Expr | null | undefined,
  limit?: number,
  offset?: number,
  orders?: { field: string; desc: boolean }[],
): string {
  let s = exprToText(expr);
  for (const o of orders ?? []) {
    s = s ? `${s} | order(${o.field}, ${o.desc ? "desc" : "asc"})` : `order(${o.field}, ${o.desc ? "desc" : "asc"})`;
  }
  if (offset !== undefined) s = s ? `${s} | drop(${offset})` : `drop(${offset})`;
  if (limit !== undefined) s = s ? `${s} | take(${limit})` : `take(${limit})`;
  return s;
}

// ---- 树操作（顶层 AND 组快捷方式，供左栏/搜索等就地编辑）----

export function getInExpr(root: Expr | null | undefined, field: string): string[] {
  const leaves = (root?.and ?? []).filter((s) => s.c && s.c.field === field && s.c.op === "in");
  const c = leaves[0]?.c;
  if (!c) return [];
  return c.value.split(",").map((s) => s.trim()).filter(Boolean);
}

export function setInExpr(root: Expr | null | undefined, field: string, values: string[]): Expr | null {
  const children = (root?.and ?? []).filter((s) => !(s.c && s.c.field === field && s.c.op === "in"));
  if (values.length > 0) children.push(leaf(field, "in", values.join(",")));
  if (children.length === 0) return null;
  return group("and", children);
}

export function toggleInExpr(root: Expr | null | undefined, field: string, value: string): Expr | null {
  const cur = getInExpr(root, field);
  return setInExpr(
    root,
    field,
    cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value],
  );
}

export function getSimpleExpr(root: Expr | null | undefined, field: string): string {
  const c = (root?.and ?? []).find((s) => s.c && s.c.field === field)?.c;
  return c?.value ?? "";
}

export function setSimpleExpr(
  root: Expr | null | undefined,
  field: string,
  op: string,
  value: string,
): Expr | null {
  const children = (root?.and ?? []).filter((s) => !(s.c && s.c.field === field));
  if (value !== "") children.push(leaf(field, op, value));
  if (children.length === 0) return null;
  return group("and", children);
}

// ---- 通用树路径操作（编辑对话框用）----
// path: 子级索引数组；root = 顶层组。

export function childCount(e: Expr): number {
  return e.or?.length ?? e.and?.length ?? 0;
}

export function getChild(e: Expr, i: number): Expr | undefined {
  return e.or?.[i] ?? e.and?.[i];
}

export function updateTree(root: Expr | null, path: number[], upd: (e: Expr) => Expr | null): Expr | null {
  const clone = (e: Expr): Expr => ({ and: e.and?.map(clone), or: e.or?.map(clone), c: e.c ? { ...e.c } : null });
  const cloned = root ? clone(root) : null;
  if (!cloned) return upd(group("and", []));
  const rec = (node: Expr, idx: number): Expr | null => {
    if (idx === path.length) return upd(node);
    const i = path[idx];
    const kids = node.or ?? node.and ?? [];
    const sub = kids[i];
    if (!sub) return node;
    const next = rec(sub, idx + 1);
    const arr = kids.slice();
    if (next === null) arr.splice(i, 1);
    else arr[i] = next;
    if (node.or) return { or: arr };
    return { and: arr };
  };
  const out = rec(cloned, 0);
  if (!out) return null;
  if (out.and && out.and.length === 0 && path.length === 0) return null;
  if (out.or && out.or.length === 0 && path.length === 0) return null;
  return out;
}
