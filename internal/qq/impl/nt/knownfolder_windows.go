//go:build windows

package nt

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// knownFolderDocuments 用 SHGetKnownFolderPath 解析 FOLDERID_Documents
// （shell32 动态加载，无需 cgo，docs/08 §3.6）。失败返回 ""，由调用方
// 回退注册表/环境变量。
func knownFolderDocuments() string {
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetKnownFolderPath")
	var p *uint16
	// FOLDERID_Documents = {FDD39AD0-238F-46AF-ADB4-6C85480369C7}
	guid := windows.GUID{
		Data1: 0xFDD39AD0,
		Data2: 0x238F,
		Data3: 0x46AF,
		Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7},
	}
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&guid)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if r != 0 {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}
