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

// ---- 统一抽象：按内容 MIME 注册的格式处理器 ----
// 每种动图格式提供两个能力：动图判定（IsAnimated 用）与首帧解码
// （FirstFrame 用）。MIME 判定与分派是通用入口（sniffMIME + 注册表
// 查找）——**新增格式 = 注册一个 handler，不改动任何分派逻辑**。
// 处理器接收路径而非 reader：各格式自行打开（小文件，webp 回退路径
// 需要从头重读，路径语义最干净）。
type formatHandler struct {
	animated func(abs string) bool                 // 是否为动图（解析失败一律静态）
	first    func(abs string) (image.Image, error) // 首帧解码
}

var formatHandlers = map[string]formatHandler{
	"image/gif":  {animated: animatedGIF, first: gifFirstFrame},
	"image/png":  {animated: animatedPNG, first: pngFirstFrame}, // APNG：默认图像 = 第一帧
	"image/webp": {animated: animatedWebP, first: webpFirstFrame},
}

func gifFirstFrame(abs string) (image.Image, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return gif.Decode(f)
}

func pngFirstFrame(abs string) (image.Image, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// webpFirstFrame：x/image/webp 没有 ANMF chunk 处理路径（动画容器
// 解码失败，上游不支持）——失败时回退到容器级首帧提取。
func webpFirstFrame(abs string) (image.Image, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	if img, err := webp.Decode(br); err == nil {
		return img, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return firstWebPFrame(f)
}

// sniffMIME 按内容魔数判定文件类型（不信任扩展名）。
func sniffMIME(abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, _ := br.Peek(512)
	return http.DetectContentType(head), nil
}

// IsAnimated 判定文件是否为动图（多帧 gif / 动画 webp / APNG）。
// 任何读/解析错误按静态处理——该标记只门控一个显示优化（照片墙动图
// 缩略图取首帧静态变体），不参与可清性判断。
func IsAnimated(abs string) bool {
	mime, err := sniffMIME(abs)
	if err != nil {
		return false
	}
	h, ok := formatHandlers[mime]
	if !ok || h.animated == nil {
		return false
	}
	return h.animated(abs)
}

// FirstFrame 解码动图的第一帧。按内容 MIME 分派到注册的格式处理器；
// 非可解码类型/解码失败返回错误，调用方以 PlaceholderPNG 兜底
// （动图缩略图绝不得回退动画原图）。
func FirstFrame(abs string) (image.Image, error) {
	mime, err := sniffMIME(abs)
	if err != nil {
		return nil, err
	}
	h, ok := formatHandlers[mime]
	if !ok || h.first == nil {
		return nil, fmt.Errorf("media: no first-frame decoder for %s", mime)
	}
	return h.first(abs)
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
		// 帧数据形态（实测 QQ 样本 + 编码器差异）：16 字节帧头之后是
		// ① 可选 ALPH 子块（4cc + size + data + 偶对齐 pad），
		// ② 图片数据——或为完整 "VP8 "/"VP8L" chunk（4cc + size + 位流
		// + pad，QQ 转换样本即此形态），或为裸位流（规范最小形态）。
		// 两者都必须接得住：把 chunk 头剥掉得到位流再解码。
		br := bufio.NewReader(chunkData)
		if tag, err := br.Peek(4); err == nil && string(tag) == "ALPH" {
			var ahdr [8]byte
			if _, err := io.ReadFull(br, ahdr[:]); err != nil {
				return nil, err
			}
			alen := binary.LittleEndian.Uint32(ahdr[4:8])
			if _, err := br.Discard(int(alen) + int(alen&1)); err != nil { // data + 偶对齐 pad
				return nil, err
			}
		}
		if tag, err := br.Peek(4); err == nil && (string(tag) == "VP8 " || string(tag) == "VP8L") {
			// 完整 chunk 形态：剥 4cc + size 取位流
			var vhdr [8]byte
			if _, err := io.ReadFull(br, vhdr[:]); err != nil {
				return nil, err
			}
			vlen := binary.LittleEndian.Uint32(vhdr[4:8])
			bitstream := make([]byte, vlen)
			if _, err := io.ReadFull(br, bitstream); err != nil {
				return nil, err
			}
			return decodeWebPBitstream(bitstream)
		}
		// 裸位流形态
		bitstream, err := io.ReadAll(br)
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
func animatedGIF(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	br := bufio.NewReader(f)
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
func animatedWebP(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	var hdr [24]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return false
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WEBP" || string(hdr[12:16]) != "VP8X" {
		return false
	}
	return hdr[20]&0x02 != 0
}

// animatedPNG 扫描 PNG chunk 结构：首个 IDAT 前出现 acTL = APNG。
// chunk 长度做合理性上限——acTL 必须出现在文件头部。
func animatedPNG(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	br := bufio.NewReader(f)
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
