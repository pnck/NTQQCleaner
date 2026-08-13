//go:build windows

package nt

import (
	"os"
	"path/filepath"

	"qqcleaner/internal/qq"
)

func init() {
	qq.RegisterRoots(func() []string {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return []string{filepath.Join(appdata, "Tencent", "QQ")}
		}
		return nil
	})
}
