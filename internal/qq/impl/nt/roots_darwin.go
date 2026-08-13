//go:build darwin

package nt

import (
	"os"
	"path/filepath"

	"qqcleaner/internal/qq"
)

func init() {
	qq.RegisterRoots(func() []string {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		return []string{filepath.Join(home,
			"Library", "Containers", "com.tencent.qq", "Data", "Library", "Application Support", "QQ")}
	})
}
