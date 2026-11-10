package ocr

import (
	"testing"
)

// 红冲行金额是负数，取整不能把 -2.65 变成 -2.64。
func TestRound2Negative(t *testing.T) {
	cases := map[float64]float64{-2.65: -2.65, -0.35: -0.35, -12.345: -12.35, 2.65: 2.65}
	for in, want := range cases {
		if got := round2(in); got != want {
			t.Errorf("round2(%v) = %v，期望 %v", in, got, want)
		}
	}
}

// 只识别到「合计」行（金额 + 税额分列）时，价税合计应回退为金额 + 税额。
func TestParseTotalFromSum(t *testing.T) {
	rows := []Row{
		{Text: "项目名称规格型号单位数量单价金额税率/征收率税额", Cells: []Cell{
			{Text: "项目名称", X: 40, W: 40, H: 10},
			{Text: "规格型号", X: 120, W: 40, H: 10},
			{Text: "单位", X: 200, W: 20, H: 10},
			{Text: "数量", X: 270, W: 20, H: 10},
			{Text: "单价", X: 340, W: 20, H: 10},
			{Text: "金额", X: 410, W: 20, H: 10},
			{Text: "税率", X: 450, W: 20, H: 10},
			{Text: "税额", X: 550, W: 20, H: 10},
		}},
		{Text: "螺丝 个 100 0.18 17.72 1% 0.18", Cells: []Cell{
			{Text: "螺丝", X: 12, W: 40, H: 10},
			{Text: "个", X: 200, W: 10, H: 10},
			{Text: "100", X: 286, W: 15, H: 10},
			{Text: "0.18", X: 349, W: 15, H: 10},
			{Text: "17.72", X: 411, W: 20, H: 10},
			{Text: "1%", X: 465, W: 10, H: 10},
			{Text: "0.18", X: 564, W: 20, H: 10},
		}},
		{Text: "合计¥17.72 ¥0.18", Cells: []Cell{
			{Text: "合计", X: 58, W: 20, H: 10},
			{Text: "¥17.72", X: 407, W: 25, H: 10},
			{Text: "¥0.18", X: 555, W: 25, H: 10},
		}},
	}
	res := Parse(rows, map[string]string{"categories": "办公用品,电子设备,其他"})
	if got := res.Fields["total"].Value; got != 17.90 {
		t.Fatalf("价税合计 = %v，期望 17.90", got)
	}
	if got := res.Fields["amount"].Value; got != 17.72 {
		t.Fatalf("金额 = %v，期望 17.72", got)
	}
}
