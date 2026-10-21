package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

func TestKlineIndicatorRespJSON(t *testing.T) {
	klines := make([]*Kline, 6)
	for i := range klines {
		c := float64(100 + i)
		klines[i] = &Kline{StartTime: int64(i) * dayMillis, Symbol: "BTCUSDT", HighPrice: c + 1, LowPrice: c - 1, ClosePrice: c}
	}
	ind := indicator.Calc(KlinesToBars(klines))
	resps := toKlineIndicatorResps(klines, ind)

	b, err := json.Marshal(resps)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	last := rows[len(rows)-1]
	if last["symbol"] != "BTCUSDT" || last["close_price"] != 105.0 {
		t.Errorf("K线字段未展平: %v", last)
	}
	// 6 根K线：MA5 已成形，MA10 未成形应为 null
	if last["ma5"] != 103.0 {
		t.Errorf("ma5 = %v, want 103", last["ma5"])
	}
	if v, ok := last["ma10"]; !ok || v != nil {
		t.Errorf("ma10 = %v, want null", v)
	}
	if !strings.Contains(string(b), `"k":`) || last["k"] == nil || last["dif"] == nil {
		t.Errorf("KDJ/MACD 缺失: %v", last)
	}
}

func TestSliceIndicators(t *testing.T) {
	klines := make([]*Kline, 10)
	for i := range klines {
		klines[i] = &Kline{ClosePrice: float64(i + 1), HighPrice: float64(i + 2), LowPrice: float64(i)}
	}
	ind := indicator.Calc(KlinesToBars(klines))
	s := sliceIndicators(ind, 4)
	if len(s.MA5) != 6 || len(s.KDJ.J) != 6 || len(s.BOLL.Lower) != 6 || len(s.MACD.DEA) != 6 {
		t.Fatal("裁剪后长度错误")
	}
	if s.MA5[0] != ind.MA5[4] || s.MACD.DIF[0] != ind.MACD.DIF[4] {
		t.Fatal("裁剪后下标错位")
	}
}
