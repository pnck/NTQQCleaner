// Package platform 是 OS 适配层：一切依赖操作系统 API 的能力都收敛
// 在这里，上层（clean / app）只依赖本包接口。
// 新增平台 = 新增一个 build-tagged 文件实现 Adapter，上层零改动。
package platform

// Adapter 是每个操作系统必须提供的能力集合。
type Adapter interface {
	// QQProcesses 返回疑似 QQ 客户端进程的命令行（空 = 未运行）。
	// 匹配模式属于平台适配的一部分（进程名/路径随 OS 不同）。
	QQProcesses() []string

	// Reveal 在系统文件管理器中定位文件（仅展示，不操作文件）。
	Reveal(path string) error
}

// impl 由各平台 build-tagged 文件的 defaultAdapter() 初始化。
var impl Adapter = defaultAdapter()

// Current 返回当前平台的适配器。
func Current() Adapter { return impl }

// Install 替换适配器（测试或未来动态适配时使用）。
func Install(a Adapter) { impl = a }

// IsQQRunning 便捷方法：是否有疑似 QQ 的进程在运行。
func IsQQRunning() bool { return len(impl.QQProcesses()) > 0 }
