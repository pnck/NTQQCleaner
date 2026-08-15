package classify

import (
	"bufio"
	"image/gif"
	"io"
	"os"
	"path/filepath"
)

// sniffAnimated reports whether the file at abs is an animated image
// (multi-frame gif or animated webp). Any read/parse error is treated as
// static — the flag only gates a display optimization (photos-wall
// thumbnails render the first frame statically instead of all animating
// at once), never correctness.
func sniffAnimated(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	switch filepath.Ext(abs) {
	case ".gif":
		return animatedGIF(f)
	case ".webp":
		return animatedWebP(f)
	}
	return false
}

// animatedGIF decodes only the first frame (gif.Decode reads exactly
// through frame 1's data); if the next byte is not the trailer (0x3B),
// more frames follow.
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

// animatedWebP checks the VP8X chunk flags byte: bit 1 = animation (ANIM).
// VP8/VP8L files have no VP8X chunk and are always static.
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
