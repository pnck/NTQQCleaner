package app

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestServeFirstFrame：动图 → 首帧 PNG；解码失败/非动图扩展名 → 原文件回退。
func TestServeFirstFrame(t *testing.T) {
	dir := t.TempDir()
	gifPath := filepath.Join(dir, "a.gif")
	var buf bytes.Buffer
	frames := []*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}),
		image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.White, color.Black}),
	}
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: frames, Delay: []int{0, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gifPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &Backend{}
	req := httptest.NewRequest(http.MethodGet, "/preview/1?static=1", nil)

	rec := httptest.NewRecorder()
	b.serveFirstFrame(rec, req, gifPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	// PNG magic: 89 50 4E 47
	body := rec.Body.Bytes()
	if len(body) < 4 || !bytes.Equal(body[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("response is not a PNG")
	}

	// 不可解码文件：回退占位 PNG（200）——?static=1 只挂在已判定的动图
	// 上，解码失败绝不回退动画原图（那会让照片墙重新开始播放）。
	rec2 := httptest.NewRecorder()
	b.serveFirstFrame(rec2, req, filepath.Join(dir, "b.txt"))
	if rec2.Code != http.StatusOK {
		t.Fatalf("fallback for undecodable file = %d, want 200 placeholder", rec2.Code)
	}
	body2 := rec2.Body.Bytes()
	if len(body2) < 4 || !bytes.Equal(body2[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("placeholder fallback is not a PNG")
	}
}

// TestWallSafeExt：媒体扩展名不可作为 <img> 回退（配对缩略图被清理后
// 墙面空瓦片而非图裂，docs/07 §4.1）；图片与未知扩展名保持回退。
func TestWallSafeExt(t *testing.T) {
	for _, ext := range []string{"mp4", "MOV", "webm", "mp3", "m4a"} {
		if wallSafeExt(ext) {
			t.Errorf("media ext %q must not be wall-safe", ext)
		}
	}
	for _, ext := range []string{"png", "jpg", "gif", "webp", "dat", "unknownext"} {
		if !wallSafeExt(ext) {
			t.Errorf("ext %q should fall back to self", ext)
		}
	}
}

// TestPreviewURLVersioned：预览 URL 带每次扫描的 epoch 版本参数
// （docs/07 §4.1）——清理后重扫复用 id 空间，版本参数防浏览器缓存
// （含 404）跨扫描串台。
func TestPreviewURLVersioned(t *testing.T) {
	b := &Backend{previewBase: "http://127.0.0.1:1"}
	if got := b.previewURL(3, 7); got != "http://127.0.0.1:1/preview/7?v=3" {
		t.Fatalf("got %q", got)
	}
	fallback := &Backend{}
	if got := fallback.previewURL(0, 5); got != "/preview/5?v=0" {
		t.Fatalf("got %q", got)
	}
}
