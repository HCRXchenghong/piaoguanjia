package ocr

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// /Contents 指向一个数组对象的 PDF（gp-template 生成的电子发票就是这样）。
func TestPageContentIndirectArray(t *testing.T) {
	raw := []byte("BT /F1 12 Tf (test) Tj ET")
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := &PDF{Objs: map[int]*pdfObj{
		28: {Value: []any{pdfRef{Num: 29}}},
		29: {Value: map[string]any{"Length": int64(len(raw))}, Stream: zbuf.Bytes(), HasStream: true},
	}}
	page := pdfPage{Dict: map[string]any{"Contents": pdfRef{Num: 28}}}
	if got := string(p.pageContent(page)); got != string(raw) {
		t.Fatalf("pageContent = %q，期望 %q", got, raw)
	}
}

func TestPageContentDirectStream(t *testing.T) {
	raw := []byte("q Q")
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write(raw)
	zw.Close()
	p := &PDF{Objs: map[int]*pdfObj{
		5: {Value: map[string]any{"Length": int64(len(raw))}, Stream: zbuf.Bytes(), HasStream: true},
	}}
	page := pdfPage{Dict: map[string]any{"Contents": pdfRef{Num: 5}}}
	if got := string(p.pageContent(page)); got != string(raw) {
		t.Fatalf("pageContent = %q，期望 %q", got, raw)
	}
}
