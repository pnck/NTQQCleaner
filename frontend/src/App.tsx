import { useCallback, useEffect, useMemo, useState } from "react";
import { api, events } from "./api";
import { BottomBar } from "./components/BottomBar";
import { DupesDialog } from "./components/DupesDialog";
import { FilterEditor } from "./components/FilterEditor";
import { LeftTree } from "./components/LeftTree";
import { PhotoWall } from "./components/PhotoWall";
import { PreviewPanel } from "./components/PreviewPanel";
import { SettingsDialog, getBackupDir } from "./components/SettingsDialog";
import { TopBar } from "./components/TopBar";
import {
  getInExpr,
  getSimpleExpr,
  setInExpr,
  setSimpleExpr,
  toggleInExpr,
} from "./expression";
import {
  allFilters,
  loadUserFilters,
  saveUserFilters,
  BUILTIN_FILTERS,
  type NamedFilter,
} from "./filters";
import { applyTheme, getTheme, nextTheme, type Theme } from "./theme";
import type {
  AccountReport,
  CleanResult,
  DupGroup,
  Expr,
  FileRow,
  GroupStat,
  PageQuery,
  Progress,
  Stats,
} from "./types";
import { fmtSize } from "./types";

// UI state machine (docs/07 §3): idle → scanning → ready → cleaning → done
type Phase = "idle" | "scanning" | "ready" | "cleaning";

// 工具栏排序按钮：再次点击同字段切换升降序。
const SORT_FIELDS = [
  { field: "size", label: "大小", descDefault: true },
  { field: "mtime", label: "时间", descDefault: true },
  { field: "month", label: "月份", descDefault: true },
] as const;

export default function App() {
  const [phase, setPhase] = useState<Phase>("idle");
  const [roots, setRoots] = useState<string[]>([]);
  const [root, setRoot] = useState("");
  const [accounts, setAccounts] = useState<AccountReport[]>([]);
  const [account, setAccount] = useState("");
  const [progress, setProgress] = useState<Progress>({ stage: "", done: 0, total: 0 });
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [scanGen, setScanGen] = useState(0);

  // 筛选状态 = 表达式树 + order/take/drop 管道（左栏/快捷控件/筛选器编辑器共享）
  const [expr, setExpr] = useState<Expr | null>(null);
  const [orders, setOrders] = useState<{ field: string; desc: boolean }[]>([]);
  const [limit, setLimit] = useState<number | undefined>(undefined);
  const [offset, setOffset] = useState<number | undefined>(undefined);
  const [sort, setSort] = useState<{ field: string; desc: boolean }>({ field: "size", desc: true });
  const [userFilters, setUserFilters] = useState<NamedFilter[]>(loadUserFilters);
  const [appliedFilter, setAppliedFilter] = useState("");
  const [moreOpen, setMoreOpen] = useState(false);
  const [selected, setSelected] = useState<number | null>(null);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [rows, setRows] = useState<FileRow[]>([]);
  const [stats, setStats] = useState<Stats>({ count: 0, size: 0 });
  const [bizGroups, setBizGroups] = useState<GroupStat[]>([]);
  const [monthGroups, setMonthGroups] = useState<GroupStat[]>([]);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [filterOpen, setFilterOpen] = useState(false);
  const [dupesOpen, setDupesOpen] = useState(false);
  const [dupes, setDupes] = useState<DupGroup[]>([]);
  const [theme, setTheme] = useState<Theme>(getTheme());

  const filter = useMemo(
    () => ({ account, expr, orders, limit, offset }),
    [account, expr, orders, limit, offset],
  );
  const queryKey = useMemo(
    () => JSON.stringify([expr, orders, limit, offset, sort, account, scanGen]),
    [expr, orders, limit, offset, sort, account, scanGen],
  );
  const pageQuery: PageQuery = useMemo(
    () => ({ filter, sort, page: 1, pageSize: 200 }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queryKey],
  );

  const changeTheme = useCallback((t: Theme) => {
    applyTheme(t);
    setTheme(t);
  }, []);
  const cycleTheme = useCallback(() => changeTheme(nextTheme(theme)), [theme, changeTheme]);

  // ---- startup ----
  useEffect(() => {
    void api.discoverRoots().then((rs) => {
      setRoots(rs);
      if (rs.length > 0) setRoot(rs[0]);
    });
    const offs = [
      events.onProgress(setProgress),
      events.onDone((d) => {
        setAccounts(d.accounts);
        setPhase("ready");
        setScanGen((g) => g + 1);
        setError(d.error || "");
        if (d.error) setToast(`扫描结束（含错误）：${d.error}`);
      }),
      events.onError((d) => {
        setPhase("idle");
        setError(d.error || "scan failed");
      }),
      events.onState((s) => {
        if (!s.scanning) setProgress({ stage: "", done: 0, total: 0 });
      }),
    ];
    return () => offs.forEach((off) => off());
  }, []);

  // ---- scan ----
  const startScan = useCallback(() => {
    if (!root) return;
    setPhase("scanning");
    setError("");
    setSelected(null);
    setChecked(new Set());
    setRows([]);
    void api
      .scan({ root, account, minAgeDays: 3, minSize: 0, onlyBizs: [], aggressive: false })
      .catch((e) => {
        setPhase("idle");
        setError(String(e));
      });
  }, [root, account]);

  const stopScan = useCallback(() => void api.stop(), []);

  // ---- stats / groups ----
  useEffect(() => {
    if (phase !== "ready") return;
    setSelected(null);
    setChecked(new Set());
    void api.getStats(filter).then(setStats).catch(console.error);
    const treeFilter = { account, expr: null };
    void api.getGroups(treeFilter, "biz").then(setBizGroups).catch(console.error);
    void api.getGroups(treeFilter, "month").then(setMonthGroups).catch(console.error);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey, phase]);

  // ---- 手动编辑 = 脱离筛选器（自定义状态）----
  const editExpr = useCallback((next: Expr | null) => {
    setAppliedFilter("");
    setExpr(next);
  }, []);
  const editExprFn = useCallback(
    (fn: (e: Expr | null) => Expr | null) => {
      setAppliedFilter("");
      setExpr(fn);
    },
    [],
  );

  const applyFilter = useCallback((f: NamedFilter) => {
    setExpr(f.expr ? JSON.parse(JSON.stringify(f.expr)) : null);
    setSort({ ...f.sort });
    setOrders((f.orders ?? []).map((o) => ({ ...o })));
    setLimit(f.limit);
    setOffset(f.offset);
    setAppliedFilter(f.name);
    setMoreOpen(false);
  }, []);

  const editStages = useCallback(
    (s: { limit?: number; offset?: number; orders?: { field: string; desc: boolean }[] }) => {
      setAppliedFilter("");
      setLimit(s.limit);
      setOffset(s.offset);
      setOrders(s.orders ?? []);
    },
    [],
  );

  // ---- 左栏与快捷控件 ----
  const toggleBiz = (biz: string) => editExprFn((e) => toggleInExpr(e, "biz", biz));
  const toggleMonth = (month: string) => editExprFn((e) => toggleInExpr(e, "month", month));
  const setBizs = useCallback(
    (bizs: string[]) => editExprFn((e) => setInExpr(e, "biz", bizs)),
    [editExprFn],
  );
  const setMonths = useCallback(
    (months: string[]) => editExprFn((e) => setInExpr(e, "month", months)),
    [editExprFn],
  );
  const onlyThumb = getSimpleExpr(expr, "thumb") === "true";
  const setOnlyThumb = (v: boolean) =>
    editExprFn((e) => setSimpleExpr(e, "thumb", "eq", v ? "true" : ""));
  const searchQ = getSimpleExpr(expr, "md5");
  const setSearchQ = (q: string) => editExprFn((e) => setSimpleExpr(e, "md5", "contains", q));

  // ---- selection ----
  const toggleChecked = useCallback((id: number) => {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = useCallback(async () => {
    try {
      const ids = await api.getIDs(filter);
      setChecked(new Set(ids));
      setToast(`已勾选当前筛选中的 ${ids.length} 个文件`);
    } catch (e) {
      setToast(`全选失败：${e}`);
    }
  }, [filter]);

  const checkedCount = checked.size;
  const checkedBytes = useMemo(
    () => rows.filter((r) => checked.has(r.id)).reduce((acc, r) => acc + r.size, 0),
    [rows, checked],
  );

  const handleRowsChange = useCallback((rs: FileRow[]) => setRows(rs), []);

  // ---- clean ----
  const clean = useCallback(async () => {
    const backup = getBackupDir();
    const msg =
      `将清理 ${checkedCount} 个文件（${fmtSize(checkedBytes)}）。\n` +
      (backup ? `文件将移动到：${backup}` : "未设置备份目录：删除前会写入审计日志（路径/大小/时间）。") +
      "\n\n确定继续？";
    const answer = await api.confirmClean(msg);
    if (answer !== "清理") return;
    setPhase("cleaning");
    const runClean = (ignoreRunning: boolean) =>
      api.clean({
        ids: [...checked],
        backupDir: backup,
        force: true,
        confirmed: true,
        ignoreRunning,
      });
    let res: CleanResult;
    try {
      res = await runClean(false);
    } catch (e) {
      // 后端哨兵错误：QQ 运行中。POSIX 下 unlink 不被写锁阻塞，但 QQ
      // 正在写的缓存条目可能失效（重新下载即恢复）—— 二次确认后可覆盖。
      if (String(e) === "qq-running") {
        const again = await api.confirm(
          "QQ 正在运行",
          "检测到 QQ 进程正在运行。\n\n删除本身不会被写入锁阻塞，但 QQ 正在写入的缓存条目可能失效（重新下载即可恢复）。\n\n仍要继续清理吗？",
          ["仍要清理", "取消"],
          "取消",
        );
        if (again !== "仍要清理") {
          setPhase("ready");
          return;
        }
        try {
          res = await runClean(true);
        } catch (e2) {
          setPhase("ready");
          setError(String(e2));
          setToast(`清理被拒绝：${e2}`);
          return;
        }
      } else {
        setPhase("ready");
        setError(String(e));
        setToast(`清理被拒绝：${e}`);
        return;
      }
    }
    setPhase("idle");
    setRows([]);
    setChecked(new Set());
    setToast(
      `清理完成：处理 ${res.processed}，移动 ${res.moved}，删除 ${res.deleted}，` +
        `跳过 ${res.skipped}，失败 ${res.failed}，释放 ${fmtSize(res.bytesFreed)}`,
    );
    if (res.errors.length > 0) setError(res.errors.join("\n"));
  }, [checked, checkedBytes, checkedCount]);

  const openDupes = useCallback(async () => {
    try {
      const groups = await api.getDupes(filter);
      setDupes(groups);
      setDupesOpen(true);
    } catch (e) {
      setToast(`去重分析失败：${e}`);
    }
  }, [filter]);

  const pickRoot = useCallback(async () => {
    const d = await api.pickDirectory("选择 QQ 数据根目录");
    if (!d) return;
    const ok = await api.isInstanceRoot(d);
    if (!ok) {
      setToast("该目录下没有找到 nt_qq_* 账号目录");
      return;
    }
    setRoot(d);
    setRoots((rs) => (rs.includes(d) ? rs : [d, ...rs]));
  }, []);

  const allFs = allFilters(userFilters);
  const quickFilters = allFs.filter((f) => f.builtin || f.pinned);

  return (
    <div className="app">
      <TopBar
        roots={roots}
        root={root}
        accounts={accounts}
        account={account}
        scanning={phase === "scanning" || phase === "cleaning"}
        progress={progress}
        theme={theme}
        onRootChange={setRoot}
        onAccountChange={setAccount}
        onScan={startScan}
        onStop={stopScan}
        onPickRoot={() => void pickRoot()}
        onOpenSettings={() => setSettingsOpen(true)}
        onCycleTheme={cycleTheme}
      />
      {error && (
        <div style={{ padding: "6px 12px", color: "#f87171", fontSize: 12, whiteSpace: "pre-wrap" }}>
          {error}
        </div>
      )}
      <div className="main">
        <LeftTree
          bizGroups={bizGroups}
          monthGroups={monthGroups}
          activeBizs={getInExpr(expr, "biz")}
          activeMonths={getInExpr(expr, "month")}
          onToggleBiz={toggleBiz}
          onToggleMonth={toggleMonth}
          onSetBizs={setBizs}
          onSetMonths={setMonths}
        />
        <div className="center">
          <div className="toolbar">
            {quickFilters.map((f) => {
              const active = appliedFilter === f.name;
              return (
                <button
                  key={f.name}
                  className={`chip${active ? " on" : ""}`}
                  onClick={() => (active ? applyFilter(BUILTIN_FILTERS[0]) : applyFilter(f))}
                  title={active ? "点击还原「全部」" : "应用此筛选器"}
                >
                  {f.name}
                </button>
              );
            })}
            <div className="dropdown-wrap">
              <button
                className="chip"
                onClick={() => setMoreOpen((o) => !o)}
                title="全部筛选器（可置顶到工具栏）"
              >
                更多 ▾
              </button>
              {moreOpen && (
                <div className="dropdown">
                  {/* 更多 = 工具栏（内置+置顶）以外的筛选器 */}
                  {userFilters.filter((f) => !f.pinned).length === 0 && (
                    <div className="dropdown-item" style={{ color: "var(--text-dim)", cursor: "default" }}>
                      暂无更多筛选器（未置顶的自定义筛选器会出现在这里）
                    </div>
                  )}
                  {userFilters
                    .filter((f) => !f.pinned)
                    .map((f) => (
                      <div key={f.name} className="dropdown-item">
                        <span style={{ flex: 1 }} onClick={() => applyFilter(f)}>
                          {f.name}
                        </span>
                        <button
                          className="mini"
                          onClick={() => {
                            const next = userFilters.map((x) =>
                              x.name === f.name ? { ...x, pinned: true } : x,
                            );
                            setUserFilters(next);
                            saveUserFilters(next);
                          }}
                          title="固定到工具栏直选"
                        >
                          置顶
                        </button>
                      </div>
                    ))}
                </div>
              )}
            </div>
            <button
              className={`chip${appliedFilter === "" && expr !== null ? " on" : ""}`}
              onClick={() => {
                setFilterOpen(true);
                setMoreOpen(false);
              }}
              title="编辑筛选表达式（列表/表达式双视图）"
            >
              ⚙ 编辑
            </button>
            <button
              className="chip"
              onClick={() => void openDupes()}
              title="同一内容存了多份时，每份只留一份的建议清单"
            >
              去重建议
            </button>
            <span className="toolbar-sep" />
            {SORT_FIELDS.map((sf) => {
              const active = sort.field === sf.field;
              const dimmed = orders.length > 0;
              return (
                <button
                  key={sf.field}
                  className={`chip${active && !dimmed ? " on" : ""}${dimmed ? " dim" : ""}`}
                  onClick={() =>
                    setSort((prev) =>
                      prev.field === sf.field
                        ? { field: prev.field, desc: !prev.desc }
                        : { field: sf.field, desc: sf.descDefault },
                    )
                  }
                  title={
                    dimmed
                      ? "排序由表达式中的 order() 管道控制（编辑筛选器查看）"
                      : active
                        ? "再次点击切换升降序"
                        : undefined
                  }
                >
                  {sf.label}
                  {active && !dimmed ? (sort.desc ? " ▼" : " ▲") : ""}
                </button>
              );
            })}
            <label style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12 }}>
              <input type="checkbox" checked={onlyThumb} onChange={(e) => setOnlyThumb(e.target.checked)} />
              仅缩略图
            </label>
            <input
              placeholder="搜索 md5…"
              value={searchQ}
              onChange={(e) => setSearchQ(e.target.value)}
              style={{ width: 140 }}
            />
          </div>
          {phase === "ready" ? (
            <PhotoWall
              query={pageQuery}
              queryKey={queryKey}
              selected={selected}
              checked={checked}
              onSelect={setSelected}
              onToggle={toggleChecked}
              onRowsChange={handleRowsChange}
            />
          ) : (
            <div className="wall-scroll">
              <div className="wall-empty">
                {phase === "scanning"
                  ? "扫描中…"
                  : phase === "cleaning"
                    ? "清理中…"
                    : "点击「扫描」开始 dry-run 统计（不会删除任何文件）。"}
              </div>
            </div>
          )}
        </div>
        <PreviewPanel
          key={selected ?? -1}
          row={rows.find((r) => r.id === selected) ?? null}
          rows={rows}
          onNavigate={(r) => setSelected(r.id)}
        />
      </div>
      <BottomBar
        stats={stats}
        checkedCount={checkedCount}
        checkedBytes={checkedBytes}
        busy={phase === "scanning" || phase === "cleaning"}
        onSelectAll={() => void selectAll()}
        onClearSelection={() => setChecked(new Set())}
        onClean={() => void clean()}
      />
      <FilterEditor
        open={filterOpen}
        expr={expr}
        onChangeExpr={editExpr}
        limit={limit}
        offset={offset}
        orders={orders}
        onStagesChange={editStages}
        sort={sort}
        onSortChange={setSort}
        filters={userFilters}
        onSaveFilters={(list) => {
          setUserFilters(list);
          saveUserFilters(list);
        }}
        onApply={applyFilter}
        onClose={() => setFilterOpen(false)}
      />
      <DupesDialog
        open={dupesOpen}
        groups={dupes}
        onSelectGroup={(ids) =>
          setChecked((prev) => {
            const next = new Set(prev);
            ids.forEach((id) => next.add(id));
            return next;
          })
        }
        onSelectAll={(ids) => setChecked(new Set(ids))}
        onClose={() => setDupesOpen(false)}
      />
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        theme={theme}
        onThemeChange={changeTheme}
      />
      {toast && (
        <div className="toast" onClick={() => setToast("")}>
          {toast}
        </div>
      )}
    </div>
  );
}
