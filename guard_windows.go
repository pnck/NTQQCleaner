//go:build windows

package main

import (
	"qqcleaner/internal/app"
	"qqcleaner/internal/nativecrash"
)

// setupCrashGuard 安装原生异常兜底（docs/09 §3.3）：仅 Windows 有
// 实现，其余平台 no-op（guard_other.go）。minidump 只在原生异常
// 发生时写入 %TEMP%\ntqq-cleaner\，无常态文件、无清理逻辑。
func setupCrashGuard() {
	nativecrash.Install(app.ConfigDir())
}
