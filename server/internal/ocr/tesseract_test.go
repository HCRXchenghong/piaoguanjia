package ocr

import "testing"

// 用一段真实的 tesseract TSV 片段验证坐标解析与行聚合。
const sampleTSV = `level	page_num	block_num	par_num	line_num	word_num	left	top	width	height	conf	text
1	1	0	0	0	0	0	0	1200	800	-1	
2	1	1	0	0	0	0	0	1200	800	-1	
4	1	1	1	0	0	40	60	300	40	-1	
5	1	1	1	1	1	40	60	80	40	96.5	发票号码：
5	1	1	1	1	2	130	60	180	40	94.2	26442000012762554386
4	1	1	1	0	0	40	140	400	40	-1	
5	1	1	1	2	1	40	140	60	40	91.0	销
5	1	1	1	2	2	105	140	60	40	88.4	售
5	1	1	1	2	3	170	140	60	40	87.1	方
4	1	1	1	0	0	40	220	400	40	-1	
5	1	1	1	3	1	40	220	240	40	93.0	价税合计
5	1	1	1	3	2	300	220	120	40	90.5	¥242.95
`

func TestParseTesseractTSV(t *testing.T) {
	res := parseTesseractTSV(sampleTSV)
	if len(res.Lines) != 3 {
		t.Fatalf("期望 3 行，实际 %d 行", len(res.Lines))
	}
	if got, want := res.Lines[0].Text, "发票号码： 26442000012762554386"; got != want {
		t.Errorf("第一行 = %q，期望 %q", got, want)
	}
	if len(res.Lines[1].Words) != 3 {
		t.Errorf("第二行应有 3 个词，实际 %d", len(res.Lines[1].Words))
	}
	// 词框应保留原始坐标，供版面重建使用
	first := res.Lines[1].Words[0]
	if first.X != 40 || first.Y != 140 || first.W != 60 || first.H != 40 {
		t.Errorf("词框坐标错误：%+v", first)
	}
	if res.Width != 1200 || res.Height != 800 {
		t.Errorf("画布尺寸 = %dx%d，期望 1200x800", res.Width, res.Height)
	}
}

func TestParseTesseractTSVSkipsLowConfidence(t *testing.T) {
	tsv := `level	page_num	block_num	par_num	line_num	word_num	left	top	width	height	conf	text
5	1	1	1	1	1	10	10	50	20	-1	
5	1	1	1	1	2	70	10	50	20	0	乱码
5	1	1	1	1	3	130	10	50	20	88	正常
`
	res := parseTesseractTSV(tsv)
	if len(res.Lines) != 1 || len(res.Lines[0].Words) != 1 {
		t.Fatalf("低置信词应被丢弃，实际 %+v", res.Lines)
	}
	if res.Lines[0].Text != "正常" {
		t.Errorf("剩余词 = %q", res.Lines[0].Text)
	}
}
