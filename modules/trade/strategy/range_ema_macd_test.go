package strategy

import (
	"math"
	"testing"
)

// rangeFlat 构造 n 根价格为 100 的K线，及无信号的指标：EMA5 > EMA20、MACD 柱为负
func rangeFlat(n int) ([]Candle, RangeEMAMACDIndicators) {
	c := make([]Candle, n)
	for i := range c {
		c[i] = Candle{Time: int64(i), Open: 100, High: 100, Low: 100, Close: 100, Volume: 1}
	}
	return c, RangeEMAMACDIndicators{
		EMAShort: fill(n, 101), EMALong: fill(n, 100),
		MACD: fill(n, 0), Signal: fill(n, 1), Delta: fill(n, -1),
		BBBasis: fill(n, 100), BBUpper: fill(n, 110), BBLower: fill(n, 90),
	}
}

func runRange(c []Candle, ind RangeEMAMACDIndicators) Result {
	return Simulate(&rangeEMAMACDEval{c: c, ind: ind}, c, 0, 1000, 0)
}

func TestRangeBuyEMACrossAndSellEMACross(t *testing.T) {
	c, ind := rangeFlat(6)
	ind.EMAShort = []float64{99, 101, 101, 99, 99, 99} // 第1根 EMA5 上穿 EMA20，第3根下穿
	ind.Delta = fill(6, 1)                             // MACD 柱全程为正：卖出不应等待柱翻负
	c[2].Open, c[4].Open = 101, 105

	res := runRange(c, ind)
	if len(res.Trades) != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}
	buy, sell := res.Trades[0], res.Trades[1]
	if buy.ReasonCode != ReasonRangeEMACrossUp || buy.Time != 2 || buy.Price != 101 {
		t.Errorf("buy = %+v", buy)
	}
	if sell.ReasonCode != ReasonRangeEMACrossDwn || sell.Time != 4 || sell.Price != 105 {
		t.Errorf("sell = %+v", sell)
	}
	if buy.Indicators["ema5"] != 101 || buy.Indicators["delta"] != 1 || buy.Indicators["bb_upper"] != 110 {
		t.Errorf("snapshot = %v", buy.Indicators)
	}
}

func TestRangeEntryConditions(t *testing.T) {
	// MACD 柱上穿 0 且 EMA5 > EMA20，但没有 EMA 金叉：不买
	c, ind := rangeFlat(4)
	ind.Delta[1] = 1
	if res := runRange(c, ind); len(res.Trades) != 0 {
		t.Errorf("没有 EMA 金叉时不应买入: %+v", res.Trades)
	}

	// EMA 金叉但 MACD 柱为负：不买
	c, ind = rangeFlat(4)
	ind.EMAShort = []float64{99, 101, 101, 101}
	if res := runRange(c, ind); len(res.Trades) != 0 {
		t.Errorf("MACD 柱为负时不应买入: %+v", res.Trades)
	}
}

func TestRangeExitIgnoresMACD(t *testing.T) {
	// 持仓后 MACD 死叉且柱为负，但 EMA 没有死叉：不卖
	c, ind := rangeFlat(5)
	ind.EMAShort = []float64{99, 101, 101, 101, 101}
	ind.Delta = []float64{1, 1, 1, -1, -1}
	ind.MACD = []float64{2, 2, 2, 0, 0}
	ind.Signal = fill(5, 1)
	if res := runRange(c, ind); len(res.Trades) != 1 || !res.Summary.Holding {
		t.Errorf("EMA 未死叉时不应卖出: %+v", res.Trades)
	}
}

func TestCalcRangeEMAMACDIndicators(t *testing.T) {
	candles := make([]Candle, 700)
	for i := range candles {
		// 叠加短周期波动：平滑长波的 EMA 金叉总早于慢速 MACD 柱翻正，不会产生买入
		p := 100 + 15*math.Sin(float64(i)/20) + 5*math.Sin(float64(i)/3) + float64(i)*0.05
		candles[i] = Candle{Time: int64(i), Open: p, High: p + 1, Low: p - 1, Close: p, Volume: 10}
	}
	ind := CalcRangeEMAMACDIndicators(candles)
	// EMA104 在下标 103 成形，信号线以之后 36 个 MACD 值的均值起算 → 下标 138 起柱有效
	if math.IsNaN(ind.Delta[138]) || !math.IsNaN(ind.Delta[137]) {
		t.Errorf("MACD 柱有效起点错误: %v %v", ind.Delta[137], ind.Delta[138])
	}
	if math.IsNaN(ind.EMAShort[4]) || !math.IsNaN(ind.EMAShort[3]) || math.IsNaN(ind.EMALong[19]) {
		t.Error("EMA 有效起点错误")
	}
	if math.Abs(ind.BBUpper[50]-ind.BBBasis[50]-(ind.BBBasis[50]-ind.BBLower[50])) > 1e-9 {
		t.Error("布林带应对称")
	}

	res := Backtest(RangeEMAMACD{}, candles, 300, 1000, Params{FeeRate: DefaultFeeRate})
	if len(res.Trades) == 0 {
		t.Fatal("长短周期叠加的行情应产生交易")
	}
	for i := 1; i < len(res.Trades); i++ {
		if res.Trades[i].Action == res.Trades[i-1].Action {
			t.Fatalf("买卖应交替: %+v", res.Trades)
		}
	}
}
