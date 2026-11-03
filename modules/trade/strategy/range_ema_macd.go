package strategy

import (
	"fmt"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

// RangeEMAMACD 震荡市策略：由 TradingView Pine Script「10/50 EMA + MACD + Bollinger Bands Strategy」（我的策略.txt）优化而来
//
// 指标：EMA5、EMA20、EMA60，慢速 MACD(48,104,36)（标准参数×4），KDJ(9,3,3)，RSI(14)，布林带(20,2)
//
// 两类买入，持仓由买入它的那一类规则卖出（空仓时，同时满足按趋势单处理）：
//   - 趋势单：EMA5 上穿 EMA20、MACD 柱 > 0、收盘 > EMA60 且 K > D；EMA5 下穿 EMA20 时卖出
//   - 超跌单：近 3 天出现过 RSI < 35，且当天收阳、RSI 回升（先超跌、后企稳，不接下跌中的飞刀）；收盘回到布林中轨时卖出
//
// 与原脚本的差异：
//   - 原脚本均线条件比较的是两个常量 macdFast(48) 与 macdSlow(104)，永远不会成立，导致从不买入；
//     这里改为比较原脚本中已计算但未使用的 EMA5 与 EMA20
//   - 原脚本的买卖都依赖慢速 MACD 翻转，操作频率低且卖出滞后，这里 MACD 只用于过滤买入，卖出不再等待 MACD
//   - 原脚本卖出信号为开空单，现货只能做多，这里改为平仓
//   - 原脚本仓位为 10% 权益，这里与引擎统一为满仓
type RangeEMAMACD struct{}

const (
	ReasonRangeEMACrossUp  ReasonCode = "range_ema_cross_up"   // EMA金叉，MACD柱为正、站上EMA60且K>D
	ReasonRangeEMACrossDwn ReasonCode = "range_ema_cross_down" // EMA死叉
	ReasonRangeOversold    ReasonCode = "range_oversold"       // RSI超跌后企稳
	ReasonRangeBBMid       ReasonCode = "range_bb_mid"         // 回到布林中轨
)

const (
	rangeEMAShort   = 5
	rangeEMALong    = 20
	rangeEMATrend   = 60
	rangeRSIPeriod  = 14
	rangeRSIOversld = 35.0
	rangeOversldBar = 3 // 超跌信号的有效天数，含当天
	rangeBBPeriod   = 20
	rangeBBMult     = 2.0
	rangeMACDFast   = 12 * 4
	rangeMACDSlow   = 26 * 4
	rangeMACDSignal = 9 * 4
)

func (RangeEMAMACD) Info() Info {
	return Info{
		ID:          "range_ema_macd",
		Name:        "EMA趋势 + RSI超跌",
		Market:      MarketRange,
		MarketLabel: MarketLabels[MarketRange],
		Source:      "我的策略.txt",
		Description: "趋势单：EMA5 上穿 EMA20、慢速 MACD 柱为正、站上 EMA60 且 K > D 时买入，EMA5 下穿 EMA20 卖出；超跌单：近 3 天 RSI < 35 且当天收阳、RSI 回升时买入，回到布林中轨卖出",
		Notes: []string{
			"由 TradingView Pine Script「10/50 EMA + MACD + Bollinger Bands Strategy」优化而来，指标为 TradingView 口径",
			"原脚本均线条件比较的是常量 macdFast(48) 与 macdSlow(104)，永远不成立导致从不买入；已按意图改为比较 EMA5 与 EMA20",
			"原规则的买卖都依赖慢速 MACD 翻转，操作频率低且卖出滞后；现在 MACD 只用于过滤买入，卖出不再等待 MACD",
			"原脚本卖出信号为开空，现货只做多，已改为平仓；原脚本仓位为 10% 权益，回测统一按满仓",
			"原脚本布林带(20,2) 只绘图未参与信号；现在用中轨作为超跌单的卖出线",
			"趋势单增加收盘 > EMA60 和 K > D 过滤，减少弱势反弹中的假金叉；超跌单增加低买高卖的机会，并要求收阳且 RSI 回升，避免在下跌途中买入",
			"超跌单没有固定止损，单独运行本策略时不会因转入熊市清仓；市场阶段自适应策略转熊时会清仓",
		},
		Options: []OptionInfo{},
		ReasonLabels: map[ReasonCode]string{
			ReasonRangeEMACrossUp:  "EMA金叉(MACD为正、站上EMA60、K>D)",
			ReasonRangeEMACrossDwn: "EMA死叉",
			ReasonRangeOversold:    "RSI超跌后企稳",
			ReasonRangeBBMid:       "回到布林中轨",
		},
		Indicators: []IndicatorInfo{
			{"close", "收盘"}, {"ema5", "EMA5"}, {"ema20", "EMA20"}, {"ema60", "EMA60"}, {"k", "K"}, {"d", "D"}, {"rsi", "RSI"}, {"delta", "MACD柱"}, {"macd", "MACD"},
			{"signal", "信号线"}, {"bb_upper", "布林上轨"}, {"bb_basis", "布林中轨"}, {"bb_lower", "布林下轨"},
		},
		// MACD 慢线 EMA104 + 信号线 EMA36 才成形，再留出收敛时间
		WarmupBars: 300,
	}
}

func (RangeEMAMACD) Rules(p Params) Rules {
	return Rules{
		Indicators: "EMA5、EMA20、EMA60、MACD(48,104,36)（标准参数×4）、KDJ(9,3,3)、RSI(14)、布林带(20,2)",
		Buy:        "空仓时：①趋势单 — EMA5 上穿 EMA20、MACD 柱 > 0、收盘 > EMA60 且 K > D；②超跌单 — 近 3 天出现过 RSI < 35，且当天收阳、RSI 回升",
		Sell:       "持仓时由买入它的规则卖出：趋势单 — EMA5 下穿 EMA20；超跌单 — 收盘回到布林中轨",
		Execution:  ExecutionRule(p.FeeRate),
	}
}

// RangeEMAMACDIndicators 震荡市策略用到的指标
type RangeEMAMACDIndicators struct {
	EMAShort []float64
	EMALong  []float64
	EMATrend []float64
	K        []float64
	D        []float64
	RSI      []float64
	MACD     []float64
	Signal   []float64
	Delta    []float64
	BBBasis  []float64
	BBUpper  []float64
	BBLower  []float64
}

// CalcRangeEMAMACDIndicators 按 TradingView 口径计算指标，KDJ 为通达信口径
func CalcRangeEMAMACDIndicators(candles []Candle) RangeEMAMACDIndicators {
	n := len(candles)
	closes := make([]float64, n)
	bars := make([]indicator.Bar, n)
	for i, c := range candles {
		closes[i] = c.Close
		bars[i] = indicator.Bar{High: c.High, Low: c.Low, Close: c.Close}
	}
	kdj := indicator.KDJ(bars)
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
		EMATrend: indicator.PineEMA(closes, rangeEMATrend),
		K:        kdj.K,
		D:        kdj.D,
		RSI:      indicator.RSI(closes, rangeRSIPeriod),
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
		"ema5": ind.EMAShort, "ema20": ind.EMALong, "ema60": ind.EMATrend, "k": ind.K, "d": ind.D, "rsi": ind.RSI,
		"macd": ind.MACD, "signal": ind.Signal, "delta": ind.Delta,
		"bb_basis": ind.BBBasis, "bb_upper": ind.BBUpper, "bb_lower": ind.BBLower,
	})
}

// trendEntry 第 i 根K线收盘时是否满足趋势单买入条件
func (e *rangeEMAMACDEval) trendEntry(i int) bool {
	ind := e.ind
	return indicator.CrossOver(ind.EMAShort, ind.EMALong, i) && ind.Delta[i] > 0 &&
		e.c[i].Close > ind.EMATrend[i] && ind.K[i] > ind.D[i]
}

func (e *rangeEMAMACDEval) Entry(i int) Signal {
	ind := e.ind
	if e.trendEntry(i) {
		return Signal{ActionBuy, ReasonRangeEMACrossUp, fmt.Sprintf(
			"EMA5(%.2f) 上穿 EMA20(%.2f)，MACD 柱 %.2f > 0，收盘 %.2f > EMA60(%.2f)，K(%.1f) > D(%.1f)",
			ind.EMAShort[i], ind.EMALong[i], ind.Delta[i], e.c[i].Close, ind.EMATrend[i], ind.K[i], ind.D[i])}
	}
	if e.oversoldEntry(i) {
		k := e.c[i]
		return Signal{ActionBuy, ReasonRangeOversold, fmt.Sprintf(
			"近 %d 天 RSI 低于 %.0f，当天企稳：收阳(开 %.2f → 收 %.2f)，RSI 回升(%.1f → %.1f)",
			rangeOversldBar, rangeRSIOversld, k.Open, k.Close, ind.RSI[i-1], ind.RSI[i])}
	}
	return Signal{}
}

// oversoldEntry 第 i 根K线收盘时是否满足超跌单买入条件
func (e *rangeEMAMACDEval) oversoldEntry(i int) bool {
	ind := e.ind
	if i == 0 || e.c[i].Close <= e.c[i].Open || ind.RSI[i] <= ind.RSI[i-1] {
		return false
	}
	from := i - rangeOversldBar + 1
	if from < 0 {
		from = 0
	}
	for _, rsi := range ind.RSI[from : i+1] {
		if rsi < rangeRSIOversld {
			return true
		}
	}
	return false
}

func (e *rangeEMAMACDEval) Exit(i int, pos *Position) Signal {
	ind := e.ind
	// 买入信号在成交前一根K线收盘产生，据此判断这笔持仓属于哪一类
	if e.trendEntry(pos.EntryIndex - 1) {
		if indicator.CrossUnder(ind.EMAShort, ind.EMALong, i) {
			return Signal{ActionSell, ReasonRangeEMACrossDwn, fmt.Sprintf(
				"EMA5(%.2f) 下穿 EMA20(%.2f)", ind.EMAShort[i], ind.EMALong[i])}
		}
		return Signal{}
	}
	if e.c[i].Close >= ind.BBBasis[i] {
		return Signal{ActionSell, ReasonRangeBBMid, fmt.Sprintf(
			"收盘 %.2f 回到布林中轨(%.2f)", e.c[i].Close, ind.BBBasis[i])}
	}
	return Signal{}
}
