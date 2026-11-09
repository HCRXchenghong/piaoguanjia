package ocr

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"invoice/server/internal/model"
)

// ---------------------------------------------------------------- 结构
// Token 一段文字及其版面坐标。
type Token struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Src  string  `json:"-"`
}

// Cell 同一行内相邻文字合并后的单元格。
type Cell struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// Row 视觉上的一行。
type Row struct {
	Text   string  `json:"text"`
	Cells  []Cell  `json:"cells"`
	Tokens []Token `json:"tokens,omitempty"`
	Src    string  `json:"-"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
}

// ---------------------------------------------------------------- 归一化
var (
	reSpace      = regexp.MustCompile(`[ \t\x{00A0}]+`)
	reDotSep     = regexp.MustCompile(`(\d)\s*[·・]\s*(\d)`)
	reDotSpace   = regexp.MustCompile(`(\d)\.\s+(\d)`)
	reDotSpace2  = regexp.MustCompile(`(\d)\s+\.\s*(\d)`)
	reMoneySpace = regexp.MustCompile(`([¥$])\s+`)
	reSpacePunct = regexp.MustCompile(`\s+([,.:;%])`)
	rePunctSpace = regexp.MustCompile(`([,.:;%])\s+`)
	reCJKSpace   = regexp.MustCompile(`([\p{Han}])\s+([\p{Han}])`)
	reCJKNum     = regexp.MustCompile(`([\p{Han}])\s+([¥\d(])`)
	reNumCJK     = regexp.MustCompile(`([\d¥%])\s+([\p{Han}])`)
	reMultiSpace = regexp.MustCompile(`\s{2,}`)
	// OCR 常把小数点识别成逗号：0,18 / 17,90；真正的千分位（1,234.56）不会命中
	reCommaDecimal = regexp.MustCompile(`(\d),(\d{2})([^\d]|$)`)
)

var fullWidthReplacer = strings.NewReplacer(
	"：", ":", "，", ",", "．", ".", "（", "(", "）", ")",
	"￥", "¥", "—", "-", "–", "-", "％", "%", "／", "/", "＊", "*", "　", " ",
)

// OCR 偶尔把简体识别成繁体，统一回简体，否则「項目名称」匹配不到「项目名称」。
var traditionalReplacer = strings.NewReplacer(
	"項", "项", "規", "规", "稱", "称", "額", "额", "稅", "税",
	"單", "单", "數", "数", "價", "价", "號", "号", "碼", "码",
	"個", "个", "圓", "圆", "開", "开", "購", "购", "銷", "销",
	"發", "发", "據", "据", "幣", "币", "萬", "万", "類", "类",
	"貨", "货", "運", "运", "輸", "输", "務", "务", "費", "费",
)

// NormalizeLine 统一全角符号、去掉汉字间空格、修复小数点误识别。
func NormalizeLine(s string) string {
	if s == "" {
		return ""
	}
	s = traditionalReplacer.Replace(s)
	s = fullWidthReplacer.Replace(s)
	s = reSpace.ReplaceAllString(s, " ")
	s = reDotSep.ReplaceAllString(s, "$1.$2")
	s = reDotSpace.ReplaceAllString(s, "$1.$2")
	s = reDotSpace2.ReplaceAllString(s, "$1.$2")
	s = reCommaDecimal.ReplaceAllString(s, "$1.$2$3")
	s = reMoneySpace.ReplaceAllString(s, "$1")
	s = reSpacePunct.ReplaceAllString(s, "$1")
	s = rePunctSpace.ReplaceAllString(s, "$1")
	for i := 0; i < 2; i++ {
		s = reCJKSpace.ReplaceAllString(s, "$1$2")
		s = reCJKNum.ReplaceAllString(s, "$1$2")
		s = reNumCJK.ReplaceAllString(s, "$1$2")
	}
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------- 版面
func tokenHeight(t Token) float64 {
	if t.H > 0 {
		return t.H
	}
	return 12
}

func tokenCenter(t Token) float64 {
	return t.Y + tokenHeight(t)*0.5
}

func groupCells(items []Token) []Cell {
	if len(items) == 0 {
		return nil
	}
	ordered := make([]Token, len(items))
	copy(ordered, items)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].X < ordered[j].X })
	var cells []Cell
	for _, tok := range ordered {
		h := tokenHeight(tok)
		if n := len(cells); n > 0 {
			last := &cells[n-1]
			gap := tok.X - (last.X + last.W)
			limit := 1.15 * h
			if last.H > h {
				limit = 1.15 * last.H
			}
			if limit < 6 {
				limit = 6
			}
			if gap <= limit {
				last.Text += tok.Text
				end := tok.X + tok.W
				if end-last.X > last.W {
					last.W = end - last.X
				}
				continue
			}
		}
		cells = append(cells, Cell{Text: tok.Text, X: tok.X, W: tok.W, H: h})
	}
	out := cells[:0]
	for _, c := range cells {
		c.Text = NormalizeLine(c.Text)
		if c.Text != "" {
			out = append(out, c)
		}
	}
	return out
}

// BuildRows 按垂直中心聚类成行，并把行内文字切成单元格。
func BuildRows(tokens []Token) []Row {
	if len(tokens) == 0 {
		return nil
	}
	type node struct {
		tok Token
		yc  float64
		h   float64
	}
	nodes := make([]node, 0, len(tokens))
	for _, t := range tokens {
		if strings.TrimSpace(t.Text) == "" {
			continue
		}
		h := tokenHeight(t)
		nodes = append(nodes, node{tok: t, yc: tokenCenter(t), h: h})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].yc != nodes[j].yc {
			return nodes[i].yc < nodes[j].yc
		}
		return nodes[i].tok.X < nodes[j].tok.X
	})

	type rowAcc struct {
		items []Token
		yc    float64
		h     float64
	}
	var accs []rowAcc
	for _, n := range nodes {
		placed := false
		start := len(accs) - 8
		if start < 0 {
			start = 0
		}
		for i := len(accs) - 1; i >= start; i-- {
			tol := 0.5 * accs[i].h
			if 0.5*n.h > tol {
				tol = 0.5 * n.h
			}
			if tol < 4 {
				tol = 4
			}
			if absFloat(accs[i].yc-n.yc) <= tol {
				accs[i].items = append(accs[i].items, n.tok)
				placed = true
				break
			}
		}
		if !placed {
			accs = append(accs, rowAcc{items: []Token{n.tok}, yc: n.yc, h: n.h})
		}
	}

	rows := make([]Row, 0, len(accs))
	for _, acc := range accs {
		cells := groupCells(acc.items)
		if len(cells) == 0 {
			continue
		}
		sort.SliceStable(acc.items, func(i, j int) bool { return acc.items[i].X < acc.items[j].X })
		var sb strings.Builder
		var prevEnd float64
		first := true
		for _, item := range acc.items {
			if !first {
				gap := item.X - prevEnd
				limit := 0.5 * tokenHeight(item)
				if limit < 4 {
					limit = 4
				}
				if gap > limit && !strings.HasSuffix(sb.String(), " ") {
					sb.WriteByte(' ')
				}
			}
			sb.WriteString(item.Text)
			prevEnd = item.X + item.W
			first = false
		}
		text := NormalizeLine(sb.String())
		if text == "" {
			continue
		}
		x0, x1 := acc.items[0].X, acc.items[0].X+acc.items[0].W
		for _, it := range acc.items {
			if it.X < x0 {
				x0 = it.X
			}
			if it.X+it.W > x1 {
				x1 = it.X + it.W
			}
		}
		raw := make([]Token, 0, len(acc.items))
		src := ""
		for _, it := range acc.items {
			raw = append(raw, Token{Text: it.Text, X: it.X, Y: it.Y, W: it.W, H: it.H, Src: it.Src})
			if src == "" {
				src = it.Src
			}
		}
		rows = append(rows, Row{Text: text, Cells: cells, Tokens: raw, Src: src, X: x0, Y: acc.yc - acc.h, W: x1 - x0, H: acc.h})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Y < rows[j].Y })
	return rows
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---------------------------------------------------------------- 票据类型
var kindRules = []struct {
	Code  string
	Label string
	Keys  []string
}{
	{"vat_special", "增值税专用发票", []string{"增值税专用发票", "专用发票"}},
	{"vat_normal", "增值税普通发票", []string{"增值税普通发票", "增值税电子普通发票", "普通发票"}},
	{"e_invoice", "全面数字化电子发票", []string{"电子发票(普通发票)", "电子发票(专用发票)", "数电票", "全电发票", "电子发票"}},
	{"train", "铁路电子客票", []string{"铁路电子客票", "火车票", "中国铁路", "电子客票"}},
	{"flight", "航空运输电子客票行程单", []string{"航空运输电子客票", "行程单", "机票"}},
	{"taxi", "出租汽车/网约车发票", []string{"出租汽车", "网约车", "出租车", "滴滴出行"}},
	{"quota", "定额发票", []string{"定额发票"}},
}

var travelKinds = map[string]bool{"train": true, "flight": true, "taxi": true}

const moneyExpr = `(\d{1,6}(?:,\d{3})*\.\d{2}|\d+\.\d{2})`

var (
	reInvoiceLabel = regexp.MustCompile(`发票号码|发票号|票据号码|数电票号码`)
	reDigitsRun    = regexp.MustCompile(`\d{8,20}`)
	reSpaceHyphen  = regexp.MustCompile(`[\s\-—–]`)
)

// extractInvoiceNo 兼容两种情况：号码紧跟在标签后、号码在下一行；
// 也兼容 PDF/OCR 把长号码拆成「2644 2000 0127 6255 4386」这种分组。
func extractInvoiceNo(texts []string) (string, float64) {
	for i, row := range texts {
		loc := reInvoiceLabel.FindStringIndex(row)
		if loc == nil {
			continue
		}
		tail := row[loc[1]:]
		if code := digitsAcrossSeparators(tail); code != "" {
			return code, 0.98
		}
		for j := i + 1; j <= i+2 && j < len(texts); j++ {
			if code := digitsAcrossSeparators(texts[j]); code != "" {
				return code, 0.9
			}
		}
	}
	return "", 0
}

func digitsAcrossSeparators(s string) string {
	s = strings.ReplaceAll(s, "，", ",")
	// 先去掉数字之间的空格与短横线，避免分组导致匹配不到
	normalized := reSpaceHyphen.ReplaceAllString(s, "")
	if m := reDigitsRun.FindString(normalized); m != "" {
		return m
	}
	return ""
}

var (
	reInvoiceCode = regexp.MustCompile(`发票代码[:]?\s*(\d{10,12})`)
	reDate        = []*regexp.Regexp{
		regexp.MustCompile(`(20\d{2}\s*年\s*\d{1,2}\s*月\s*\d{1,2}\s*日)`),
		regexp.MustCompile(`开票日期[:]?\s*(20\d{2}[-/.]\d{1,2}[-/.]\d{1,2})`),
		regexp.MustCompile(`(20\d{2}[-/.]\d{1,2}[-/.]\d{1,2})`),
	}
	reTotal = []*regexp.Regexp{
		regexp.MustCompile(`价税合计[^0-9¥]{0,12}[¥￥]?\s*` + moneyExpr),
		regexp.MustCompile(`小写[^0-9¥]{0,8}[¥￥]?\s*` + moneyExpr),
		regexp.MustCompile(`(?:票价|合计金额|金额合计)[:]?\s*[¥￥]?\s*` + moneyExpr),
		regexp.MustCompile(`合计[^0-9¥]{0,8}[¥￥]?\s*` + moneyExpr),
	}
	reMoneyAny = regexp.MustCompile(moneyExpr)
	reYenMoney = regexp.MustCompile(`¥\s?(\d{1,6}(?:,\d{3})*\.\d{2})`)
	reCapital  = regexp.MustCompile(`([零壹贰叁肆伍陆柒捌玖拾佰仟万亿元圆角分整]{4,})`)
	reName     = regexp.MustCompile(`名称[:]?\s*([^\s:]{2,40})`)
	reTaxID    = regexp.MustCompile(`(?:识别号|信用代码)[:]?\s*([0-9A-Z]{15,20})`)
	reTaxIDAny = regexp.MustCompile(`([0-9A-Z]{18})`)
)

func detectKind(flat string) (string, string) {
	for _, r := range kindRules {
		for _, k := range r.Keys {
			if strings.Contains(flat, strings.ReplaceAll(k, " ", "")) {
				return r.Code, r.Label
			}
		}
	}
	if strings.Contains(flat, "增值税") {
		return "vat_normal", "增值税发票"
	}
	return "unknown", "其他票据"
}

func normDate(raw string) string {
	m := regexp.MustCompile(`(20\d{2})\D{0,2}(\d{1,2})\D{0,2}(\d{1,2})`).FindStringSubmatch(raw)
	if m != nil {
		mm, _ := strconv.Atoi(m[2])
		dd, _ := strconv.Atoi(m[3])
		return pad2(m[1]) + "-" + pad2Int(mm) + "-" + pad2Int(dd)
	}
	return raw
}

func pad2(s string) string {
	if len(s) >= 4 {
		return s
	}
	if len(s) == 2 {
		return s
	}
	return "0" + s
}

func pad2Int(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

// ---------------------------------------------------------------- 解析入口
// Parse 从版面行中抽取发票要素并给出表单建议。
func Parse(rows []Row, settings map[string]string) *model.ParseResult {
	texts := make([]string, 0, len(rows))
	for _, r := range rows {
		texts = append(texts, NormalizeLine(r.Text))
	}
	text := strings.Join(texts, "\n")
	flat := strings.ReplaceAll(text, " ", "")
	flat = strings.ReplaceAll(flat, "\n", "")

	kind, kindLabel := detectKind(flat)
	fields := map[string]model.Field{}
	warnings := []string{}

	// 票号
	if code, conf := extractInvoiceNo(texts); code != "" {
		fields["invoice_no"] = model.Field{Value: code, Confidence: conf, Source: "label"}
	} else if m := regexp.MustCompile(`(?:^|\D)(\d{20})(?:\D|$)`).FindStringSubmatch(flat); m != nil {
		fields["invoice_no"] = model.Field{Value: m[1], Confidence: 0.6, Source: "pattern"}
	}
	if m := findFirst(texts, reInvoiceCode); m != nil {
		fields["invoice_code"] = model.Field{Value: m[1], Confidence: 0.95, Source: "label"}
	}

	// 日期
	for _, re := range reDate {
		if m := findFirst(texts, re); m != nil {
			conf := 0.7
			if strings.Contains(m[0], "日期") {
				conf = 0.92
			}
			fields["invoice_date"] = model.Field{Value: normDate(m[1]), Confidence: conf, Source: "label"}
			break
		}
	}

	// 购销方
	// 购销方：优先按版面（左购右销）抽取，失败再退回标签匹配
	buyerName, sellerName, buyerTax, sellerTax := extractParties(rows)
	if buyerName != "" {
		fields["buyer"] = model.Field{Value: buyerName, Confidence: 0.9, Source: "layout"}
		if buyerTax != "" {
			fields["buyer_tax_id"] = model.Field{Value: buyerTax, Confidence: 0.8, Source: "layout"}
		}
	} else if name, taxID, ok := findParty(texts, []string{"购买方", "购方", "付款方", "抬头"}); ok {
		fields["buyer"] = model.Field{Value: name, Confidence: 0.9, Source: "label"}
		fields["buyer_tax_id"] = model.Field{Value: taxID, Confidence: boolConf(taxID != "", 0.75), Source: "label"}
	}
	if sellerName != "" {
		fields["seller"] = model.Field{Value: sellerName, Confidence: 0.9, Source: "layout"}
		if sellerTax != "" {
			fields["seller_tax_id"] = model.Field{Value: sellerTax, Confidence: 0.8, Source: "layout"}
		}
	} else if name, taxID, ok := findParty(texts, []string{"销售方", "销方", "开票方", "收款方"}); ok {
		fields["seller"] = model.Field{Value: name, Confidence: 0.9, Source: "label"}
		fields["seller_tax_id"] = model.Field{Value: taxID, Confidence: boolConf(taxID != "", 0.75), Source: "label"}
	}

	// 金额
	total := extractTotal(texts, flat)
	if total != nil {
		fields["total"] = *total
	}
	tax := extractTax(texts)
	if tax != nil {
		fields["tax"] = *tax
	}
	amount := extractAmount(texts)
	if amount != nil {
		fields["amount"] = *amount
	}
	if total != nil && tax != nil && amount != nil {
		tv := toFloatAny(total.Value)
		diff := (toFloatAny(amount.Value) + toFloatAny(tax.Value)) - tv
		if diff < 0 {
			diff = -diff
		}
		if diff <= 0.02 {
			total.Confidence = maxF(total.Confidence, 0.95)
			amount.Confidence = maxF(amount.Confidence, 0.9)
			tax.Confidence = maxF(tax.Confidence, 0.9)
			fields["total"] = *total
			fields["tax"] = *tax
			fields["amount"] = *amount
		} else if diff > 1 {
			warnings = append(warnings, "金额与税额之和与价税合计不一致，请核对")
		}
	}

	// 明细
	items := extractItems(rows)
	if len(items) > 0 && total == nil {
		sum := 0.0
		for _, it := range items {
			if it.Amount != nil {
				sum += *it.Amount
			}
		}
		if sum > 0 {
			fields["total"] = model.Field{Value: round2(sum), Confidence: 0.6, Source: "sum"}
		}
	}

	travel := map[string]any{}
	if travelKinds[kind] || looksTravel(flat) {
		travel = extractTravel(texts, flat, kind)
	}

	suggest := buildSuggest(kind, fields, items, travel, flat, settings)
	conf := 0.0
	n := 0
	for _, k := range []string{"invoice_no", "invoice_date", "total", "seller"} {
		if f, ok := fields[k]; ok {
			conf += f.Confidence
			n++
		}
	}
	if n > 0 {
		conf /= float64(n)
	}

	return &model.ParseResult{
		Kind: kind, KindLabel: kindLabel,
		SuggestType: suggest["type"].(string),
		Fields:      fields, Items: items, Travel: travel, Suggest: suggest,
		Warnings: warnings, RawText: text,
		Confidence: round2(conf), TokenCount: len(rows),
	}
}

func boolConf(cond bool, v float64) float64 {
	if cond {
		return v
	}
	return 0
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func toFloatAny(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(t, ",", ""), 64)
		return f
	}
	return 0
}

func findFirst(texts []string, re *regexp.Regexp) []string {
	for _, t := range texts {
		if m := re.FindStringSubmatch(t); m != nil {
			return m
		}
	}
	return nil
}

func parseMoney(s string) float64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return f
}

// extractParties 按版面顺序抽取购销方：表头之前、按 x 从左到右，前者为购买方、后者为销售方。
func extractParties(rows []Row) (buyer, seller, buyerTax, sellerTax string) {
	headerIdx := findHeaderRow(rows)
	if headerIdx < 0 {
		headerIdx = len(rows)
	}
	type named struct {
		text string
		x    float64
	}
	var names []named
	var taxes []named
	reTaxCode := regexp.MustCompile(`[0-9A-Z]{15,20}`)

	for i := 0; i < headerIdx && i < len(rows); i++ {
		row := rows[i]
		for j, cell := range row.Cells {
			text := cell.Text
			if strings.Contains(text, "名称") && !strings.Contains(text, "项目名称") {
				val := strings.TrimLeft(text[strings.Index(text, "名称")+len("名称"):], ":： ")
				// 值常常在右边一格，或者干脆在下一行
				if val == "" && j+1 < len(row.Cells) {
					val = strings.TrimLeft(row.Cells[j+1].Text, ":： ")
				}
				if val == "" && i+1 < headerIdx && len(rows[i+1].Cells) > 0 {
					val = strings.TrimLeft(rows[i+1].Cells[0].Text, ":： ")
				}
				val = cleanPartyName(val)
				if val != "" {
					names = append(names, named{text: val, x: cell.X})
				}
			}
			if idx := strings.Index(text, "识别号"); idx >= 0 {
				rest := text[idx:]
				for _, code := range reTaxCode.FindAllString(rest, -1) {
					taxes = append(taxes, named{text: code, x: cell.X})
				}
			}
		}
	}

	sort.SliceStable(names, func(i, j int) bool { return names[i].x < names[j].x })
	sort.SliceStable(taxes, func(i, j int) bool { return taxes[i].x < taxes[j].x })

	if len(names) > 0 {
		buyer = names[0].text
	}
	if len(names) > 1 {
		seller = names[1].text
	}
	if len(taxes) > 0 {
		buyerTax = taxes[0].text
	}
	if len(taxes) > 1 {
		sellerTax = taxes[1].text
	}
	return
}

func isLabelWord(s string) bool {
	switch s {
	case "名称", "全称", "纳税人识别号", "统一社会信用代码", "购买方", "销售方", "信息",
		"地址", "电话", "开户行", "账号", "项目名称", "货物或应税劳务", "服务名称",
		"规格", "规格型号", "型号", "单位", "数量", "单价", "金额", "税额", "税率", "征收率":
		return true
	}
	return false
}

// cleanPartyName 去掉残留标签、冒号与长度异常的值，避免把「名称」当成公司名。
func cleanPartyName(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < 3; i++ {
		trimmed := strings.TrimLeft(s, ":： 　")
		if strings.HasPrefix(trimmed, "名称") {
			trimmed = trimmed[len("名称"):]
		}
		trimmed = strings.TrimLeft(trimmed, ":： 　")
		if trimmed == s {
			break
		}
		s = trimmed
	}
	s = strings.Trim(s, "*＊:： 　")
	if isLabelWord(s) {
		return ""
	}
	if strings.Contains(s, "纳税人识别号") || strings.Contains(s, "社会信用代码") {
		return ""
	}
	runes := []rune(s)
	if len(runes) < 2 {
		return ""
	}
	if len(runes) > 40 {
		s = string(runes[:40])
	}
	// 纯数字或纯符号不是公司名
	if regexp.MustCompile(`^[\d\s\-—:：()（）]+$`).MatchString(s) {
		return ""
	}
	// 表头里的词（规格型号/单位/数量…）不是公司名
	for _, k := range headerKeywords {
		if strings.Contains(s, k) {
			return ""
		}
	}
	return s
}

func findParty(texts []string, labels []string) (string, string, bool) {
	for i, row := range texts {
		matched := false
		for _, lab := range labels {
			if strings.Contains(row, lab) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		name := ""
		if m := reName.FindStringSubmatch(row); m != nil {
			name = cleanPartyName(m[1])
		}
		if name == "" {
			for j := i + 1; j < len(texts) && j <= i+2; j++ {
				if m := reName.FindStringSubmatch(texts[j]); m != nil {
					if v := cleanPartyName(m[1]); v != "" {
						name = v
						break
					}
				}
			}
		}
		if name == "" {
			tail := row
			for _, lab := range labels {
				if idx := strings.Index(tail, lab); idx >= 0 {
					tail = tail[idx+len(lab):]
				}
			}
			tail = strings.TrimLeft(tail, ": ")
			if idx := strings.Index(tail, "名称"); idx >= 0 {
				tail = tail[idx+len("名称"):]
			}
			if v := cleanPartyName(tail); v != "" {
				name = v
			}
		}
		if name == "" {
			continue
		}
		name = strings.Trim(name, "*＊ ")
		taxID := ""
		for j := i; j < len(texts) && j <= i+2; j++ {
			if m := reTaxID.FindStringSubmatch(texts[j]); m != nil {
				taxID = m[1]
				break
			}
		}
		if taxID == "" {
			if m := reTaxIDAny.FindStringSubmatch(row); m != nil {
				taxID = m[1]
			}
		}
		return name, taxID, true
	}
	return "", "", false
}

func extractTotal(texts []string, flat string) *model.Field {
	for _, re := range reTotal {
		for _, t := range texts {
			if m := re.FindStringSubmatch(t); m != nil {
				return &model.Field{Value: parseMoney(m[1]), Confidence: 0.9, Source: "label"}
			}
		}
	}
	for _, t := range texts {
		if strings.Contains(t, "价税合计") || strings.Contains(t, "大写") {
			if m := reCapital.FindStringSubmatch(t); m != nil {
				if v, ok := ParseCNAmount(m[1]); ok {
					return &model.Field{Value: v, Confidence: 0.85, Source: "capital"}
				}
			}
		}
	}
	if ms := reYenMoney.FindAllStringSubmatch(flat, -1); len(ms) > 0 {
		return &model.Field{Value: parseMoney(ms[len(ms)-1][1]), Confidence: 0.55, Source: "symbol"}
	}
	return nil
}

func extractTax(texts []string) *model.Field {
	for _, t := range texts {
		if strings.HasPrefix(t, "合计") || strings.Contains(t, "合计") && strings.Contains(t, "税") {
			if ms := reMoneyAny.FindAllString(t, -1); len(ms) >= 2 {
				return &model.Field{Value: parseMoney(ms[len(ms)-1]), Confidence: 0.8, Source: "row"}
			}
		}
	}
	return nil
}

func extractAmount(texts []string) *model.Field {
	for _, t := range texts {
		if strings.HasPrefix(t, "合计") || strings.Contains(t, "合 计") {
			if ms := reMoneyAny.FindAllString(t, -1); len(ms) >= 1 {
				return &model.Field{Value: parseMoney(ms[0]), Confidence: 0.8, Source: "row"}
			}
		}
	}
	return nil
}

func looksTravel(flat string) bool {
	keys := []string{"车次", "航班", "出发站", "到达站", "乘车", "行程", "起点", "终点", "登机", "座位"}
	n := 0
	for _, k := range keys {
		if strings.Contains(flat, k) {
			n++
		}
	}
	return n >= 2
}

// ---------------------------------------------------------------- 明细
var headerLabels = []struct {
	Key    string
	Labels []string
}{
	{"name", []string{"项目名称", "货物或应税劳务", "货物名称", "品名", "服务名称"}},
	{"spec", []string{"规格型号", "型号", "规格"}},
	{"unit", []string{"单位"}},
	{"qty", []string{"数量"}},
	{"price", []string{"单价"}},
	{"amount", []string{"金额"}},
	{"rate", []string{"税率", "征收率"}},
	{"tax", []string{"税额"}},
}

func extractItems(rows []Row) []model.Item {
	headerIdx := findHeaderRow(rows)
	if headerIdx < 0 {
		return nil
	}
	cols := map[string]float64{}
	for _, cell := range rows[headerIdx].Cells {
		for _, hl := range headerLabels {
			if _, exists := cols[hl.Key]; exists {
				continue
			}
			for _, lab := range hl.Labels {
				if idx := strings.Index(cell.Text, lab); idx >= 0 {
					ratio := float64(idx) / float64(maxInt(len(cell.Text), 1))
					cols[hl.Key] = cell.X + cell.W*ratio
					break
				}
			}
		}
	}

	if len(cols) < 2 {
		return nil
	}

	var items []model.Item
	for _, row := range rows[headerIdx+1:] {
		if isStopRow(row.Text) {
			break
		}
		if item, ok := assignRow(row, cols); ok {
			items = append(items, postItem(item))
		}
	}
	return items
}

var headerKeywords = []string{"规格", "型号", "单位", "数量", "单价", "金额", "税额", "税率", "征收率"}

// findHeaderRow 找明细表头行；OCR 可能把「项」识别成繁体，也可能只认出半个表头，所以放宽条件。
func findHeaderRow(rows []Row) int {
	for i, row := range rows {
		text := row.Text
		if strings.Contains(text, "项目名称") || strings.Contains(text, "货物或应税劳务") ||
			strings.Contains(text, "货物名称") || strings.Contains(text, "品名") || strings.Contains(text, "服务名称") {
			return i
		}
		hits := 0
		for _, k := range headerKeywords {
			if strings.Contains(text, k) {
				hits++
			}
		}
		if hits >= 3 && (strings.Contains(text, "名称") || strings.Contains(text, "项目")) {
			return i
		}
	}
	return -1
}

func isStopRow(text string) bool {
	for _, p := range []string{"合计", "价税合计", "小写", "备注", "收款人", "开票人", "销售方", "购买方"} {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// 数值列：用 token 逐个归属，避免相邻数字被合并成一个单元格。
func assignRow(row Row, cols map[string]float64) (model.Item, bool) {
	if len(row.Cells) == 0 {
		return model.Item{}, false
	}
	item := model.Item{}
	var names, specs []string
	needTokenNumeric := false

	for _, cell := range row.Cells {
		key, dist := nearestColumn(cell.X, cols)
		if key == "" || dist > 170 {
			// 表头没认全时，远离任何已知列的内容多半是别的列，宁可不收也别拼进名称
			continue
		}
		switch key {
		case "name":
			names = append(names, cell.Text)
		case "spec":
			specs = append(specs, cell.Text)
		case "unit":
			if item.Unit == "" {
				item.Unit = cell.Text
			}
		default:
			// PDF 文字层的数字经常与相邻列贴在一起，统一按 token 逐个分配；
			// OCR 结果的单词已按视觉切分，只有明显塞了多个数字时才拆。
			if row.Src == "pdf" && len(row.Tokens) > 1 {
				needTokenNumeric = true
				continue
			}
			if countNumberFragments(cell.Text) >= 2 && len(row.Tokens) > 1 {
				needTokenNumeric = true
				continue
			}
			setItemNumber(&item, key, cell.Text)
		}
	}

	if needTokenNumeric {
		for _, tok := range row.Tokens {
			if !looksNumeric(tok.Text) {
				continue
			}
			key, dist := nearestColumn(tok.X+tok.W/2, cols)
			if key == "" || dist > 170 || !isNumericKey(key) {
				continue
			}
			setItemNumber(&item, key, tok.Text)
		}
	}

	item.Name = cleanItemName(strings.Join(names, " "))
	item.Spec = strings.Join(specs, " ")
	if item.Name == "" && item.Amount == nil {
		return model.Item{}, false
	}
	return item, true
}

func nearestColumn(cx float64, cols map[string]float64) (string, float64) {
	bestKey, bestDist := "", 1e9
	for key, colX := range cols {
		d := absFloat(cx - colX)
		if d < bestDist {
			bestKey, bestDist = key, d
		}
	}
	return bestKey, bestDist
}

func isNumericKey(key string) bool {
	switch key {
	case "qty", "price", "amount", "rate", "tax":
		return true
	}
	return false
}

func setItemNumber(item *model.Item, key, text string) {
	val := ptrFloat(strings.TrimSuffix(text, "%"))
	if val == nil {
		return
	}
	switch key {
	case "qty":
		if item.Qty == nil {
			item.Qty = val
		}
	case "price":
		if item.Price == nil {
			item.Price = val
		}
	case "amount":
		if item.Amount == nil {
			item.Amount = val
		}
	case "rate":
		if item.Rate == nil {
			item.Rate = val
		}
	case "tax":
		if item.Tax == nil {
			item.Tax = val
		}
	}
}

var reNumberFrag = regexp.MustCompile(`\d+(?:\.\d+)?`)
var reNumericToken = regexp.MustCompile(`^[¥￥]?-?\d+(?:,\d{3})*(?:\.\d+)?%?$`)

// textWidth 估算一段文字在给定字号下的宽度（CJK 按 1em、ASCII 按 0.55em）。
func textWidth(text string, size float64) float64 {
	var w float64
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3000 && r <= 0x303F, r >= 0xFF00 && r <= 0xFF60:
			w += size
		case r == ' ':
			w += size * 0.3
		default:
			w += size * 0.55
		}
	}
	if w <= 0 {
		w = size
	}
	return w
}

func countNumberFragments(s string) int {
	return len(reNumberFrag.FindAllString(s, -1))
}

// looksNumeric 判断 token 是否是一个完整数字（允许货币符号与百分号）。
func looksNumeric(s string) bool {
	return reNumericToken.MatchString(strings.TrimSpace(s))
}

func ptrFloat(s string) *float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	m := regexp.MustCompile(`-?\d+(?:\.\d+)?`).FindString(strings.ReplaceAll(s, ",", ""))
	if m == "" {
		return nil
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	return &v
}

var reItemPrefix = regexp.MustCompile(`^\s*[*＊][^*＊]{0,16}[*＊]\s*`)

func cleanItemName(s string) string {
	s = strings.TrimSpace(reMultiSpace.ReplaceAllString(s, " "))
	// 发票品名常见写法：*金属制品*螺丝 / 金属制品*螺丝 —— 取最后一段作为品名
	if idx := strings.LastIndex(s, "*"); idx > 0 && idx < len(s)-1 {
		head := s[:idx]
		if len([]rune(head)) <= 12 {
			s = s[idx+1:]
		}
	}
	s = reItemPrefix.ReplaceAllString(s, "")
	return strings.Trim(s, "*＊ ")
}

func postItem(it model.Item) model.Item {
	// 票面单价常是 151/1.01 这类长小数，统一收敛到分
	if it.Price != nil {
		v := round2(*it.Price)
		it.Price = &v
	}
	if it.Amount != nil {
		v := round2(*it.Amount)
		it.Amount = &v
	}
	if it.Tax != nil {
		v := round2(*it.Tax)
		it.Tax = &v
	}
	if it.Qty != nil && it.Price != nil && it.Amount != nil {
		diff := *it.Qty * *it.Price - *it.Amount
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.05 && diff > *it.Amount*0.02 {
			it.Amount = nil
		}
	}
	return it
}

// ---------------------------------------------------------------- 出行
var transportRules = []struct {
	Name string
	Keys []string
}{
	{"飞机", []string{"航班", "机票", "航空", "登机"}},
	{"火车", []string{"高铁", "动车", "火车", "铁路", "车次"}},
	{"网约车", []string{"网约车", "滴滴", "曹操", "出行"}},
	{"出租车", []string{"出租汽车", "出租车", "的士"}},
	{"公共交通", []string{"地铁", "公交", "轻轨"}},
	{"汽车", []string{"客运", "长途", "大巴"}},
}

var stopLabels = []string{"乘车日期", "出发站", "到达站", "出发地", "目的地", "车次", "座位号",
	"票价", "开票日期", "身份证", "身份", "证件", "购买方", "销售方", "备注", "号码", "金额", "税额", "小写",
	"身", "证", "座"}

func trimLabel(s string) string {
	for _, stop := range stopLabels {
		if idx := strings.Index(s, stop); idx > 0 {
			s = s[:idx]
		}
	}
	return strings.Trim(s, " :-·")
}

var (
	reTravelDate = regexp.MustCompile(`(?:乘车日期|出发日期|乘机日期|用车时间|上车时间|发车时间)[:]?\s*(20\d{2}\s*年\s*\d{1,2}\s*月\s*\d{1,2}\s*日)`)
	reAnyDate    = regexp.MustCompile(`(20\d{2}\s*年\s*\d{1,2}\s*月\s*\d{1,2}\s*日)`)
	reISODate    = regexp.MustCompile(`(20\d{2}-\d{2}-\d{2})`)
	reClock      = regexp.MustCompile(`(\d{2}:\d{2})`)
	reRoute1     = regexp.MustCompile(`出发站[:]?\s*([\p{Han}]{2,10}?)\s*(?:到达站|终点站)[:]?\s*([\p{Han}]{2,10})`)
	reRoute2     = regexp.MustCompile(`(?:出发地|起点|上车地点)[:]?\s*([\p{Han}A-Za-z0-9]{2,14}?)\s*(?:到达地|目的地|终点|下车地点)[:]?\s*([\p{Han}A-Za-z0-9]{2,14})`)
	reRoute3     = regexp.MustCompile(`([\p{Han}]{2,8}站?)\s*[-—至]\s*([\p{Han}]{2,8}站?)`)
	reTraveler   = regexp.MustCompile(`(?:乘车人|乘客|旅客|出差人)[:]?\s*([\p{Han}]{2,4})`)
	reTrainNo    = regexp.MustCompile(`车次[:]?\s*([GDCZTKgdcztk]\d{1,5})`)
	reFlightNo   = regexp.MustCompile(`航班号?[:]?\s*([A-Z]{2}\d{3,4})`)
)

func extractTravel(texts []string, flat string, kind string) map[string]any {
	out := map[string]any{}
	transport := ""
	for _, r := range transportRules {
		for _, k := range r.Keys {
			if strings.Contains(flat, k) {
				transport = r.Name
				break
			}
		}
		if transport != "" {
			break
		}
	}
	if transport == "" {
		switch kind {
		case "train":
			transport = "火车"
		case "flight":
			transport = "飞机"
		case "taxi":
			transport = "网约车"
		}
	}
	if transport != "" {
		out["transport"] = transport
	}

	date := ""
	if m := reTravelDate.FindStringSubmatch(flat); m != nil {
		date = normDate(m[1])
	} else if m := reAnyDate.FindStringSubmatch(flat); m != nil {
		date = normDate(m[1])
	} else if m := reISODate.FindStringSubmatch(flat); m != nil {
		date = m[1]
	}
	if date != "" {
		out["travel_time"] = date
	}
	if m := reClock.FindStringSubmatch(flat); m != nil {
		out["travel_clock"] = m[1]
	}

	route := ""
	for _, re := range []*regexp.Regexp{reRoute1, reRoute2, reRoute3} {
		if m := re.FindStringSubmatch(flat); m != nil {
			route = trimLabel(m[1]) + "-" + trimLabel(m[2])
			break
		}
	}
	if route != "" {
		out["route"] = route
	}
	if m := reTraveler.FindStringSubmatch(flat); m != nil {
		out["traveler"] = trimLabel(m[1])
	}
	if m := reTrainNo.FindStringSubmatch(flat); m != nil {
		out["train_no"] = m[1]
	}
	if m := reFlightNo.FindStringSubmatch(flat); m != nil {
		out["flight_no"] = m[1]
	}
	return out
}

// ---------------------------------------------------------------- 表单建议
var categoryRules = []struct {
	Name string
	Keys []string
}{
	{"办公用品", []string{"办公", "文具", "纸", "笔", "文件夹", "打印", "墨盒", "耗材", "资料"}},
	{"电子设备", []string{"电脑", "显示器", "硬盘", "主板", "服务器", "键盘", "鼠标", "路由器", "电子", "芯片", "配件"}},
	{"软件服务", []string{"软件", "云服务", "saas", "订阅", "授权", "技术服务", "开发", "平台"}},
	{"差旅交通", []string{"机票", "火车", "高铁", "动车", "打车", "出租", "网约车", "住宿", "酒店", "宾馆", "差旅", "客运"}},
	{"餐饮招待", []string{"餐饮", "餐", "食品", "饮料", "招待", "咖啡", "茶"}},
	{"通讯网络", []string{"话费", "流量", "宽带", "通信", "电信", "移动", "联通", "网络"}},
	{"物流运输", []string{"快递", "物流", "运输", "运费", "货运"}},
}

func buildSuggest(kind string, fields map[string]model.Field, items []model.Item,
	travel map[string]any, flat string, settings map[string]string) map[string]any {
	totalField, hasTotal := fields["total"]
	var totalVal float64
	if hasTotal {
		totalVal = toFloatAny(totalField.Value)
	}
	categories := []string{}
	for _, c := range strings.Split(settings["categories"], ",") {
		if c = strings.TrimSpace(c); c != "" {
			categories = append(categories, c)
		}
	}
	isTravel := travelKinds[kind]
	suggest := map[string]any{"type": "procurement"}
	if isTravel {
		suggest["type"] = "travel"
	}

	name := ""
	var item model.Item
	if len(items) > 0 {
		item = items[0]
		name = item.Name
	}
	if name == "" {
		if m := regexp.MustCompile(`[*＊]([^*＊]{2,12})[*＊]`).FindStringSubmatch(flat); m != nil {
			name = m[1]
		}
	}
	if name == "" {
		if f, ok := fields["seller"]; ok {
			name = toStr(f.Value)
		}
	}

	category := ""
	if isTravel {
		if len(categories) == 0 || containsStr(categories, "差旅交通") {
			category = "差旅交通"
		} else {
			category = categories[len(categories)-1]
		}
	}
	if category == "" {
		haystack := strings.ToLower(name + " " + item.Spec + " " + firstRunes(flat, 1200))
		for _, rule := range categoryRules {
			for _, k := range rule.Keys {
				if strings.Contains(haystack, strings.ToLower(k)) {
					category = rule.Name
					break
				}
			}
			if category != "" {
				break
			}
		}
	}
	if category == "" && len(categories) > 0 {
		category = categories[len(categories)-1]
	}

	if isTravel {
		title := toStr(travel["route"])
		if title == "" {
			title = name
		}
		if title == "" {
			title = "出行费用"
		}
		suggest["title"] = title
		suggest["category"] = category
		if v, ok := travel["travel_time"]; ok {
			suggest["travel_time"] = v
		} else if f, ok := fields["invoice_date"]; ok {
			suggest["travel_time"] = f.Value
		}
		suggest["transport"] = toStr(travel["transport"])
		suggest["route"] = toStr(travel["route"])
		suggest["traveler"] = toStr(travel["traveler"])
		if hasTotal {
			suggest["amount"] = totalVal
		}
		return suggest
	}

	qty := item.Qty
	unitPrice := item.Price
	amount := item.Amount
	if hasTotal {
		amount = &totalVal
		unitPrice = &totalVal
		if qty != nil && *qty > 1 {
			v := round2(totalVal / *qty)
			unitPrice = &v
		}
	}
	if name == "" {
		name = "采购支出"
	}
	suggest["title"] = name
	suggest["category"] = category
	suggest["spec"] = item.Spec
	suggest["qty"] = qty
	suggest["unit_price"] = unitPrice
	suggest["shipping"] = 0
	if amount != nil {
		suggest["amount"] = *amount
	}
	if hasTotal {
		suggest["amount"] = totalVal
	}
	return suggest
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ---------------------------------------------------------------- 中文金额
var cnDigits = map[rune]int{'零': 0, '壹': 1, '贰': 2, '叁': 3, '肆': 4, '伍': 5, '陆': 6, '柒': 7, '捌': 8, '玖': 9,
	'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '〇': 0, '两': 2}

var cnUnits = map[rune]int{'拾': 10, '十': 10, '佰': 100, '百': 100, '仟': 1000, '千': 1000}
var cnBig = map[rune]int{'万': 10000, '亿': 100000000}

// ParseCNAmount 解析「贰佰肆拾贰圆玖角伍分」。
func ParseCNAmount(text string) (float64, bool) {
	s := strings.TrimSpace(text)
	s = strings.ReplaceAll(s, "圆", "元")
	s = strings.ReplaceAll(s, "整", "")
	re := regexp.MustCompile(`^([零壹贰叁肆伍陆柒捌玖一二三四五六七八九〇两拾十佰百仟千万亿]+)元` +
		`(?:([零壹贰叁肆伍陆柒捌玖一二三四五六七八九])角)?` +
		`(?:([零壹贰叁肆伍陆柒捌玖一二三四五六七八九])分)?$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	total, ok := cnInt(m[1])
	if !ok {
		return 0, false
	}
	value := float64(total)
	if m[2] != "" {
		r := []rune(m[2])[0]
		value += float64(cnDigits[r]) / 10
	}
	if m[3] != "" {
		r := []rune(m[3])[0]
		value += float64(cnDigits[r]) / 100
	}
	return round2(value), true
}

func cnInt(s string) (int, bool) {
	total, section, number := 0, 0, 0
	for _, ch := range s {
		if d, ok := cnDigits[ch]; ok {
			number = d
		} else if u, ok := cnUnits[ch]; ok {
			if number == 0 {
				number = 1
			}
			section += number * u
			number = 0
		} else if b, ok := cnBig[ch]; ok {
			section = (section + number) * b
			total += section
			section, number = 0, 0
		} else {
			return 0, false
		}
	}
	return total + section + number, true
}
