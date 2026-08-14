import { useCallback, useEffect, useMemo, useState } from "react";
import { QQRunningError, api, events } from "./api";
import { BottomBar } from "./components/BottomBar";
import { CleanReportDialog } from "./components/CleanReportDialog";
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
import { leaf } from "./exprbase";
import { DEFAULT_SORT, loadFilters, saveFilters, type NamedFilter } from "./filters";
import { applyTheme, getTheme, nextTheme, type Theme } from "./theme";
import type {
  AccountReport,
  CleanResult,
  Config,
  DupGroup,
  Expr,
  FileRow,
  GroupStat,
  PageQuery,
  Progress,
  Stage,
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
  // 预览面板「同内容 N 份」按钮的勾选模式（per-row：dups=只勾副本 / all=含保留份）
  const [dupModes, setDupModes] = useState<Map<number, "dups" | "all">>(new Map());

  // 筛选状态 = 表达式树 + 管道 stages（左栏/快捷控件/筛选器编辑器共享）
  const [expr, setExpr] = useState<Expr | null>(null);
  const [stages, setStages] = useState<Stage[]>([]);
  const [sort, setSort] = useState<{ field: string; desc: boolean }>({ field: "size", desc: true });
  const [filters, setFilters] = useState<NamedFilter[]>(loadFilters);
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
  // 清理结果对话框（清理完成后自动弹出，逐文件回显）
  const [cleanReport, setCleanReport] = useState<CleanResult | null>(null);
  const [theme, setTheme] = useState<Theme>(getTheme());
  // 后端 config（设置对话框的高级门控/备份策略；普通门控 GUI 恒全开）
  const [cfg, setCfg] = useState<Config | null>(null);

  const filter = useMemo(() => ({ account, expr, stages }), [account, expr, stages]);
  const queryKey = useMemo(
    () => JSON.stringify([expr, stages, sort, account, scanGen]),
    [expr, stages, sort, account, scanGen],
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

  // toast 自动消失：提示性消息 ~6s 后隐去（错误提示也一并处理；用户中途
  // 停止/重扫不会留下旧贴片）。
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(""), 6000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---- startup ----
  useEffect(() => {
    void api.discoverRoots().then((rs) => {
      setRoots(rs);
      if (rs.length > 0) setRoot(rs[0]);
    });
    void api.getConfig().then(setCfg).catch(console.error);
    const offs = [
      events.onProgress(setProgress),
      events.onDone((d) => {
        setAccounts(d.accounts);
        setPhase("ready");
        setScanGen((g) => g + 1);
        setError(d.error || "");
        if (d.error) {
          setToast(`扫描结束（含错误）：${d.error}`);
        } else {
          const files = d.accounts.reduce((a, x) => a + x.totalFiles, 0);
          // 参与内容哈希的文件数不再贴出（每个文件是否计算了哈希在详情
          // 面板可见，贴总数是过拟合的冗余信息）。
          setToast(`扫描完成：${files} 个文件`);
        }
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
    setToast("");
    setSelected(null);
    setChecked(new Set());
    setDupModes(new Map()); // 行 id 是本次扫描内的位置编号，重扫后作废
    setRows([]);
    void api
      .scan({ root, account, minAgeDays: 3, minSize: 0, onlyBizs: [] })
      .catch((e) => {
        setPhase("idle");
        setError(String(e));
      });
  }, [root, account]);

  const stopScan = useCallback(() => void api.stop(), []);

  // ---- stats / groups ----
  // 只在筛选/扫描结果变化（queryKey）时清勾选并刷新统计；phase 进出
  // ready（如清理被拒绝/取消后回来）不动勾选——用户辛苦勾好的文件在
  // 报错后仍保留。重扫会经 scanGen 改变 queryKey，照常清空。
  useEffect(() => {
    if (phase !== "ready") return;
    setSelected(null);
    setChecked(new Set());
    void api.getStats(filter).then(setStats).catch(console.error);
    const treeFilter = { account, expr: null };
    void api.getGroups(treeFilter, "biz").then(setBizGroups).catch(console.error);
    void api.getGroups(treeFilter, "month").then(setMonthGroups).catch(console.error);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey]);

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
    setStages(
      (f.stages ?? []).map((s) => ({ ...s, kinds: s.kinds ? [...s.kinds] : undefined })),
    );
    setAppliedFilter(f.name);
    setMoreOpen(false);
  }, []);

  // 还原 = 清空筛选回到初始状态（不依赖「全部」是否仍存在于列表）
  const resetFilter = useCallback(() => {
    setExpr(null);
    setSort({ ...DEFAULT_SORT });
    setStages([]);
    setAppliedFilter("");
    setMoreOpen(false);
  }, []);

  const editStages = useCallback((s: Stage[]) => {
    setAppliedFilter("");
    setStages(s);
  }, []);

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
  const searchQ = getSimpleExpr(expr, "fileId");
  const setSearchQ = (q: string) => editExprFn((e) => setSimpleExpr(e, "fileId", "contains", q));

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
      // 后端哨兵：QQ 运行中（api 层已恢复为类型化 QQRunningError）。
      // POSIX 下 unlink 不被写锁阻塞，但 QQ 正在写的缓存条目可能失效
      // （重新下载即恢复）—— 二次确认后可覆盖。
      if (e instanceof QQRunningError) {
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
    // 清理后索引已失效（文件被移走）：自动重扫恢复墙面与筛选结果。
    // 先调 startScan（它会清 toast/error），再写清理结果提示。
    startScan();
    const errs = res.errors ?? [];
    if (errs.length > 0) setError(errs.join("\n"));
    setToast(
      `清理完成：处理 ${res.processed}，移动 ${res.moved}，删除 ${res.deleted}，` +
        `跳过 ${res.skipped}，失败 ${res.failed}，释放 ${fmtSize(res.bytesFreed)}` +
        (errs.length > 0 ? `；${errs.length} 个文件被跳过` : "") +
        "；正在重新扫描…",
    );
    // 逐文件结果自动弹出（清理结果对话框）。
    setCleanReport(res);
  }, [checked, checkedBytes, checkedCount, startScan]);

  const openDupes = useCallback(async () => {
    try {
      const groups = await api.getDupes(filter);
      setDupes(groups);
      setDupesOpen(true);
    } catch (e) {
      setToast(`去重分析失败：${e}`);
    }
  }, [filter]);

  // 预览面板「同内容 N 份」按钮：在「勾选副本」（保留一份不勾）与「勾选
  // 全部」（含保留的那份）之间切换。反复点击来回切换，便于两种批量选择
  // 方式快速换用。模式按行 id 记录（PreviewPanel 每行重挂载）。
  const selectContentDups = useCallback(
    async (row: FileRow) => {
      try {
        const expr = leaf("contentHash", "eq", row.contentHash);
        const groups = await api.getDupes({ account: "", expr });
        const g = groups.find((x) => x.hash === row.contentHash);
        if (!g || g.dupIds.length === 0) {
          setToast("没有可勾选的副本");
          return;
        }
        const mode = dupModes.get(row.id) ?? "dups";
        setChecked((prev) => {
          const next = new Set(prev);
          if (mode === "dups") {
            // 「勾选副本」：只勾副本，保留份保持不勾
            next.delete(g.keepId);
            g.dupIds.forEach((id) => next.add(id));
          } else {
            // 「勾选全部」：在副本基础上补上保留份
            next.add(g.keepId);
            g.dupIds.forEach((id) => next.add(id));
          }
          return next;
        });
        setDupModes((m) => {
          const nm = new Map(m);
          nm.set(row.id, mode === "dups" ? "all" : "dups");
          return nm;
        });
        setToast(
          mode === "dups"
            ? `已勾选 ${g.dupIds.length} 个相同内容副本（保留 ${g.keepLabel}）`
            : `已勾选全部 ${g.dupIds.length + 1} 份相同内容（含保留的 ${g.keepLabel}）`,
        );
      } catch (e) {
        setToast(`去重分析失败：${e}`);
      }
    },
    [dupModes],
  );

  // 保存设置：高级门控变化影响扫描入库范围，变化时自动重扫生效。
  const saveConfig = useCallback(
    (c: Config) => {
      const advancedChanged =
        !cfg ||
        c.cleanLog !== cfg.cleanLog ||
        c.cleanDatalineTmp !== cfg.cleanDatalineTmp ||
        c.cleanAvatar !== cfg.cleanAvatar;
      void api
        .setConfig(c)
        .then(() => {
          setCfg(c);
          if (advancedChanged && root) startScan();
        })
        .catch((e) => setToast(`保存设置失败：${e}`));
    },
    [cfg, root, startScan],
  );

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

  // 内置筛选器已作为种子并入 filters（filters.ts loadFilters），此处只有
  // 一个列表：工具栏 = 置顶的，更多 = 未置顶的。
  const quickFilters = filters.filter((f) => f.pinned);

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
                  onClick={() => (active ? resetFilter() : applyFilter(f))}
                  title={active ? "点击清空筛选" : "应用此筛选器"}
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
                  {/* 更多 = 工具栏（置顶）以外的筛选器 */}
                  {filters.filter((f) => !f.pinned).length === 0 && (
                    <div className="dropdown-item" style={{ color: "var(--text-dim)", cursor: "default" }}>
                      暂无更多筛选器（未置顶的筛选器会出现在这里）
                    </div>
                  )}
                  {filters
                    .filter((f) => !f.pinned)
                    .map((f) => (
                      <div key={f.name} className="dropdown-item">
                        <span style={{ flex: 1 }} onClick={() => applyFilter(f)}>
                          {f.name}
                        </span>
                        <button
                          className="mini"
                          onClick={() => {
                            const next = filters.map((x) =>
                              x.name === f.name ? { ...x, pinned: true } : x,
                            );
                            setFilters(next);
                            saveFilters(next);
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
              const dimmed = stages.some((s) => s.kind === "order");
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
              placeholder="搜索 文件ID…"
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
          dupMode={selected !== null ? (dupModes.get(selected) ?? "dups") : "dups"}
          onNavigate={(r) => setSelected(r.id)}
          onToast={setToast}
          onSelectDups={(r) => void selectContentDups(r)}
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
        stages={stages}
        onStagesChange={editStages}
        sort={sort}
        onSortChange={setSort}
        filters={filters}
        onSaveFilters={(list) => {
          setFilters(list);
          saveFilters(list);
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
      {cleanReport && (
        <CleanReportDialog res={cleanReport} onClose={() => setCleanReport(null)} />
      )}
      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        theme={theme}
        onThemeChange={changeTheme}
        config={cfg}
        onConfigSave={saveConfig}
      />
      {toast && (
        <div className="toast" onClick={() => setToast("")}>
          {toast}
        </div>
      )}
    </div>
  );
}
