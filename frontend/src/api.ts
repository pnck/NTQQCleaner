// Thin typed wrappers over the objects Wails v2 injects at runtime
// (window.go.backend.* for bound methods, window.runtime.* for the event
// bus and native dialogs). No generated bindings required — the generated
// wailsjs module can replace this later without touching call sites.
import type {
  AccountReport,
  CleanRequest,
  CleanResult,
  Config,
  DupGroup,
  Filter,
  GroupStat,
  PageQuery,
  PageResult,
  Progress,
  ScanDone,
  ScanOptions,
  Stats,
} from "./types";

declare global {
  interface Window {
    go: {
      // Wails 运行时命名空间 = window.go.<Go包名>.<绑定结构体名>
      // （见 wails 生成的 wailsjs/go/app/Backend.js、main/Dialogs.js）
      app: {
        Backend: Record<string, (...args: unknown[]) => Promise<unknown>>;
      };
      main: {
        Dialogs: Record<string, (...args: unknown[]) => Promise<unknown>>;
      };
    };
    runtime: {
      // 注意：wails v2 的注入运行时没有对话框函数 —— 对话框走
      // window.go.main.Dialogs（Go 侧实现）。
      EventsOn(event: string, cb: (...data: unknown[]) => void): void;
      EventsOff(event: string): void;
      Quit(): void;
    };
  }
}

function call<T>(name: string, ...args: unknown[]): Promise<T> {
  const fn = window.go?.app?.Backend?.[name];
  if (!fn) throw new Error(`backend method ${name} not bound`);
  return fn(...args) as Promise<T>;
}

// QQRunningError：后端预检哨兵的**类型化封装**。Wails 绑定只序列化
// error 文本，Go 侧错误类型无法穿越边界——消息到类型的恢复集中在
// 这一个边界点（clean 包装内），业务代码只 instanceof，不再匹配字符串。
export class QQRunningError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "QQRunningError";
  }
}

function errText(e: unknown): string {
  if (typeof e === "string") return e;
  const m = (e as { message?: unknown })?.message;
  return m === undefined ? String(e) : String(m);
}

// clean 包装：捕获 Wails 的拒绝，把 QQ 运行守卫的错误恢复为类型化
// 错误；其余错误原样抛出。后端哨兵（"qq-running"）与 clean 层
// ErrQQRunning（"QQ is running…"）两种消息形态都在此归一。
async function cleanBound(r: CleanRequest): Promise<CleanResult> {
  try {
    return await call<CleanResult>("Clean", r);
  } catch (e) {
    const msg = errText(e).toLowerCase();
    if (msg.includes("qq-running") || msg.includes("qq is running")) {
      throw new QQRunningError(errText(e));
    }
    throw e;
  }
}

function callDialogs<T>(name: string, ...args: unknown[]): Promise<T> {
  const fn = window.go?.main?.Dialogs?.[name];
  if (!fn) throw new Error(`dialogs method ${name} not bound`);
  return fn(...args) as Promise<T>;
}

export const api = {
  discoverRoots: () => call<string[]>("DiscoverRoots"),
  isInstanceRoot: (root: string) => call<boolean>("IsInstanceRoot", root),
  scan: (o: ScanOptions) => call<void>("Scan", o),
  stop: () => call<void>("Stop"),
  scanState: () =>
    call<{ scanning: boolean; root: string; accounts: AccountReport[] }>("ScanState"),
  queryRows: (q: PageQuery) => call<PageResult>("QueryRows", q),
  getStats: (f: Filter) => call<Stats>("GetStats", f),
  getIDs: (f: Filter) => call<number[]>("GetIDs", f),
  getDupes: (f: Filter) => call<DupGroup[]>("GetDupes", f),
  getGroups: (f: Filter, by: string) => call<GroupStat[]>("GetGroups", f, by),
  clean: cleanBound,
  getConfig: () => call<Config>("GetConfig"),
  setConfig: (c: Config) => call<void>("SetConfig", c),
  reveal: (id: number) => call<void>("Reveal", id),
  pickDirectory: (title: string) => callDialogs<string>("PickDirectory", title),
  confirmClean: (msg: string) => callDialogs<string>("ConfirmClean", msg),
  // 原生 YES/NO 确认（QQ 运行守卫覆盖确认）：Go 侧统一 Yes/No 契约——
  // Windows 的 MessageDialog 忽略自定义按钮文案（MessageBoxW 无自定义
  // 按钮），自定义文案方案在 Windows 上点确认不生效。
  confirmYesNo: (title: string, msg: string) => callDialogs<string>("ConfirmYesNo", title, msg),
};

export const events = {
  onProgress(cb: (p: Progress) => void): () => void {
    window.runtime.EventsOn("scan:progress", (p) => cb(p as Progress));
    return () => window.runtime.EventsOff("scan:progress");
  },
  onDone(cb: (d: ScanDone) => void): () => void {
    window.runtime.EventsOn("scan:done", (d) => cb(d as ScanDone));
    return () => window.runtime.EventsOff("scan:done");
  },
  onError(cb: (d: ScanDone) => void): () => void {
    window.runtime.EventsOn("scan:error", (d) => cb(d as ScanDone));
    return () => window.runtime.EventsOff("scan:error");
  },
  onState(cb: (s: { scanning: boolean }) => void): () => void {
    window.runtime.EventsOn("scan:state", (s) => cb(s as { scanning: boolean }));
    return () => window.runtime.EventsOff("scan:state");
  },
};
