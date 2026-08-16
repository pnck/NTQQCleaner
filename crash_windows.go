//go:build windows

package main

import (
	"os"
	"runtime"

	"qqcleaner/internal/app"
	"qqcleaner/internal/logring"
)

// setupCrashReport 启用崩溃文件方案（docs/09 §3.5：仅 Windows——
// build tag 静态分派，不做 runtime 判断）：崩溃转储 + SEH 原生异常
// 过滤器（minidump）+ 逐操作 ops 日志共用同一文件，启动即落首行。
func setupCrashReport() {
	// 先清扫上次正常退出但删除失败的残留（docs/09 §3.5：只删带
	// clean-exit 标记的文件，无标记的是潜在证据不动）。
	logring.SweepStale(app.ConfigDir())
	logring.EnableCrashLog(app.ConfigDir())
	logring.Crumb("boot: version=%s goos=%s goarch=%s pid=%d", version, runtime.GOOS, runtime.GOARCH, os.Getpid())
}

// teardownCrashReport 只看 cleanExit flag（main 在 run() 的响应上
// 设置，docs/09 §3.5）：正常退出（run() 返回 nil）落 clean-exit 标记
// 后删除本次会话的崩溃文件——正常退出后只剩 ops 痕迹、无诊断价值；
// 错误退出直接返回、文件保留（已落 "exit: error" 行）。删除失败
// （文件仍被句柄持有）时标记留在文件尾部，下次启动由 SweepStale
// 清扫。panic 时本函数不执行——证据保留；被 KILL 时进程直接死亡——
// 同样保留。不能以 defer 调用（LIFO 先于 Recover、毁掉证据）。
func teardownCrashReport(cleanExit bool) {
	if !cleanExit {
		return
	}
	logring.MarkCleanExit()
	logring.Cleanup()
}
