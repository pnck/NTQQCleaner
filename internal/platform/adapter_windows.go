//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createNoWindow（CREATE_NO_WINDOW）抑制控制台子进程自建可见窗口：
// 本 exe 是 windowsgui 子系统（无控制台），cmd 等控制台程序作为子
// 进程派生时会自建一个控制台窗口。
const createNoWindow = 0x08000000

var noConsoleWindow = &syscall.SysProcAttr{CreationFlags: createNoWindow}

type windowsAdapter struct{}

func defaultAdapter() Adapter { return windowsAdapter{} }

// QQProcesses 用 Toolhelp32 快照 API 枚举 QQ.exe（纯 syscall）——
// 不 spawn tasklist 子进程：无控制台闪窗、无进程创建开销，也不在
// 枚举 QQ 进程时向防护组件暴露「陌生进程派生命令」的挑衅行为。
func (windowsAdapter) QQProcesses() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var procs []string
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, "qq.exe") {
			procs = append(procs, fmt.Sprintf("qq.exe (pid %d)", entry.ProcessID))
		}
	}
	return procs
}

// DeleteFile 的 Windows 语义（docs/09 §3.1）：只读属性先行清除
// （DeleteFileW 对只读文件恒失败，POSIX unlink 不会），删除本体走
// deleteLadder 三级阶梯——共享冲突退避重试、持久锁登记重启删除
// （成功时返回 platform.ErrDeferredReboot，由上层计入 reboot）。
func (windowsAdapter) DeleteFile(path string) error {
	err := deleteLadder(path)
	if err == nil || errors.Is(err, ErrDeferredReboot) {
		return err
	}
	// 常见于「只读属性」：清除后重试
	if cerr := os.Chmod(path, 0o666); cerr == nil {
		err = deleteLadder(path)
	}
	return err
}

// MoveFile 的 Windows 语义（docs/09 §3.1）：moveLadder 三级阶梯——
// 同卷 MoveFileExW(REPLACE_EXISTING)；跨卷复制后删源（删源同阶梯）；
// 持久锁同卷登记重启移动、跨卷在备份已生成后登记重启删源。
// 只读属性同样会阻止移动，清除后重试。
func (windowsAdapter) MoveFile(src, dst string) error {
	err := moveLadder(src, dst)
	if err == nil || errors.Is(err, ErrDeferredReboot) {
		return err
	}
	if cerr := os.Chmod(src, 0o666); cerr == nil {
		err = moveLadder(src, dst)
	}
	return err
}

// Reveal 用资源管理器定位文件（explorer /select）。
func (windowsAdapter) Reveal(path string) error {
	return exec.Command("explorer", "/select,", path).Start()
}

// OpenFile 用默认程序打开文件（cmd /c start，隐藏 cmd 窗口——
// 被打开的默认程序照常显示自己的窗口）。
func (windowsAdapter) OpenFile(path string) error {
	cmd := exec.Command("cmd", "/c", "start", "", path)
	cmd.SysProcAttr = noConsoleWindow
	return cmd.Start()
}

// FreezeAnimatedThumbs：Windows 专有显示政策——WebView2 没有关闭图片
// 动画的设置项（CoreWebView2Settings 全表无此类能力，实测确认），
// 照片墙里几十个动图同时解码会持续占满 CPU（gif 每帧解码为整幅位图）。
// 缩略图一律取首帧静态变体（?static=1，后端解码首帧输出 PNG）。
func (windowsAdapter) FreezeAnimatedThumbs() bool { return true }
