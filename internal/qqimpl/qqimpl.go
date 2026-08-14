// Package qqimpl 汇集所有已逆向的 QQ 实现（副作用导入：注册 probe 与
// 根路径候选）。新增一个 QQ 平台/版本实现时，在此加一行空白导入。
// 导入顺序不决定优先级：qq.Detect 两遍扫描，ScanCapable 的实现优先
//（见 internal/qq/qq.go）。
package qqimpl

import (
	_ "qqcleaner/internal/qq/impl/nt"
)
