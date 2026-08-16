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
	logring.EnableCrashLog(app.ConfigDir())
	logring.Crumb("boot: version=%s goos=%s goarch=%s pid=%d", version, runtime.GOOS, runtime.GOARCH, os.Getpid())
}

// teardownCrashReport 在受控退出路径删除本次会话的崩溃文件
// （docs/09 §3.5）：正常退出后只剩 ops 痕迹、无诊断价值。panic 时
// Recover 重新抛出、本函数不执行——证据保留；被 KILL 时进程直接
// 死亡——同样保留。不能以 defer 调用（LIFO 先于 Recover、毁掉证据）。
func teardownCrashReport() {
	logring.Cleanup()
}
