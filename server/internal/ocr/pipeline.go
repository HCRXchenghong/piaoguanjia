package ocr

import (
	"bytes"
	"path/filepath"
	"strings"
	"time"

	"invoice/server/internal/config"
	"invoice/server/internal/model"
)

// Analyze 识别入口：PDF 文字层 → Windows OCR → PDF 内嵌图片 OCR。
func Analyze(cfg *config.Config, data []byte, filename string, settings map[string]string) *model.ParseResult {
	started := time.Now()
	ext := strings.ToLower(filepath.Ext(filename))
	var tokens []Token
	engine := "none"
	pages := 1
	warnings := []string{}

	isPDF := ext == ".pdf" || bytes.HasPrefix(data, []byte("%PDF-"))
	if isPDF {
		doc, err := OpenPDF(data)
		if err != nil {
			warnings = append(warnings, "PDF 解析失败："+err.Error())
		} else {
			text := doc.ExtractText()
			pages = text.Pages
			cjk := countCJK(text.Text)
			if cjk >= 15 {
				tokens = pdfTokens(text.Segments)
				engine = "pdf-text"
			} else {
				images := doc.ExtractImages()
				if len(images) == 0 {
					warnings = append(warnings, "PDF 没有文字层，也没有可提取的图片")
				} else {
					for i, im := range images {
						if i >= 2 {
							break
						}
						var pngBytes []byte
						if im.Kind == "jpeg" {
							img, err := Decode(im.Data)
							if err != nil {
								continue
							}
							pngBytes, err = EncodePNG(PrepareForOCR(img))
							if err != nil {
								continue
							}
						} else {
							raw := RawToImage(im.Data, im.CS, im.BPC, im.Width, im.Height)
							if raw == nil {
								continue
							}
							pngBytes, _ = EncodePNG(PrepareForOCR(raw))
						}
						if len(pngBytes) == 0 {
							continue
						}
						result, err := RecognizePNG(cfg, pngBytes)
						if err != nil {
							warnings = append(warnings, "PDF 图片识别失败："+err.Error())
							continue
						}
						tokens = append(tokens, ocrTokens(result)...)
						engine = "pdf-image-ocr"
					}
				}
			}
		}
	} else {
		img, err := Decode(data)
		if err != nil {
			warnings = append(warnings, "图片解析失败："+err.Error())
		} else {
			pngBytes, err := EncodePNG(PrepareForOCR(img))
			if err != nil {
				warnings = append(warnings, "图片预处理失败："+err.Error())
			} else if result, err := RecognizePNG(cfg, pngBytes); err != nil {
				warnings = append(warnings, err.Error())
			} else {
				tokens = ocrTokens(result)
				engine = "windows-ocr"
			}
		}
	}

	rows := BuildRows(tokens)
	parsed := Parse(rows, settings)
	parsed.Engine = engine
	parsed.Pages = pages
	parsed.FileName = filename
	parsed.ElapsedMS = time.Since(started).Milliseconds()
	parsed.Warnings = append(warnings, parsed.Warnings...)
	if parsed.Suggest == nil {
		parsed.Suggest = map[string]any{"type": "procurement"}
	}
	return parsed
}

// AnalyzeImage 仅对图片做 OCR，返回原始行（用于调试）。
func AnalyzeImage(cfg *config.Config, data []byte) ([]Token, error) {
	img, err := Decode(data)
	if err != nil {
		return nil, err
	}
	pngBytes, err := EncodePNG(PrepareForOCR(img))
	if err != nil {
		return nil, err
	}
	result, err := RecognizePNG(cfg, pngBytes)
	if err != nil {
		return nil, err
	}
	return ocrTokens(result), nil
}

func countCJK(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			n++
		}
	}
	return n
}

func pdfTokens(segments []Segment) []Token {
	out := make([]Token, 0, len(segments))
	for _, s := range segments {
		text := NormalizeLine(s.Text)
		if text == "" {
			continue
		}
		size := s.Size
		if size <= 0 {
			size = 12
		}
		out = append(out, Token{
			Text: text,
			X:    s.X,
			Y:    s.Y,
			W:    textWidth(text, size),
			H:    size,
			Src:  "pdf",
		})
	}
	return out
}

func ocrTokens(result *OCRResult) []Token {
	var out []Token
	for _, line := range result.Lines {
		for _, w := range line.Words {
			text := NormalizeLine(w.T)
			if text == "" {
				continue
			}
			out = append(out, Token{Text: text, X: w.X, Y: w.Y, W: w.W, H: w.H, Src: "ocr"})
		}
	}
	return out
}
