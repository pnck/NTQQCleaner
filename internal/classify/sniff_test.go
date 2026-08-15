package classify

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func gifBytes(t *testing.T, frames int) []byte {
	t.Helper()
	var buf bytes.Buffer
	imgs := make([]*image.Paletted, frames)
	dels := make([]int, frames)
	for i := 0; i < frames; i++ {
		imgs[i] = image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
		imgs[i].Set(0, 0, color.White)
		dels[i] = 0
	}
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: imgs, Delay: dels}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnimatedGIF(t *testing.T) {
	if animatedGIF(bytes.NewReader(gifBytes(t, 1))) {
		t.Error("single-frame gif must be static")
	}
	if !animatedGIF(bytes.NewReader(gifBytes(t, 2))) {
		t.Error("two-frame gif must be animated")
	}
	if animatedGIF(bytes.NewReader([]byte("not a gif"))) {
		t.Error("garbage must be treated as static")
	}
}

func TestAnimatedWebP(t *testing.T) {
	// 手拼 VP8X 头：RIFF + WEBP + VP8X + chunk size + flags（bit1 = ANIM）。
	mk := func(flags byte) []byte {
		h := make([]byte, 24)
		copy(h[0:4], "RIFF")
		copy(h[8:12], "WEBP")
		copy(h[12:16], "VP8X")
		h[16] = 0x0a // chunk size
		h[20] = flags
		return h
	}
	if !animatedWebP(bytes.NewReader(mk(0x02))) {
		t.Error("VP8X with animation bit must be animated")
	}
	if animatedWebP(bytes.NewReader(mk(0x00))) {
		t.Error("VP8X without animation bit must be static")
	}
	if animatedWebP(bytes.NewReader([]byte("RIFF\x00\x00\x00\x00WEBPVP8 \x00\x00\x00\x00"))) {
		t.Error("plain VP8 must be static")
	}
	if animatedWebP(bytes.NewReader([]byte("garbage"))) {
		t.Error("garbage must be treated as static")
	}
}
