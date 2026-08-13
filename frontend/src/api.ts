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
  clean: (r: CleanRequest) => call<CleanResult>("Clean", r),
  getConfig: () => call<Config>("GetConfig"),
  setConfig: (c: Config) => call<void>("SetConfig", c),
  reveal: (id: number) => call<void>("Reveal", id),
  pickDirectory: (title: string) => callDialogs<string>("PickDirectory", title),
  confirmClean: (msg: string) => callDialogs<string>("ConfirmClean", msg),
  confirm: (title: string, msg: string, buttons: string[], def: string) =>
    callDialogs<string>("Confirm", title, msg, buttons, def),
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
