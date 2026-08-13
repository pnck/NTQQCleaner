//go:build windows

package qq

import (
	"os"
	"path/filepath"
)

// RootCandidates Windows：新版 NT 架构在 %APPDATA%\Tencent\QQ，
// 旧版候选 Documents\Tencent Files（需实现时探测）。
func RootCandidates() []string {
	var out []string
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		out = append(out, filepath.Join(appdata, "Tencent", "QQ"))
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		out = append(out, filepath.Join(profile, "Documents", "Tencent Files"))
	}
	return out
}
