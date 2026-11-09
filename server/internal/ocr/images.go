package ocr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	ocrMinWidth = 1800
	ocrMaxWidth = 2600
)

// Decode 解码常见图片格式。
func Decode(data []byte) (image.Image, error) {
	if len(data) == 0 {
		return nil, errors.New("空文件")
	}
	if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := webp.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := bmp.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := gif.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	return nil, errors.New("不支持的图片格式")
}

// PrepareForOCR 放大到合适字号并增强对比度，Windows OCR 在这个尺度下最准。
func PrepareForOCR(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := 1.0
	if w < ocrMinWidth {
		scale = float64(ocrMinWidth) / float64(w)
		if scale > 2.6 {
			scale = 2.6
		}
	}
	if float64(w)*scale > ocrMaxWidth {
		scale = float64(ocrMaxWidth) / float64(w)
	}
	var scaled image.Image = img
	if scale > 1.02 {
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
		scaled = dst
	}

	// 灰度 + 自动对比度
	sb := scaled.Bounds()
	gray := image.NewGray(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	hist := make([]int, 256)
	draw.Draw(gray, gray.Bounds(), scaled, sb.Min, draw.Src)
	for i := 0; i < len(gray.Pix); i++ {
		hist[gray.Pix[i]]++
	}
	low, high := percentile(hist, 0.01), percentile(hist, 0.99)
	if high-low < 32 {
		low, high = 0, 255
	}
	span := float64(high - low)
	lookup := make([]uint8, 256)
	for i := range lookup {
		v := (float64(i) - float64(low)) / span * 255
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		lookup[i] = uint8(v)
	}
	for i := 0; i < len(gray.Pix); i++ {
		gray.Pix[i] = lookup[gray.Pix[i]]
	}
	return gray
}

func percentile(hist []int, p float64) int {
	total := 0
	for _, v := range hist {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := int(float64(total) * p)
	sum := 0
	for i, v := range hist {
		sum += v
		if sum >= target {
			return i
		}
	}
	return 255
}

// EncodePNG 编码为 PNG。
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Thumbnail 生成预览图（JPEG）。
func Thumbnail(img image.Image, maxW int) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	if w > maxW {
		nw, nh := maxW, h*maxW/w
		if nh < 1 {
			nh = 1
		}
		scaled := image.NewRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), dst, dst.Bounds(), xdraw.Over, nil)
		dst = scaled
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RawToImage 把 PDF 里的原始像素转成图片。
func RawToImage(data []byte, cs string, bpc, width, height int) image.Image {
	if width <= 0 || height <= 0 {
		return nil
	}
	channels := 3
	mode := "rgb"
	switch {
	case contains(cs, "Gray"):
		channels, mode = 1, "gray"
	case contains(cs, "CMYK"):
		channels, mode = 4, "cmyk"
	}
	needed := width * height * channels
	if bpc != 8 || len(data) < needed {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height; i++ {
		var c color.RGBA
		switch mode {
		case "gray":
			v := data[i]
			c = color.RGBA{v, v, v, 255}
		case "cmyk":
			k := data[i*4+3]
			c = color.RGBA{
				uint8(255 - minByte(data[i*4], k)),
				uint8(255 - minByte(data[i*4+1], k)),
				uint8(255 - minByte(data[i*4+2], k)),
				255,
			}
		default:
			c = color.RGBA{data[i*3], data[i*3+1], data[i*3+2], 255}
		}
		img.SetRGBA(i%width, i/width, c)
	}
	return img
}

func minByte(a, b byte) byte {
	if a < b {
		return a
	}
	return b
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && bytes.Contains([]byte(s), []byte(sub))
}
