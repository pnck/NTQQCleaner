//go:build windows

package platform

import (
	"os/exec"
	"strings"
)

type windowsAdapter struct{}

func defaultAdapter() Adapter { return windowsAdapter{} }

// QQProcesses 用 tasklist 匹配 QQ.exe。
func (windowsAdapter) QQProcesses() []string {
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
}

// Reveal 用资源管理器定位文件（explorer /select）。
func (windowsAdapter) Reveal(path string) error {
	return exec.Command("explorer", "/select,", path).Start()
}
