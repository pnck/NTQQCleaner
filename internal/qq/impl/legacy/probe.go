package legacy

import (
	"os"
	"path/filepath"
	"strconv"

	"qqcleaner/internal/qq"
)

// Probe 探测旧版布局：存在含 msg3.0.db 的数字账号目录。
func Probe(root string) qq.Knowledge {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "msg3.0.db")); err == nil {
			return &Legacy{}
		}
	}
	return nil
}

func init() { qq.RegisterProbe(Probe) }
