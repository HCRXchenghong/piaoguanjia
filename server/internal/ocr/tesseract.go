package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"invoice/server/internal/config"
)

// Tesseract 引擎：Linux / macOS 上的离线识别方案，需安装 tesseract-ocr 与中文语言包。

var (
	tessOnce   sync.Once
	tessOK     bool
	tessDetail string
	tessLang   string
)

// TesseractAvailable 探测可执行文件与中文语言包。
func TesseractAvailable() (bool, string) {
	tessOnce.Do(func() {
		bin, ok := CheckTesseractBinary()
		if !ok {
			tessOK, tessDetail = false, "未安装 Tesseract：sudo apt install tesseract-ocr tesseract-ocr-chi-sim"
			return
		}
		out, err := exec.Command(bin, "--list-langs").CombinedOutput()
		if err != nil {
			tessOK, tessDetail = false, "无法执行 tesseract --list-langs"
			return
		}
		langs := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "List of available") {
				continue
			}
			langs[line] = true
		}
		var picked []string
		hasChi := false
		for _, l := range []string{"chi_sim", "chi_tra"} {
			if langs[l] {
				picked = append(picked, l)
				hasChi = true
				break
			}
		}
		if langs["eng"] {
			picked = append(picked, "eng")
		}
		if !hasChi {
			tessOK = false
			tessDetail = "缺少中文语言包：sudo apt install tesseract-ocr-chi-sim"
			return
		}
		tessOK = true
		tessLang = strings.Join(picked, "+")
		tessDetail = "语言包：" + tessLang
	})
	return tessOK, tessDetail
}

func tesseractLang() string {
	TesseractAvailable()
	if tessLang == "" {
		return "eng"
	}
	return tessLang
}

func tesseractPSM() string {
	if v := strings.TrimSpace(os.Getenv("INVOICE_TESSERACT_PSM")); v != "" {
		return v
	}
	return "6"
}

// recognizeWithTesseract 调 tesseract 输出 TSV，再按坐标还原成行。
func recognizeWithTesseract(cfg *config.Config, png []byte) (*OCRResult, error) {
	bin, ok := CheckTesseractBinary()
	if !ok {
		return nil, &EngineUnavailableError{Detail: tessDetail}
	}
	if ok, detail := TesseractAvailable(); !ok {
		return nil, &EngineUnavailableError{Detail: detail}
	}

	dir := filepath.Join(cfg.DataDir, "tmp")
	_ = os.MkdirAll(dir, 0o755)
	name := fmt.Sprintf("tess_%d_%d.png", os.Getpid(), time.Now().UnixNano()%1e9)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, png, 0o644); err != nil {
		return nil, err
	}
	defer os.Remove(path)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, path, "stdout",
		"-l", tesseractLang(), "--psm", tesseractPSM(), "tsv")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tesseract 执行失败: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("tesseract 执行失败: %w", err)
	}
	result := parseTesseractTSV(string(out))
	result.Lang = tesseractLang()
	return result, nil
}

// parseTesseractTSV 解析 tesseract 的 TSV 输出（level=5 为单词）。
func parseTesseractTSV(text string) *OCRResult {
	type key struct{ page, block, par, line int }
	groups := map[key][]OCRWord{}
	var order []key

	maxX, maxY := 0.0, 0.0
	pageW, pageH := 0.0, 0.0
	for _, raw := range strings.Split(text, "\n") {
		fields := strings.Split(strings.TrimRight(raw, "\r"), "\t")
		if len(fields) < 11 {
			continue
		}
		// level=1 是整页，用它记录画布尺寸
		if strings.TrimSpace(fields[0]) == "1" {
			pageW, pageH = parseF(fields[8]), parseF(fields[9])
			continue
		}
		if strings.TrimSpace(fields[0]) != "5" {
			continue
		}
		conf, err := strconv.ParseFloat(strings.TrimSpace(fields[10]), 64)
		if err != nil || conf <= 0 {
			continue
		}
		word := ""
		if len(fields) > 11 {
			word = strings.TrimSpace(strings.Join(fields[11:], " "))
		}
		if word == "" {
			continue
		}
		left := parseF(fields[6])
		top := parseF(fields[7])
		w := parseF(fields[8])
		h := parseF(fields[9])
		page, _ := strconv.Atoi(fields[1])
		block, _ := strconv.Atoi(fields[2])
		par, _ := strconv.Atoi(fields[3])
		line, _ := strconv.Atoi(fields[4])
		k := key{page, block, par, line}
		if _, exists := groups[k]; !exists {
			order = append(order, k)
		}
		groups[k] = append(groups[k], OCRWord{T: word, X: left, Y: top, W: w, H: h})
		if left+w > maxX {
			maxX = left + w
		}
		if top+h > maxY {
			maxY = top + h
		}
	}

	if pageW == 0 {
		pageW, pageH = maxX, maxY
	}
	result := &OCRResult{Width: int(pageW), Height: int(pageH)}
	for _, k := range order {
		words := groups[k]
		sort.SliceStable(words, func(i, j int) bool { return words[i].X < words[j].X })
		parts := make([]string, 0, len(words))
		for _, w := range words {
			parts = append(parts, w.T)
		}
		result.Lines = append(result.Lines, OCRLine{
			Text:  strings.Join(parts, " "),
			Words: words,
		})
	}
	sort.SliceStable(result.Lines, func(i, j int) bool {
		yi := result.Lines[i].Words
		yj := result.Lines[j].Words
		if len(yi) == 0 || len(yj) == 0 {
			return false
		}
		return yi[0].Y < yj[0].Y
	})
	return result
}

func parseF(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}
