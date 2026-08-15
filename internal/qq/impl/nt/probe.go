package nt

import "qqcleaner/internal/qq"

// Probe 探测 NT 布局（docs/08 §3.3）：当前 OS spec 能在根目录枚举到
// ≥1 个账号实例目录。枚举逻辑与 InstanceDirs 同一份（单一事实来源）：
//   - mac: 根下存在 nt_qq_<32hex> 目录；
//   - windows: 根下存在数字目录且含 nt_qq/nt_data（纯旧版残留的数字
//     目录由 legacy probe 认领）。
func Probe(root string) qq.Knowledge {
	k := &NT{}
	insts, err := k.InstanceDirs(root)
	if err != nil || len(insts) == 0 {
		return nil
	}
	return k
}

func init() { qq.RegisterProbe(Probe) }
