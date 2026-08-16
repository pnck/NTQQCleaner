//go:build !windows

package logring

// installNativeFilter 在非 Windows 平台是 no-op（docs/09 §3.3）：原生
// 崩溃兜底仅 Windows 需要——非 Go 线程异常的等价物（macOS
// DiagnosticReports / Linux core）由系统默认开启覆盖，且本包经
// debug.SetCrashOutput 已覆盖全部 Go 级崩溃。
func installNativeFilter() {}
