// Package logring 提供紧急情况用的内存环形日志：进程内关键事件
// （扫描/清理阶段、错误摘要）先写进固定容量的环形缓冲，未处理的
// panic/fatal 经 debug.SetCrashOutput（Go 1.23+）把运行时崩溃转储
// 落盘到工具 tmp 目录——Windows 批量清理中途闪退（-H windowsgui
// 无控制台，stderr 不可见）后仍能拿到崩溃现场。
//
// 原生崩溃（WebView2 等 C 层段错误）由系统崩溃报告器覆盖（Windows
// WER / macOS DiagnosticReports 默认开启，无需本包）。全平台、
// 全构建模式（含 release）启用，不做任何 build tag 门控。
package logring

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
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
