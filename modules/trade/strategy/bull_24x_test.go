package strategy

import (
	"math"
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

const eps = 1e-9

var nan = math.NaN()

// run24 用给定指标运行牛市策略，初始资金 1000
func run24(c []Candle, ind Bull24XIndicators, fee float64, stops bool) Result {
	return Simulate(&bull24xEval{c: c, ind: ind, stops: stops}, c, 0, 1000, fee)
}

func fill(n int, v float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = v
	}
	return s
}

// flat 构造 n 根价格为 100 的K线，及默认处于牛市（收盘 > MA60）但无信号的指标
func flat(n int) ([]Candle, Bull24XIndicators) {
	candles := make([]Candle, n)
	for i := range candles {
		candles[i] = Candle{Time: int64(i), Open: 100, High: 100, Low: 100, Close: 100, Volume: 1}
	}
	return candles, Bull24XIndicators{
		MA20: fill(n, 95), MA60: fill(n, 90), ATR: fill(n, 5), RSI: fill(n, 50),
		MACD: fill(n, 0), Signal: fill(n, 0), Delta: fill(n, -1),
		Upper: fill(n, 200), Lower: fill(n, 50), VolMA: fill(n, 1), Lowest2: fill(n, 100),
	}
}

func TestBullEntryAndUpperTakeProfit(t *testing.T) {
	c, ind := flat(6)
	ind.Delta[1] = 1                                                   // 第1根收盘 MACD 柱上穿 0 → 顺势追涨
	c[2] = Candle{Time: 2, Open: 101, High: 102, Low: 99, Close: 102}  // 第2根开盘 101 买入
	c[3] = Candle{Time: 3, Open: 104, High: 210, Low: 103, Close: 105} // 第3根最高上穿上轨 200 → 止盈
	c[4] = Candle{Time: 4, Open: 108, High: 109, Low: 107, Close: 108} // 第4根开盘 108 卖出

	res := run24(c, ind, 0, false)
	if len(res.Trades) != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}
	buy, sell := res.Trades[0], res.Trades[1]
	if buy.Action != ActionBuy || buy.Time != 2 || buy.SignalTime != 1 || buy.Price != 101 || buy.ReasonCode != ReasonBullMomentum {
		t.Errorf("buy = %+v", buy)
	}
	if sell.Action != ActionSell || sell.Time != 4 || sell.SignalTime != 3 || sell.Price != 108 || sell.ReasonCode != ReasonBullUpper {
		t.Errorf("sell = %+v", sell)
	}
	if math.Abs(sell.ProfitRate-(108.0/101-1)) > eps || buy.Indicators["close"] != 100 || buy.Indicators["delta"] != 1 {
		t.Errorf("sell = %+v, buy.Indicators = %v", sell, buy.Indicators)
	}
	if s := res.Summary; s.TradeCount != 1 || s.WinCount != 1 || s.Holding || res.Pending != nil {
		t.Errorf("summary = %+v pending = %+v", s, res.Pending)
	}
}

func TestBullEntryRequiresConditions(t *testing.T) {
	cases := map[string]func(c []Candle, ind *Bull24XIndicators){
		"收盘在MA20下方": func(c []Candle, ind *Bull24XIndicators) { ind.MA20[1] = 101 },
		"收盘在MA60下方": func(c []Candle, ind *Bull24XIndicators) { ind.MA60[1] = 101 },
		"MA60未成形":   func(c []Candle, ind *Bull24XIndicators) { ind.MA60[1] = nan },
		"上一根柱已为正":   func(c []Candle, ind *Bull24XIndicators) { ind.Delta[0] = 0.5 },
	}
	for name, mutate := range cases {
		c, ind := flat(4)
		ind.Delta[1] = 1
		mutate(c, &ind)
		if res := run24(c, ind, 0, false); len(res.Trades) != 0 {
			t.Errorf("%s: 不应买入 %+v", name, res.Trades)
		}
	}
}

// pinbar 构造满足超跌抄底的第1根K线：熊市、长下影、刺破下轨、放量、RSI<35
func pinbar() ([]Candle, Bull24XIndicators) {
	c, ind := flat(5)
	ind.MA60 = fill(5, 120)
	c[1] = Candle{Time: 1, Open: 100, High: 101, Low: 80, Close: 99, Volume: 2} // 下影 19 > 实体 1×2
	ind.Lower[1] = 85                                                           // 最低 80 < 85×1.01
	ind.RSI[1] = 30
	return c, ind
}

func TestBearPinbarEntryAndReboundTakeProfit(t *testing.T) {
	c, ind := pinbar()
	c[2] = Candle{Time: 2, Open: 98, High: 99, Low: 97, Close: 98} // 开盘 98 买入
	c[3] = Candle{Time: 3, Open: 95.5, High: 96, Low: 95, Close: 96}
	ind.MA20 = []float64{95, 95, 99.5, 95, 95}                     // 第2根最高 99 <= MA20 99.5，第3根最高 96 > 95 → 上穿
	c[4] = Candle{Time: 4, Open: 97, High: 98, Low: 96, Close: 97} // 第4根开盘 97 卖出

	res := run24(c, ind, 0.001, false)
	if len(res.Trades) != 2 || res.Trades[0].ReasonCode != ReasonBearPinbar || res.Trades[1].ReasonCode != ReasonBearRebound {
		t.Fatalf("trades = %+v", res.Trades)
	}
	buy, sell := res.Trades[0], res.Trades[1]
	if buy.Price != 98 || sell.Price != 97 || math.Abs(buy.Fee-1) > eps {
		t.Errorf("buy = %+v sell = %+v", buy, sell)
	}
	// 到手 999/98 个，卖出 97 扣 0.1%
	want := 999.0 / 98 * 97 * 0.999
	if math.Abs(sell.Amount-want) > eps || math.Abs(sell.Profit-(want-1000)) > eps || res.Summary.WinCount != 0 {
		t.Errorf("sell = %+v", sell)
	}
}

func TestBearPinbarRequiresConditions(t *testing.T) {
	cases := map[string]func(c []Candle, ind *Bull24XIndicators){
		"RSI不够低":  func(c []Candle, ind *Bull24XIndicators) { ind.RSI[1] = 35 },
		"未放量":     func(c []Candle, ind *Bull24XIndicators) { ind.VolMA[1] = 2 },
		"未刺破下轨":   func(c []Candle, ind *Bull24XIndicators) { ind.Lower[1] = 79 },
		"下影线不够长":  func(c []Candle, ind *Bull24XIndicators) { c[1].Close, c[1].Open = 90, 99 },
		"在MA60上方": func(c []Candle, ind *Bull24XIndicators) { ind.MA60[1] = 90 },
	}
	for name, mutate := range cases {
		c, ind := pinbar()
		mutate(c, &ind)
		if res := run24(c, ind, 0, false); len(res.Trades) != 0 {
			t.Errorf("%s: 不应买入 %+v", name, res.Trades)
		}
	}
}

func TestNoPyramiding(t *testing.T) {
	c, ind := flat(6)
	ind.Delta = []float64{-1, 1, -1, 1, -1, 1} // 第1、3、5根都出现买入信号
	res := run24(c, ind, 0, false)
	if len(res.Trades) != 1 || res.Trades[0].Time != 2 {
		t.Fatalf("持仓中不应再次买入: %+v", res.Trades)
	}
	if !res.Summary.Holding || res.Pending != nil {
		t.Errorf("summary = %+v pending = %+v", res.Summary, res.Pending)
	}
}

func TestPendingSignalOnLastBar(t *testing.T) {
	c, ind := flat(3)
	ind.Delta[2] = 1
	res := run24(c, ind, 0, false)
	if len(res.Trades) != 0 || res.Pending == nil || res.Pending.Action != ActionBuy || res.Pending.SignalTime != 2 {
		t.Fatalf("最后一根的信号应为待执行: trades = %+v pending = %+v", res.Trades, res.Pending)
	}
}

func TestStartIndex(t *testing.T) {
	c, ind := flat(5)
	ind.Delta[1] = 1 // 预热期内的信号不交易
	if res := Simulate(&bull24xEval{c: c, ind: ind}, c, 3, 1000, 0); len(res.Trades) != 0 {
		t.Fatalf("预热期不应交易: %+v", res.Trades)
	}
}

// crash 买入后持续下跌的行情：第2根开盘 100 买入，之后跌到 70
func crash() ([]Candle, Bull24XIndicators) {
	c, ind := flat(6)
	ind.Delta[1] = 1
	c[3] = Candle{Time: 3, Open: 99, High: 99, Low: 90, Close: 92}
	c[4] = Candle{Time: 4, Open: 85, High: 86, Low: 70, Close: 72}
	c[5] = Candle{Time: 5, Open: 72, High: 73, Low: 70, Close: 71}
	ind.Lowest2 = []float64{100, 100, 100, 90, 70, 70}
	ind.ATR = fill(6, 5)
	return c, ind
}

func TestStopsDisabledByDefault(t *testing.T) {
	// 原脚本 entry_p 为 na，止损不生效：一路下跌也不卖出
	c, ind := crash()
	res := run24(c, ind, 0, false)
	if len(res.Trades) != 1 || !res.Summary.Holding {
		t.Fatalf("默认不应止损: %+v", res.Trades)
	}
	if math.Abs(res.Summary.FloatingProfitRate-(71.0/100-1)) > eps {
		t.Errorf("浮动收益率 = %v", res.Summary.FloatingProfitRate)
	}
}

func TestHardStop(t *testing.T) {
	c, ind := crash()
	res := run24(c, ind, 0, true)
	// 第2根收盘挂止损 min(lowest2=100, 100-1.2*5=94)=94；第3根最低 90 触及，开盘 99 未跳空 → 94 成交
	if len(res.Trades) != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}
	sell := res.Trades[1]
	if sell.ReasonCode != ReasonHardStop || sell.Time != 3 || sell.Price != 94 {
		t.Errorf("sell = %+v", sell)
	}
}

func TestHardStopGapDown(t *testing.T) {
	c, ind := crash()
	c[3] = Candle{Time: 3, Open: 90, High: 91, Low: 88, Close: 89} // 跳空低开到 90，低于止损 94
	res := run24(c, ind, 0, true)
	if len(res.Trades) != 2 || res.Trades[1].Price != 90 {
		t.Fatalf("跳空应按开盘价成交: %+v", res.Trades)
	}
}

func TestBreakevenStop(t *testing.T) {
	c, ind := flat(6)
	ind.Delta[1] = 1
	c[2] = Candle{Time: 2, Open: 100, High: 110, Low: 100, Close: 104} // 开盘 100 买入，收阳线
	c[3] = Candle{Time: 3, Open: 108, High: 108, Low: 104, Close: 105} // 盈利超 3% 收阴线 → 挂保本 100.5
	c[4] = Candle{Time: 4, Open: 103, High: 103, Low: 100, Close: 101} // 最低 100 触及 100.5
	ind.Lowest2 = fill(6, 99)
	ind.ATR = fill(6, 1) // 初始止损 min(99, 98.8)=98.8，低于保本价
	res := run24(c, ind, 0, true)
	if len(res.Trades) != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}
	sell := res.Trades[1]
	if sell.ReasonCode != ReasonBreakeven || sell.Time != 4 || math.Abs(sell.Price-100.5) > eps {
		t.Errorf("sell = %+v", sell)
	}
}

func TestCalcBull24XIndicators(t *testing.T) {
	candles := make([]Candle, 100)
	for i := range candles {
		p := 100 + 10*math.Sin(float64(i)/6) + float64(i)*0.2
		candles[i] = Candle{Time: int64(i), Open: p - 0.5, High: p + 1, Low: p - 1, Close: p, Volume: 10 + float64(i%5)}
	}
	ind := CalcBull24XIndicators(candles)
	for name, s := range map[string][]float64{
		"MA20": ind.MA20, "MA60": ind.MA60, "ATR": ind.ATR, "RSI": ind.RSI, "Delta": ind.Delta,
		"Upper": ind.Upper, "Lower": ind.Lower, "VolMA": ind.VolMA, "Lowest2": ind.Lowest2,
	} {
		if len(s) != 100 || math.IsNaN(s[99]) {
			t.Errorf("%s 长度或最后一个值错误", name)
		}
	}
	if !math.IsNaN(ind.MA60[58]) || math.IsNaN(ind.MA60[59]) {
		t.Error("MA60 有效起点错误")
	}
	// 动态布林：收盘在 MA60 上方用 2.2 倍标准差，否则 1.8 倍
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	stdev := indicator.StdDev(closes, 20)
	bull, bear := 0, 0
	for i := 20; i < 100; i++ {
		want := 1.8
		if candles[i].Close > ind.MA60[i] {
			want, bull = 2.2, bull+1
		} else {
			bear++
		}
		if math.Abs((ind.Upper[i]-ind.MA20[i])/stdev[i]-want) > 1e-6 || math.Abs((ind.MA20[i]-ind.Lower[i])/stdev[i]-want) > 1e-6 {
			t.Fatalf("第 %d 根布林倍数错误", i)
		}
	}
	if bull == 0 || bear == 0 {
		t.Fatalf("测试数据应同时覆盖牛熊: bull=%d bear=%d", bull, bear)
	}
	// 完整回测可运行
	res := Backtest(Bull24X{}, candles, 60, 1000, Params{FeeRate: DefaultFeeRate})
	for i := 1; i < len(res.Trades); i++ {
		if res.Trades[i].Action == res.Trades[i-1].Action {
			t.Fatalf("买卖应交替: %+v", res.Trades)
		}
	}
}
