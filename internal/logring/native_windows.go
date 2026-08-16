//go:build windows

package logring

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 原生崩溃兜底（docs/09 §3.3）：Go runtime 的 VEH 只覆盖 Go 代码路径，
// 非 Go 线程（WebView2 宿主进程内线程等）的原生异常没有任何记录且
// WER 可能被系统策略关闭——当初「WER 默认开启」的假设在真机被证伪。
// SetUnhandledExceptionFilter 安装后：写 minidump（dbghelp，纯
// syscall/LazyDLL，无 cgo，容器可交叉编译）+ 环形缓冲追加进 crash
// log，然后返回 EXCEPTION_CONTINUE_SEARCH 链回默认处理器——系统
// bugreport/WER 按用户系统策略正常介入。与 Go VEH 共存：Go panic/
// fatal 仍走 SetCrashOutput（logring.go），不会到达本过滤器。

const (
	miniDumpNormal                         = 0x00000000
	miniDumpWithIndirectlyReferencedMemory = 0x00000002
	exceptionContinueSearch                = 0 // 链回默认处理器（WER 照常介入）
)

var (
	nativeOnce sync.Once
	dumpDir    string
	miniDump   *windows.LazyProc
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

// installNativeFilter 安装原生异常过滤器（幂等，仅首次生效）。在
// EnableCrashLog 创建崩溃文件后调用——dumpDir 取崩溃文件所在目录。
func installNativeFilter() {
	nativeOnce.Do(func() {
		mu.Lock()
		dumpDir = filepath.Dir(crashN)
		mu.Unlock()
		dbghelp := windows.NewLazySystemDLL("dbghelp.dll")
		miniDump = dbghelp.NewProc("MiniDumpWriteDump")
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		kernel32.NewProc("SetUnhandledExceptionFilter").Call(
			windows.NewCallback(nativeExceptionFilter))
	})
}

// nativeExceptionFilter 是非 Go 线程原生异常的兜底：写 minidump +
// 环形缓冲后把异常交回系统。隐私约束（docs/09 §3.3）：MiniDumpNormal
// + 间接引用内存，**不用 FullMemory**——进程内存可能含 QQ 账号/路径
// 之外的敏感数据。
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
	DumpCrash()
	Crumb("native exception 0x%08x: minidump %s", code, dmp)
	return exceptionContinueSearch
}
