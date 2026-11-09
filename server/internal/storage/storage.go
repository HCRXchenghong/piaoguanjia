package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"invoice/server/internal/config"
	"invoice/server/internal/model"
	"invoice/server/internal/ocr"
	"invoice/server/internal/store"
)

// SavedFile 落盘结果。
type SavedFile struct {
	StorageName  string
	OriginalName string
	Mime         string
	Ext          string
	Size         int64
	SHA256       string
	Path         string
}

var magicTable = []struct {
	Prefix []byte
	Mime   string
	Ext    string
}{
	{[]byte("%PDF-"), "application/pdf", ".pdf"},
	{[]byte{0xFF, 0xD8, 0xFF}, "image/jpeg", ".jpg"},
	{[]byte{0x89, 0x50, 0x4E, 0x47}, "image/png", ".png"},
	{[]byte("GIF87a"), "image/gif", ".gif"},
	{[]byte("GIF89a"), "image/gif", ".gif"},
	{[]byte("BM"), "image/bmp", ".bmp"},
}

// DetectType 以文件头为准判断类型，避免伪造扩展名。
func DetectType(data []byte, filename string) (string, string, error) {
	for _, m := range magicTable {
		if bytes.HasPrefix(data, m.Prefix) {
			if m.Mime == "image/gif" {
				return "image/png", ".png", nil
			}
			return m.Mime, m.Ext, nil
		}
	}
	if len(data) > 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp", ".webp", nil
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf", ".pdf", nil
	case ".jpg", ".jpeg":
		return "image/jpeg", ".jpg", nil
	case ".png":
		return "image/png", ".png", nil
	case ".webp":
		return "image/webp", ".webp", nil
	case ".bmp":
		return "image/bmp", ".bmp", nil
	}
	return "", "", errors.New("不支持的文件格式，仅支持 PDF / JPG / PNG / WEBP / BMP")
}

// MaxUploadBytes 读取设置项。
func MaxUploadBytes() int64 {
	mb, err := strconv.Atoi(store.GetSetting("max_upload_mb"))
	if err != nil || mb <= 0 {
		mb = 20
	}
	return int64(mb) << 20
}

// Save 把上传内容写入 data/files。
func Save(cfg *config.Config, data []byte, filename string) (*SavedFile, error) {
	mime, ext, err := DetectType(data, filename)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxUploadBytes() {
		return nil, fmt.Errorf("文件超过 %dMB 限制", MaxUploadBytes()>>20)
	}
	name, err := randomName(ext)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(cfg.DataDir, "files", name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return &SavedFile{
		StorageName:  name,
		OriginalName: sanitizeName(filename),
		Mime:         mime,
		Ext:          ext,
		Size:         int64(len(data)),
		SHA256:       hex.EncodeToString(sum[:]),
		Path:         path,
	}, nil
}

// MoveInto 把临时文件移动到附件目录（复用已落盘的临时上传）。
func MoveInto(cfg *config.Config, tempPath, ext string) (*SavedFile, string, error) {
	name, err := randomName(ext)
	if err != nil {
		return nil, "", err
	}
	dst := filepath.Join(cfg.DataDir, "files", name)
	if err := os.Rename(tempPath, dst); err != nil {
		data, rerr := os.ReadFile(tempPath)
		if rerr != nil {
			return nil, "", rerr
		}
		if werr := os.WriteFile(dst, data, 0o644); werr != nil {
			return nil, "", werr
		}
		_ = os.Remove(tempPath)
	}
	sum := fileSHA(dst)
	return &SavedFile{StorageName: name, Path: dst, SHA256: sum, Ext: ext}, name, nil
}

func fileSHA(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Path 返回附件绝对路径。
func Path(cfg *config.Config, storageName string) string {
	return filepath.Join(cfg.DataDir, "files", filepath.Base(storageName))
}

// Delete 删除附件（忽略不存在）。
func Delete(cfg *config.Config, storageName string) {
	if storageName == "" {
		return
	}
	_ = os.Remove(Path(cfg, storageName))
	_ = os.Remove(filepath.Join(cfg.DataDir, "thumbs", thumbName(storageName)))
}

func thumbName(storageName string) string {
	return strings.TrimSuffix(storageName, filepath.Ext(storageName)) + "_thumb.jpg"
}

// Thumbnail 生成或读取缩略图缓存。
func Thumbnail(cfg *config.Config, f model.File) ([]byte, error) {
	cache := filepath.Join(cfg.DataDir, "thumbs", thumbName(f.StorageName))
	if data, err := os.ReadFile(cache); err == nil {
		return data, nil
	}
	data, err := os.ReadFile(Path(cfg, f.StorageName))
	if err != nil {
		return nil, err
	}
	var img image.Image
	if f.Mime == "application/pdf" {
		doc, err := ocr.OpenPDF(data)
		if err != nil {
			return nil, err
		}
		images := doc.ExtractImages()
		if len(images) == 0 {
			return nil, errors.New("PDF 里没有可预览的图片")
		}
		im := images[0]
		if im.Kind == "jpeg" {
			img, err = ocr.Decode(im.Data)
			if err != nil {
				return nil, err
			}
		} else {
			img = ocr.RawToImage(im.Data, im.CS, im.BPC, im.Width, im.Height)
			if img == nil {
				return nil, errors.New("PDF 图片无法解析")
			}
		}
	} else {
		img, err = ocr.Decode(data)
		if err != nil {
			return nil, err
		}
	}
	out, err := ocr.Thumbnail(img, 520)
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Join(cfg.DataDir, "thumbs"), 0o755)
	_ = os.WriteFile(cache, out, 0o644)
	return out, nil
}

// Usage 统计附件占用。
func Usage(cfg *config.Config) (files int, bytes int64) {
	entries, err := os.ReadDir(filepath.Join(cfg.DataDir, "files"))
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			files++
			bytes += info.Size()
		}
	}
	return files, bytes
}

// CleanupTemp 清理过期临时文件。
func CleanupTemp(cfg *config.Config, maxAge time.Duration) int {
	dir := filepath.Join(cfg.DataDir, "tmp")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			removed++
		}
	}
	return removed
}

func randomName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf) + ext, nil
}

func sanitizeName(name string) string {
	name = filepath.Base(name)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\r", "", "\n", "")
	out := strings.TrimSpace(replacer.Replace(name))
	if out == "" {
		out = "file"
	}
	if len([]rune(out)) > 120 {
		r := []rune(out)
		out = string(r[:120])
	}
	return out
}
