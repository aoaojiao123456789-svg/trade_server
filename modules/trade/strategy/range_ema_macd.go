package strategy

import (
	"fmt"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

// RangeEMAMACD 震荡市策略：TradingView Pine Script「10/50 EMA + MACD + Bollinger Bands Strategy」（我的策略.txt）的复刻
//
// 指标：EMA5、EMA20，慢速 MACD(48,104,36)（标准参数×4），布林带(20,2)（原脚本只绘图，信号未使用）
//
// 买入（空仓时）：
//   - MACD 柱上穿 0 且 EMA5 > EMA20
//   - 或 EMA5 上穿 EMA20 且 MACD 柱 > 0
//
// 卖出（持仓时）：
//   - MACD 线下穿信号线 且 EMA5 < EMA20
//   - 或 EMA5 下穿 EMA20 且 MACD 柱 < 0
//
// 与原脚本的差异（按原脚本意图修正）：
//   - 原脚本均线条件比较的是两个常量 macdFast(48) 与 macdSlow(104)，永远不会成立，导致从不买入；
//     这里改为比较原脚本中已计算但未使用的 EMA5 与 EMA20
//   - 原脚本卖出信号为开空单，现货只能做多，这里改为平仓
//   - 原脚本仓位为 10% 权益，这里与引擎统一为满仓
type RangeEMAMACD struct{}

const (
	ReasonRangeMACDUp      ReasonCode = "range_macd_up"        // MACD柱上穿0轴且EMA多头
	ReasonRangeEMACrossUp  ReasonCode = "range_ema_cross_up"   // EMA金叉且MACD柱为正
	ReasonRangeMACDDown    ReasonCode = "range_macd_down"      // MACD死叉且EMA空头
	ReasonRangeEMACrossDwn ReasonCode = "range_ema_cross_down" // EMA死叉且MACD柱为负
)

const (
	rangeEMAShort   = 5
	rangeEMALong    = 20
	rangeBBPeriod   = 20
	rangeBBMult     = 2.0
	rangeMACDFast   = 12 * 4
	rangeMACDSlow   = 26 * 4
	rangeMACDSignal = 9 * 4
)

func (RangeEMAMACD) Info() Info {
	return Info{
		ID:          "range_ema_macd",
		Name:        "EMA5/20 + 慢速MACD",
		Market:      MarketRange,
		MarketLabel: MarketLabels[MarketRange],
		Source:      "我的策略.txt",
		Description: "EMA5/EMA20 与慢速 MACD(48,104,36) 共振：MACD 柱翻正且短均线在上方、或均线金叉且 MACD 柱为正时买入，反向共振时卖出",
		Notes: []string{
			"TradingView Pine Script「10/50 EMA + MACD + Bollinger Bands Strategy」的 Go 复刻，指标为 TradingView 口径",
			"原脚本均线条件比较的是常量 macdFast(48) 与 macdSlow(104)，永远不成立导致从不买入；已按意图改为比较 EMA5 与 EMA20",
			"原脚本卖出信号为开空，现货只做多，已改为平仓；原脚本仓位为 10% 权益，回测统一按满仓",
			"布林带(20,2) 在原脚本中只绘图未参与信号，快照中保留供分析",
		},
		Options: []OptionInfo{},
		ReasonLabels: map[ReasonCode]string{
			ReasonRangeMACDUp:      "MACD翻正(EMA多头)",
			ReasonRangeEMACrossUp:  "EMA金叉(MACD为正)",
			ReasonRangeMACDDown:    "MACD死叉(EMA空头)",
			ReasonRangeEMACrossDwn: "EMA死叉(MACD为负)",
		},
		Indicators: []IndicatorInfo{
			{"close", "收盘"}, {"ema5", "EMA5"}, {"ema20", "EMA20"}, {"delta", "MACD柱"}, {"macd", "MACD"},
			{"signal", "信号线"}, {"bb_upper", "布林上轨"}, {"bb_basis", "布林中轨"}, {"bb_lower", "布林下轨"},
		},
		// MACD 慢线 EMA104 + 信号线 EMA36 才成形，再留出收敛时间
		WarmupBars: 300,
	}
}

func (RangeEMAMACD) Rules(p Params) Rules {
	return Rules{
		Indicators: "EMA5、EMA20、MACD(48,104,36)（标准参数×4）、布林带(20,2)（仅供参考，不参与信号）",
		Buy:        "空仓时：①MACD 柱上穿 0 且 EMA5 > EMA20；②EMA5 上穿 EMA20 且 MACD 柱 > 0",
		Sell:       "持仓时：①MACD 线下穿信号线且 EMA5 < EMA20；②EMA5 下穿 EMA20 且 MACD 柱 < 0",
		Execution:  ExecutionRule(p.FeeRate),
	}
}

// RangeEMAMACDIndicators 震荡市策略用到的指标
type RangeEMAMACDIndicators struct {
	EMAShort []float64
	EMALong  []float64
	MACD     []float64
	Signal   []float64
	Delta    []float64
	BBBasis  []float64
	BBUpper  []float64
	BBLower  []float64
}

// CalcRangeEMAMACDIndicators 按 TradingView 口径计算指标
func CalcRangeEMAMACDIndicators(candles []Candle) RangeEMAMACDIndicators {
	n := len(candles)
	closes := make([]float64, n)
	for i, c := range candles {
		closes[i] = c.Close
	}
	macd := indicator.PineMACD(closes, rangeMACDFast, rangeMACDSlow, rangeMACDSignal)
	basis := indicator.MA(closes, rangeBBPeriod)
	stdev := indicator.StdDev(closes, rangeBBPeriod)
	upper, lower := make([]float64, n), make([]float64, n)
	for i := range closes {
		upper[i] = basis[i] + rangeBBMult*stdev[i]
		lower[i] = basis[i] - rangeBBMult*stdev[i]
	}
	return RangeEMAMACDIndicators{
		EMAShort: indicator.PineEMA(closes, rangeEMAShort),
		EMALong:  indicator.PineEMA(closes, rangeEMALong),
		MACD:     macd.MACD,
		Signal:   macd.Signal,
		Delta:    macd.Hist,
		BBBasis:  basis,
		BBUpper:  upper,
		BBLower:  lower,
	}
}

func (RangeEMAMACD) Prepare(candles []Candle, _ Params) Evaluator {
	return &rangeEMAMACDEval{c: candles, ind: CalcRangeEMAMACDIndicators(candles)}
}

type rangeEMAMACDEval struct {
	c   []Candle
	ind RangeEMAMACDIndicators
}

func (e *rangeEMAMACDEval) Snapshot(i int) map[string]float64 {
	ind := e.ind
	return snapshot(e.c[i], i, map[string][]float64{
		"ema5": ind.EMAShort, "ema20": ind.EMALong, "macd": ind.MACD, "signal": ind.Signal, "delta": ind.Delta,
		"bb_basis": ind.BBBasis, "bb_upper": ind.BBUpper, "bb_lower": ind.BBLower,
	})
}

func (e *rangeEMAMACDEval) Entry(i int) Signal {
	ind := e.ind
	short, long, delta := ind.EMAShort[i], ind.EMALong[i], ind.Delta[i]
	if indicator.CrossOver(ind.Delta, zeros(len(ind.Delta)), i) && short > long {
		return Signal{ActionBuy, ReasonRangeMACDUp, fmt.Sprintf(
			"MACD 柱上穿 0 轴(%.2f)，EMA5(%.2f) > EMA20(%.2f)", delta, short, long)}
	}
	if indicator.CrossOver(ind.EMAShort, ind.EMALong, i) && delta > 0 {
		return Signal{ActionBuy, ReasonRangeEMACrossUp, fmt.Sprintf(
			"EMA5(%.2f) 上穿 EMA20(%.2f)，MACD 柱 %.2f > 0", short, long, delta)}
	}
	return Signal{}
}

func (e *rangeEMAMACDEval) Exit(i int, _ *Position) Signal {
	ind := e.ind
	short, long, delta := ind.EMAShort[i], ind.EMALong[i], ind.Delta[i]
	if indicator.CrossUnder(ind.MACD, ind.Signal, i) && short < long {
		return Signal{ActionSell, ReasonRangeMACDDown, fmt.Sprintf(
			"MACD 线(%.2f) 下穿信号线(%.2f)，EMA5(%.2f) < EMA20(%.2f)", ind.MACD[i], ind.Signal[i], short, long)}
	}
	if indicator.CrossUnder(ind.EMAShort, ind.EMALong, i) && delta < 0 {
		return Signal{ActionSell, ReasonRangeEMACrossDwn, fmt.Sprintf(
			"EMA5(%.2f) 下穿 EMA20(%.2f)，MACD 柱 %.2f < 0", short, long, delta)}
	}
	return Signal{}
}
