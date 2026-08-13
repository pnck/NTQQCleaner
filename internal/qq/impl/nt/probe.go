package nt

import (
	"os"

	"qqcleaner/internal/qq"
)

// Probe 探测 NT 布局：根目录下存在 nt_qq_<32hex> 实例目录。
func Probe(root string) qq.Knowledge {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() && instanceRe.MatchString(e.Name()) {
			return &NT{}
		}
	}
	return nil
}

func init() { qq.RegisterProbe(Probe) }
