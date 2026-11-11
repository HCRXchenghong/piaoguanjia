package exporter

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"invoice/server/internal/config"
	"invoice/server/internal/model"
	"invoice/server/internal/storage"
)

//go:embed templates/acceptance.xlsx
var acceptanceTemplate []byte

//go:embed templates/parts_list.xlsx
var partsListTemplate []byte

var typeLabel = map[string]string{"procurement": "采购报销", "travel": "出行报销"}
var statusLabel = map[string]string{"pending": "未报销", "reimbursed": "已报销"}

// Meta 导出上下文：区间、状态等，会写进文件名与表头。
type Meta struct {
	RangeLabel  string // 2026年 / 2026年10月 / 全部
	StatusLabel string // 全部 / 未报销 / 已报销
	TypeFilter  string // 全部 / 采购报销 / 出行报销
}

// colDef 列定义，Key 用于取值与定位合计列。
type colDef struct {
	Key   string
	Title string
	Width float64
}

var procurementCols = []colDef{
	{"seq", "序号", 6}, {"attachment", "附件文件", 38}, {"title", "名称", 22},
	{"category", "类别", 12}, {"spec", "规格", 18}, {"qty", "数量", 8},
	{"unit_price", "税后单价", 12}, {"shipping", "运费", 10}, {"amount", "税后金额", 12},
	{"status", "报销状态", 10}, {"invoice_no", "发票号码", 24}, {"invoice_date", "开票日期", 12},
	{"seller", "销售方", 26}, {"user", "上报人", 10}, {"created", "上报时间", 22},
	{"note", "备注", 30}, {"link", "链接", 28},
}

var travelCols = []colDef{
	{"seq", "序号", 6}, {"attachment", "附件文件", 38}, {"title", "出行事由", 24},
	{"travel_time", "出行时间", 12}, {"transport", "交通工具", 10}, {"route", "出行路线", 26},
	{"traveler", "出行人", 10}, {"amount", "税后金额", 12}, {"status", "报销状态", 10},
	{"invoice_no", "发票号码", 24}, {"invoice_date", "开票日期", 12}, {"seller", "销售方", 26},
	{"user", "上报人", 10}, {"created", "上报时间", 18}, {"note", "备注", 30},
}

// BuildZip 生成「附件重命名 + 分类 Excel」压缩包。
func BuildZip(cfg *config.Config, rows []model.Invoice, files map[int64][]model.File,
	meta Meta) ([]byte, string, error) {
	stamp := time.Now().Format("20060102_1504")
	base := "发票汇总"
	if meta.RangeLabel != "" && meta.RangeLabel != "全部" {
		base += "_" + meta.RangeLabel
	}
	if meta.StatusLabel != "" && meta.StatusLabel != "全部" {
		base += "_" + meta.StatusLabel
	}
	base = sanitize(base)
	zipName := fmt.Sprintf("%s_%s.zip", base, stamp)

	// 先统一分配附件名（采购在前、出行在后），保证两个 Excel 里引用的文件名一致
	ordered := make([]model.Invoice, 0, len(rows))
	for _, r := range rows {
		if r.Type == "procurement" {
			ordered = append(ordered, r)
		}
	}
	for _, r := range rows {
		if r.Type != "procurement" {
			ordered = append(ordered, r)
		}
	}
	for i := range ordered {
		seq := i + 1
		var names []string
		for j, f := range files[ordered[i].ID] {
			name := attachmentName(seq, ordered[i], f)
			if j > 0 {
				ext := filepath.Ext(name)
				name = strings.TrimSuffix(name, ext) + fmt.Sprintf("_%d%s", j+1, ext)
			}
			names = append(names, name)
		}
		ordered[i].Attachments = names
	}

	byID := map[int64]model.Invoice{}
	for _, r := range ordered {
		byID[r.ID] = r
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// 附件
	for _, inv := range ordered {
		for idx, f := range files[inv.ID] {
			if idx >= len(inv.Attachments) {
				break
			}
			src := storage.Path(cfg, f.StorageName)
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			w, err := zw.Create(inv.Attachments[idx])
			if err != nil {
				continue
			}
			_, _ = w.Write(data)
		}
	}

	// 分类型的 Excel：哪一类没有数据就不生成
	var procRows, travelRows []model.Invoice
	for _, r := range ordered {
		if r.Type == "procurement" {
			procRows = append(procRows, r)
		} else {
			travelRows = append(travelRows, r)
		}
	}
	var parts []string
	if l := strings.TrimSpace(meta.RangeLabel); l != "" && l != "全部" {
		parts = append(parts, l)
	}
	if l := strings.TrimSpace(meta.StatusLabel); l != "" && l != "全部" {
		parts = append(parts, l)
	}
	suffix := sanitize(strings.Join(parts, "_"))

	// 编号按本次实际生成的文件顺序排，不出现的类型不占号。
	seq := 0
	next := func(label string) string {
		seq++
		return fmt.Sprintf("%02d_%s", seq, label)
	}
	writeEntry := func(name string, data []byte) {
		if suffix != "" {
			name += "_" + suffix
		}
		if w, err := zw.Create(name + ".xlsx"); err == nil {
			_, _ = w.Write(data)
		}
	}

	if len(procRows) > 0 {
		data, err := buildWorkbook(procRows, "procurement", meta)
		if err != nil {
			return nil, "", err
		}
		writeEntry(next("采购报销明细"), data)

		if data, err := buildPartsListForm(partsListTemplate, procRows); err == nil {
			writeEntry(next("采购报销清单"), data)
		} else {
			return nil, "", err
		}
		if data, err := buildAcceptanceForm(acceptanceTemplate, procRows); err == nil {
			writeEntry(next("采购报销验收单"), data)
		} else {
			return nil, "", err
		}
	}

	if len(travelRows) > 0 {
		data, err := buildWorkbook(travelRows, "travel", meta)
		if err != nil {
			return nil, "", err
		}
		writeEntry(next("出行报销明细"), data)

		data, err = buildTravelAcceptanceForm(travelRows)
		if err != nil {
			return nil, "", err
		}
		writeEntry(next("出行报销验收单"), data)
	}

	if w, err := zw.Create("导出说明.txt"); err == nil {
		_, _ = w.Write([]byte(readme(procRows, travelRows, meta)))
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), zipName, nil
}

func attachmentName(seq int, inv model.Invoice, f model.File) string {
	cat := inv.Category
	if cat == "" {
		cat = typeLabel[inv.Type]
	}
	cat = sanitize(cat)
	ext := filepath.Ext(f.StorageName)
	if ext == "" {
		ext = ".pdf"
	}
	return fmt.Sprintf("附件%03d_%s_%.2f元%s", seq, cat, inv.Amount, ext)
}

func readme(proc, travel []model.Invoice, meta Meta) string {
	var sb strings.Builder
	sb.WriteString("发票汇总导出包\n")
	sb.WriteString("========================================\n")
	sb.WriteString(fmt.Sprintf("导出时间：%s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("统计区间：%s\n", orDefault(meta.RangeLabel, "全部")))
	sb.WriteString(fmt.Sprintf("报销状态：%s\n", orDefault(meta.StatusLabel, "全部")))
	sb.WriteString(fmt.Sprintf("票据数量：%d 张（采购 %d 张，出行 %d 张）\n", len(proc)+len(travel), len(proc), len(travel)))
	total := 0.0
	for _, r := range append(append([]model.Invoice{}, proc...), travel...) {
		total += r.Amount
	}
	sb.WriteString(fmt.Sprintf("合计金额：¥%.2f\n\n", total))
	sb.WriteString("文件说明：\n")
	no := 0
	line := func(text string) {
		no++
		sb.WriteString(fmt.Sprintf("  %d. %s\n", no, text))
	}
	line("原始发票文件已按「附件001_类别_金额元.扩展名」重命名，与 Excel 中「附件文件」列一一对应；")
	seq := 0
	named := func(label string) string {
		seq++
		return fmt.Sprintf("%02d_%s.xlsx", seq, label)
	}
	if len(proc) > 0 {
		line(named("采购报销明细") + "：采购类票据，含「明细」「汇总」「类别统计」三个工作表；")
		line(named("采购报销清单") + "：按模板填入本次采购票据；")
		line(named("采购报销验收单") + "：按模板填入本次采购票据，保留原表格式与签字栏；")
	}
	if len(travel) > 0 {
		line(named("出行报销明细") + "：出行类票据，含「明细」「汇总」「交通工具统计」三个工作表；")
		line(named("出行报销验收单") + "：出行报销专用验收单；")
	}
	line("编号按本次实际生成的文件顺序排列，采购与出行分开成表，没有数据的类型不占号；")
	line("报销状态以导出时刻系统记录为准。")
	return sb.String()
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\r", "", "\n", "")
	return strings.TrimSpace(r.Replace(s))
}

// buildWorkbook 生成单个类型的 Excel（明细 + 汇总 + 统计）。
func buildWorkbook(rows []model.Invoice, kind string, meta Meta) ([]byte, error) {
	cols := procurementCols
	if kind == "travel" {
		cols = travelCols
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "明细"
	f.SetSheetName("Sheet1", sheet)
	hideGrid(f, sheet)

	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 16, Bold: true, Color: "1F2A44"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	subStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 9, Color: "6B7280"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	headStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10, Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F2A44"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	bodyStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Family: "微软雅黑", Size: 10},
		Alignment: &excelize.Alignment{
			Horizontal: "center", Vertical: "center", WrapText: true,
		},
		Border: thinBorder(),
	})
	qtyStyle, _ := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Family: "微软雅黑", Size: 10},
		NumFmt: 1,
		Alignment: &excelize.Alignment{
			Horizontal: "center", Vertical: "center",
		},
		Border: thinBorder(),
	})
	moneyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10},
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	centerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	totalStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF2FA"}, Pattern: 1},
		Border:    thinBorder(),
	})
	totalNumStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10, Bold: true, Color: "1F2A44"},
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF2FA"}, Pattern: 1},
		Border:    thinBorder(),
	})

	lastCol := colName(len(cols))
	_ = f.MergeCell(sheet, "A1", lastCol+"1")
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("发票汇总 · %s明细", typeLabel[kind]))
	_ = f.SetCellStyle(sheet, "A1", lastCol+"1", titleStyle)
	_ = f.SetRowHeight(sheet, 1, 30)

	total := 0.0
	for _, r := range rows {
		total += r.Amount
	}
	_ = f.MergeCell(sheet, "A2", lastCol+"2")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf(
		"统计区间：%s    报销状态：%s    票据 %d 张    合计 ¥%.2f    导出时间 %s",
		orDefault(meta.RangeLabel, "全部"), orDefault(meta.StatusLabel, "全部"),
		len(rows), total, time.Now().Format("2006-01-02 15:04")))
	_ = f.SetCellStyle(sheet, "A2", lastCol+"2", subStyle)
	_ = f.SetRowHeight(sheet, 2, 20)

	headRow := 4
	for i, c := range cols {
		cell, _ := excelize.CoordinatesToCellName(i+1, headRow)
		_ = f.SetCellValue(sheet, cell, c.Title)
		_ = f.SetColWidth(sheet, colName(i+1), colName(i+1), c.Width)
	}
	_ = f.SetCellStyle(sheet, "A4", lastCol+"4", headStyle)
	_ = f.SetRowHeight(sheet, headRow, 26)

	moneyKeys := map[string]bool{"unit_price": true, "shipping": true, "amount": true}
	for i, inv := range rows {
		row := headRow + 1 + i
		for j, c := range cols {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			_ = f.SetCellValue(sheet, cell, cellValue(inv, c.Key, i+1))
			switch {
			case moneyKeys[c.Key]:
				_ = f.SetCellStyle(sheet, cell, cell, moneyStyle)
			case c.Key == "qty":
				_ = f.SetCellStyle(sheet, cell, cell, qtyStyle)
			case c.Key == "seq" || c.Key == "status" || c.Key == "created" || c.Key == "invoice_date":
				_ = f.SetCellStyle(sheet, cell, cell, centerStyle)
			default:
				_ = f.SetCellStyle(sheet, cell, cell, bodyStyle)
			}
		}
		_ = f.SetRowHeight(sheet, row, detailRowHeight(inv, cols))
	}

	// 合计行：直接写数值，避免公式在没有缓存值时显示空白
	totalRow := headRow + len(rows) + 1
	amountCol := colIndex(cols, "amount")
	_ = f.MergeCell(sheet, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("%s%d", colName(amountCol-1), totalRow))
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", totalRow), "合计")
	_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("%s%d", colName(amountCol-1), totalRow), totalStyle)
	_ = f.SetRowHeight(sheet, totalRow, 28)
	_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName(amountCol), totalRow), round2(total))
	_ = f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName(amountCol), totalRow),
		fmt.Sprintf("%s%d", colName(amountCol), totalRow), totalNumStyle)
	_ = f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName(amountCol+1), totalRow),
		fmt.Sprintf("%s%d", lastCol, totalRow), totalStyle)

	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: 0, YSplit: headRow,
		TopLeftCell: fmt.Sprintf("A%d", headRow+1), ActivePane: "bottomLeft"})
	_ = f.AutoFilter(sheet, fmt.Sprintf("A%d:%s%d", headRow, lastCol, headRow+maxInt(len(rows), 1)), nil)

	buildSummarySheet(f, rows, kind, meta)
	if kind == "travel" {
		buildBreakdownSheet(f, rows, "交通工具", func(r model.Invoice) string {
			if r.Transport == "" {
				return "未填写"
			}
			return r.Transport
		}, "交通工具统计")
	} else {
		buildBreakdownSheet(f, rows, "类别", func(r model.Invoice) string {
			if r.Category == "" {
				return "未分类"
			}
			return r.Category
		}, "类别统计")
	}
	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hideGrid 关闭网格线，让表格只显示自己的边框。
func hideGrid(f *excelize.File, sheet string) {
	show := false
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{ShowGridLines: &show})
}

func thinBorder() []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: "D9DEE9", Style: 1},
		{Type: "right", Color: "D9DEE9", Style: 1},
		{Type: "top", Color: "D9DEE9", Style: 1},
		{Type: "bottom", Color: "D9DEE9", Style: 1},
	}
}

func cellValue(inv model.Invoice, key string, seq int) any {
	switch key {
	case "seq":
		return seq
	case "attachment":
		return strings.Join(inv.Attachments, "；")
	case "title":
		return inv.Title
	case "category":
		return inv.Category
	case "spec":
		return inv.Spec
	case "qty":
		return nilOrFloat(inv.Qty)
	case "unit_price":
		return nilOrFloat(inv.UnitPrice)
	case "shipping":
		return nilOrFloat(inv.Shipping)
	case "amount":
		return inv.Amount
	case "status":
		return statusLabel[inv.Status]
	case "travel_time":
		return inv.TravelTime
	case "transport":
		return inv.Transport
	case "route":
		return inv.Route
	case "traveler":
		return inv.Traveler
	case "invoice_no":
		return inv.InvoiceNo
	case "invoice_date":
		return inv.InvoiceDate
	case "seller":
		return inv.Seller
	case "user":
		return inv.DisplayName
	case "created":
		return inv.CreatedAtText
	case "note":
		return inv.Note
	case "link":
		return inv.Link
	}
	return ""
}

func nilOrFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func colIndex(cols []colDef, key string) int {
	for i, c := range cols {
		if c.Key == key {
			return i + 1
		}
	}
	return len(cols)
}

func buildSummarySheet(f *excelize.File, rows []model.Invoice, kind string, meta Meta) {
	sheet := "汇总"
	_, _ = f.NewSheet(sheet)
	hideGrid(f, sheet)
	_ = f.SetColWidth(sheet, "A", "A", 22)
	_ = f.SetColWidth(sheet, "B", "E", 16)
	st := makeReportStyles(f)

	_ = f.MergeCell(sheet, "A1", "E1")
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("%s汇总", typeLabel[kind]))
	_ = f.SetCellStyle(sheet, "A1", "E1", st.title)
	_ = f.SetRowHeight(sheet, 1, 32)

	_ = f.MergeCell(sheet, "A2", "E2")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf(
		"统计区间：%s    报销状态：%s    票据 %d 张    导出时间 %s",
		orDefault(meta.RangeLabel, "全部"), orDefault(meta.StatusLabel, "全部"),
		len(rows), time.Now().Format("2006-01-02 15:04")))
	_ = f.SetCellStyle(sheet, "A2", "E2", st.sub)
	_ = f.SetRowHeight(sheet, 2, 20)

	for i, h := range []string{"上报人", "票据数", "未报销金额", "已报销金额", "合计金额"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet, cell, h)
	}
	_ = f.SetCellStyle(sheet, "A3", "E3", st.head)
	_ = f.SetRowHeight(sheet, 3, 26)

	type agg struct {
		count                 int
		amount, pending, done float64
	}
	byUser := map[string]*agg{}
	var order []string
	for _, r := range rows {
		name := r.DisplayName
		if name == "" {
			name = "未知"
		}
		a, ok := byUser[name]
		if !ok {
			a = &agg{}
			byUser[name] = a
			order = append(order, name)
		}
		a.count++
		a.amount += r.Amount
		if r.Status == "reimbursed" {
			a.done += r.Amount
		} else {
			a.pending += r.Amount
		}
	}
	sort.Slice(order, func(i, j int) bool { return byUser[order[i]].amount > byUser[order[j]].amount })

	row := 4
	for _, name := range order {
		a := byUser[name]
		values := []any{name, a.count, round2(a.pending), round2(a.done), round2(a.amount)}
		for c, v := range values {
			cell := cellName(c+1, row)
			_ = f.SetCellValue(sheet, cell, v)
			switch c {
			case 1:
				_ = f.SetCellStyle(sheet, cell, cell, st.number)
			case 2, 3, 4:
				_ = f.SetCellStyle(sheet, cell, cell, st.money)
			default:
				_ = f.SetCellStyle(sheet, cell, cell, st.body)
			}
		}
		_ = f.SetRowHeight(sheet, row, 26)
		row++
	}

	total, pending := 0.0, 0.0
	for _, r := range rows {
		total += r.Amount
		if r.Status != "reimbursed" {
			pending += r.Amount
		}
	}
	_ = f.SetCellValue(sheet, cellName(1, row), "总计")
	_ = f.SetCellStyle(sheet, cellName(1, row), cellName(1, row), st.total)
	_ = f.SetCellValue(sheet, cellName(2, row), len(rows))
	_ = f.SetCellStyle(sheet, cellName(2, row), cellName(2, row), st.total)
	for c, v := range []float64{round2(pending), round2(total - pending), round2(total)} {
		cell := cellName(3+c, row)
		_ = f.SetCellValue(sheet, cell, v)
		_ = f.SetCellStyle(sheet, cell, cell, st.totalMoney)
	}
	_ = f.SetRowHeight(sheet, row, 26)

	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: 0, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

func buildBreakdownSheet(f *excelize.File, rows []model.Invoice, dim string,
	keyFn func(model.Invoice) string, sheetName string) {
	sheet := sheetName
	_, _ = f.NewSheet(sheet)
	hideGrid(f, sheet)
	_ = f.SetColWidth(sheet, "A", "A", 22)
	_ = f.SetColWidth(sheet, "B", "B", 14)
	_ = f.SetColWidth(sheet, "C", "C", 18)
	_ = f.SetColWidth(sheet, "D", "D", 12)
	st := makeReportStyles(f)

	_ = f.MergeCell(sheet, "A1", "D1")
	_ = f.SetCellValue(sheet, "A1", fmt.Sprintf("按%s统计", dim))
	_ = f.SetCellStyle(sheet, "A1", "D1", st.title)
	_ = f.SetRowHeight(sheet, 1, 32)

	_ = f.MergeCell(sheet, "A2", "D2")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("共 %d 张票据，按金额从高到低排列", len(rows)))
	_ = f.SetCellStyle(sheet, "A2", "D2", st.sub)
	_ = f.SetRowHeight(sheet, 2, 20)

	for i, h := range []string{dim, "票据数", "金额", "占比"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet, cell, h)
	}
	_ = f.SetCellStyle(sheet, "A3", "D3", st.head)
	_ = f.SetRowHeight(sheet, 3, 26)

	type agg struct {
		count  int
		amount float64
	}
	byKey := map[string]*agg{}
	var order []string
	total := 0.0
	for _, r := range rows {
		k := keyFn(r)
		a, ok := byKey[k]
		if !ok {
			a = &agg{}
			byKey[k] = a
			order = append(order, k)
		}
		a.count++
		a.amount += r.Amount
		total += r.Amount
	}
	sort.Slice(order, func(i, j int) bool { return byKey[order[i]].amount > byKey[order[j]].amount })

	row := 4
	for _, k := range order {
		a := byKey[k]
		_ = f.SetCellValue(sheet, cellName(1, row), k)
		_ = f.SetCellStyle(sheet, cellName(1, row), cellName(1, row), st.body)
		_ = f.SetCellValue(sheet, cellName(2, row), a.count)
		_ = f.SetCellStyle(sheet, cellName(2, row), cellName(2, row), st.number)
		_ = f.SetCellValue(sheet, cellName(3, row), round2(a.amount))
		_ = f.SetCellStyle(sheet, cellName(3, row), cellName(3, row), st.money)
		pct := 0.0
		if total > 0 {
			pct = a.amount / total
		}
		_ = f.SetCellValue(sheet, cellName(4, row), pct)
		_ = f.SetCellStyle(sheet, cellName(4, row), cellName(4, row), st.percent)
		_ = f.SetRowHeight(sheet, row, 26)
		row++
	}

	_ = f.SetCellValue(sheet, cellName(1, row), "合计")
	_ = f.SetCellStyle(sheet, cellName(1, row), cellName(1, row), st.total)
	_ = f.SetCellValue(sheet, cellName(2, row), len(rows))
	_ = f.SetCellStyle(sheet, cellName(2, row), cellName(2, row), st.total)
	_ = f.SetCellValue(sheet, cellName(3, row), round2(total))
	_ = f.SetCellStyle(sheet, cellName(3, row), cellName(3, row), st.totalMoney)
	_ = f.SetCellValue(sheet, cellName(4, row), 1.0)
	_ = f.SetCellStyle(sheet, cellName(4, row), cellName(4, row), st.totalPercent)
	_ = f.SetRowHeight(sheet, row, 26)

	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: 0, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

type reportStyles struct {
	title, sub, head, body, money, number, percent, total, totalMoney, totalPercent, label, value int
}

func makeReportStyles(f *excelize.File) reportStyles {
	font := func(size float64, bold bool, color string) *excelize.Font {
		return &excelize.Font{Family: "微软雅黑", Size: size, Bold: bold, Color: color}
	}
	title, _ := f.NewStyle(&excelize.Style{
		Font:      font(15, true, "1F2A44"),
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	sub, _ := f.NewStyle(&excelize.Style{
		Font:      font(9, false, "6B7280"),
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	head, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, true, "FFFFFF"),
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F2A44"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	body, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, false, ""),
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder(),
	})
	money, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, false, ""),
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	number, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, false, ""),
		NumFmt:    1,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	pctFmt := "0.0%"
	percent, _ := f.NewStyle(&excelize.Style{
		Font:         font(10, false, ""),
		CustomNumFmt: &pctFmt,
		Alignment:    &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:       thinBorder(),
	})
	total, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, true, ""),
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF2FA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	totalMoney, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, true, "1F2A44"),
		NumFmt:    4,
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF2FA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	totalPercent, _ := f.NewStyle(&excelize.Style{
		Font:         font(10, true, "1F2A44"),
		CustomNumFmt: &pctFmt,
		Fill:         excelize.Fill{Type: "pattern", Color: []string{"EEF2FA"}, Pattern: 1},
		Alignment:    &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:       thinBorder(),
	})
	label, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, true, "6B7280"),
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	value, _ := f.NewStyle(&excelize.Style{
		Font:      font(10, false, "1F2A44"),
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	return reportStyles{title: title, sub: sub, head: head, body: body, money: money, number: number,
		percent: percent, total: total, totalMoney: totalMoney, totalPercent: totalPercent, label: label, value: value}
}

func detailRowHeight(inv model.Invoice, cols []colDef) float64 {
	lines := 1
	for _, c := range cols {
		switch c.Key {
		case "attachment", "title", "spec", "seller", "note", "link", "invoice_no", "user":
			if n := textLines(fmt.Sprint(cellValue(inv, c.Key, 1)), c.Width, 10); n > lines {
				lines = n
			}
		}
	}
	if lines <= 1 {
		return 26
	}
	h := float64(lines)*14 + 10
	if h > 120 {
		h = 120
	}
	return h
}

func textLines(s string, width, fontSize float64) int {
	if s == "" {
		return 1
	}
	capacity := width * 0.9 * (11 / fontSize)
	if capacity < 4 {
		capacity = 4
	}
	lines := 0
	for _, part := range strings.Split(s, "\n") {
		units := 0.0
		for _, r := range part {
			if r > 127 {
				units += 2
			} else {
				units += 1
			}
		}
		n := int(units/capacity) + 1
		if n < 1 {
			n = 1
		}
		lines += n
	}
	if lines < 1 {
		return 1
	}
	return lines
}

func colName(i int) string {
	name, _ := excelize.ColumnNumberToName(i)
	return name
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }
