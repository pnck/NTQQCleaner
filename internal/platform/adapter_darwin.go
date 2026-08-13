//go:build darwin

package platform

import (
	"os/exec"
	"strings"
)

type darwinAdapter struct{ unixBase }

func defaultAdapter() Adapter { return darwinAdapter{} }

// QQProcesses 用 pgrep 匹配 QQ.app 应用包内的真实二进制，
// 锚定完整路径以避免误报（如路径含 "QQ" 的其它应用）。
func (darwinAdapter) QQProcesses() []string {
	out, err := exec.Command("pgrep", "-fl", "QQ.app/Contents/MacOS/QQ").Output()
	if err != nil {
		return nil // pgrep 无匹配时退出码非零
	}
	var procs []string
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			procs = append(procs, s)
		}
	}
	return procs
}

// Reveal 用 Finder 定位文件（open -R）。
func (darwinAdapter) Reveal(path string) error {
	return exec.Command("open", "-R", path).Start()
}
