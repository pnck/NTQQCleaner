// 表达式树构造原语。独立成文件：expression.ts（解析/序列化）与
// filters.ts（字段定义/命名筛选）都依赖这里，避免循环 import
// （曾导致模块求值期 TDZ ReferenceError → 白屏）。

import type { Expr } from "./types";

export const leaf = (field: string, op: string, value: string): Expr => ({
  c: { field, op, value },
});

export const group = (kind: "and" | "or", children: Expr[] = []): Expr =>
  kind === "and" ? { and: children } : { or: children };

export const isGroup = (e: Expr | null | undefined): boolean => !!e && !e.c;
