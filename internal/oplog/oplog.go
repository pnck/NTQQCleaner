// Package oplog 是 GUI 的操作日志：时间戳行直接输出到 stdout——
// 把 exe 拖进 PowerShell 运行时，控制台与 GUI 并行滚动 ops 日志，
// 进程被外部击毙（TerminateProcess）时控制台最后一行即死点；双击
// 启动（windowsgui 无控制台）时写入静默失败、零成本。CLI 不启用：
// stdout 已被结构化输出（--json）占用，精确记录由审计文件承担
// （docs/06 §3）。
//
// 机制极简（docs/09 §3.5）：不写文件、不设 flag、无清理逻辑、无
// 删除成败判断——日志流随进程死亡自然终结，留下的就是最后一行。
package oplog

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	mu sync.Mutex
	on bool
)

// Enable 开启输出（GUI 启动时调用）。
func Enable() { mu.Lock(); on = true; mu.Unlock() }

// Disable 关闭输出（测试隔离用）。
func Disable() { mu.Lock(); on = false; mu.Unlock() }

// Printf 输出一行带时间戳的日志（未启用时 no-op）。
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !on {
		return
	}
	fmt.Fprintf(os.Stdout, "%s %s\n",
		time.Now().Format("2006-01-02T15:04:05.000"), fmt.Sprintf(format, args...))
}
