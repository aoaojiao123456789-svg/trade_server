package indicator

import (
	"math"
	"testing"
)

func TestPineEMA(t *testing.T) {
	// N=3, alpha=0.5：前 3 个的 SMA=2 作为初始值，之后 0.5*x + 0.5*prev
	assertSeries(t, "EMA3", PineEMA([]float64{1, 2, 3, 6, 4}, 3), []float64{nan, nan, 2, 4, 4})
	// 开头的 NaN 被跳过，从第一个完整窗口开始
	assertSeries(t, "EMA2", PineEMA([]float64{nan, nan, 2, 4, 8}, 2), []float64{nan, nan, nan, 3, 6.333333333333333})
}

func TestRMA(t *testing.T) {
	// N=2, alpha=0.5：SMA(2,4)=3，之后 0.5*6+0.5*3=4.5
	assertSeries(t, "RMA2", RMA([]float64{2, 4, 6}, 2), []float64{nan, 3, 4.5})
}

func TestStdDev(t *testing.T) {
	// 总体标准差：2,4,4,4,5,5,7,9 → 2
	got := StdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9}, 8)
	if math.Abs(got[7]-2) > eps || IsValid(got[6]) {
		t.Fatalf("stdev = %v", got)
	}
}

func TestATR(t *testing.T) {
	bars := []OHLCV{
		{High: 10, Low: 8, Close: 9},   // TR=2（首根为高-低）
		{High: 12, Low: 10, Close: 11}, // TR=max(2, |12-9|, |10-9|)=3
		{High: 11, Low: 7, Close: 8},   // TR=max(4, 0, 4)=4
	}
	// N=2：SMA(2,3)=2.5，之后 (4 + 2.5)/2 = 3.25
	assertSeries(t, "ATR2", ATR(bars, 2), []float64{nan, 2.5, 3.25})
}

func TestRSI(t *testing.T) {
	// 涨跌：+1 +1 -1 +2；N=2：up SMA(1,1)=1，down SMA(0,0)=0 → 100
	// 第3个变化：up=0.5*0+0.5*1=0.5，down=0.5*1+0=0.5 → 50
	// 第4个变化：up=0.5*2+0.5*0.5=1.25，down=0.25 → 100-100/6
	got := RSI([]float64{1, 2, 3, 2, 4}, 2)
	assertSeries(t, "RSI2", got, []float64{nan, nan, 100, 50, 100 - 100.0/6})

	// 一路下跌 RSI=0
	if v := RSI([]float64{5, 4, 3, 2}, 2)[3]; v != 0 {
		t.Errorf("一路下跌 RSI = %v", v)
	}
}

func TestPineMACD(t *testing.T) {
	// fast=1 → EMA 即原值；slow=2：SMA 初始，alpha=2/3；signal=1 → 信号线即 MACD
	values := []float64{1, 3, 6}
	r := PineMACD(values, 1, 2, 1)
	// EMA2: [nan, 2, 2/3*6 + 1/3*2 = 4.666…]；MACD = [nan, 1, 1.333…]
	assertSeries(t, "MACD", r.MACD, []float64{nan, 1, 6 - 14.0/3})
	assertSeries(t, "Signal", r.Signal, []float64{nan, 1, 6 - 14.0/3})
	assertSeries(t, "Hist", r.Hist, []float64{nan, 0, 0})

	// signal=2：信号线以前 2 个有效 MACD 的平均为初始值
	r = PineMACD([]float64{1, 3, 6, 10}, 1, 2, 2)
	// EMA2 = [nan, 2, 14/3, 2/3*10 + 1/3*14/3 = 74/9]；MACD = [nan, 1, 4/3, 16/9]
	// Signal = [nan, nan, (1+4/3)/2 = 7/6, 2/3*16/9 + 1/3*7/6 = 85/54]
	assertSeries(t, "Signal2", r.Signal, []float64{nan, nan, 7.0 / 6, 85.0 / 54})
	assertSeries(t, "Hist2", r.Hist, []float64{nan, nan, 4.0/3 - 7.0/6, 16.0/9 - 85.0/54})
}

func TestLowestAndCrossOver(t *testing.T) {
	assertSeries(t, "Lowest2", Lowest([]float64{5, 3, 4, 1}, 2), []float64{nan, 3, 3, 1})

	a := []float64{-1, 0, 1, 2, -1}
	zero := []float64{0, 0, 0, 0, 0}
	want := []bool{false, false, true, false, false} // 上一根 <= 0 且当前 > 0
	for i, w := range want {
		if got := CrossOver(a, zero, i); got != w {
			t.Errorf("CrossOver[%d] = %v, want %v", i, got, w)
		}
	}
	if CrossOver([]float64{nan, 1}, []float64{0, 0}, 1) {
		t.Error("含 NaN 时不应判定为上穿")
	}
}

func TestCrossUnder(t *testing.T) {
	a := []float64{1, 0, -1, -2, 1}
	zero := []float64{0, 0, 0, 0, 0}
	want := []bool{false, false, true, false, false} // 上一根 >= 0 且当前 < 0
	for i, w := range want {
		if got := CrossUnder(a, zero, i); got != w {
			t.Errorf("CrossUnder[%d] = %v, want %v", i, got, w)
		}
	}
}
