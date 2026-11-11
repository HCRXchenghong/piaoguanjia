package ocr

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PDF 精简解析器：目标是把电子发票的文字层与内嵌图片取出来。
// 只实现发票场景需要的子集（对象扫描、Flate 流、页面树、内容流文本、ToUnicode）。

type pdfRef struct{ Num int }

type pdfObj struct {
	Value     any
	Stream    []byte
	HasStream bool
}

// PDF 文档。
type PDF struct {
	Data    []byte
	Objs    map[int]*pdfObj
	Trailer map[string]any
}

// Segment 一段文本及其坐标。
type Segment struct {
	Page int     `json:"page"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
	Text string  `json:"text"`
}

// PDFImage PDF 内嵌图片。
type PDFImage struct {
	Kind   string // jpeg / raw
	Data   []byte
	Width  int
	Height int
	BPC    int
	CS     string
}

// PDFText 提取结果。
type PDFText struct {
	Pages     int
	Segments  []Segment
	PageTexts []string
	Text      string
}

var objHeaderRe = regexp.MustCompile(`(?m)(?:^|[\r\n\s])(\d+)\s+(\d+)\s+obj\b`)

// OpenPDF 扫描全部对象。
func OpenPDF(data []byte) (*PDF, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) && !bytes.Contains(data[:minInt(len(data), 1024)], []byte("%PDF-")) {
		return nil, errors.New("不是有效的 PDF 文件")
	}
	p := &PDF{Data: data, Objs: map[int]*pdfObj{}, Trailer: map[string]any{}}
	p.scanObjects()
	p.expandObjectStreams()
	return p, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *PDF) scanObjects() {
	locs := objHeaderRe.FindAllSubmatchIndex(p.Data, -1)
	for _, loc := range locs {
		num, _ := strconv.Atoi(string(p.Data[loc[2]:loc[3]]))
		pos := loc[1]
		parser := &objParser{data: p.Data, pos: pos}
		value, err := parser.parseValue()
		if err != nil {
			continue
		}
		obj := &pdfObj{Value: value}
		pos = skipWS(p.Data, parser.pos)
		if bytes.HasPrefix(p.Data[pos:], []byte("stream")) {
			start := pos + 6
			if bytes.HasPrefix(p.Data[start:], []byte("\r\n")) {
				start += 2
			} else if start < len(p.Data) && (p.Data[start] == '\n' || p.Data[start] == '\r') {
				start++
			}
			raw, ok := p.readStreamBytes(value, start)
			if ok {
				obj.Stream = raw
				obj.HasStream = true
			}
		}
		p.Objs[num] = obj
	}

	// trailer
	trailerRe := regexp.MustCompile(`trailer\s*<<`)
	for _, loc := range trailerRe.FindAllIndex(p.Data, -1) {
		parser := &objParser{data: p.Data, pos: loc[1] - 2}
		if v, err := parser.parseValue(); err == nil {
			if d, ok := v.(map[string]any); ok {
				for k, val := range d {
					p.Trailer[k] = val
				}
			}
		}
	}
	for _, obj := range p.Objs {
		if d, ok := obj.Value.(map[string]any); ok {
			if d["Type"] == "XRef" {
				for _, key := range []string{"Root", "Info"} {
					if _, exists := p.Trailer[key]; !exists {
						if v, ok := d[key]; ok {
							p.Trailer[key] = v
						}
					}
				}
			}
		}
	}
}

func (p *PDF) readStreamBytes(dict any, start int) ([]byte, bool) {
	d, _ := dict.(map[string]any)
	length := 0
	if d != nil {
		if v, ok := d["Length"].(int64); ok {
			length = int(v)
		} else if r, ok := d["Length"].(pdfRef); ok {
			if obj, ok := p.Objs[r.Num]; ok {
				if n, ok := obj.Value.(int64); ok {
					length = int(n)
				}
			}
		}
	}
	if length > 0 && start+length <= len(p.Data) {
		end := skipWS(p.Data, start+length)
		if bytes.HasPrefix(p.Data[end:], []byte("endstream")) {
			return p.Data[start : start+length], true
		}
	}
	idx := bytes.Index(p.Data[start:], []byte("endstream"))
	if idx < 0 {
		return nil, false
	}
	raw := p.Data[start : start+idx]
	raw = bytes.TrimSuffix(raw, []byte("\r\n"))
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	return raw, true
}

// expandObjectStreams 展开 PDF 1.5+ 的压缩对象流。
func (p *PDF) expandObjectStreams() {
	for _, obj := range p.Objs {
		d, ok := obj.Value.(map[string]any)
		if !ok || d["Type"] != "ObjStm" {
			continue
		}
		data, err := p.decodeStream(obj)
		if err != nil {
			continue
		}
		count := int(toInt64(d["N"]))
		first := int(toInt64(d["First"]))
		header := strings.Fields(string(data[:minInt(first, len(data))]))
		for i := 0; i+1 < len(header) && i/2 < count; i += 2 {
			n, err1 := strconv.Atoi(header[i])
			off, err2 := strconv.Atoi(header[i+1])
			if err1 != nil || err2 != nil {
				continue
			}
			if _, exists := p.Objs[n]; exists {
				continue
			}
			parser := &objParser{data: data, pos: first + off}
			if v, err := parser.parseValue(); err == nil {
				p.Objs[n] = &pdfObj{Value: v}
			}
		}
	}
}

func (p *PDF) decodeStream(obj *pdfObj) ([]byte, error) {
	if obj == nil || !obj.HasStream {
		return nil, errors.New("no stream")
	}
	d, _ := obj.Value.(map[string]any)
	name := "FlateDecode"
	if d != nil {
		if f, ok := d["Filter"]; ok {
			name = filterName(p.resolve(f))
		}
	}
	switch name {
	case "", "FlateDecode", "Fl":
		reader, err := zlib.NewReader(bytes.NewReader(obj.Stream))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(reader); err != nil && buf.Len() == 0 {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return obj.Stream, nil
	}
}

func filterName(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			return filterName(t[0])
		}
	}
	return ""
}

func (p *PDF) resolve(v any) any {
	for i := 0; i < 32; i++ {
		r, ok := v.(pdfRef)
		if !ok {
			return v
		}
		obj, ok := p.Objs[r.Num]
		if !ok {
			return nil
		}
		v = obj.Value
	}
	return v
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// ---------------------------------------------------------------- 页面
type pdfPage struct {
	Dict      map[string]any
	Resources map[string]any
	MediaBox  []any
}

func (p *PDF) pages() []pdfPage {
	var out []pdfPage
	root, _ := p.resolve(p.Trailer["Root"]).(map[string]any)
	if root == nil {
		for _, obj := range p.Objs {
			if d, ok := obj.Value.(map[string]any); ok && d["Type"] == "Catalog" {
				root = d
				break
			}
		}
	}
	if root != nil {
		p.walkPages(p.resolve(root["Pages"]), map[string]any{}, &out, 0)
	}
	if len(out) == 0 {
		for _, obj := range p.Objs {
			if d, ok := obj.Value.(map[string]any); ok && d["Type"] == "Page" {
				res, _ := p.resolve(d["Resources"]).(map[string]any)
				box, _ := p.resolve(d["MediaBox"]).([]any)
				out = append(out, pdfPage{Dict: d, Resources: res, MediaBox: box})
			}
		}
	}
	return out
}

func (p *PDF) walkPages(node any, inherited map[string]any, out *[]pdfPage, depth int) {
	d, ok := node.(map[string]any)
	if !ok || depth > 32 {
		return
	}
	merged := map[string]any{}
	for k, v := range inherited {
		merged[k] = v
	}
	for _, key := range []string{"Resources", "MediaBox"} {
		if v, exists := d[key]; exists {
			merged[key] = p.resolve(v)
		}
	}
	kids, hasKids := p.resolve(d["Kids"]).([]any)
	if hasKids && len(kids) > 0 && d["Type"] != "Page" {
		for _, kid := range kids {
			p.walkPages(p.resolve(kid), merged, out, depth+1)
		}
		return
	}
	res, _ := merged["Resources"].(map[string]any)
	box, _ := merged["MediaBox"].([]any)
	*out = append(*out, pdfPage{Dict: d, Resources: res, MediaBox: box})
}

// ExtractText 提取全文与坐标片段。
func (p *PDF) ExtractText() *PDFText {
	pages := p.pages()
	result := &PDFText{Pages: len(pages)}
	for i, page := range pages {
		content := p.pageContent(page)
		fonts := p.loadFonts(page.Resources)
		segs := runContent(content, fonts, i)
		height := 792.0
		if len(page.MediaBox) >= 4 {
			height = toFloat(p.resolve(page.MediaBox[3])) - toFloat(p.resolve(page.MediaBox[1]))
		}
		for j := range segs {
			segs[j].Y = math.Round((height-segs[j].Y)*10) / 10
		}
		result.Segments = append(result.Segments, segs...)
		result.PageTexts = append(result.PageTexts, assembleText(segs))
	}
	result.Text = strings.Join(result.PageTexts, "\n")
	return result
}

func (p *PDF) pageContent(page pdfPage) []byte {
	contents := page.Dict["Contents"]
	if contents == nil {
		return nil
	}
	// /Contents 可能直接是流，也可能是指向「流数组」的引用（常见于模板生成的发票）。
	// 必须先 resolve，否则会把数组对象当成流去解码，整个页面文字都会丢失。
	var refs []any
	if arr, ok := p.resolve(contents).([]any); ok {
		refs = arr
	} else if r, ok := contents.(pdfRef); ok {
		refs = []any{r}
	} else {
		refs = []any{contents}
	}
	var chunks [][]byte
	for _, item := range refs {
		if r, ok := item.(pdfRef); ok {
			if obj, ok := p.Objs[r.Num]; ok {
				if data, err := p.decodeStream(obj); err == nil {
					chunks = append(chunks, data)
				}
			}
		}
	}
	return bytes.Join(chunks, []byte("\n"))
}

// ExtractImages 提取内嵌位图（扫描件 OCR 用）。
func (p *PDF) ExtractImages() []PDFImage {
	var out []PDFImage
	for _, obj := range p.Objs {
		d, ok := obj.Value.(map[string]any)
		if !ok || d["Subtype"] != "Image" || !obj.HasStream {
			continue
		}
		width := int(toInt64(p.resolve(d["Width"])))
		height := int(toInt64(p.resolve(d["Height"])))
		filter := filterName(p.resolve(d["Filter"]))
		if filter == "DCTDecode" || filter == "DCT" {
			out = append(out, PDFImage{Kind: "jpeg", Data: obj.Stream, Width: width, Height: height})
			continue
		}
		if filter == "JPXDecode" {
			continue
		}
		data, err := p.decodeStream(obj)
		if err != nil {
			continue
		}
		out = append(out, PDFImage{
			Kind: "raw", Data: data, Width: width, Height: height,
			BPC: int(toInt64(p.resolve(d["BitsPerComponent"]))),
			CS:  colorSpaceName(p.resolve(d["ColorSpace"])),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Width*out[i].Height > out[j].Width*out[j].Height
	})
	return out
}

func colorSpaceName(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return "DeviceRGB"
}

// ---------------------------------------------------------------- 字体
type fontDecoder struct {
	twoByte     bool
	cmap        map[string]string
	ucs2        bool
	differences map[byte]string
}

func (p *PDF) loadFonts(resources map[string]any) map[string]*fontDecoder {
	fonts := map[string]*fontDecoder{}
	dict, _ := p.resolve(resources["Font"]).(map[string]any)
	for name, ref := range dict {
		font, ok := p.resolve(ref).(map[string]any)
		if !ok {
			continue
		}
		fd := &fontDecoder{cmap: map[string]string{}, differences: map[byte]string{}}
		subtype, _ := p.resolve(font["Subtype"]).(string)
		fd.twoByte = subtype == "Type0"
		if tu, ok := font["ToUnicode"]; ok {
			if r, ok := tu.(pdfRef); ok {
				if obj, ok := p.Objs[r.Num]; ok {
					if data, err := p.decodeStream(obj); err == nil {
						fd.cmap = parseToUnicode(data)
					}
				}
			}
		}
		enc, _ := p.resolve(font["Encoding"]).(string)
		if fd.twoByte && len(fd.cmap) == 0 && (strings.Contains(enc, "UCS2") || strings.Contains(enc, "UTF16")) {
			fd.ucs2 = true
		}
		if diff, ok := p.resolve(font["Encoding"]).(map[string]any); ok {
			if arr, ok := p.resolve(diff["Differences"]).([]any); ok {
				code := 0
				for _, item := range arr {
					switch t := p.resolve(item).(type) {
					case int64:
						code = int(t)
					case string:
						if code >= 0 && code < 256 {
							fd.differences[byte(code)] = t
						}
						code++
					}
				}
			}
		}
		fonts[name] = fd
	}
	return fonts
}

var (
	bfcharRe  = regexp.MustCompile(`(?s)beginbfchar(.*?)endbfchar`)
	bfrangeRe = regexp.MustCompile(`(?s)beginbfrange(.*?)endbfrange`)
	hexPairRe = regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
)

func parseToUnicode(data []byte) map[string]string {
	out := map[string]string{}
	text := string(data)
	for _, block := range bfcharRe.FindAllStringSubmatch(text, -1) {
		for _, pair := range hexPairRe.FindAllStringSubmatch(block[1], -1) {
			src, err1 := hex.DecodeString(padHex(pair[1]))
			if err1 != nil {
				continue
			}
			out[string(src)] = utf16HexToString(pair[2])
		}
	}
	rangeRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*(?:<([0-9A-Fa-f]+)>|\[(.*?)\])`)
	for _, block := range bfrangeRe.FindAllStringSubmatch(text, -1) {
		for _, m := range rangeRe.FindAllStringSubmatch(block[1], -1) {
			lo, err1 := strconv.ParseInt(m[1], 16, 64)
			hi, err2 := strconv.ParseInt(m[2], 16, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			width := len(m[1]) / 2
			if width < 1 {
				width = 1
			}
			if m[3] != "" {
				base, err := strconv.ParseInt(m[3], 16, 64)
				if err != nil {
					continue
				}
				for code := lo; code <= hi && code-lo < 4096; code++ {
					out[string(intToBytes(int(code), width))] = string(rune(base + code - lo))
				}
			} else {
				items := regexp.MustCompile(`<([0-9A-Fa-f]+)>`).FindAllStringSubmatch(m[4], -1)
				for i, item := range items {
					if lo+int64(i) > hi {
						break
					}
					out[string(intToBytes(int(lo)+i, width))] = utf16HexToString(item[1])
				}
			}
		}
	}
	return out
}

func padHex(s string) string {
	if len(s)%2 == 1 {
		return s + "0"
	}
	return s
}

func intToBytes(v, width int) []byte {
	out := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		out[i] = byte(v & 0xFF)
		v >>= 8
	}
	return out
}

func utf16HexToString(hexStr string) string {
	raw, err := hex.DecodeString(padHex(hexStr))
	if err != nil {
		return ""
	}
	if len(raw) >= 2 && len(raw)%2 == 0 {
		runes := make([]rune, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			runes = append(runes, rune(uint16(raw[i])<<8|uint16(raw[i+1])))
		}
		return string(runes)
	}
	return string(raw)
}

var glyphUnicode = map[string]rune{
	"space": ' ', "period": '.', "comma": ',', "colon": ':', "semicolon": ';',
	"parenleft": '(', "parenright": ')', "hyphen": '-', "slash": '/', "percent": '%',
	"yen": '¥', "dollar": '$', "asterisk": '*', "plus": '+', "equal": '=',
	"question": '?', "exclam": '!', "at": '@', "ampersand": '&',
}

func (f *fontDecoder) decode(data []byte) string {
	var sb strings.Builder
	if f.twoByte {
		for i := 0; i+1 < len(data); i += 2 {
			code := data[i : i+2]
			if s, ok := f.cmap[string(code)]; ok {
				sb.WriteString(s)
			} else if f.ucs2 {
				sb.WriteRune(rune(uint16(code[0])<<8 | uint16(code[1])))
			}
		}
		return sb.String()
	}
	for _, b := range data {
		if s, ok := f.cmap[string([]byte{b})]; ok {
			sb.WriteString(s)
			continue
		}
		if name, ok := f.differences[b]; ok {
			if r, ok := glyphNameToRune(name); ok {
				sb.WriteRune(r)
				continue
			}
		}
		if b >= 32 && b < 127 {
			sb.WriteByte(b)
		}
	}
	return sb.String()
}

var uniRe = regexp.MustCompile(`^uni([0-9A-Fa-f]{4,6})$`)

func glyphNameToRune(name string) (rune, bool) {
	if r, ok := glyphUnicode[name]; ok {
		return r, true
	}
	if m := uniRe.FindStringSubmatch(name); m != nil {
		if v, err := strconv.ParseInt(m[1], 16, 32); err == nil {
			return rune(v), true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------- 内容流
type contentToken struct {
	isOp bool
	op   []byte
	val  any
}

func runContent(content []byte, fonts map[string]*fontDecoder, page int) []Segment {
	if len(content) == 0 {
		return nil
	}
	var segs []Segment
	var stack []any
	var font *fontDecoder
	fontSize := 12.0
	leading := 0.0
	tm := [6]float64{1, 0, 0, 1, 0, 0}
	tlm := tm
	ctm := [6]float64{1, 0, 0, 1, 0, 0}
	var ctmStack [][6]float64

	emit := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		// 最终位置 = 文本矩阵 × 当前图形状态矩阵
		x := tm[4]*ctm[0] + tm[5]*ctm[2] + ctm[4]
		y := tm[4]*ctm[1] + tm[5]*ctm[3] + ctm[5]
		scaleY := tm[3] * ctm[3]
		if scaleY == 0 {
			scaleY = tm[0] * ctm[0]
		}
		size := math.Abs(fontSize * scaleY)
		if size < 0.5 {
			size = 12
		}
		segs = append(segs, Segment{Page: page, X: round1(x), Y: round1(y), Size: round1(size), Text: text})
	}

	for _, tok := range tokenizeContent(content) {
		if tok.isOp {
			switch string(tok.op) {
			case "q":
				ctmStack = append(ctmStack, ctm)
			case "Q":
				if n := len(ctmStack); n > 0 {
					ctm = ctmStack[n-1]
					ctmStack = ctmStack[:n-1]
				}
			case "cm":
				if len(stack) >= 6 {
					m := [6]float64{
						numF(stack[len(stack)-6]), numF(stack[len(stack)-5]),
						numF(stack[len(stack)-4]), numF(stack[len(stack)-3]),
						numF(stack[len(stack)-2]), numF(stack[len(stack)-1]),
					}
					ctm = matMul(m, ctm)
				}
			case "BT":
				tm = [6]float64{1, 0, 0, 1, 0, 0}
				tlm = tm
			case "Tf":
				if len(stack) >= 2 {
					if name, ok := stack[len(stack)-2].(string); ok {
						font = fonts[name]
					}
					if sz := numF(stack[len(stack)-1]); sz > 0 {
						fontSize = sz
					}
				}
			case "TL":
				if len(stack) > 0 {
					leading = numF(stack[len(stack)-1])
				}
			case "Td", "TD":
				if len(stack) >= 2 {
					tx := numF(stack[len(stack)-2])
					ty := numF(stack[len(stack)-1])
					if string(tok.op) == "TD" {
						leading = -ty
					}
					tlm[4] += tx*tlm[0] + ty*tlm[2]
					tlm[5] += tx*tlm[1] + ty*tlm[3]
					tm = tlm
				}
			case "Tm":
				if len(stack) >= 6 {
					for i := 0; i < 6; i++ {
						tm[i] = numF(stack[len(stack)-6+i])
					}
					tlm = tm
				}
			case "T*":
				tlm[4] += -leading * tlm[2]
				tlm[5] += -leading * tlm[3]
				tm = tlm
			case "Tj", "'":
				if len(stack) > 0 {
					if raw, ok := stack[len(stack)-1].([]byte); ok {
						if string(tok.op) == "'" {
							tlm[4] += -leading * tlm[2]
							tlm[5] += -leading * tlm[3]
							tm = tlm
						}
						emit(decodeText(font, raw))
					}
				}
			case "\"":
				if len(stack) >= 3 {
					if raw, ok := stack[len(stack)-1].([]byte); ok {
						emit(decodeText(font, raw))
					}
				}
			case "TJ":
				if len(stack) > 0 {
					if arr, ok := stack[len(stack)-1].([]any); ok {
						var sb strings.Builder
						for _, item := range arr {
							switch t := item.(type) {
							case []byte:
								sb.WriteString(decodeText(font, t))
							default:
								if numF(item) < -180 {
									sb.WriteByte(' ')
								}
							}
						}
						emit(sb.String())
					}
				}
			}
			stack = stack[:0]
			continue
		}
		stack = append(stack, tok.val)
	}
	return segs
}

// numF 把 PDF 数值统一转成 float64（整数与浮点都要支持）。
func numF(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	}
	return 0
}

// matMul 矩阵右乘：返回 m × n（PDF 的 [a b c d e f] 表示）。
func matMul(m, n [6]float64) [6]float64 {
	return [6]float64{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func decodeText(font *fontDecoder, raw []byte) string {
	if font == nil {
		return string(raw)
	}
	return font.decode(raw)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func tokenizeContent(data []byte) []contentToken {
	var out []contentToken
	pos := 0
	for pos < len(data) {
		pos = skipWS(data, pos)
		if pos >= len(data) {
			break
		}
		c := data[pos]
		switch {
		case c == '(':
			p := &objParser{data: data, pos: pos}
			val := p.parseLiteralString()
			pos = p.pos
			out = append(out, contentToken{val: val})
		case c == '<' && pos+1 < len(data) && data[pos+1] == '<':
			p := &objParser{data: data, pos: pos}
			if v, err := p.parseValue(); err == nil {
				out = append(out, contentToken{val: v})
			}
			pos = p.pos
		case c == '<':
			p := &objParser{data: data, pos: pos}
			val := p.parseHexString()
			pos = p.pos
			out = append(out, contentToken{val: val})
		case c == '[':
			p := &objParser{data: data, pos: pos}
			if v, err := p.parseValue(); err == nil {
				out = append(out, contentToken{val: v})
			}
			pos = p.pos
		case c == '/':
			p := &objParser{data: data, pos: pos}
			val := p.parseName()
			pos = p.pos
			out = append(out, contentToken{val: val})
		case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
			p := &objParser{data: data, pos: pos}
			if v, err := p.parseValue(); err == nil {
				out = append(out, contentToken{val: v})
			}
			pos = p.pos
			if pos == 0 {
				pos++
			}
		case c == '%':
			nl := bytes.IndexByte(data[pos:], '\n')
			if nl < 0 {
				pos = len(data)
			} else {
				pos += nl + 1
			}
		default:
			start := pos
			for pos < len(data) && !isDelim(data[pos]) {
				pos++
			}
			if pos == start {
				pos++
				continue
			}
			word := data[start:pos]
			if string(word) == "BI" {
				if idx := bytes.Index(data[pos:], []byte("EI")); idx >= 0 {
					pos += idx + 2
				} else {
					pos = len(data)
				}
				continue
			}
			out = append(out, contentToken{isOp: true, op: word})
		}
	}
	return out
}

func assembleText(segs []Segment) string {
	if len(segs) == 0 {
		return ""
	}
	type row struct {
		y    float64
		segs []Segment
	}
	var rows []row
	for _, s := range segs {
		placed := false
		for i := len(rows) - 1; i >= 0 && i >= len(rows)-8; i-- {
			if math.Abs(rows[i].y-s.Y) <= 3 {
				rows[i].segs = append(rows[i].segs, s)
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, row{y: s.Y, segs: []Segment{s}})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].y < rows[j].y })
	var lines []string
	for _, r := range rows {
		sort.SliceStable(r.segs, func(i, j int) bool { return r.segs[i].X < r.segs[j].X })
		var sb strings.Builder
		for _, s := range r.segs {
			sb.WriteString(s.Text)
		}
		t := strings.TrimSpace(sb.String())
		if t != "" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- 底层解析
func skipWS(data []byte, pos int) int {
	for pos < len(data) {
		c := data[pos]
		if c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' ' {
			pos++
			continue
		}
		if c == '%' {
			nl := bytes.IndexByte(data[pos:], '\n')
			if nl < 0 {
				return len(data)
			}
			pos += nl + 1
			continue
		}
		break
	}
	return pos
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

type objParser struct {
	data []byte
	pos  int
}

func (p *objParser) parseValue() (any, error) {
	p.pos = skipWS(p.data, p.pos)
	if p.pos >= len(p.data) {
		return nil, errors.New("eof")
	}
	switch c := p.data[p.pos]; {
	case c == '<' && p.pos+1 < len(p.data) && p.data[p.pos+1] == '<':
		return p.parseDict()
	case c == '<':
		return p.parseHexString(), nil
	case c == '[':
		return p.parseArray()
	case c == '/':
		return p.parseName(), nil
	case c == '(':
		return p.parseLiteralString(), nil
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.parseNumberOrRef()
	default:
		start := p.pos
		for p.pos < len(p.data) && !isDelim(p.data[p.pos]) {
			p.pos++
		}
		word := string(p.data[start:p.pos])
		switch word {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		}
		if p.pos == start {
			p.pos++
		}
		return word, nil
	}
}

func (p *objParser) parseDict() (any, error) {
	p.pos += 2
	dict := map[string]any{}
	for {
		p.pos = skipWS(p.data, p.pos)
		if p.pos >= len(p.data) {
			break
		}
		if bytes.HasPrefix(p.data[p.pos:], []byte(">>")) {
			p.pos += 2
			break
		}
		key, err := p.parseValue()
		if err != nil {
			break
		}
		keyName, ok := key.(string)
		if !ok {
			continue
		}
		val, err := p.parseValue()
		if err != nil {
			break
		}
		dict[keyName] = val
	}
	return dict, nil
}

func (p *objParser) parseArray() (any, error) {
	p.pos++
	var arr []any
	for {
		p.pos = skipWS(p.data, p.pos)
		if p.pos >= len(p.data) || p.data[p.pos] == ']' {
			p.pos++
			break
		}
		before := p.pos
		v, err := p.parseValue()
		if err != nil {
			break
		}
		arr = append(arr, v)
		if p.pos == before {
			p.pos++
		}
	}
	return arr, nil
}

func (p *objParser) parseName() string {
	p.pos++
	start := p.pos
	for p.pos < len(p.data) && !isDelim(p.data[p.pos]) {
		p.pos++
	}
	raw := p.data[start:p.pos]
	var sb strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && i+2 < len(raw) {
			if v, err := strconv.ParseUint(string(raw[i+1:i+3]), 16, 8); err == nil {
				sb.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		sb.WriteByte(raw[i])
	}
	return sb.String()
}

func (p *objParser) parseHexString() []byte {
	p.pos++
	end := bytes.IndexByte(p.data[p.pos:], '>')
	if end < 0 {
		end = len(p.data) - p.pos
	}
	raw := p.data[p.pos : p.pos+end]
	p.pos += end + 1
	clean := make([]byte, 0, len(raw))
	for _, c := range raw {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			clean = append(clean, c)
		}
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	out := make([]byte, hex.DecodedLen(len(clean)))
	if _, err := hex.Decode(out, clean); err != nil {
		return nil
	}
	return out
}

func (p *objParser) parseLiteralString() []byte {
	p.pos++
	var out []byte
	depth := 1
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		switch c {
		case '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return out
			}
			e := p.data[p.pos]
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, e)
			case '\r', '\n':
				// 续行
			default:
				if e >= '0' && e <= '7' {
					oct := []byte{e}
					for len(oct) < 3 && p.pos+1 < len(p.data) && p.data[p.pos+1] >= '0' && p.data[p.pos+1] <= '7' {
						p.pos++
						oct = append(oct, p.data[p.pos])
					}
					if v, err := strconv.ParseInt(string(oct), 8, 32); err == nil {
						out = append(out, byte(v))
					}
				} else {
					out = append(out, e)
				}
			}
			p.pos++
		case '(':
			depth++
			out = append(out, c)
			p.pos++
		case ')':
			depth--
			p.pos++
			if depth == 0 {
				return out
			}
			out = append(out, c)
		default:
			out = append(out, c)
			p.pos++
		}
	}
	return out
}

func (p *objParser) parseNumberOrRef() (any, error) {
	start := p.pos
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.' {
			p.pos++
			continue
		}
		break
	}
	text := string(p.data[start:p.pos])
	if strings.Contains(text, ".") {
		v, err := strconv.ParseFloat(text, 64)
		return v, err
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return float64(0), err
	}
	if v >= 0 {
		save := p.pos
		pos := skipWS(p.data, p.pos)
		// 判断是否为 "num gen R"
		j := pos
		for j < len(p.data) && p.data[j] >= '0' && p.data[j] <= '9' {
			j++
		}
		if j > pos {
			k := skipWS(p.data, j)
			if k < len(p.data) && p.data[k] == 'R' && (k+1 >= len(p.data) || isDelim(p.data[k+1])) {
				p.pos = k + 1
				return pdfRef{Num: int(v)}, nil
			}
		}
		p.pos = save
	}
	return v, nil
}

// ExtractTextBytes 便捷入口。
func ExtractTextBytes(data []byte) (*PDFText, error) {
	doc, err := OpenPDF(data)
	if err != nil {
		return nil, err
	}
	return doc.ExtractText(), nil
}

func (t *PDFText) String() string {
	return fmt.Sprintf("%d pages, %d segments", t.Pages, len(t.Segments))
}
