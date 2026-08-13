//go:build linux

package platform

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type linuxAdapter struct{ unixBase }

func defaultAdapter() Adapter { return linuxAdapter{} }

// QQProcesses：Linux 桌面版 QQ 若存在，进程形态与 macOS 近似
// （QQ.app 路径不适用时按需调整此模式——适配器层的职责所在）。
func (linuxAdapter) QQProcesses() []string {
	out, err := exec.Command("pgrep", "-fl", "QQ.app/Contents/MacOS/QQ").Output()
	if err != nil {
		return nil
	}
	var procs []string
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			procs = append(procs, s)
		}
	}
	return procs
}

// Reveal 用默认文件管理器打开所在目录（xdg-open）。
func (linuxAdapter) Reveal(path string) error {
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}
