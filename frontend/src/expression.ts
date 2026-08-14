import type { Expr } from "./types";
import { FILTER_FIELDS } from "./filters";
import { leaf, group } from "./exprbase";

export { leaf, group, isGroup } from "./exprbase";

// 表达式（JQL 风格）：解析 / 序列化 / 树操作。
// 语法：字段 操作符 值 [AND|OR 字段 操作符 值 ...]，支持括号与引号；
// in 的多值列表必须写在括号内（in(a, b)），逗号不能并列语句。
// 尾部可接最小函数管道：| select(ori|thumb|dup, 可多个) / order(field, asc|desc) /
// take(n) / drop(n)，语义顺序 select → order → drop → take。
// 不设 first/tail 别名（= take(1)/drop(1)，避免功能重复）。
// 操作符：= != ~（包含，LIKE %x%） in > >= < <=

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
  if (e.c) {
    // in 的列表序列化为括号列表（树内存储为逗号连接的字符串，
    // 与后端 matchOne 的 split(",") 契约一致）。
    if (e.c.op === "in") {
      const vals = e.c.value
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      return `${e.c.field} in (${vals.map(quoteValue).join(", ")})`;
    }
    return `${e.c.field} ${OPS_SYM[e.c.op] ?? e.c.op} ${quoteValue(e.c.value)}`;
  }
  const isOr = Array.isArray(e.or);
  const parts = (e.or ?? e.and ?? []).map((sub) => {
    const inner = exprToText(sub);
    // 任何嵌套组都加括号，忠实镜像列表视图的分组结构（含单子组）。
    // 解析后单子组与其子条件语义等价，规范化消失无害。
    const kids = sub.or ?? sub.and;
    return kids ? `(${inner})` : inner;
  });
  return parts.join(isOr ? " OR " : " AND ");
}

// ---- 解析 ----

type Tok =
  | { t: "word"; v: string }
  | { t: "str"; v: string }
  | { t: "op"; v: string }
  | { t: "pipe" }
  | { t: "comma" }
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
    if (ch === "|") {
      // 管道分隔符：此前被单词扫描吞掉，任何带 | 的表达式都会报
      // 「无法解析」——管道（order/take/drop/select）全部失效。
      toks.push({ t: "pipe" });
      i++;
      continue;
    }
    if (ch === ",") {
      // 逗号是独立记号：曾是「并进单词 + 剥尾逗号」的隐式并列（biz in pic,
      // size>0 这类迷惑语法由此而来）。现在裸逗号一律报错，列表只出现在
      // in(...) / select(...) / order(...) 的括号内。
      toks.push({ t: "comma" });
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
    // 单词（逗号/空格/括号为界；括号内的函数参数由逗号记号分隔）
    let j = i;
    while (j < text.length && !/[\s(),]/.test(text[j])) {
      const c2 = text[j];
      if ("=><~".includes(c2) && j > i) break; // 值紧跟操作符时截断
      j++;
    }
    const w = text.slice(i, j);
    if (!w) {
      i = j;
      continue;
    }
    // `in` 是关键字操作符（左栏业务类型多选会序列化成 `biz in pic,video`）；
    // 曾作为普通单词解析导致「缺少操作符」——in 值里不允许出现字段名 in。
    if (w.toLowerCase() === "in") toks.push({ t: "op", v: "in" });
    else toks.push({ t: "word", v: w });
    i = j;
  }
  return toks;
}

// order() 可用的排序字段（与后端 sortIDs 一致）
export const ORDERABLE_FIELDS = ["size", "mtime", "month", "md5"];

export interface ParseResult {
  expr: Expr | null;
  error?: string;
  limit?: number;
  offset?: number;
  orders?: { field: string; desc: boolean }[];
  select?: string[]; // select(ori|thumb|dup)
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
    const op = SYM_OPS[o.v] ?? o.v;
    if (op === "in") {
      // in 的列表必须写在括号内：in (a, b)（逗号是列表分隔符，不再能
      // 并列语句）；单值写法 in pic 保留兼容（左栏多选序列化为列表）。
      if (peek()?.t === "lparen") {
        pos++;
        const vals: string[] = [];
        for (;;) {
          const vt = next();
          if (!vt || (vt.t !== "word" && vt.t !== "str")) {
            return { expr: null, error: "in(...) 的列表里缺少值（含空格的值请加引号）" };
          }
          vals.push(vt.v);
          const nxt = peek();
          if (nxt?.t === "comma") {
            pos++;
            continue;
          }
          if (nxt?.t === "rparen") {
            pos++;
            break;
          }
          return { expr: null, error: "in(...) 列表值之间用逗号分隔，如 in (pic, video)" };
        }
        return { expr: leaf(f.v, op, vals.join(",")) };
      }
      const v = next();
      if (!v || (v.t !== "word" && v.t !== "str")) {
        return { expr: null, error: "in 后缺少值（多值列表写在括号内，如 biz in (pic, video)）" };
      }
      return { expr: leaf(f.v, op, v.v) };
    }
    const v = next();
    if (!v || (v.t !== "word" && v.t !== "str")) {
      return { expr: null, error: `操作符后缺少值（含空格的值请加引号）` };
    }
    return { expr: leaf(f.v, op, v.v) };
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

  // 纯管道表达式（无过滤条件，如 select(ori, thumb) | take(100)）：首个
  // 记号若是管道函数名，跳过条件解析直接进管道循环（否则会被当字段名
  // 报「未知字段」——filterToText 对无条件筛选就序列化出这种形式）。
  const firstTok = toks[0];
  let startsWithPipeFn =
    firstTok?.t === "word" &&
    ["select", "order", "take", "drop"].includes(firstTok.v.toLowerCase());
  const res = startsWithPipeFn ? { expr: null } : parseOr();
  if (res.error) return res;

  // 尾部最小函数管道：| select(ori|thumb|dup) / order(field, asc|desc) /
  // take(n) / drop(n)。首轮允许省略 |（纯管道表达式），之后每轮必须 |
  // 分隔。
  let limit: number | undefined;
  let offset: number | undefined;
  let select: string[] | undefined;
  const orders: { field: string; desc: boolean }[] = [];
  let t = peek();
  while (t && (t.t === "pipe" || startsWithPipeFn)) {
    if (t.t === "pipe") pos++;
    startsWithPipeFn = false;
    const fnTok = next();
    if (!fnTok || fnTok.t !== "word") {
      return {
        expr: null,
        error: "| 后需要函数名：select(ori|thumb|dup) / order(field, asc|desc) / take(n) / drop(n)",
      };
    }
    const fn = fnTok.v.toLowerCase();
    if (fn === "select") {
      if (peek()?.t !== "lparen") {
        return { expr: null, error: `select 需要括号参数，如 select(ori, thumb)` };
      }
      pos++;
      const kinds: string[] = [];
      for (;;) {
        const kindTok = next();
        if (!kindTok || (kindTok.t !== "word" && kindTok.t !== "str")) {
          return { expr: null, error: `select() 的参数必须是 ori / thumb / dup（可多个，逗号分隔）` };
        }
        const part = kindTok.v.toLowerCase();
        if (!["ori", "thumb", "dup"].includes(part)) {
          return { expr: null, error: `select() 的参数必须是 ori / thumb / dup 之一（收到「${part}」）` };
        }
        if (!kinds.includes(part)) kinds.push(part);
        const nxt = peek();
        if (nxt?.t === "comma") {
          pos++;
          continue;
        }
        if (nxt?.t === "rparen") {
          pos++;
          break;
        }
        return { expr: null, error: "select() 参数用逗号分隔，如 select(ori, thumb)" };
      }
      select = kinds;
      t = peek();
      continue;
    }
    if (fn === "order") {
      if (peek()?.t !== "lparen") {
        return { expr: null, error: `order 需要括号参数，如 order(size, desc)` };
      }
      pos++;
      const fieldTok = next();
      if (!fieldTok || fieldTok.t !== "word" || !ORDERABLE_FIELDS.includes(fieldTok.v)) {
        return {
          expr: null,
          error: `order() 的字段必须是 ${ORDERABLE_FIELDS.join(" / ")} 之一`,
        };
      }
      if (peek()?.t !== "comma") {
        return { expr: null, error: "order() 的字段与方向用逗号分隔，如 order(size, desc)" };
      }
      pos++;
      const dirTok = next();
      const dir = dirTok?.t === "word" ? dirTok.v.toLowerCase() : "";
      if (dir !== "asc" && dir !== "desc") {
        return { expr: null, error: `order() 的方向只能是 asc 或 desc` };
      }
      if (peek()?.t !== "rparen") return { expr: null, error: `order() 括号未闭合` };
      pos++;
      orders.push({ field: fieldTok.v, desc: dir === "desc" });
      t = peek();
      continue;
    }
    if (fn === "take" || fn === "drop") {
      if (peek()?.t !== "lparen") {
        return { expr: null, error: `函数 ${fn} 需要括号参数，如 ${fn}(10)` };
      }
      pos++;
      const numTok = next();
      if (!numTok || (numTok.t !== "word" && numTok.t !== "str") || !/^\d+$/.test(numTok.v)) {
        return { expr: null, error: `${fn}() 的参数必须是非负整数` };
      }
      if (peek()?.t === "comma") {
        return { expr: null, error: `${fn}() 只能有一个参数` };
      }
      if (peek()?.t !== "rparen") return { expr: null, error: `${fn}() 括号未闭合` };
      pos++;
      const n = Number(numTok.v);
      if (fn === "take") limit = n;
      else offset = n;
      t = peek();
      continue;
    }
    return { expr: null, error: `未知管道函数「${fnTok.v}」（可用 select / order / take / drop）` };
  }

  if (pos < toks.length) {
    const t = toks[pos];
    if (
      t.t === "word" &&
      ["select", "order", "take", "drop"].includes(t.v.toLowerCase())
    ) {
      return { expr: null, error: `无法解析「${t.v}」：管道函数之间用 | 连接` };
    }
    if (t.t === "comma") {
      // 裸逗号不再并列语句（此前 biz in pic, size>0 一类写法报
      // 「无法解析「size」」让人费解）：给出指向性的错误。
      return {
        expr: null,
        error: "无法解析「,」：不能用逗号并列条件——条件之间用 AND/OR 连接，in 的多值列表写在括号内（如 biz in (pic, video)）",
      };
    }
    const v =
      t.t === "str"
        ? `"${t.v}"`
        : t.t === "word" || t.t === "op"
          ? t.v
          : t.t === "lparen"
            ? "("
            : t.t === "rparen"
              ? ")"
              : "|";
    return { expr: null, error: `无法解析「${v}」：两个条件之间需要 AND 或 OR 连接` };
  }
  return { expr: res.expr ?? null, limit, offset, orders, select };
}

// filterToText 序列化完整筛选（表达式 + select/order/drop/take 管道，
// 规范顺序与后端 applyStages 一致：select → order → drop → take）。
export function filterToText(
  expr: Expr | null | undefined,
  limit?: number,
  offset?: number,
  orders?: { field: string; desc: boolean }[],
  select?: string[],
): string {
  let s = exprToText(expr);
  if (select && select.length > 0) s = s ? `${s} | select(${select.join(", ")})` : `select(${select.join(", ")})`;
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
