//go:build windows

package legacy

import (
	"os"
	"path/filepath"

	"qqcleaner/internal/qq"
)

func init() {
	qq.RegisterRoots(func() []string {
		if profile := os.Getenv("USERPROFILE"); profile != "" {
			return []string{filepath.Join(profile, "Documents", "Tencent Files")}
		}
		return nil
	})
}
