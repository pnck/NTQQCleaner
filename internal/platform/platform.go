// Package platform 是 OS 适配层：一切依赖操作系统 API 的能力都收敛
// 在这里，上层（clean / app）只依赖本包接口。
// 新增平台 = 新增一个 build-tagged 文件实现 Adapter，上层零改动。
//
// 删除/移动也属于本层：POSIX 的 unlink 与 Windows 的 DeleteFile 语义
// 不同（只读属性、共享模式、跨卷移动），由各平台实现负责。
package platform

// Adapter 是每个操作系统必须提供的能力集合。
type Adapter interface {
	// QQProcesses 返回疑似 QQ 客户端进程的命令行（空 = 未运行）。
	// 匹配模式属于平台适配的一部分（进程名/路径随 OS 不同）。
	QQProcesses() []string

	// DeleteFile 按本平台语义删除文件。
	DeleteFile(path string) error

	// MoveFile 移动文件（同卷 rename；跨卷复制后删除）。
	MoveFile(src, dst string) error

	// Reveal 在系统文件管理器中定位文件（仅展示，不操作文件）。
	Reveal(path string) error

	// OpenFile 用系统默认程序打开文件（审计报告等）。
	OpenFile(path string) error

	// FreezeAnimatedThumbs 报告本平台是否必须把动图缩略图静态化
	// （照片墙/预览缩略图只显示首帧静态变体，点 ▶ / 自动播放后仍是
	// 动画原图）。Windows 专有政策：WebView2 没有任何关闭图片动画的
	// 设置（CoreWebView2Settings 无此项），墙内几十个动图同时解码
	// 极耗 CPU；darwin/linux 保持动图正常播放。
	FreezeAnimatedThumbs() bool
}

// impl 由各平台 build-tagged 文件的 defaultAdapter() 初始化。
var impl Adapter = defaultAdapter()

// Current 返回当前平台的适配器。
func Current() Adapter { return impl }

// Install 替换适配器（测试或未来动态适配时使用）。
func Install(a Adapter) { impl = a }

// IsQQRunning 便捷方法：是否有疑似 QQ 的进程在运行。
func IsQQRunning() bool { return len(impl.QQProcesses()) > 0 }
