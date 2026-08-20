//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// consoleKind 报告进程的控制台归属（docs/10 §2.2）：
// 0 = 无控制台（24H2 detached 策略下双击启动）；
// 1 = 孤儿控制台（旧系统双击启动时系统分配的）；
// 2 = 继承自父 shell（PowerShell/cmd 启动）。
func consoleKind() int {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getList := kernel32.NewProc("GetConsoleProcessList")
	var pids [1]uint32
	n, _, _ := getList.Call(uintptr(unsafe.Pointer(&pids[0])), 1)
	switch n {
	case 0:
		return 0
	case 1:
		return 1
	default:
		return 2
	}
}

// detachOrphanConsole 关闭旧系统双击时分配的孤儿控制台（短暂闪烁
// 后消失，docs/10 §1）。
func detachOrphanConsole() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	kernel32.NewProc("FreeConsole").Call()
}

// bareShowsUsage：控制台语境的裸运行显示 usage 而非 GUI（docs/10
// §2.2 产品决策）。
func bareShowsUsage() bool { return true }
