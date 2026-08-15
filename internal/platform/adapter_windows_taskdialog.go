//go:build windows

package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConfirmYesNo 用 TaskDialogIndirect（comctl32，Vista+）弹现代化确认
// 对话框。wails 的 MessageDialog 在 Windows 上走 MessageBoxW——旧式
// WIN32 样式、无自定义按钮、忽略按钮文案，与 UI 割裂；TaskDialog 是
// MessageBox 的官方现代继任者（主题化渲染，wails 上游 issue #4233
// 也在提议迁到它）。契约：返回 true = Yes。Esc/关闭 = No（fail-safe）。
//
// 纯 syscall 实现（与 WebView2 后端同理），无 cgo——容器内可交叉编译。
// 现代主题需要 exe manifest 声明 comctl32 v6（Wails 打包自带 manifest）。
func (windowsAdapter) ConfirmYesNo(title, message string) (bool, error) {
	comctl32 := windows.NewLazySystemDLL("comctl32.dll")
	taskDialogIndirect := comctl32.NewProc("TaskDialogIndirect")

	yesText, err := windows.UTF16PtrFromString("Yes")
	if err != nil {
		return false, err
	}
	noText, err := windows.UTF16PtrFromString("No")
	if err != nil {
		return false, err
	}
	buttons := [2]taskDialogButton{
		{nButtonID: 1, pszButtonText: yesText},
		{nButtonID: 2, pszButtonText: noText},
	}

	titlePtr, _ := windows.UTF16PtrFromString(title)
	msgPtr, _ := windows.UTF16PtrFromString(message)
	appPtr, _ := windows.UTF16PtrFromString("NTQQ Cleaner")

	const (
		tdWarningIcon               = 0xFFFF // MAKEINTRESOURCE(-1)
		tdUseHiconMain              = 0x0002
		tdfAllowDialogCancellation  = 0x0008
		tdfPositionRelativeToWindow = 0x1000
		idiWarning                  = 32515
	)

	// 图标：优先 LoadIconW 的 HICON（TDF_USE_HICON_MAIN——经典样式
	// （无 comctl32 v6 主题）下同样渲染）；失败则回退 MAKEINTRESOURCE
	// 的 TD_WARNING_ICON。
	icon := loadStandardIcon(idiWarning)

	var cfg taskDialogConfig
	cfg.cbSize = uint32(unsafe.Sizeof(cfg))
	cfg.hwndParent = foregroundWindow()
	cfg.dwFlags = tdfAllowDialogCancellation | tdfPositionRelativeToWindow
	cfg.pszWindowTitle = appPtr
	// 现代 TaskDialog 布局：大标题（主指令）+ 正文 + 图标，不再是
	// 单调的两行文本。
	cfg.pszMainInstruction = titlePtr
	cfg.pszContent = msgPtr
	if icon != 0 {
		cfg.dwFlags |= tdUseHiconMain
		cfg.mainIcon = icon
	} else {
		cfg.mainIcon = tdWarningIcon
	}
	cfg.cButtons = uint32(len(buttons))
	cfg.pButtons = &buttons[0]
	cfg.nDefaultButton = 2 // No：回车不触发危险动作（安全默认）

	var pressed int32
	ret, _, _ := taskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&cfg)),
		uintptr(unsafe.Pointer(&pressed)),
		0, 0)
	if ret != 0 { // HRESULT：0 = S_OK
		return false, fmt.Errorf("TaskDialogIndirect: HRESULT 0x%x", ret)
	}
	// 仅按钮 1（Yes）返回 true；No/IDCANCEL(2)/异常值一律 false。
	return pressed == 1, nil
}

func foregroundWindow() uintptr {
	user32 := windows.NewLazySystemDLL("user32.dll")
	hwnd, _, _ := user32.NewProc("GetForegroundWindow").Call()
	return hwnd
}

// loadStandardIcon 加载系统标准图标（IDI_WARNING 等），失败返回 0。
func loadStandardIcon(id uintptr) uintptr {
	user32 := windows.NewLazySystemDLL("user32.dll")
	h, _, _ := user32.NewProc("LoadIconW").Call(0, id)
	return h
}

// taskDialogConfig 镜像 commctrl.h 的 TASKDIALOGCONFIG（Win64 布局：
// 指针 8 字节对齐）。字段按声明顺序排列，Go 的自然对齐与 Win64 C
// 布局一致（cbSize=176）。仅支持 64 位目标（windows-amd64/arm64，
// 项目不构建 386）。
type taskDialogConfig struct {
	cbSize                  uint32
	hwndParent              uintptr
	hInstance               uintptr
	dwFlags                 uint32
	dwCommonButtons         uint32
	pszWindowTitle          *uint16
	mainIcon                uintptr // union：HICON / PCWSTR（TD_*_ICON 走 MAKEINTRESOURCE）
	pszMainInstruction      *uint16
	pszContent              *uint16
	cButtons                uint32
	pButtons                *taskDialogButton
	nDefaultButton          int32
	cRadioButtons           uint32
	pRadioButtons           *taskDialogButton
	nDefaultRadioButton     int32
	pszVerificationText     *uint16
	pszExpandedInformation  *uint16
	pszExpandedControlText  *uint16
	pszCollapsedControlText *uint16
	footerIcon              uintptr // union：HICON / PCWSTR
	pszFooter               *uint16
	pfCallback              uintptr
	lpCallbackData          uintptr
	cxWidth                 uint32
}

// taskDialogButton 镜像 TASKDIALOG_BUTTON。
type taskDialogButton struct {
	nButtonID     int32
	pszButtonText *uint16
}
