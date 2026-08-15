// Package media 提供媒体文件的内容判定。MIME 魔数识别用标准库
// net/http.DetectContentType（浏览器同款内容嗅探算法）；动图语义与
// 首帧解码是本包职责——覆盖常见动图格式：多帧 gif / 动画 webp（含
// 手机互传常见的动画容器）/ APNG。MIME 判定只报告 image/gif 等类型，
// 不区分静态与动画。
//
// 消费者：classify（扫描期动图标记，经 platform 政策门控）与
// app.Backend（?static=1 首帧静态变体）。QQ 缓存的扩展名与内容常
// 不符（personal_emoji 的 gif 存为 .jpg，docs/01 §3）——判定一律按
// 内容，不信任扩展名。
package media

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"

	"golang.org/x/image/riff"
	"golang.org/x/image/webp"
)

// IsAnimated 判定文件是否为动图（多帧 gif / 动画 webp / APNG）。
// 任何读/解析错误按静态处理——该标记只门控一个显示优化（照片墙动图
// 缩略图取首帧静态变体），不参与可清性判断。
func IsAnimated(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, _ := br.Peek(512) // Peek 不消费，后续解析仍从文件头开始
	switch http.DetectContentType(head) {
	case "image/gif":
		return animatedGIF(br)
	case "image/webp":
		return animatedWebP(br)
	case "image/png":
		return animatedPNG(br)
	}
	return false
}

// FirstFrame 解码动图的第一帧（gif / 动画 webp / APNG）。按内容 MIME
// 分派；非可解码类型/解码失败返回错误，调用方以 PlaceholderPNG 兜底
// （动图缩略图绝不得回退动画原图）。
func FirstFrame(abs string) (image.Image, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, _ := br.Peek(512)
	switch http.DetectContentType(head) {
	case "image/gif":
		return gif.Decode(br) // 只解第一帧
	case "image/webp":
		if img, err := webp.Decode(br); err == nil {
			return img, nil
		}
		// 动画 webp：x/image/webp 没有 ANMF chunk 处理路径，动画容器
		// 解码失败——从容器提取首帧位流，重建最小 RIFF 容器解码。
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return firstWebPFrame(f)
	case "image/png":
		return png.Decode(br) // APNG：默认图像 = 第一帧
	}
	return nil, fmt.Errorf("media: no first-frame decoder for %s", http.DetectContentType(head))
}

// PlaceholderPNG 是 2×2 中性灰 PNG——首帧解码失败的兜底图。
var PlaceholderPNG = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, color.RGBA{128, 128, 128, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}()

// firstWebPFrame 从动画 webp 容器提取首帧位流（第一个 ANMF chunk 内的
// VP8/VP8L 位流——与 "VP8 "/"VP8L" chunk 的载荷同构），包装成最小
// RIFF 容器解码。
func firstWebPFrame(r io.Reader) (image.Image, error) {
	formType, rr, err := riff.NewReader(r)
	if err != nil {
		return nil, err
	}
	if formType != (riff.FourCC{'W', 'E', 'B', 'P'}) { // if 头里的复合字面量必须加括号
		return nil, errors.New("media: not a webp container")
	}
	for {
		chunkID, chunkLen, chunkData, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunkID != (riff.FourCC{'A', 'N', 'M', 'F'}) {
			continue
		}
		if chunkLen < 16 {
			return nil, errors.New("media: truncated ANMF chunk")
		}
		hdr := make([]byte, 16)
		if _, err := io.ReadFull(chunkData, hdr); err != nil {
			return nil, err
		}
		bitstream, err := io.ReadAll(chunkData)
		if err != nil {
			return nil, err
		}
		return decodeWebPBitstream(bitstream)
	}
	return nil, errors.New("media: no ANMF frame found")
}

// decodeWebPBitstream 把 ANMF 内的位流包装成最小 RIFF 容器交给
// x/image/webp（VP8X 头不放进合成容器——位流自带帧尺寸）。帧头不做
// 硬编码魔数判定（编码器差异多，实测样本带前缀字节）——依次尝试
// VP8 / VP8L 包装，以解码成功为准。
func decodeWebPBitstream(bs []byte) (image.Image, error) {
	for _, cc := range []string{"VP8 ", "VP8L"} {
		if img, err := decodeWrappedWebP(cc, bs); err == nil {
			return img, nil
		}
	}
	return nil, errors.New("media: unrecognized ANMF bitstream")
}

func decodeWrappedWebP(cc string, bs []byte) (image.Image, error) {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(4+4+4+len(bs)+(len(bs)&1)))
	buf.WriteString("WEBP")
	buf.WriteString(cc)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(bs)))
	buf.Write(bs)
	if len(bs)&1 == 1 {
		buf.WriteByte(0) // RIFF chunk 偶对齐
	}
	return webp.Decode(bytes.NewReader(buf.Bytes()))
}

// animatedGIF 只解第一帧（gif.Decode 精确读到帧 1 数据结束）；下一个
// 字节不是 trailer（0x3B）则还有后续帧。
func animatedGIF(r io.Reader) bool {
	br := bufio.NewReader(r)
	if _, err := gif.Decode(br); err != nil {
		return false
	}
	b, err := br.Peek(1)
	if err != nil {
		return false
	}
	return b[0] != 0x3B
}

// animatedWebP 检查 VP8X chunk 的 flags 字节：bit 1 = ANIM。
// VP8/VP8L 无 VP8X chunk，恒为静态。
func animatedWebP(r io.Reader) bool {
	var hdr [24]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return false
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WEBP" || string(hdr[12:16]) != "VP8X" {
		return false
	}
	return hdr[20]&0x02 != 0
}

// animatedPNG 扫描 PNG chunk 结构：首个 IDAT 前出现 acTL = APNG。
// chunk 长度做合理性上限——acTL 必须出现在文件头部。
func animatedPNG(r io.Reader) bool {
	br := bufio.NewReader(r)
	sig := make([]byte, 8)
	if _, err := io.ReadFull(br, sig); err != nil {
		return false
	}
	if !bytes.Equal(sig, []byte("\x89PNG\r\n\x1a\n")) {
		return false
	}
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return false
		}
		length := binary.BigEndian.Uint32(hdr[0:4])
		switch string(hdr[4:8]) {
		case "acTL":
			return true
		case "IDAT", "IEND":
			return false
		}
		if length > 1<<26 { // 异常长度：结构不可信
			return false
		}
		if _, err := br.Discard(int(length) + 4); err != nil { // data + CRC
			return false
		}
	}
}
