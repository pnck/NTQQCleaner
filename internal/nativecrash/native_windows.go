//go:build windows

// Package nativecrash 安装 Windows 原生异常兜底（docs/09 §3.3）：
// Go runtime 的 VEH 只覆盖 Go 代码路径，非 Go 线程（WebView2 宿主
// 进程内线程等）的原生异常没有任何记录且 WER 可能被系统策略关闭。
// SetUnhandledExceptionFilter 安装后：写 minidump（dbghelp，纯
// syscall/LazyDLL，无 cgo，容器可交叉编译）并输出一行到 stderr
// （控制台可见），然后返回 EXCEPTION_CONTINUE_SEARCH 链回默认
// 处理器——系统 bugreport/WER 按用户系统策略正常介入。与 Go VEH
// 共存：Go panic/fatal 走默认 stderr 输出，不经过本过滤器。
package nativecrash

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	miniDumpNormal                         = 0x00000000
	miniDumpWithIndirectlyReferencedMemory = 0x00000002
	exceptionContinueSearch                = 0 // 链回默认处理器（WER 照常介入）
)

// exceptionRecord 镜像 Win64 的 EXCEPTION_RECORD（x/sys/windows 未导出
// 该结构；字段仅取 ExceptionCode，其余保留占位保证布局）。
type exceptionRecord struct {
	ExceptionCode    uint32
	ExceptionFlags   uint32
	ExceptionRecord  uintptr
	ExceptionAddress uintptr
	NumberParameters uint32
	_                uint32 // 对齐填充
	ExceptionInfo    [15]uintptr
}

// exceptionPointers 镜像 Win64 的 EXCEPTION_POINTERS。
type exceptionPointers struct {
	ExceptionRecord *exceptionRecord
	ContextRecord   uintptr
}

var (
	once     sync.Once
	dumpDir  string
	miniDump *windows.LazyProc
)

// Install 安装原生异常过滤器（幂等，仅首次生效）。dumpDir 是
// minidump 落盘目录（crash-native-<ts>.dmp），仅在原生异常发生
// 时写入——不产生任何常态文件、无清理逻辑。
func Install(dir string) {
	once.Do(func() {
		dumpDir = dir
		dbghelp := windows.NewLazySystemDLL("dbghelp.dll")
		miniDump = dbghelp.NewProc("MiniDumpWriteDump")
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		kernel32.NewProc("SetUnhandledExceptionFilter").Call(
			windows.NewCallback(nativeExceptionFilter))
	})
}

// nativeExceptionFilter 是非 Go 线程原生异常的兜底：写 minidump +
// stderr 一行后把异常交回系统。隐私约束（docs/09 §3.3）：
// MiniDumpNormal + 间接引用内存，**不用 FullMemory**——进程内存
// 可能含 QQ 账号/路径之外的敏感数据。
func nativeExceptionFilter(ep *exceptionPointers) uintptr {
	// 兜底自身绝不能 panic：任何失败都静默返回 CONTINUE_SEARCH。
	defer func() { _ = recover() }()
	code := uint32(0)
	if ep != nil && ep.ExceptionRecord != nil {
		code = ep.ExceptionRecord.ExceptionCode
	}
	dmp := filepath.Join(dumpDir,
		fmt.Sprintf("crash-native-%s.dmp", time.Now().Format("20060102-150405")))
	if f, err := os.Create(dmp); err == nil {
		miniDump.Call(
			uintptr(windows.CurrentProcess()),
			uintptr(windows.GetCurrentProcessId()),
			f.Fd(),
			miniDumpNormal|miniDumpWithIndirectlyReferencedMemory,
			uintptr(unsafe.Pointer(ep)), 0, 0)
		f.Close()
	}
	fmt.Fprintf(os.Stderr, "native exception 0x%08x: minidump %s\n", code, dmp)
	return exceptionContinueSearch
}
