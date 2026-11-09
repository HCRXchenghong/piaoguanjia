package ocr

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"invoice/server/internal/config"
	"invoice/server/internal/store"
)

// EngineInfo 描述当前可用的离线识别引擎。
type EngineInfo struct {
	Name      string `json:"name"`      // windows-ocr / tesseract / none
	Label     string `json:"label"`     // 展示名
	Available bool   `json:"available"` // 是否可用
	Detail    string `json:"detail"`    // 补充说明（语言包、安装提示等）
	Platform  string `json:"platform"`
}

var (
	engineMu    sync.Mutex
	engineCache = map[string]EngineInfo{}
)

// Engine 返回指定偏好下的引擎信息（结果缓存，避免反复探测）。
// pref: auto | windows | tesseract | none
func Engine(pref string) EngineInfo {
	if pref == "" {
		pref = "auto"
	}
	engineMu.Lock()
	defer engineMu.Unlock()
	if info, ok := engineCache[pref]; ok {
		return info
	}
	info := detectEngine(pref)
	engineCache[pref] = info
	return info
}

// CurrentEngine 按系统设置解析当前引擎。
func CurrentEngine() EngineInfo {
	return Engine(store.GetSetting("ocr_engine"))
}

func detectEngine(pref string) EngineInfo {
	info := EngineInfo{Platform: runtime.GOOS}
	winOK := runtime.GOOS == "windows" && WindowsOCRAvailable()
	tessOK, tessDetail := TesseractAvailable()

	pick := func(name string) EngineInfo {
		switch name {
		case "windows-ocr":
			info.Name, info.Label, info.Available = "windows-ocr", "Windows 内置 OCR", winOK
			if winOK {
				info.Detail = "系统自带，无需安装"
			} else {
				info.Detail = "当前系统没有可用的中文识别语言包"
			}
		case "tesseract":
			info.Name, info.Label, info.Available = "tesseract", "Tesseract OCR", tessOK
			info.Detail = tessDetail
		default:
			info.Name, info.Label, info.Available = "none", "未启用", false
		}
		return info
	}

	switch pref {
	case "none":
		return pick("none")
	case "windows":
		return pick("windows-ocr")
	case "tesseract":
		return pick("tesseract")
	}

	// auto：Windows 优先用系统引擎（零安装），其余平台用 Tesseract
	if winOK {
		return pick("windows-ocr")
	}
	if tessOK {
		return pick("tesseract")
	}
	info.Name, info.Label, info.Available = "none", "未启用", false
	if runtime.GOOS == "windows" {
		info.Detail = "可在「设置 → 时间和语言 → 语言」中安装中文 OCR 语言包"
	} else {
		info.Detail = "安装 Tesseract 及中文语言包：sudo apt install tesseract-ocr tesseract-ocr-chi-sim"
	}
	return info
}

// RecognizePNG 用当前可用引擎识别 PNG 字节。
func RecognizePNG(cfg *config.Config, png []byte) (*OCRResult, error) {
	info := CurrentEngine()
	switch info.Name {
	case "windows-ocr":
		return recognizeWithWindows(cfg, png)
	case "tesseract":
		return recognizeWithTesseract(cfg, png)
	}
	if info.Detail != "" {
		return nil, &EngineUnavailableError{Detail: info.Detail}
	}
	return nil, &EngineUnavailableError{Detail: "未检测到可用的离线 OCR 引擎"}
}

// EngineUnavailableError 表示没有可用引擎。
type EngineUnavailableError struct{ Detail string }

func (e *EngineUnavailableError) Error() string { return e.Detail }

// CheckTesseractBinary 供外部探测使用。
func CheckTesseractBinary() (string, bool) {
	path, err := exec.LookPath("tesseract")
	if err != nil {
		return "", false
	}
	return path, true
}

// joinUnique 小工具：去重拼接。
func joinUnique(items []string, sep string) string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return strings.Join(out, sep)
}
