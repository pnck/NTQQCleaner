package platform

import (
	"io"
	"os"
	"path/filepath"
)

// RenameOrCopy 跨平台的「移动」实现：同卷 rename；跨卷（EXDEV 或
// rename 失败）复制内容后删除源，保留权限与 mtime。Windows 与 POSIX
// 的跨卷行为都由本函数统一处理。
func RenameOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
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
	return os.Remove(src)
}

// unixBase 提供 POSIX 语义的默认实现（darwin/linux 复用）。
type unixBase struct{}

func (unixBase) DeleteFile(path string) error { return os.Remove(path) }

func (unixBase) MoveFile(src, dst string) error { return RenameOrCopy(src, dst) }

// FreezeAnimatedThumbs：darwin/linux 不静态化——WKWebView/webkit2gtk
// 对动图解码的 CPU 开销可接受，动图在照片墙正常播放。
func (unixBase) FreezeAnimatedThumbs() bool { return false }
