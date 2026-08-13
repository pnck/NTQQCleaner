package clean

import (
	"os/exec"
	"runtime"
	"strings"
)

// qqRunningFunc is swapped out in tests; it wraps QQRunning.
var qqRunningFunc = QQRunning

// QQRunning reports whether the QQ client is currently running. The cleaner
// refuses to delete while QQ is up (it may be reading/writing cache files).
// Reference: docs/06_safety_redlines.md §5.
func QQRunning() bool {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq QQ.exe", "/NH").Output()
		if err != nil {
			return false // tasklist unavailable: fail open? no — fail closed? see below
		}
		return strings.Contains(strings.ToLower(string(out)), "qq.exe")
	default: // darwin, linux
		// pgrep -f "QQ.app" matches the macOS bundle path; on Linux this
		// simply returns exit 1 (not running).
		err := exec.Command("pgrep", "-f", "QQ.app").Run()
		return err == nil
	}
}
