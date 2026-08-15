//go:build wails

package main

import (
	"context"
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"qqcleaner/internal/app"
	"qqcleaner/internal/logring"
	"qqcleaner/internal/platform"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

// openInspectorOnStartup 由构建注入（ldflags -X main.openInspectorOnStartup=1）：
// 开发构建自动打开 WebKit inspector，封版构建为空即不打开。
var openInspectorOnStartup = ""

// wailsEmitter forwards Engine/Backend events to the frontend via the
// Wails event bus (docs/04 §3).
type wailsEmitter struct{ ctx context.Context }

func (w *wailsEmitter) Emit(ev string, data any) {
	if w.ctx != nil {
		runtime.EventsEmit(w.ctx, ev, data)
	}
}

// Dialogs hosts native dialogs. Wails v2 has no frontend dialog bridge
// (the injected runtime.js contains no dialog functions in any v2
// version) — dialogs are Go-side only. Keeping the cleanup confirmation
// in Go also matches the redline "UI 不可信" (docs/06 §5b).
// Bound as window.go.main.Dialogs.
type Dialogs struct{ ctx context.Context }

// PickDirectory prompts for a directory; returns "" when cancelled.
func (d *Dialogs) PickDirectory(title string) (string, error) {
	return runtime.OpenDirectoryDialog(d.ctx, runtime.OpenDialogOptions{Title: title})
}

// ConfirmClean shows the pre-cleanup final confirmation dialog. Contract:
// returns "Yes" or "No" on every OS (frontend compares against "Yes").
func (d *Dialogs) ConfirmClean(msg string) (string, error) {
	return d.confirmYesNo("确认清理", msg)
}

// ConfirmYesNo 通用 YES/NO 确认对话框（危险操作二次确认，如 QQ 运行守卫
// 的覆盖确认）；返回 "Yes" / "No"。
func (d *Dialogs) ConfirmYesNo(title, msg string) (string, error) {
	return d.confirmYesNo(title, msg)
}

// confirmYesNo 统一为系统标准 YES/NO 形态。Windows 走平台层的
// TaskDialogIndirect（现代化样式；wails 的 MessageDialog 在 Windows 上
// 是 MessageBoxW——旧式 WIN32 样式且完全忽略 Buttons 自定义文案，
// WarningDialog 只会渲染单 OK 按钮返回 "Ok"，此前与前端「清理」文案比较
// 永远不相等，表现为「点了确认没反应」）。darwin/linux 平台层不支持时
// 回退 wails MessageDialog（macOS NSAlert 按 Buttons 文案显示并返回所选
// 文案，本就是本机现代化样式）。两边契约一致：返回 "Yes" / "No"，
// 默认按钮 No（回车不误触发危险动作）。
func (d *Dialogs) confirmYesNo(title, msg string) (string, error) {
	if ok, err := platform.Current().ConfirmYesNo(title, msg); err == nil {
		if ok {
			return "Yes", nil
		}
		return "No", nil
	}
	return runtime.MessageDialog(d.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       msg,
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No", // 安全默认：回车不触发危险动作
		CancelButton:  "No",
	})
}

// fitWindowToScreen 启动时按主屏分辨率自适应窗口尺寸：约 85%×84% 的
// 屏幕（上限 1600×1000、下限与 MinWidth/MinHeight 一致），设置后居中。
// ScreenGetAll 需要 runtime ctx（OnStartup 之后才可用），窗口先以默认
// 尺寸创建、随即调整一次。Size 是逻辑像素（Windows 侧已按 DPI 折算），
// 与 WindowSetSize 同一坐标系。
func fitWindowToScreen(ctx context.Context) {
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return
	}
	sc := screens[0]
	for _, s := range screens {
		if s.IsPrimary {
			sc = s
			break
		}
	}
	if sc.Size.Width <= 0 || sc.Size.Height <= 0 {
		return
	}
	w := int(float64(sc.Size.Width) * 0.85)
	h := int(float64(sc.Size.Height) * 0.84)
	if w > 1600 {
		w = 1600
	}
	if h > 1000 {
		h = 1000
	}
	if w < 960 {
		w = 960
	}
	if h < 600 {
		h = 600
	}
	if w > sc.Size.Width {
		w = sc.Size.Width
	}
	if h > sc.Size.Height {
		h = sc.Size.Height
	}
	runtime.WindowSetSize(ctx, w, h)
	runtime.WindowCenter(ctx)
}

// runGUI starts the embedded web UI. Only the backend is bound — the
// frontend can reach the filesystem exclusively through the whitelisted
// preview handler (docs/06 §5b).
func runGUI() error {
	// panic 时把环形缓冲写进崩溃文件后重新 panic（运行时崩溃转储随后
	// 追加进同一文件）。
	defer logring.Recover()
	logring.Logf("gui starting")
	backend := app.NewBackend(configPath(), nil)
	emitter := &wailsEmitter{}
	dlgs := &Dialogs{}
	return wails.Run(&options.App{
		Title:     "NTQQ Cleaner",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  frontendAssets,
			Handler: http.HandlerFunc(backend.PreviewHandler),
		},
		OnStartup: func(ctx context.Context) {
			emitter.ctx = ctx
			dlgs.ctx = ctx
			backend.SetEmitter(emitter)
			fitWindowToScreen(ctx)
		},
		// 开发构建自动打开 inspector（需 -tags debug + ldflags -X 注入）
		Debug: options.Debug{OpenInspectorOnStartup: openInspectorOnStartup == "1"},
		Bind:  []interface{}{backend, dlgs},
	})
}
