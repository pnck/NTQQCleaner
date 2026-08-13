package clean

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// qqRunningFunc is swapped out in tests; it wraps QQRunning.
var qqRunningFunc = QQRunning

// qqProcessesFunc supplies the matched process list for diagnostics; also
// swapped in tests.
var qqProcessesFunc = QQProcesses

// QQProcesses returns the command lines of processes that look like the QQ
// client (e.g. /Applications/QQ.app/Contents/MacOS/QQ). The pattern is
// anchored on the binary inside the app bundle to avoid false positives.
func QQProcesses() []string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq QQ.exe", "/FO", "CSV", "/NH").Output()
		if err != nil {
			return nil
		}
		var procs []string
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(strings.ToLower(line), "qq.exe") {
				procs = append(procs, line)
			}
		}
		return procs
	default: // darwin, linux
		out, err := exec.Command("pgrep", "-fl", "QQ.app/Contents/MacOS/QQ").Output()
		if err != nil {
			return nil // pgrep exit 1 = no match
		}
		var procs []string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				procs = append(procs, strings.TrimSpace(line))
			}
		}
		return procs
	}
}

// QQRunning reports whether the QQ client is currently running. The cleaner
// refuses to delete while QQ is up: QQ may be reading/writing its cache
// concurrently, and deleting under it risks corrupting QQ's view of the
// cache (Finder lets you, but Finder has no consistency contract).
// Reference: docs/06_safety_redlines.md §5.
func QQRunning() bool {
	return len(QQProcesses()) > 0
}

// qqRunningError builds the refusal error with the matched processes so the
// user can see exactly what triggered the guard.
func qqRunningError() error {
	procs := qqProcessesFunc()
	if len(procs) == 0 {
		return ErrQQRunning
	}
	return fmt.Errorf("%w；匹配进程：%s", ErrQQRunning, strings.Join(procs, " | "))
}
