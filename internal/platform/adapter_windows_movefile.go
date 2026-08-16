//go:build windows

package platform

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Windows 删除/移动语义重做（docs/09 §3.1）：os.Rename（MoveFileExW
// 无 COPY_ALLOWED）+ POSIX 风格复制兜底无法处理 Windows 的共享冲突与
// 跨卷组合——QQ 以不含 FILE_SHARE_DELETE 的共享模式持有缓存句柄时，
// rename/delete 被拒（ERROR_SHARING_VIOLATION），跨卷时每个文件都走
// 「复制→删源」兜底，占用错误集中在删源一步。本文件实现三级阶梯：
// ① 共享/锁定冲突退避重试（瞬态锁：杀软/缩略图）；② 持久锁登记
// MOVEFILE_DELAY_UNTIL_REBOOT（重启生效，返回 ErrDeferredReboot）；
// ③ 跨卷移动复制后删源（删源同阶梯；已备份 + 源登记重启删除不算
// 降级——备份副本已生成）。
//
// 全部纯 syscall（x/sys/windows），无 cgo——容器内可交叉编译。

const (
	movefileReplaceExisting  = 0x1
	movefileDelayUntilReboot = 0x4
)

// retryDelays 是共享/锁定冲突的退避序列（docs/09 §3.1：
// 100ms/500ms/2s——杀软实时扫描与缩略图缓存锁多为瞬态）。
var retryDelays = [...]time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

// inUseErr 判断是否为可重试的共享/锁定冲突（32/33）。
func inUseErr(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

// deleteLadder 三级删除：DeleteFileW → 冲突退避重试 → 重启删除登记。
func deleteLadder(path string) error {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	err = windows.DeleteFile(ptr)
	if err == nil {
		return nil
	}
	if !inUseErr(err) {
		return err
	}
	// 瞬态锁退避重试。
	for _, d := range retryDelays {
		time.Sleep(d)
		if err = windows.DeleteFile(ptr); err == nil {
			return nil
		}
		if !inUseErr(err) {
			return err
		}
	}
	// 持久锁：登记重启删除（需管理员；登记失败原样返回占用错误）。
	// MoveFileExW(src, nil, DELAY_UNTIL_REBOOT) 写系统
	// PendingFileRenameOperations，重启时生效。
	if regErr := windows.MoveFileEx(ptr, nil, movefileDelayUntilReboot); regErr == nil {
		return fmt.Errorf("%w（已登记重启后删除）", ErrDeferredReboot)
	}
	return err
}

// moveLadder 三级移动：同卷 MoveFileExW → 冲突退避重试 →
// 同卷重启移动登记；跨卷（ERROR_NOT_SAME_DEVICE）复制 + deleteLadder
// 删源（备份副本已生成后再登记删源不是降级——docs/09 §3.1）。
func moveLadder(src, dst string) error {
	srcPtr, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPtr, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err = windows.MoveFileEx(srcPtr, dstPtr, movefileReplaceExisting)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) {
		// 跨卷：复制 + 删源（删源同三级阶梯）。
		if cerr := copyFile(src, dst); cerr != nil {
			return cerr
		}
		return deleteLadder(src)
	}
	if !inUseErr(err) {
		return err
	}
	for _, d := range retryDelays {
		time.Sleep(d)
		if err = windows.MoveFileEx(srcPtr, dstPtr, movefileReplaceExisting); err == nil {
			return nil
		}
		if !inUseErr(err) {
			return err
		}
	}
	// 持久锁 + 同卷：登记重启时执行移动（同卷 rename 可登记）。
	if regErr := windows.MoveFileEx(srcPtr, dstPtr, movefileReplaceExisting|movefileDelayUntilReboot); regErr == nil {
		return fmt.Errorf("%w（已登记重启后移动）", ErrDeferredReboot)
	}
	return err
}

// copyFile 跨卷移动的复制段：保留权限与 mtime，失败清理目标残留。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	os.Chtimes(dst, info.ModTime(), info.ModTime())
	return nil
}
