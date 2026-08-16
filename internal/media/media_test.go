package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
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

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestIsAnimatedByContent：动图判定按内容而非扩展名——personal_emoji
// 的 gif 存成 .jpg（实测样本），扩展名门控会漏。
func TestIsAnimatedByContent(t *testing.T) {
	dir := t.TempDir()

	gifAsJpg := writeFile(t, dir, "41DDAFE1F79EC676817C8F54409C8031.jpg", gifBytes(t, 2))
	if !IsAnimated(gifAsJpg) {
		t.Error("gif content behind a .jpg extension must be detected as animated")
	}
	staticGif := writeFile(t, dir, "s.gif", gifBytes(t, 1))
	if IsAnimated(staticGif) {
		t.Error("single-frame gif must be static")
	}

	// 手拼 VP8X 头：RIFF + WEBP + VP8X + chunk size + flags（bit1 = ANIM）。
	mkWebP := func(flags byte) []byte {
		h := make([]byte, 24)
		copy(h[0:4], "RIFF")
		copy(h[8:12], "WEBP")
		copy(h[12:16], "VP8X")
		h[16] = 0x0a
		h[20] = flags
		return h
	}
	if !IsAnimated(writeFile(t, dir, "a.webp", mkWebP(0x02))) {
		t.Error("VP8X with animation bit must be animated")
	}
	if IsAnimated(writeFile(t, dir, "s.webp", mkWebP(0x00))) {
		t.Error("VP8X without animation bit must be static")
	}

	// APNG：签名 + acTL + IDAT（chunk 头即可，检测不看数据）。
	mkChunk := func(typ string, data []byte) []byte {
		out := make([]byte, 8+len(data)+4)
		binary.BigEndian.PutUint32(out[0:4], uint32(len(data)))
		copy(out[4:8], typ)
		copy(out[8:], data)
		return out
	}
	pngSig := []byte("\x89PNG\r\n\x1a\n")
	apng := append(append(append([]byte{}, pngSig...), mkChunk("acTL", make([]byte, 8))...), mkChunk("IDAT", []byte{0})...)
	if !IsAnimated(writeFile(t, dir, "a.png", apng)) {
		t.Error("acTL before IDAT must be APNG")
	}
	staticPng := append(append([]byte{}, pngSig...), mkChunk("IDAT", []byte{0})...)
	if IsAnimated(writeFile(t, dir, "s.png", staticPng)) {
		t.Error("png without acTL must be static")
	}

	jpeg := writeFile(t, dir, "x.jpg", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"))
	if IsAnimated(jpeg) {
		t.Error("plain jpeg must be static")
	}
	if IsAnimated(writeFile(t, dir, "y.bin", []byte("hello"))) {
		t.Error("unknown content must be treated as static")
	}
}

func TestFirstFrame(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.gif", gifBytes(t, 2))
	img, err := FirstFrame(p)
	if err != nil {
		t.Fatalf("FirstFrame: %v", err)
	}
	if img == nil || img.Bounds().Dx() != 1 {
		t.Fatalf("unexpected first frame: %v", img)
	}
	if _, err := FirstFrame(writeFile(t, dir, "x.jpg", []byte("\xff\xd8\xff\xe0"))); err == nil {
		t.Error("jpeg must not be decodable as animated first frame")
	}
}

// vp8BitsB64：2430 字节真实 VP8 位流（150×100，取自 x/image webp
// testdata 的 blue-purple-pink.lossy.webp 的 "VP8 " chunk 载荷）。
var vp8BitsB64 = "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"

// TestFirstFrameAnimatedWebP：动画 webp 容器的首帧提取（x/image/webp
// 无 ANMF 路径——「动图 webp 回退原文件继续动画」的根因）。
func TestFirstFrameAnimatedWebP(t *testing.T) {
	bits, err := base64.StdEncoding.DecodeString(vp8BitsB64)
	if err != nil {
		t.Fatal(err)
	}
	// VP8X（anim 标志 + 画布 150×100）+ ANIM + ANMF（16B 帧头 + 位流）
	chunk := func(cc string, payload []byte) []byte {
		out := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(out[0:4], cc)
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(payload)))
		copy(out[8:], payload)
		return out
	}
	// canvas width-1 / height-1 是 24 位小端
	putU24 := func(b []byte, v uint32) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }
	vp8xBody := make([]byte, 10)
	vp8xBody[0] = 0x02
	putU24(vp8xBody[4:7], 149)
	putU24(vp8xBody[7:10], 99)
	anmfBody := append(make([]byte, 16), bits...) // 帧头 16 字节全零（位流自带帧尺寸）
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(4+4+len(chunk("VP8X", vp8xBody))+len(chunk("ANIM", make([]byte, 6)))+len(chunk("ANMF", anmfBody))))
	buf.WriteString("WEBP")
	buf.Write(chunk("VP8X", vp8xBody))
	buf.Write(chunk("ANIM", make([]byte, 6)))
	buf.Write(chunk("ANMF", anmfBody))
	p := writeFile(t, t.TempDir(), "anim.webp", buf.Bytes())
	img, err := FirstFrame(p)
	if err != nil {
		t.Fatalf("FirstFrame(animated webp): %v", err)
	}
	if img == nil || img.Bounds().Dx() != 150 || img.Bounds().Dy() != 100 {
		t.Fatalf("unexpected first frame: %v", img)
	}
}

// TestFirstFrameAnimatedWebPVariants：ANMF 帧数据的三种真实形态——
// 完整 "VP8 " chunk（QQ 转换样本形态，实测 12MB 样本）/ ALPH 子块 +
// chunk / 裸位流——都必须能解出首帧。
func TestFirstFrameAnimatedWebPVariants(t *testing.T) {
	bits, err := base64.StdEncoding.DecodeString(vp8BitsB64)
	if err != nil {
		t.Fatal(err)
	}
	chunk := func(cc string, payload []byte) []byte {
		out := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(out[0:4], cc)
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(payload)))
		copy(out[8:], payload)
		return out
	}
	putU24 := func(b []byte, v uint32) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }
	build := func(t *testing.T, frameData []byte) string {
		t.Helper()
		vp8xBody := make([]byte, 10)
		vp8xBody[0] = 0x02
		putU24(vp8xBody[4:7], 149)
		putU24(vp8xBody[7:10], 99)
		anmfBody := append(make([]byte, 16), frameData...)
		var buf bytes.Buffer
		buf.WriteString("RIFF")
		_ = binary.Write(&buf, binary.LittleEndian, uint32(4+4+len(chunk("VP8X", vp8xBody))+len(chunk("ANIM", make([]byte, 6)))+len(chunk("ANMF", anmfBody))))
		buf.WriteString("WEBP")
		buf.Write(chunk("VP8X", vp8xBody))
		buf.Write(chunk("ANIM", make([]byte, 6)))
		buf.Write(chunk("ANMF", anmfBody))
		return writeFile(t, t.TempDir(), "anim.webp", buf.Bytes())
	}
	variants := map[string][]byte{
		"bare-bitstream":  bits,
		"full-vp8-chunk":  chunk("VP8 ", bits), // QQ 转换样本形态
		"alph-plus-chunk": append(chunk("ALPH", make([]byte, 4)), chunk("VP8 ", bits)...),
	}
	for name, frame := range variants {
		img, err := FirstFrame(build(t, frame))
		if err != nil {
			t.Fatalf("FirstFrame(%s): %v", name, err)
		}
		if img == nil || img.Bounds().Dx() != 150 {
			t.Fatalf("FirstFrame(%s): unexpected frame %v", name, img)
		}
	}
}

// TestPlaceholderPNG：兜底占位图必须是可解码 PNG（解码失败路径不回退
// 动画原图）。
func TestPlaceholderPNG(t *testing.T) {
	if len(PlaceholderPNG) < 4 || !bytes.Equal(PlaceholderPNG[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("placeholder is not a PNG")
	}
	if _, err := png.Decode(bytes.NewReader(PlaceholderPNG)); err != nil {
		t.Fatalf("placeholder decode: %v", err)
	}
}
