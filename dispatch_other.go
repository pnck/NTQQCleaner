//go:build !windows

package main

// consoleKind 在 POSIX 恒为 2（CLI 语义），裸运行保持 gui 默认——
// docs/10 §2.2：POSIX 行为不变。
func consoleKind() int { return 2 }

// detachOrphanConsole 在 POSIX 是 no-op。
func detachOrphanConsole() {}

// bareShowsUsage 在 POSIX 为 false：裸运行仍进 GUI。
func bareShowsUsage() bool { return false }
