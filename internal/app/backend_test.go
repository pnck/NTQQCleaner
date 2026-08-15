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
