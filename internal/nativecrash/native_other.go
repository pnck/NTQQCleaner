//go:build !windows

package nativecrash

// Install 在非 Windows 平台是 no-op（docs/09 §3.5）：原生异常兜底仅
// Windows 需要——非 Go 线程异常的等价物（macOS DiagnosticReports /
// Linux core）由系统默认开启覆盖，Go 级崩溃走默认 stderr。
func Install(dir string) {}
