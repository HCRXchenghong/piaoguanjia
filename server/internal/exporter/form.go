package exporter

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"invoice/server/internal/model"
)

const (
	acceptanceSheet = "采购报销验收单"
	partsSheet      = "采购报销清单"
)

// buildAcceptanceForm 用模板 Sheet2 的正式验收单页填数据，只保留这一张表。
func buildAcceptanceForm(tpl []byte, rows []model.Invoice) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(tpl))
	if err != nil {
		return nil, fmt.Errorf("打开验收单模板失败：%w", err)
	}
	defer f.Close()

	const source = "Sheet2"
	totalRow, err := findTotalRow(f, source)
	if err != nil {
		return nil, err
	}
	// 模板里印了两张空白验收单，导出只保留第一张，删掉下面的重复块。
	removeRowsFrom(f, source, 15, 19)
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("清理模板工作表失败：%w", err)
	}
	if err := f.SetSheetName(source, acceptanceSheet); err != nil {
		return nil, fmt.Errorf("重命名工作表失败：%w", err)
	}
	hideGrid(f, acceptanceSheet)

	// 原模板 B 列只有 11 字符宽，规格会被全部截断；按内容重新分配列宽。
	for col, w := range map[string]float64{"A": 22, "B": 26, "C": 8, "D": 7, "E": 12, "F": 14, "G": 8} {
		_ = f.SetColWidth(acceptanceSheet, col, col, w)
	}

	const dataStart = 4
	totalRow, err = prepareTable(f, acceptanceSheet, dataStart, rows, dataStart)
	if err != nil {
		return nil, err
	}
	if err := writeRows(f, acceptanceSheet, dataStart, totalRow, rows); err != nil {
		return nil, err
	}

	// 验收单的品名/规格列窄，行高统一放宽到两行。
	for r := dataStart; r < totalRow; r++ {
		_ = f.SetRowHeight(acceptanceSheet, r, 33)
	}

	// 明细多时默认 100% 会打成两页，按模板备注的缩放比例压到一页。
	a4, scale := 9, uint(73)
	if err := f.SetPageLayout(acceptanceSheet, &excelize.PageLayoutOptions{Size: &a4, AdjustTo: &scale}); err != nil {
		return nil, fmt.Errorf("设置打印比例失败：%w", err)
	}

	// 同一家供应商才写进抬头，多家留空由人工填写。
	if seller, ok := commonSeller(rows); ok {
		_ = f.SetCellValue(acceptanceSheet, "A2", "供应单位："+seller)
	}

	return saveForm(f)
}

// buildPartsListForm 用清单模板 Sheet1 填数据，只保留这一张表。
func buildPartsListForm(tpl []byte, rows []model.Invoice) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(tpl))
	if err != nil {
		return nil, fmt.Errorf("打开零件耗材清单模板失败：%w", err)
	}
	defer f.Close()

	const sheet = "Sheet1"
	const dataStart = 2
	totalRow, err := prepareTable(f, sheet, dataStart, rows, dataStart+1)
	if err != nil {
		return nil, err
	}
	if err := writeRows(f, sheet, dataStart, totalRow, rows); err != nil {
		return nil, err
	}
	for r := dataStart; r < totalRow; r++ {
		_ = f.SetRowHeight(sheet, r, 30)
	}

	if err := f.DeleteSheet("Sheet2"); err != nil {
		return nil, fmt.Errorf("清理模板工作表失败：%w", err)
	}
	if err := f.SetSheetName(sheet, partsSheet); err != nil {
		return nil, fmt.Errorf("重命名工作表失败：%w", err)
	}
	hideGrid(f, partsSheet)
	return saveForm(f)
}

// prepareTable 定位合计行，按需插行并清空明细区，返回新的合计行号。
func prepareTable(f *excelize.File, sheet string, dataStart int, rows []model.Invoice, donor int) (int, error) {
	totalRow, err := findTotalRow(f, sheet)
	if err != nil {
		return 0, err
	}
	slots := totalRow - dataStart
	if slots < 1 {
		return 0, fmt.Errorf("模板明细区异常")
	}
	if need := len(rows) - slots; need > 0 {
		if err := insertRows(f, sheet, totalRow, need, donor); err != nil {
			return 0, fmt.Errorf("扩展模板行失败：%w", err)
		}
		totalRow += need
	}
	for r := dataStart; r < totalRow; r++ {
		for c := 1; c <= 7; c++ {
			_ = f.SetCellValue(sheet, cellName(c, r), "")
		}
	}
	return totalRow, nil
}

// writeRows 写入明细、统一单元格样式与合计公式。
func writeRows(f *excelize.File, sheet string, dataStart, totalRow int, rows []model.Invoice) error {
	for i, inv := range rows {
		r := dataStart + i
		_ = f.SetCellValue(sheet, cellName(1, r), inv.Title)
		_ = f.SetCellValue(sheet, cellName(2, r), inv.Spec)
		_ = f.SetCellValue(sheet, cellName(3, r), inv.Unit)
		if inv.Qty != nil {
			_ = f.SetCellValue(sheet, cellName(4, r), *inv.Qty)
		}
		if inv.UnitPrice != nil {
			_ = f.SetCellValue(sheet, cellName(5, r), *inv.UnitPrice)
		}
		_ = f.SetCellValue(sheet, cellName(6, r), inv.Amount)
	}

	st, err := newFormStyles(f)
	if err != nil {
		return fmt.Errorf("表格格式失败：%w", err)
	}
	for r := dataStart; r < totalRow; r++ {
		_ = f.SetCellStyle(sheet, cellName(1, r), cellName(2, r), st.text)
		_ = f.SetCellStyle(sheet, cellName(3, r), cellName(3, r), st.cell)
		_ = f.SetCellStyle(sheet, cellName(4, r), cellName(4, r), st.qty)
		_ = f.SetCellStyle(sheet, cellName(5, r), cellName(6, r), st.money)
		_ = f.SetCellStyle(sheet, cellName(7, r), cellName(7, r), st.cell)
	}

	labelCol := 0
	for c := 1; c <= 7; c++ {
		v, _ := f.GetCellValue(sheet, cellName(c, totalRow))
		if strings.Contains(strings.ReplaceAll(v, " ", ""), "合计") {
			labelCol = c
			break
		}
	}
	for c := 1; c <= 7; c++ {
		if c == labelCol || c == 6 {
			continue
		}
		_ = f.SetCellStyle(sheet, cellName(c, totalRow), cellName(c, totalRow), st.cell)
	}
	if labelCol > 0 {
		_ = f.SetCellStyle(sheet, cellName(labelCol, totalRow), cellName(labelCol, totalRow), st.label)
	}
	_ = f.SetCellStyle(sheet, cellName(6, totalRow), cellName(6, totalRow), st.totalMoney)
	_ = f.SetRowHeight(sheet, totalRow, 36)

	if err := f.SetCellFormula(sheet, cellName(6, totalRow),
		fmt.Sprintf("SUM(F%d:F%d)", dataStart, totalRow-1)); err != nil {
		return fmt.Errorf("写入合计失败：%w", err)
	}
	return nil
}

// buildTravelAcceptanceForm 生成出行报销验收单（出行专用表头）。
func buildTravelAcceptanceForm(rows []model.Invoice) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "出行报销验收单"
	_ = f.SetSheetName("Sheet1", sheet)
	hideGrid(f, sheet)

	for col, w := range map[string]float64{"A": 26, "B": 14, "C": 12, "D": 28, "E": 10, "F": 14, "G": 16} {
		_ = f.SetColWidth(sheet, col, col, w)
	}

	title, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 22, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	cell, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    blackBorder(),
	})
	head, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11, Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"F2F5FA"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    blackBorder(),
	})
	left, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center", WrapText: true},
		Border:    blackBorder(),
	})
	label, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 14, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	money, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11, Bold: true},
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	note, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 9},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center", WrapText: true},
		Border:    blackBorder(),
	})

	_ = f.MergeCell(sheet, "A1", "G1")
	_ = f.SetCellValue(sheet, "A1", "出行报销验收单")
	_ = f.SetCellStyle(sheet, "A1", "G1", title)
	_ = f.SetRowHeight(sheet, 1, 55)

	_ = f.MergeCell(sheet, "A2", "D2")
	_ = f.SetCellValue(sheet, "A2", "报销部门：")
	_ = f.SetCellStyle(sheet, "A2", "D2", left)
	_ = f.MergeCell(sheet, "E2", "G2")
	_ = f.SetCellValue(sheet, "E2", "填报日期："+time.Now().Format("2006-01-02"))
	_ = f.SetCellStyle(sheet, "E2", "G2", left)
	_ = f.SetRowHeight(sheet, 2, 26)

	for i, h := range []string{"出行事由", "出行时间", "交通工具", "出行路线", "出行人", "金额(元)", "备注"} {
		_ = f.SetCellValue(sheet, cellName(i+1, 3), h)
	}
	_ = f.SetCellStyle(sheet, "A3", "G3", head)
	_ = f.SetRowHeight(sheet, 3, 26)

	const dataStart = 4
	for i, inv := range rows {
		r := dataStart + i
		_ = f.SetCellValue(sheet, cellName(1, r), inv.Title)
		_ = f.SetCellValue(sheet, cellName(2, r), inv.TravelTime)
		_ = f.SetCellValue(sheet, cellName(3, r), inv.Transport)
		_ = f.SetCellValue(sheet, cellName(4, r), inv.Route)
		_ = f.SetCellValue(sheet, cellName(5, r), inv.Traveler)
		_ = f.SetCellValue(sheet, cellName(6, r), inv.Amount)
		_ = f.SetCellValue(sheet, cellName(7, r), inv.Note)
		for c := 1; c <= 7; c++ {
			if c == 6 {
				_ = f.SetCellStyle(sheet, cellName(c, r), cellName(c, r), money)
				continue
			}
			_ = f.SetCellStyle(sheet, cellName(c, r), cellName(c, r), cell)
		}
		lines := 1
		for _, item := range []struct {
			text  string
			width float64
		}{{inv.Title, 26}, {inv.Route, 28}, {inv.Note, 16}} {
			if n := textLines(item.text, item.width, 11); n > lines {
				lines = n
			}
		}
		height := 30.0
		if lines > 1 {
			height = float64(lines)*14 + 10
		}
		_ = f.SetRowHeight(sheet, r, height)
	}

	totalRow := dataStart + len(rows)
	_ = f.MergeCell(sheet, cellName(1, totalRow), cellName(5, totalRow))
	_ = f.SetCellValue(sheet, cellName(1, totalRow), "合     计")
	_ = f.SetCellStyle(sheet, cellName(1, totalRow), cellName(5, totalRow), label)
	total := 0.0
	for _, inv := range rows {
		total += inv.Amount
	}
	_ = f.SetCellValue(sheet, cellName(6, totalRow), round2(total))
	_ = f.SetCellStyle(sheet, cellName(6, totalRow), cellName(6, totalRow), money)
	_ = f.SetCellStyle(sheet, cellName(7, totalRow), cellName(7, totalRow), cell)
	_ = f.SetRowHeight(sheet, totalRow, 36)

	sig := totalRow + 1
	_ = f.MergeCell(sheet, cellName(1, sig), cellName(2, sig))
	_ = f.SetCellValue(sheet, cellName(1, sig), "部门主管：")
	_ = f.SetCellStyle(sheet, cellName(1, sig), cellName(2, sig), left)
	_ = f.MergeCell(sheet, cellName(3, sig), cellName(5, sig))
	_ = f.SetCellValue(sheet, cellName(3, sig), "验收人：")
	_ = f.SetCellStyle(sheet, cellName(3, sig), cellName(5, sig), left)
	_ = f.MergeCell(sheet, cellName(6, sig), cellName(7, sig))
	_ = f.SetCellValue(sheet, cellName(6, sig), "报销人：")
	_ = f.SetCellStyle(sheet, cellName(6, sig), cellName(7, sig), left)
	_ = f.SetRowHeight(sheet, sig, 42)

	noteRow := sig + 1
	_ = f.MergeCell(sheet, cellName(1, noteRow), cellName(7, noteRow))
	_ = f.SetCellValue(sheet, cellName(1, noteRow), "备注：出行报销专用，请附发票及行程凭证。")
	_ = f.SetCellStyle(sheet, cellName(1, noteRow), cellName(7, noteRow), note)
	_ = f.SetRowHeight(sheet, noteRow, 26)

	fit := true
	_ = f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fit})
	a4, one := 9, 1
	_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Size: &a4, FitToWidth: &one})

	return saveForm(f)
}

func saveForm(f *excelize.File) ([]byte, error) {
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("生成表格失败：%w", err)
	}
	return buf.Bytes(), nil
}

func commonSeller(rows []model.Invoice) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	seller := strings.TrimSpace(rows[0].Seller)
	if seller == "" {
		return "", false
	}
	for _, r := range rows[1:] {
		if strings.TrimSpace(r.Seller) != seller {
			return "", false
		}
	}
	return seller, true
}

// findTotalRow 找「合 计」行；验收单在 A 列，清单在 E 列。
func findTotalRow(f *excelize.File, sheet string) (int, error) {
	for r := 2; r <= 60; r++ {
		for c := 1; c <= 7; c++ {
			v, err := f.GetCellValue(sheet, cellName(c, r))
			if err != nil {
				continue
			}
			v = strings.ReplaceAll(strings.ReplaceAll(v, " ", ""), "　", "")
			if strings.Contains(v, "合计") {
				return r, nil
			}
		}
	}
	return 0, fmt.Errorf("模板中找不到合计行")
}

// removeRowsFrom 删除指定行开始的 n 行（先解除合并，避免留下悬空区域）。
func removeRowsFrom(f *excelize.File, sheet string, fromRow, n int) {
	merges, _ := f.GetMergeCells(sheet)
	for _, m := range merges {
		_, r, err := excelize.CellNameToCoordinates(m.GetStartAxis())
		if err == nil && r >= fromRow {
			_ = f.UnmergeCell(sheet, m.GetStartAxis(), m.GetEndAxis())
		}
	}
	for i := 0; i < n; i++ {
		_ = f.RemoveRow(sheet, fromRow)
	}
}

// insertRows 在合计行前插入 n 行，并复制模板数据行的格式与行高。
func insertRows(f *excelize.File, sheet string, row, n, donor int) error {
	merges, _ := f.GetMergeCells(sheet)
	type span struct{ c1, r1, c2, r2 int }
	var moved []span
	for _, m := range merges {
		start, end := m.GetStartAxis(), m.GetEndAxis()
		c1, r1, e1 := excelize.CellNameToCoordinates(start)
		c2, r2, e2 := excelize.CellNameToCoordinates(end)
		if e1 != nil || e2 != nil {
			continue
		}
		if r1 >= row {
			moved = append(moved, span{c1, r1, c2, r2})
			if err := f.UnmergeCell(sheet, start, end); err != nil {
				return err
			}
		}
	}
	if err := f.InsertRows(sheet, row, n); err != nil {
		return err
	}
	for _, m := range moved {
		a, _ := excelize.CoordinatesToCellName(m.c1, m.r1+n)
		b, _ := excelize.CoordinatesToCellName(m.c2, m.r2+n)
		if err := f.MergeCell(sheet, a, b); err != nil {
			return err
		}
	}
	for r := row; r < row+n; r++ {
		for c := 1; c <= 7; c++ {
			style, err := f.GetCellStyle(sheet, cellName(c, donor))
			if err == nil {
				_ = f.SetCellStyle(sheet, cellName(c, r), cellName(c, r), style)
			}
		}
		if h, err := f.GetRowHeight(sheet, donor); err == nil && h > 0 {
			_ = f.SetRowHeight(sheet, r, h)
		}
	}
	return nil
}

type formStyles struct {
	text, cell, qty, money, totalMoney, label int
}

// newFormStyles 统一明细区样式：宋体、居中、全边框；数量取整，金额两位小数。
func newFormStyles(f *excelize.File) (formStyles, error) {
	text, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 9},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    blackBorder(),
	})
	if err != nil {
		return formStyles{}, err
	}
	cell, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 10},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	qty, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 10},
		NumFmt:    1,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	money, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 10},
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	totalMoney, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 11, Bold: true},
		NumFmt:    4,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	label, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "宋体", Size: 14, Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blackBorder(),
	})
	return formStyles{text: text, cell: cell, qty: qty, money: money, totalMoney: totalMoney, label: label}, nil
}

func blackBorder() []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: "000000", Style: 1},
		{Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1},
		{Type: "bottom", Color: "000000", Style: 1},
	}
}

func cellName(col, row int) string {
	name, _ := excelize.ColumnNumberToName(col)
	return fmt.Sprintf("%s%d", name, row)
}
