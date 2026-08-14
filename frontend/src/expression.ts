import type { Expr, Stage } from "./types";
import { FILTER_FIELDS } from "./filters";
import { leaf, group } from "./exprbase";

export { leaf, group, isGroup } from "./exprbase";

// ============================================================
// 轻量表达式引擎：Lexer（词法）→ Parser（递归下降语法）→
// Builder（序列化：每个 AST 节点自述）。结构参照 GORM 的
// clause.Expression/Build 模式——节点类型负责自己的 Build，
// 组装处只做接线，不做针对某个写法的特判。
//
// 语法（docs/04 §3）：
//   text      := [expr] [pipeline]
//   expr      := orExpr
//   orExpr    := andExpr ("OR" andExpr)*
//   andExpr   := unary ("AND" unary)*
//   unary     := "(" orExpr ")" | condition
//   condition := FIELD op operand
//   op        := "=" | "!=" | "~" | "in" | ">" | ">=" | "<" | "<="
//              | "after" | "before"
//   after/before 是 >/< 的可读别名（age/month 等时间性字段的自然写法），
//   解析期规范化为 gt/lt（序列化回显为 >/<）。
//   operand   := WORD | STRING            （in 固定为括号列表，见下）
//   size 的 WORD 可为带二进制单位的数字：1k/1m/1g/1t（解析期折算为字节）
//   inOperand := "(" WORD ("," WORD)* ")"
//   pipeline  := stepFirst ["|" step]*
//   stepFirst := step                     （文本开头可省略 |）
//   step      := "select" "(" KIND ("," KIND)* ")"   KIND ∈ origin|thumb|dup
//              | "order" "(" FIELD "," ("asc"|"desc") ")"
//              | ("take"|"drop") "(" INT ")"
//
// 管道按书写顺序从左到右组合：take(10) | select(dup) 与
// select(dup) | take(10) 语义不同。
// ============================================================

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
  after: "gt",
  before: "lt",
  ">": "gt",
  ">=": "gte",
  "<": "lt",
  "<=": "lte",
};

// 管道函数名（解析器的 FIRST 集与步进循环共用）。
const STEP_NAMES = ["select", "order", "take", "drop"];

// select() 的合法维度（docs/04 §3：正交并集）。
const SELECT_KINDS = ["origin", "thumb", "dup"];

// order() 可用的排序字段（与后端 sortIDs 一致）
export const ORDERABLE_FIELDS = ["size", "mtime", "month", "fileId"];

// ---- 词法 ----

type Tok =
  | { t: "word"; v: string }
  | { t: "str"; v: string }
  | { t: "op"; v: string }
  | { t: "pipe" }
  | { t: "comma" }
  | { t: "lparen" }
  | { t: "rparen" };

class Lexer {
  constructor(private src: string) {}

  scan(): Tok[] {
    const toks: Tok[] = [];
    let i = 0;
    while (i < this.src.length) {
      const ch = this.src[i];
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
        toks.push({ t: "pipe" });
        i++;
        continue;
      }
      if (ch === ",") {
        toks.push({ t: "comma" });
        i++;
        continue;
      }
      if (ch === '"' || ch === "'") {
        const quote = ch;
        let j = i + 1;
        let v = "";
        while (j < this.src.length && this.src[j] !== quote) {
          v += this.src[j];
          j++;
        }
        if (j >= this.src.length) return toks; // 未闭合引号 → 解析器报错
        toks.push({ t: "str", v });
        i = j + 1;
        continue;
      }
      // 操作符（长优先）
      const two = this.src.slice(i, i + 2);
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
      // 单词：空白/括号/逗号为界；值紧跟操作符时截断
      let j = i;
      while (j < this.src.length && !/[\s(),]/.test(this.src[j])) {
        const c2 = this.src[j];
        if ("=><~".includes(c2) && j > i) break;
        j++;
      }
      const w = this.src.slice(i, j);
      if (!w) {
        i = j;
        continue;
      }
      // in / after / before 是操作符关键字（after/before = >/< 别名）
      const kw = w.toLowerCase();
      if (kw === "in" || kw === "after" || kw === "before") toks.push({ t: "op", v: kw });
      else toks.push({ t: "word", v: w });
      i = j;
    }
    return toks;
  }
}

// ---- 语法 ----

export interface ParseResult {
  expr: Expr | null;
  stages: Stage[];
  error?: string;
}

class Parser {
  private pos = 0;

  constructor(private toks: Tok[]) {}

  parse(): ParseResult {
    const exprRes = this.parseExprPart();
    if (exprRes.error) return { expr: null, stages: [], error: exprRes.error };
    const stRes = this.parsePipeline(exprRes.expr !== null);
    if (stRes.error) return { expr: null, stages: [], error: stRes.error };
    if (this.pos < this.toks.length) {
      return { expr: null, stages: [], error: this.trailingError() };
    }
    return { expr: exprRes.expr, stages: stRes.stages };
  }

  private peek(): Tok | undefined {
    return this.toks[this.pos];
  }

  private next(): Tok | undefined {
    return this.toks[this.pos++];
  }

  private isStepName(v: string): boolean {
    return STEP_NAMES.includes(v.toLowerCase());
  }

  // 表达式部分：以管道记号/管道函数名开头 = 无条件（FIRST 集前瞻）。
  private parseExprPart(): { expr: Expr | null; error?: string } {
    const t = this.peek();
    if (!t) return { expr: null };
    if (t.t === "pipe" || (t.t === "word" && this.isStepName(t.v))) {
      return { expr: null };
    }
    const r = this.parseOr();
    return { expr: r.expr ?? null, error: r.error };
  }

  // 管道：文本开头的首步可省略 |；其后每步必须 | 分隔。
  private parsePipeline(exprPresent: boolean): { stages: Stage[]; error?: string } {
    const stages: Stage[] = [];
    if (!exprPresent) {
      const t = this.peek();
      if (t?.t === "word" && this.isStepName(t.v)) {
        const s = this.parseStep();
        if (s.error) return { stages: [], error: s.error };
        stages.push(s.stage);
      }
    }
    while (this.peek()?.t === "pipe") {
      this.pos++;
      const fnTok = this.peek();
      if (!fnTok || fnTok.t !== "word" || !this.isStepName(fnTok.v)) {
        return {
          stages: [],
          error: "| 后需要函数名：select(origin|thumb|dup) / order(field, asc|desc) / take(n) / drop(n)",
        };
      }
      const s = this.parseStep();
      if (s.error) return { stages: [], error: s.error };
      stages.push(s.stage);
    }
    return { stages };
  }

  private parseStep(): { stage: Stage; error?: string } {
    const fnTok = this.next();
    if (!fnTok || fnTok.t !== "word") {
      return {
        stage: { kind: "select", kinds: [] },
        error: "| 后需要函数名：select(origin|thumb|dup) / order(field, asc|desc) / take(n) / drop(n)",
      };
    }
    const name = fnTok.v.toLowerCase();
    switch (name) {
      case "select":
        return this.parseSelect();
      case "order":
        return this.parseOrder();
      case "take":
      case "drop":
        return this.parseInt(name);
      default:
        return { stage: { kind: "select", kinds: [] }, error: `未知管道函数「${fnTok.v}」` };
    }
  }

  // 括号单词列表："(" WORD ("," WORD)* ")"。in 操作数与 select(...) 复用。
  private parseWordList(): { words: string[]; error?: string } {
    this.pos++; // 消费 "("（调用方已确认）
    const words: string[] = [];
    for (;;) {
      const v = this.next();
      if (!v || (v.t !== "word" && v.t !== "str")) {
        return { words: [], error: "列表里缺少值（含空格的值请加引号）" };
      }
      if (!words.includes(v.v)) words.push(v.v);
      const n = this.peek();
      if (n?.t === "comma") {
        this.pos++;
        continue;
      }
      if (n?.t === "rparen") {
        this.pos++;
        return { words };
      }
      return { words: [], error: "列表值之间用逗号分隔" };
    }
  }

  private parseSelect(): { stage: Stage; error?: string } {
    if (this.peek()?.t !== "lparen") {
      return { stage: { kind: "select", kinds: [] }, error: "select 需要括号参数，如 select(origin, thumb)" };
    }
    const l = this.parseWordList();
    if (l.error) return { stage: { kind: "select", kinds: [] }, error: l.error };
    const kinds = l.words.map((w) => w.toLowerCase());
    const bad = kinds.find((k) => !SELECT_KINDS.includes(k));
    if (bad) {
      return {
        stage: { kind: "select", kinds: [] },
        error: `select() 的参数必须是 origin / thumb / dup 之一（收到「${bad}」）`,
      };
    }
    return { stage: { kind: "select", kinds } };
  }

  private parseOrder(): { stage: Stage; error?: string } {
    if (this.peek()?.t !== "lparen") {
      return { stage: { kind: "order" }, error: "order 需要括号参数，如 order(size, desc)" };
    }
    this.pos++;
    const fieldTok = this.next();
    if (!fieldTok || fieldTok.t !== "word" || !ORDERABLE_FIELDS.includes(fieldTok.v)) {
      return {
        stage: { kind: "order" },
        error: `order() 的字段必须是 ${ORDERABLE_FIELDS.join(" / ")} 之一`,
      };
    }
    if (this.peek()?.t !== "comma") {
      return { stage: { kind: "order" }, error: "order() 的字段与方向用逗号分隔，如 order(size, desc)" };
    }
    this.pos++;
    const dirTok = this.next();
    const dir = dirTok?.t === "word" ? dirTok.v.toLowerCase() : "";
    if (dir !== "asc" && dir !== "desc") {
      return { stage: { kind: "order" }, error: "order() 的方向只能是 asc 或 desc" };
    }
    if (this.peek()?.t !== "rparen") {
      return { stage: { kind: "order" }, error: "order() 括号未闭合" };
    }
    this.pos++;
    return { stage: { kind: "order", field: fieldTok.v, desc: dir === "desc" } };
  }

  private parseInt(name: "take" | "drop"): { stage: Stage; error?: string } {
    if (this.peek()?.t !== "lparen") {
      return { stage: { kind: name, n: 0 }, error: `${name} 需要括号参数，如 ${name}(10)` };
    }
    this.pos++;
    const numTok = this.next();
    if (!numTok || (numTok.t !== "word" && numTok.t !== "str") || !/^\d+$/.test(numTok.v)) {
      return { stage: { kind: name, n: 0 }, error: `${name}() 的参数必须是非负整数` };
    }
    if (this.peek()?.t === "comma") {
      return { stage: { kind: name, n: 0 }, error: `${name}() 只能有一个参数` };
    }
    if (this.peek()?.t !== "rparen") {
      return { stage: { kind: name, n: 0 }, error: `${name}() 括号未闭合` };
    }
    this.pos++;
    return { stage: { kind: name, n: Number(numTok.v) } };
  }

  // ---- 条件布尔树 ----

  private parseCondition(): { expr: Expr | null; error?: string } {
    const f = this.next();
    if (!f || f.t !== "word") {
      return { expr: null, error: `第 ${this.pos + 1} 个记号处应有字段名` };
    }
    if (!FILTER_FIELDS.some((d) => d.field === f.v)) {
      return {
        expr: null,
        error: `未知字段「${f.v}」（可用：${FILTER_FIELDS.map((d) => d.field).join(" / ")}）`,
      };
    }
    const def = FILTER_FIELDS.find((d) => d.field === f.v)!;
    // 数值字段（size，toBytes）：值必须是数字，可带二进制单位后缀
    // k/m/g/t（1k=1024B…），解析期折算为规范字节值存入树。
    const norm = (raw: string): string | null => {
      if (!def.toBytes) return raw;
      const m = /^(\d+)([kmgt]?)$/i.exec(raw);
      if (!m) return null;
      const mult: Record<string, number> = { "": 1, k: 1024, m: 1024 ** 2, g: 1024 ** 3, t: 1024 ** 4 };
      return String(Number(m[1]) * mult[m[2].toLowerCase()]);
    };
    const o = this.next();
    if (!o || o.t !== "op") {
      return { expr: null, error: `字段「${f.v}」后缺少操作符（= != ~ in after before > >= < <=）` };
    }
    const op = SYM_OPS[o.v] ?? o.v;
    const sizeErr = (raw: string) =>
      `${f.v} 的值必须是数字（可带单位 k/m/g/t，如 1g；收到「${raw}」）`;
    if (op === "in") {
      // in 的操作数固定为括号列表（树内存储为逗号连接的字符串，
      // 与后端 matchOne 的 split(",") 契约一致）。
      if (this.peek()?.t !== "lparen") {
        return { expr: null, error: `in 的列表必须写在括号内（如 biz in (pic, video)）` };
      }
      const l = this.parseWordList();
      if (l.error) return { expr: null, error: l.error };
      const vals = l.words.map(norm);
      const bad = vals.findIndex((v) => v === null);
      if (bad >= 0) return { expr: null, error: sizeErr(l.words[bad]) };
      return { expr: leaf(f.v, op, vals.join(",")) };
    }
    const v = this.next();
    if (!v || (v.t !== "word" && v.t !== "str")) {
      return { expr: null, error: "操作符后缺少值（含空格的值请加引号）" };
    }
    const nv = norm(v.v);
    if (nv === null) return { expr: null, error: sizeErr(v.v) };
    return { expr: leaf(f.v, op, nv) };
  }

  // 递归下降：orExpr := andExpr (OR andExpr)*
  private parseOr(): { expr: Expr | null; error?: string } {
    const first = this.parseAnd();
    if (first.error) return first;
    const items: Expr[] = [first.expr!];
    let t = this.peek();
    while (t && t.t === "word" && t.v.toUpperCase() === "OR") {
      this.pos++;
      const r = this.parseAnd();
      if (r.error) return r;
      items.push(r.expr!);
      t = this.peek();
    }
    return { expr: items.length === 1 ? items[0] : group("or", items) };
  }

  private parseAnd(): { expr: Expr | null; error?: string } {
    const first = this.parseUnary();
    if (first.error) return first;
    const items: Expr[] = [first.expr!];
    let t = this.peek();
    while (t && t.t === "word" && t.v.toUpperCase() === "AND") {
      this.pos++;
      const r = this.parseUnary();
      if (r.error) return r;
      items.push(r.expr!);
      t = this.peek();
    }
    return { expr: items.length === 1 ? items[0] : group("and", items) };
  }

  private parseUnary(): { expr: Expr | null; error?: string } {
    const t = this.peek();
    if (t?.t === "lparen") {
      this.pos++;
      const inner = this.parseOr();
      if (inner.error) return inner;
      if (this.peek()?.t !== "rparen") return { expr: null, error: "括号未闭合" };
      this.pos++;
      return { expr: inner.expr };
    }
    if (t && t.t === "word" && ["AND", "OR"].includes(t.v.toUpperCase())) {
      return { expr: null, error: `「${t.v}」前缺少表达式` };
    }
    return this.parseCondition();
  }

  // 无法消化的剩余记号：按记号类型给出指向性错误。
  private trailingError(): string {
    const t = this.toks[this.pos];
    const v =
      t.t === "str"
        ? `"${t.v}"`
        : t.t === "word" || t.t === "op"
          ? t.v
          : t.t === "lparen"
            ? "("
            : t.t === "rparen"
              ? ")"
              : t.t === "pipe"
                ? "|"
                : ",";
    switch (t.t) {
      case "comma":
        return `无法解析「,」：不能用逗号并列条件——条件之间用 AND/OR 连接，列表写在括号内（如 biz in (pic, video)）`;
      case "word":
        if (this.isStepName(t.v)) return `无法解析「${t.v}」：管道函数之间用 | 连接`;
        return `无法解析「${v}」：两个条件之间需要 AND 或 OR 连接`;
      default:
        return `无法解析「${v}」：两个条件之间需要 AND 或 OR 连接`;
    }
  }
}

export function parseExpr(text: string): ParseResult {
  const trimmed = text.trim();
  if (trimmed === "") return { expr: null, stages: [] };
  return new Parser(new Lexer(trimmed).scan()).parse();
}

// ---- Builder（序列化：每个 AST 节点自述，GORM clause.Build 模式）----

function quoteValue(v: string): string {
  return /[\s()]/.test(v) ? `"${v}"` : v;
}

function buildCondition(c: Expr["c"]): string {
  if (!c) return "";
  if (c.op === "in") {
    const vals = c.value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    return `${c.field} in (${vals.map(quoteValue).join(", ")})`;
  }
  return `${c.field} ${OPS_SYM[c.op] ?? c.op} ${quoteValue(c.value)}`;
}

export function exprToText(e: Expr | null | undefined): string {
  if (!e) return "";
  if (e.c) return buildCondition(e.c);
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

function buildStage(s: Stage): string {
  switch (s.kind) {
    case "select":
      return `select(${(s.kinds ?? []).join(", ")})`;
    case "order":
      return `order(${s.field}, ${s.desc ? "desc" : "asc"})`;
    case "drop":
    case "take":
      return `${s.kind}(${s.n})`;
  }
}

// filterToText 序列化完整筛选（表达式 + 管道，按书写顺序连接）。
export function filterToText(expr: Expr | null | undefined, stages?: Stage[]): string {
  const parts: string[] = [];
  const e = exprToText(expr);
  if (e) parts.push(e);
  parts.push(...(stages ?? []).map(buildStage));
  return parts.join(" | ");
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

// ---- 工具栏搜索（文件ID 或 内容哈希 任一匹配）----
// 搜索词是搜索框的专有构造：顶层 AND 中形如
// (fileId ~ q OR contentHash ~ q) 的 OR 组，在表达式里**至多一个**。
// 编辑时原位替换，绝不追加——旧实现按叶子谓词过滤移除旧条件，OR 组
// 不命中谓词导致每次击键堆叠一组 hash 条件，且回读取第一个旧组的值，
// 受控输入被锁死。
const SEARCH_FIELDS = ["fileId", "contentHash"] as const;

export function isSearchGroup(s: Expr | null | undefined): boolean {
  if (!s) return false;
  const kids = s.or;
  if (!kids || kids.length !== SEARCH_FIELDS.length) return false;
  return SEARCH_FIELDS.every((f, i) => {
    const c = kids[i]?.c;
    return c !== null && c !== undefined && c.field === f && c.op === "contains";
  });
}

function searchShape(q: string): Expr {
  return group("or", [
    leaf(SEARCH_FIELDS[0], "contains", q),
    leaf(SEARCH_FIELDS[1], "contains", q),
  ]);
}

export function getSearchExpr(root: Expr | null | undefined): string {
  // 根本身就是搜索组（表达式视图解析单组表达式后的形态）也算。
  if (root && isSearchGroup(root)) return root.or?.[0]?.c?.value ?? "";
  const kids = root?.and ?? root?.or ?? [];
  const g = kids.find(isSearchGroup);
  return g?.or?.[0]?.c?.value ?? "";
}

export function setSearchExpr(root: Expr | null | undefined, q: string): Expr | null {
  // 根本身就是搜索组：整体替换/移除（整体替换同样保证至多一个）。
  if (root && isSearchGroup(root)) {
    return q === "" ? null : searchShape(q);
  }
  // 搜索组是顶层组的一个子项（AND 或 OR 皆可——表达式视图解析会把
  // 单子 AND 组规范化为 OR 组），原位替换保证至多一个。顶层是叶子时
  // 把叶子作为整体保留、搜索组并列——不覆盖丢失用户的既有条件。
  let kind: "and" | "or" = "and";
  let children: Expr[];
  if (!root) children = [];
  else if (Array.isArray(root.and)) {
    kind = "and";
    children = [...root.and];
  } else if (Array.isArray(root.or)) {
    kind = "or";
    children = [...root.or];
  } else {
    children = [root];
  }
  const idx = children.findIndex(isSearchGroup);
  if (q === "") {
    if (idx >= 0) children.splice(idx, 1);
  } else if (idx >= 0) {
    children[idx] = searchShape(q);
  } else {
    children.push(searchShape(q));
  }
  return children.length > 0 ? group(kind, children) : null;
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
