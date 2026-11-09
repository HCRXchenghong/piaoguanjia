package ocr

import (
	"context"
	"encoding/base64"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"invoice/server/internal/config"
)

//go:embed winocr.ps1
var winocrScript string

// OCRResult Windows OCR 原始输出。
type OCRResult struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Angle  float64   `json:"angle"`
	Lang   string    `json:"lang"`
	Lines  []OCRLine `json:"lines"`
	Error  string    `json:"error"`
}

// OCRLine 一行文字及其单词坐标。
type OCRLine struct {
	Text  string     `json:"text"`
	Words []OCRWord  `json:"words"`
}

// OCRWord 单词坐标。
type OCRWord struct {
	T string  `json:"t"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

var (
	ocrAvailOnce sync.Once
	ocrAvail     bool
	ocrErr       error
	ocrSeq       int64
	ocrMu        sync.Mutex
)

// WindowsOCRAvailable 检测系统是否安装了中文识别引擎。
func WindowsOCRAvailable() bool {
	ocrAvailOnce.Do(func() {
		if runtime.GOOS != "windows" {
			ocrAvail = false
			return
		}
		ps, err := powershellPath()
		if err != nil {
			ocrErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-Command",
			"[void][Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType=WindowsRuntime];"+
				"if ([Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages.Count -gt 0) { '1' } else { '0' }")
		out, err := cmd.Output()
		ocrAvail = err == nil && len(out) > 0 && out[0] == '1'
		if err != nil {
			ocrErr = err
		}
	})
	return ocrAvail
}

// OCRError 返回检测过程中的错误。
func OCRError() error { return ocrErr }

func powershellPath() (string, error) {
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return p, nil
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", errors.New("未找到 powershell.exe")
}

// 脚本落地到数据目录（每次覆盖，便于升级）。
func ensureScript(cfg *config.Config) (string, error) {
	path := cfg.Path("winocr.ps1")
	if data, err := os.ReadFile(path); err == nil && string(data) == winocrScript {
		return path, nil
	}
	if err := os.WriteFile(path, []byte(winocrScript), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// recognizeWithWindows 用 Windows.Media.Ocr 识别 PNG。
func recognizeWithWindows(cfg *config.Config, png []byte) (*OCRResult, error) {
	if !WindowsOCRAvailable() {
		return nil, errors.New("当前系统没有可用的离线 OCR 引擎")
	}
	ps, err := powershellPath()
	if err != nil {
		return nil, err
	}
	script, err := ensureScript(cfg)
	if err != nil {
		return nil, err
	}

	ocrMu.Lock()
	ocrSeq++
	seq := ocrSeq
	ocrMu.Unlock()

	base := fmt.Sprintf("ocr_%d_%d_%d", os.Getpid(), seq, time.Now().UnixNano()%1e6)
	imgPath := filepath.Join(cfg.DataDir, "tmp", base+".png")
	outPath := filepath.Join(cfg.DataDir, "tmp", base+".json")
	defer os.Remove(imgPath)
	defer os.Remove(outPath)

	if err := os.WriteFile(imgPath, png, 0o644); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(imgPath))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", script, "-InB64", encoded, "-OutPath", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := string(out)
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return nil, fmt.Errorf("OCR 进程失败: %v (%s)", err, msg)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, errors.New("OCR 未产生输出")
	}
	var result OCRResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("OCR 结果解析失败: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("OCR 失败: %s", result.Error)
	}
	return &result, nil
}

// RecognizeImage 解码 + 预处理 + OCR。
func RecognizeImage(cfg *config.Config, data []byte) (*OCRResult, error) {
	img, err := Decode(data)
	if err != nil {
		return nil, err
	}
	prepared := PrepareForOCR(img)
	png, err := EncodePNG(prepared)
	if err != nil {
		return nil, err
	}
	result, err := RecognizePNG(cfg, png)
	if err != nil {
		return nil, err
	}
	result.Width, result.Height = prepared.Bounds().Dx(), prepared.Bounds().Dy()
	return result, nil
}

// atoiSafe 小工具。
func atoiSafe(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
