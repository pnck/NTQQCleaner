//go:build darwin

package qq

import (
	"os"
	"path/filepath"
)

// RootCandidates macOS：沙盒容器内的 QQ 数据根
// （~/Library/Containers/com.tencent.qq/Data/Library/Application Support/QQ）。
func RootCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home,
		"Library", "Containers", "com.tencent.qq", "Data", "Library", "Application Support", "QQ")}
}
