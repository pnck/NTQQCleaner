//go:build windows

package platform

import (
	"os"
	"os/exec"
	"strings"
)

type windowsAdapter struct{}

func defaultAdapter() Adapter { return windowsAdapter{} }

// QQProcesses 用 tasklist 匹配 QQ.exe。
func (windowsAdapter) QQProcesses() []string {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq QQ.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	var procs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(strings.ToLower(line), "qq.exe") {
			procs = append(procs, line)
		}
	}
	return procs
}

// DeleteFile 的 Windows 语义：只读属性的文件 DeleteFile 会失败
// （POSIX unlink 不会）——先清只读属性再重试；仍失败则返回最初的错误。
func (windowsAdapter) DeleteFile(path string) error {
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	// 常见于「只读属性」：清除后重试
	if cerr := os.Chmod(path, 0o666); cerr == nil {
		err = os.Remove(path)
	}
	return err
}

// MoveFile 的 Windows 语义：同卷 rename，跨卷复制后删除；
// 只读属性同样会阻止移动，清除后重试。
func (windowsAdapter) MoveFile(src, dst string) error {
	err := RenameOrCopy(src, dst)
	if err == nil {
		return nil
	}
	if cerr := os.Chmod(src, 0o666); cerr == nil {
		err = RenameOrCopy(src, dst)
	}
	return err
}

// Reveal 用资源管理器定位文件（explorer /select）。
func (windowsAdapter) Reveal(path string) error {
	return exec.Command("explorer", "/select,", path).Start()
}

// OpenFile 用默认程序打开文件（cmd /c start）。
func (windowsAdapter) OpenFile(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}

// FreezeAnimatedThumbs：Windows 专有显示政策——WebView2 没有关闭图片
// 动画的设置项（CoreWebView2Settings 全表无此类能力，实测确认），
// 照片墙里几十个动图同时解码会持续占满 CPU（gif 每帧解码为整幅位图）。
// 缩略图一律取首帧静态变体（?static=1，后端解码首帧输出 PNG）。
func (windowsAdapter) FreezeAnimatedThumbs() bool { return true }
