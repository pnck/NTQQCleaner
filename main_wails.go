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

// ConfirmClean shows the pre-cleanup confirmation dialog and returns the
// chosen button label ("清理" or "取消").
func (d *Dialogs) ConfirmClean(msg string) (string, error) {
	return runtime.MessageDialog(d.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "确认清理",
		Message:       msg,
		Buttons:       []string{"清理", "取消"},
		DefaultButton: "取消", // 安全默认：回车不触发清理
		CancelButton:  "取消",
	})
}

// runGUI starts the embedded web UI. Only the backend is bound — the
// frontend can reach the filesystem exclusively through the whitelisted
// preview handler (docs/06 §5b).
func runGUI() error {
	backend := app.NewBackend(configPath(), nil)
	emitter := &wailsEmitter{}
	dlgs := &Dialogs{}
	return wails.Run(&options.App{
		Title:     "QQ Cleaner",
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
		},
		// 开发构建自动打开 inspector（需 -tags debug + ldflags -X 注入）
		Debug: options.Debug{OpenInspectorOnStartup: openInspectorOnStartup == "1"},
		Bind:  []interface{}{backend, dlgs},
	})
}
