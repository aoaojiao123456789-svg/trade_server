package indicator

import (
	"math"
	"testing"
)

const eps = 1e-9

func assertSeries(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 长度不一致: got %d want %d", name, len(got), len(want))
	}
	for i := range want {
		if math.IsNaN(want[i]) {
			if !math.IsNaN(got[i]) {
				t.Errorf("%s[%d] = %v, want NaN", name, i, got[i])
			}
			continue
		}
		if math.Abs(got[i]-want[i]) > eps {
			t.Errorf("%s[%d] = %v, want %v", name, i, got[i], want[i])
		}
	}
}

var nan = math.NaN()

func TestMA(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5, 6, 7}
	assertSeries(t, "MA3", MA(closes, 3), []float64{nan, nan, 2, 3, 4, 5, 6})
	assertSeries(t, "MA5", MA(closes, 5), []float64{nan, nan, nan, nan, 3, 4, 5})
	assertSeries(t, "MA10", MA(closes, 10), []float64{nan, nan, nan, nan, nan, nan, nan})
}

func TestCalcMAGroup(t *testing.T) {
	closes := make([]float64, 60)
	for i := range closes {
		closes[i] = float64(i + 1)
	}
	g := CalcMAGroup(closes)
	// 1..60 的最后 N 个数均值 = 60 - (N-1)/2
	for _, c := range []struct {
		name   string
		series []float64
		period int
	}{{"MA5", g.MA5, 5}, {"MA10", g.MA10, 10}, {"MA30", g.MA30, 30}, {"MA60", g.MA60, 60}} {
		want := 60 - float64(c.period-1)/2
		if got := c.series[59]; math.Abs(got-want) > eps {
			t.Errorf("%s = %v, want %v", c.name, got, want)
		}
		if IsValid(c.series[c.period-2]) || !IsValid(c.series[c.period-1]) {
			t.Errorf("%s 有效起点错误", c.name)
		}
	}
}

func TestEMA(t *testing.T) {
	// α = 2/(3+1) = 0.5
	assertSeries(t, "EMA3", EMA([]float64{1, 2, 3, 4}, 3), []float64{1, 1.5, 2.25, 3.125})
}

func TestMACD(t *testing.T) {
	// 价格不变时 DIF、DEA、MACD 全为 0
	flat := []float64{10, 10, 10, 10, 10}
	r := MACD(flat)
	zero := []float64{0, 0, 0, 0, 0}
	assertSeries(t, "DIF", r.DIF, zero)
	assertSeries(t, "DEA", r.DEA, zero)
	assertSeries(t, "MACD", r.MACD, zero)

	// 小参数手算：fast=1(α=1) slow=3(α=0.5) signal=1(α=1)
	// EMA1 = 原值；EMA3 = 1, 1.5, 2.25；DIF = 0, 0.5, 0.75；DEA = DIF；MACD = 0
	r = MACDWithParams([]float64{1, 2, 3}, 1, 3, 1)
	assertSeries(t, "DIF", r.DIF, []float64{0, 0.5, 0.75})
	assertSeries(t, "DEA", r.DEA, []float64{0, 0.5, 0.75})
	assertSeries(t, "MACD", r.MACD, []float64{0, 0, 0})

	// signal=3(α=0.5)：DEA = 0, 0.25, 0.5；MACD = 2*(DIF-DEA) = 0, 0.5, 0.5
	r = MACDWithParams([]float64{1, 2, 3}, 1, 3, 3)
	assertSeries(t, "DEA", r.DEA, []float64{0, 0.25, 0.5})
	assertSeries(t, "MACD", r.MACD, []float64{0, 0.5, 0.5})
}

func TestBOLL(t *testing.T) {
	// 1..5：MID=3，总体方差=(4+1+0+1+4)/5=2，STD=√2
	r := BOLLWithParams([]float64{1, 2, 3, 4, 5}, 5, 2)
	s := 2 * math.Sqrt2
	assertSeries(t, "MID", r.Mid, []float64{nan, nan, nan, nan, 3})
	assertSeries(t, "UPPER", r.Upper, []float64{nan, nan, nan, nan, 3 + s})
	assertSeries(t, "LOWER", r.Lower, []float64{nan, nan, nan, nan, 3 - s})

	// 价格不变时上中下轨重合
	r = BOLLWithParams([]float64{7, 7, 7}, 2, 2)
	assertSeries(t, "UPPER", r.Upper, []float64{nan, 7, 7})
	assertSeries(t, "LOWER", r.Lower, []float64{nan, 7, 7})
}

func TestKDJ(t *testing.T) {
	bars := []Bar{
		{High: 10, Low: 0, Close: 5},   // RSV=50  K=50      D=50
		{High: 20, Low: 10, Close: 20}, // RSV=100 K=200/3   D=500/9
		{High: 20, Low: 10, Close: 10}, // RSV=50  K=550/9   D=1550/27
	}
	r := KDJ(bars)
	k := []float64{50, 200.0 / 3, 550.0 / 9}
	d := []float64{50, 500.0 / 9, 1550.0 / 27}
	j := make([]float64, 3)
	for i := range j {
		j[i] = 3*k[i] - 2*d[i]
	}
	assertSeries(t, "K", r.K, k)
	assertSeries(t, "D", r.D, d)
	assertSeries(t, "J", r.J, j)

	// N=2 时第三根只看后两根：HHV=20 LLV=10，RSV=(10-10)/10*100=0
	r = KDJWithParams(bars, 2, 3, 3)
	if want := (0 + 2*200.0/3) / 3; math.Abs(r.K[2]-want) > eps {
		t.Errorf("K[2] = %v, want %v", r.K[2], want)
	}

	// 最高价等于最低价时沿用上一个 RSV，不产生 NaN
	flat := KDJ([]Bar{{High: 5, Low: 5, Close: 5}, {High: 5, Low: 5, Close: 5}})
	assertSeries(t, "K", flat.K, []float64{50, 50})
}

func TestCalc(t *testing.T) {
	bars := make([]Bar, 100)
	for i := range bars {
		c := 100 + 10*math.Sin(float64(i)/5)
		bars[i] = Bar{High: c + 1, Low: c - 1, Close: c}
	}
	ind := Calc(bars)
	for name, s := range map[string][]float64{
		"MA5": ind.MA5, "MA60": ind.MA60, "DIF": ind.MACD.DIF, "UPPER": ind.BOLL.Upper, "K": ind.KDJ.K,
	} {
		if len(s) != len(bars) {
			t.Errorf("%s 长度 %d, want %d", name, len(s), len(bars))
		}
		if !IsValid(s[len(s)-1]) {
			t.Errorf("%s 最后一个值无效", name)
		}
	}
	for i := range bars {
		if IsValid(ind.BOLL.Mid[i]) && !(ind.BOLL.Lower[i] <= ind.BOLL.Mid[i] && ind.BOLL.Mid[i] <= ind.BOLL.Upper[i]) {
			t.Fatalf("BOLL[%d] 上中下轨顺序错误", i)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	ind := Calc(nil)
	if len(ind.MA5) != 0 || len(ind.MACD.DIF) != 0 || len(ind.BOLL.Mid) != 0 || len(ind.KDJ.K) != 0 {
		t.Fatal("空输入应返回空序列")
	}
}
