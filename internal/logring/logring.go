// Package logring 提供紧急情况用的内存环形日志：进程内关键事件
// （扫描/清理阶段、错误摘要）先写进固定容量的环形缓冲，未处理的
// panic/fatal 经 debug.SetCrashOutput（Go 1.23+）把运行时崩溃转储
// 落盘到工具 tmp 目录——Windows 批量清理中途闪退（-H windowsgui
// 无控制台，stderr 不可见）后仍能拿到崩溃现场。
//
// 崩溃文件方案（EnableCrashLog/Crumb）的启用由入口按平台门控
// （docs/09 §3.5 产品决策：仅 Windows 启用——main 调用；POSIX 未
// 观察到异常崩溃，不启用文件方案，Go panic 走默认 stderr）。包本身
// 平台无关：内存环形缓冲与 Recover 全平台生效，测试可在任意平台
// 直接启用文件方案。原生崩溃兜底（installNativeFilter 写 minidump）
// 仅 Windows 有实现（native_windows.go），其余平台 no-op。
package logring

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const maxEntries = 512

type entry struct {
	ts   time.Time
	text string
}

var (
	mu     sync.Mutex
	ring   = make([]entry, 0, maxEntries)
	crashF *os.File // 崩溃转储文件句柄（运行时转储与缓冲转储共用，O_APPEND）
	crashN string   // 崩溃文件路径（启动日志用）
)

// Logf 追加一行内存日志（环形，超出容量丢弃最旧）。
func Logf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	appendEntryLocked(fmt.Sprintf(format, args...))
}

// Crumb 追加一行内存日志，并在崩溃文件已启用时**立刻**把它追加写进
// 崩溃文件（docs/09 §3.2）。与 Logf 的区别：Logf 只进内存缓冲（崩溃
// 瞬间才落盘）；面包屑在进程存活期间即落盘——进程被外部击毙
// （TerminateProcess，如杀软/QQ 防护干预）时，最后一根面包屑直接
// 定位死点与进度。0 字节崩溃文件从此不再出现（启动即落首行）。
func Crumb(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	text := fmt.Sprintf(format, args...)
	appendEntryLocked(text)
	if crashF != nil {
		fmt.Fprintf(crashF, "%s %s\n", time.Now().Format("2006-01-02T15:04:05.000"), text)
	}
}

func appendEntryLocked(text string) {
	e := entry{ts: time.Now(), text: text}
	if len(ring) < maxEntries {
		ring = append(ring, e)
		return
	}
	copy(ring, ring[1:])
	ring[len(ring)-1] = e
}

// Dump 按时间顺序把缓冲写进 w。
func Dump(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	dumpLocked(w)
}

// DumpCrash 把缓冲追加写进崩溃文件（O_APPEND：与运行时崩溃转储共存
// 于同一文件）。崩溃文件未初始化时 no-op。
func DumpCrash() {
	mu.Lock()
	defer mu.Unlock()
	if crashF == nil {
		return
	}
	dumpLocked(crashF)
}

// CrashPath 返回崩溃文件路径（未启用时为空）。
func CrashPath() string {
	mu.Lock()
	defer mu.Unlock()
	return crashN
}

func dumpLocked(w io.Writer) {
	fmt.Fprintf(w, "---- ring buffer (%d entries) ----\n", len(ring))
	for _, e := range ring {
		fmt.Fprintf(w, "%s %s\n", e.ts.Format("2006-01-02T15:04:05.000"), e.text)
	}
}

// EnableCrashLog 在 dir 下创建崩溃转储文件（crash-<时间戳>.log）并经
// debug.SetCrashOutput 让未处理 panic/fatal 的运行时转储写入同一文件
// （重复调用以最后一次为准；所有平台生效）。Windows 上同时安装原生
// 异常兜底（installNativeFilter，docs/09 §3.3：非 Go 线程的崩溃写
// minidump 并继续交给系统报告器）。返回文件路径（失败时返回空串）。
func EnableCrashLog(dir string) string {
	f, err := os.OpenFile(
		filepath.Join(dir, "crash-"+time.Now().Format("20060102-150405")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return ""
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		f.Close()
		return ""
	}
	mu.Lock()
	if crashF != nil {
		crashF.Close()
	}
	crashF, crashN = f, f.Name()
	mu.Unlock()
	installNativeFilter()
	// 首行面包屑：即使进程被外部击毙，崩溃文件也至少有一条记录
	// 证明监视器活着（docs/09 §3.2）。
	Crumb("crash watcher armed: %s pid=%d", f.Name(), os.Getpid())
	return f.Name()
}

// Cleanup 关闭并删除本次会话的崩溃文件（docs/09 §3.5）：**只在受控
// 退出路径调用**——main 的 run() 返回之后。panic 时 Recover 重新抛出，
// 本函数不会执行；外部击毙（TerminateProcess）时进程直接死亡——
// 两条异常路径文件都保留证据（逐操作 ops 痕迹）。正常退出后文件只剩
// ops 痕迹、无诊断价值，删除避免 %TEMP% 堆积。不能以 defer 调用：
// LIFO 会先于 Recover 执行、毁掉证据。幂等。
func Cleanup() {
	mu.Lock()
	f := crashF
	crashF = nil
	n := crashN
	crashN = ""
	mu.Unlock()
	if f != nil {
		f.Close()
	}
	if n != "" {
		os.Remove(n)
	}
}

// cleanExitMarker 是受控退出的末尾标记（docs/09 §3.5）：正常退出
// 先落本标记再尝试删除崩溃文件。Windows 上崩溃文件若仍被其它句柄
// 持有（如运行时崩溃转储句柄），删除会失败、文件留在原地——下次
// 启动时 SweepStale 只清扫**带本标记**的文件（安全），不带标记的
// 文件是潜在崩溃证据，一律不动。
const cleanExitMarker = "clean exit"

// MarkCleanExit 在受控退出路径落末尾标记（仅 Windows 调用，
// crash_windows.go teardown）。崩溃文件未启用时 no-op。
func MarkCleanExit() { Crumb(cleanExitMarker) }

// SweepStale 清扫上一次正常退出但删除失败的崩溃文件（docs/09 §3.5，
// 仅 Windows 调用）：只删文件尾部含 cleanExitMarker 的 crash-*.log。
func SweepStale(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "crash-*.log"))
	if err != nil {
		return
	}
	for _, m := range matches {
		if tailHasCleanExit(m) {
			os.Remove(m)
		}
	}
}

// tailHasCleanExit 读文件最后 256 字节判断是否带 clean-exit 标记。
func tailHasCleanExit(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	off := st.Size() - 256
	if off < 0 {
		off = 0
	}
	buf := make([]byte, 256)
	n, err := f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return false
	}
	return strings.Contains(string(buf[:n]), cleanExitMarker)
}

// Recover 供顶层/边界 goroutine 的 defer 使用：捕获 panic，把环形
// 缓冲写进崩溃文件后重新 panic（运行时照常输出崩溃转储、系统崩溃
// 报告器照常介入）。用法：
//
//	defer logring.Recover()
//
// 注意：必须直接 defer 本函数（recover 仅在由 defer 直接调用的函数
// 中生效）。
func Recover() {
	v := recover()
	if v == nil {
		return
	}
	Logf("panic recovered for ring dump: %v", v)
	DumpCrash()
	panic(v)
}
