//go:build !windows

package main

// setupCrashReport 在非 Windows 平台是 no-op（docs/09 §3.5 平台
// 决策）：POSIX 未观察到异常崩溃，不启用崩溃文件方案——Go panic 走
// 默认 stderr 输出（CLI 终端可见）；logring 的内存环形缓冲与 Recover
// 仍全平台生效（零开销）。
func setupCrashReport() {}

// teardownCrashReport 在非 Windows 平台是 no-op：无崩溃文件可清理。
func teardownCrashReport() {}
