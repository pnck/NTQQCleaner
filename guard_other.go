//go:build !windows

package main

// setupCrashGuard 在非 Windows 平台是 no-op（docs/09 §3.5）：
// POSIX 未观察到异常崩溃，Go panic 走默认 stderr（终端可见）。
func setupCrashGuard() {}
