package clean

import (
	"fmt"
	"strings"

	"qqcleaner/internal/platform"
)

// 进程守卫：检测能力在 internal/platform 适配层（每 OS 一个实现），
// 这里保留可注入的函数变量供测试替换。

var qqRunningFunc = func() bool { return platform.IsQQRunning() }

var qqProcessesFunc = func() []string { return platform.Current().QQProcesses() }

// QQRunning 供 UI/Backend 预检使用。
func QQRunning() bool { return qqRunningFunc() }

// qqRunningError 构建拒绝错误并附匹配进程，让用户看到什么触发了守卫。
func qqRunningError() error {
	procs := qqProcessesFunc()
	if len(procs) == 0 {
		return ErrQQRunning
	}
	return fmt.Errorf("%w；匹配进程：%s", ErrQQRunning, strings.Join(procs, " | "))
}
