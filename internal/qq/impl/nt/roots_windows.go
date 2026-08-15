//go:build windows

package nt

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"qqcleaner/internal/qq"
)

// Windows 数据根只有一个候选：<Documents>\Tencent Files（真机实测，
// docs/08 §2.1）。%APPDATA%\Tencent\QQ 实测不存在，已从候选删除。
func init() {
	qq.RegisterRoots(func() []string {
		if tf := tencentFilesRoot(); tf != "" {
			return []string{tf}
		}
		return nil
	})
}

// tencentFilesRoot 解析 <Documents>\Tencent Files（docs/08 §3.6）。
// Documents 定位链（真机实测 Documents 可能重定向到其它盘符/OneDrive）：
//  1. SHGetKnownFolderPath(FOLDERID_Documents)——最权威，见 knownfolder_windows.go
//  2. 注册表 User Shell Folders\Personal（%VAR% 展开）
//  3. 环境变量 Documents（实测机上该变量即指向重定向位置）
//  4. %USERPROFILE%\Documents
func tencentFilesRoot() string {
	if p := knownFolderDocuments(); p != "" {
		return filepath.Join(p, "Tencent Files")
	}
	if p := registryPersonal(); p != "" {
		return filepath.Join(p, "Tencent Files")
	}
	if p := os.Getenv("Documents"); p != "" {
		return filepath.Join(p, "Tencent Files")
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		return filepath.Join(home, "Documents", "Tencent Files")
	}
	return ""
}

func registryPersonal() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Personal")
	if err != nil {
		return ""
	}
	return os.ExpandEnv(v) // 注册表值常为 %USERPROFILE%\Documents 形式
}
