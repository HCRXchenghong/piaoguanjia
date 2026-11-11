package exporter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"invoice/server/internal/config"
	"invoice/server/internal/model"
)

func sampleRows(n int) []model.Invoice {
	rows := make([]model.Invoice, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, model.Invoice{
			ID:        int64(i + 1),
			Type:      "procurement",
			Title:     fmt.Sprintf("测试耗材%d", i+1),
			Spec:      "规格A",
			Unit:      "个",
			Qty:       ptr(2),
			UnitPrice: ptr(10.5),
			Amount:    21,
		})
	}
	return rows
}

func ptr(v float64) *float64 { return &v }

func mustOpen(t *testing.T, data []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("打开生成文件失败：%v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func totalRowOf(t *testing.T, f *excelize.File, sheet string) int {
	t.Helper()
	row, err := findTotalRow(f, sheet)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestBuildAcceptanceForm(t *testing.T) {
	data, err := buildAcceptanceForm(acceptanceTemplate, sampleRows(10))
	if err != nil {
		t.Fatal(err)
	}
	f := mustOpen(t, data)
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != acceptanceSheet {
		t.Fatalf("工作表 = %v，期望只有 %q", sheets, acceptanceSheet)
	}
	total := totalRowOf(t, f, acceptanceSheet)
	if total != 14 {
		t.Fatalf("合计行 = %d，期望 14", total)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "A1"); !strings.Contains(v, "验") {
		t.Errorf("验收单标题丢失：A1=%q", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "A2"); !strings.Contains(v, "供应单位") {
		t.Errorf("供应单位标签丢失：A2=%q", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "A4"); v != "测试耗材1" {
		t.Errorf("首行品名 = %q", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "C4"); v != "个" {
		t.Errorf("单位 = %q", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "E4"); v != "10.50" {
		t.Errorf("单价 = %q，期望 10.50", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "F4"); v != "21.00" {
		t.Errorf("金额 = %q，期望 21.00", v)
	}
	formula, _ := f.GetCellFormula(acceptanceSheet, cellName(6, total))
	if strings.TrimPrefix(formula, "=") != "SUM(F4:F13)" {
		t.Errorf("合计公式 = %q，期望 SUM(F4:F13)", formula)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "A15"); !strings.Contains(v, "部门主管") {
		t.Errorf("签字栏丢失：A15=%q", v)
	}
	if v, _ := f.GetCellValue(acceptanceSheet, "A16"); !strings.Contains(v, "备注") {
		t.Errorf("底部备注丢失：A16=%q", v)
	}
	if _, err := f.GetCellValue(acceptanceSheet, "A20"); err != nil {
		t.Errorf("第二张空白验收单未清理干净：%v", err)
	}
}

func TestBuildAcceptanceFormCommonSeller(t *testing.T) {
	rows := sampleRows(3)
	for i := range rows {
		rows[i].Seller = "某某科技有限公司"
	}
	data, err := buildAcceptanceForm(acceptanceTemplate, rows)
	if err != nil {
		t.Fatal(err)
	}
	f := mustOpen(t, data)
	v, _ := f.GetCellValue(acceptanceSheet, "A2")
	if !strings.Contains(v, "某某科技有限公司") {
		t.Errorf("供应单位未填充：A2=%q", v)
	}
}

func TestBuildPartsListForm(t *testing.T) {
	data, err := buildPartsListForm(partsListTemplate, sampleRows(10))
	if err != nil {
		t.Fatal(err)
	}
	f := mustOpen(t, data)
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != partsSheet {
		t.Fatalf("工作表 = %v，期望只有 %q", sheets, partsSheet)
	}
	total := totalRowOf(t, f, partsSheet)
	if total != 12 {
		t.Fatalf("合计行 = %d，期望 12", total)
	}
	if v, _ := f.GetCellValue(partsSheet, "A2"); v != "测试耗材1" {
		t.Errorf("首行品名 = %q", v)
	}
	if v, _ := f.GetCellValue(partsSheet, "F2"); v != "21.00" {
		t.Errorf("金额 = %q，期望 21.00", v)
	}
	formula, _ := f.GetCellFormula(partsSheet, cellName(6, total))
	if strings.TrimPrefix(formula, "=") != "SUM(F2:F11)" {
		t.Errorf("合计公式 = %q，期望 SUM(F2:F11)", formula)
	}
}

func TestBuildPartsListClearsSampleRows(t *testing.T) {
	data, err := buildPartsListForm(partsListTemplate, sampleRows(2))
	if err != nil {
		t.Fatal(err)
	}
	f := mustOpen(t, data)
	total := totalRowOf(t, f, partsSheet)
	if total != 8 {
		t.Fatalf("合计行 = %d，期望 8", total)
	}
	for r := 4; r <= 7; r++ {
		for c := 1; c <= 7; c++ {
			if v, _ := f.GetCellValue(partsSheet, cellName(c, r)); v != "" {
				t.Errorf("多余样例行 %s = %q，期望为空", cellName(c, r), v)
			}
		}
	}
	if v, _ := f.GetCellValue(partsSheet, "E8"); !strings.Contains(v, "合") {
		t.Errorf("合计行被破坏：E8=%q", v)
	}
}

func TestBuildZipIncludesTemplates(t *testing.T) {
	data, name, err := BuildZip(&config.Config{}, sampleRows(10), map[int64][]model.File{}, Meta{RangeLabel: "2026年11月"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]bool{}
	for _, f := range zr.File {
		entries[f.Name] = true
	}
	for _, want := range []string{
		"01_采购报销明细_2026年11月.xlsx",
		"02_采购报销清单_2026年11月.xlsx",
		"03_采购报销验收单_2026年11月.xlsx",
		"导出说明.txt",
	} {
		if !entries[want] {
			t.Errorf("压缩包缺少 %q，实际：%v（%s）", want, keys(entries), name)
		}
	}
	for _, no := range []string{"04_", "05_"} {
		for entry := range entries {
			if strings.HasPrefix(entry, no) {
				t.Errorf("没有出行票据却生成了 %s", entry)
			}
		}
	}
}

func TestBuildZipTravelFiles(t *testing.T) {
	row := model.Invoice{ID: 1, Type: "travel", Title: "出差", TravelTime: "2026-11-01", Amount: 100}
	zipData, _, err := BuildZip(&config.Config{}, []model.Invoice{row}, map[int64][]model.File{}, Meta{})
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	entries := map[string]bool{}
	for _, f := range zr.File {
		entries[f.Name] = true
	}
	for _, want := range []string{"01_出行报销明细.xlsx", "02_出行报销验收单.xlsx"} {
		if !entries[want] {
			t.Errorf("纯出行导出缺少 %q，实际：%v", want, keys(entries))
		}
	}
	for entry := range entries {
		if strings.HasPrefix(entry, "03_") || strings.HasPrefix(entry, "04_") || strings.HasPrefix(entry, "05_") {
			t.Errorf("纯出行导出不应生成 %s", entry)
		}
	}
}

func TestBuildZipMixedTypes(t *testing.T) {
	rows := []model.Invoice{
		{ID: 1, Type: "procurement", Title: "耗材", Amount: 10, Qty: ptr(1), UnitPrice: ptr(10)},
		{ID: 2, Type: "travel", Title: "出差", TravelTime: "2026-11-01", Amount: 100},
	}
	zipData, _, err := BuildZip(&config.Config{}, rows, map[int64][]model.File{}, Meta{})
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	entries := map[string]bool{}
	for _, f := range zr.File {
		entries[f.Name] = true
	}
	for _, want := range []string{
		"01_采购报销明细.xlsx", "02_采购报销清单.xlsx", "03_采购报销验收单.xlsx",
		"04_出行报销明细.xlsx", "05_出行报销验收单.xlsx",
	} {
		if !entries[want] {
			t.Errorf("混合导出缺少 %q，实际：%v", want, keys(entries))
		}
	}
}

func TestBuildTravelAcceptanceForm(t *testing.T) {
	rows := []model.Invoice{
		{ID: 1, Type: "travel", Title: "赴上海参会", TravelTime: "2026-11-01", Transport: "高铁",
			Route: "南京-上海", Traveler: "刘勃", Amount: 120.5, Note: "G1234"},
		{ID: 2, Type: "travel", Title: "客户拜访", TravelTime: "2026-11-02", Transport: "网约车",
			Route: "市区-机场", Traveler: "刘勃", Amount: 89.5},
		{ID: 3, Type: "travel", Title: "返程", TravelTime: "2026-11-03", Transport: "高铁",
			Route: "上海-南京", Traveler: "刘勃", Amount: 120},
	}
	data, err := buildTravelAcceptanceForm(rows)
	if err != nil {
		t.Fatal(err)
	}
	f := mustOpen(t, data)
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "出行报销验收单" {
		t.Fatalf("工作表 = %v", sheets)
	}
	if v, _ := f.GetCellValue("出行报销验收单", "A1"); v != "出行报销验收单" {
		t.Errorf("标题 = %q", v)
	}
	if v, _ := f.GetCellValue("出行报销验收单", "A3"); v != "出行事由" {
		t.Errorf("表头 A3 = %q", v)
	}
	if v, _ := f.GetCellValue("出行报销验收单", "A4"); v != "赴上海参会" {
		t.Errorf("首行事由 = %q", v)
	}
	if v, _ := f.GetCellValue("出行报销验收单", "F4"); v != "120.50" {
		t.Errorf("首行金额 = %q", v)
	}
	total := 7 // 4 + 3 rows
	if v, _ := f.GetCellValue("出行报销验收单", cellName(6, total)); v != "330.00" {
		t.Errorf("合计 = %q，期望 330.00", v)
	}
	if v, _ := f.GetCellValue("出行报销验收单", cellName(1, total+1)); !strings.Contains(v, "部门主管") {
		t.Errorf("签字栏丢失：%q", v)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
