package strategy

import (
	"fmt"
	"math"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

// Bull24X 牛市策略：TradingView Pine Script「24X基因·日线全天候精控版」（日线2倍,胜率80.txt）的复刻
//
// 指标：MA20、MA60（牛熊线）、ATR(14)、MACD(12,26,9)、RSI(14)、成交量 MA20，
// 动态布林带 = MA20 ± mult*stdev(20)，收盘在 MA60 上方 mult=2.2，否则 1.8。
//
// 买入（空仓时）：
//   - 顺势追涨：收盘 > MA60 且 MACD 柱上穿 0 且收盘 > MA20
//   - 超跌抄底：收盘 < MA60 且 Pin Bar（下影线 > 实体*2 且最低 < 下轨*1.01）且放量（量 > 均量*1.1）且 RSI < 35
//
// 卖出（持仓时）：
//   - 空头反弹止盈：收盘 < MA60 且最高价上穿 MA20
//   - 牛市上轨止盈：收盘 > MA60 且最高价上穿布林上轨
//   - 保本/止损（开关 protective_stops）：原脚本中 entry_p 永远为 na，这两类止损在 TradingView 上
//     实际不会触发，默认关闭以保持与原脚本一致
type Bull24X struct{}

const (
	ReasonBullMomentum ReasonCode = "bull_momentum"   // 顺势追涨
	ReasonBearPinbar   ReasonCode = "bear_pinbar"     // 超跌抄底
	ReasonBearRebound  ReasonCode = "bear_rebound_tp" // 空头反弹止盈
	ReasonBullUpper    ReasonCode = "bull_upper_tp"   // 牛市上轨止盈
	ReasonBreakeven    ReasonCode = "breakeven_stop"  // 保本离场
	ReasonHardStop     ReasonCode = "hard_stop"       // 止损离场
)

// OptionProtectiveStops 启用原脚本意图中的保本/止损离场
const OptionProtectiveStops = "protective_stops"

func (Bull24X) Info() Info {
	return Info{
		ID:          "bull_24x",
		Name:        "24X基因·日线全天候精控版",
		Market:      MarketBull,
		MarketLabel: MarketLabels[MarketBull],
		Source:      "日线2倍,胜率80.txt",
		Description: "以 MA60 为牛熊线：牛市 MACD 柱翻正顺势追涨、触及动态布林上轨止盈；熊市只做放量 Pin Bar 超跌反弹、触及 MA20 止盈",
		Notes: []string{
			"TradingView Pine Script「24X基因·日线全天候精控版」的 Go 复刻，指标为 TradingView 口径",
			"原脚本 entry_p 只在 position_size > 0 且 == 0 时赋值，永远为 na，保本/止损代码在 TradingView 上不生效；默认关闭以与原脚本一致，可通过开关启用",
		},
		Options: []OptionInfo{{
			Key:         OptionProtectiveStops,
			Label:       "启用保本/止损离场",
			Description: "原脚本的保本/止损因 entry_p 恒为 na，在 TradingView 上不生效",
		}},
		ReasonLabels: map[ReasonCode]string{
			ReasonBullMomentum: "顺势追涨",
			ReasonBearPinbar:   "超跌抄底",
			ReasonBearRebound:  "空头反弹止盈",
			ReasonBullUpper:    "牛市上轨止盈",
			ReasonBreakeven:    "保本离场",
			ReasonHardStop:     "止损离场",
		},
		Indicators: []IndicatorInfo{
			{"close", "收盘"}, {"ma20", "MA20"}, {"ma60", "MA60"}, {"upper", "布林上轨"}, {"lower", "布林下轨"},
			{"delta", "MACD柱"}, {"macd", "MACD"}, {"signal", "信号线"}, {"rsi", "RSI"}, {"atr", "ATR"},
			{"volume", "成交量"}, {"vol_ma", "均量"},
		},
		WarmupBars: 120,
	}
}

func (s Bull24X) Rules(p Params) Rules {
	sell := "持仓时：①空头反弹止盈 — 收盘 < MA60 且最高价上穿 MA20；②牛市上轨止盈 — 收盘 > MA60 且最高价上穿布林上轨（与原脚本一致不设止损：原脚本的保本/止损因 entry_p 恒为 na 实际不生效）"
	if p.Option(s, OptionProtectiveStops) {
		sell = "持仓时：①空头反弹止盈 — 收盘 < MA60 且最高价上穿 MA20；②牛市上轨止盈 — 收盘 > MA60 且最高价上穿布林上轨；③保本离场 — 盈利超 3% 后收阴线，挂入场价×1.005 止损；④止损离场 — 止损价 = min(近 2 日最低, 入场价 - 1.2×ATR)"
	}
	return Rules{
		Indicators: "MA20、MA60（牛熊线）、MACD(12,26,9)、RSI(14)、ATR(14)、成交量MA20；动态布林 = MA20 ± k×标准差(20)，收盘在 MA60 上方 k=2.2，否则 1.8",
		Buy:        "空仓时：①顺势追涨 — 收盘 > MA60 且 MACD 柱上穿 0 且收盘 > MA20；②超跌抄底 — 收盘 < MA60 且 Pin Bar（下影 > 实体×2 且最低 < 下轨×1.01）且放量（量 > 均量×1.1）且 RSI < 35",
		Sell:       sell,
		Execution:  ExecutionRule(p.FeeRate),
	}
}

func (s Bull24X) Prepare(candles []Candle, p Params) Evaluator {
	return &bull24xEval{c: candles, ind: CalcBull24XIndicators(candles), stops: p.Option(s, OptionProtectiveStops)}
}

// Bull24XIndicators 牛市策略用到的指标，下标与K线一一对应，数据不足时为 NaN
type Bull24XIndicators struct {
	MA20    []float64
	MA60    []float64
	ATR     []float64
	RSI     []float64
	MACD    []float64
	Signal  []float64
	Delta   []float64 // MACD 柱 = MACD - Signal
	Upper   []float64 // 动态布林上轨
	Lower   []float64 // 动态布林下轨
	VolMA   []float64
	Lowest2 []float64 // 近 2 根最低价
}

// CalcBull24XIndicators 按 TradingView 口径计算指标
func CalcBull24XIndicators(candles []Candle) Bull24XIndicators {
	n := len(candles)
	closes, lows, volumes := make([]float64, n), make([]float64, n), make([]float64, n)
	bars := make([]indicator.OHLCV, n)
	for i, c := range candles {
		closes[i], lows[i], volumes[i] = c.Close, c.Low, c.Volume
		bars[i] = indicator.OHLCV{Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume}
	}
	macd := indicator.PineMACD(closes, 12, 26, 9)
	ind := Bull24XIndicators{
		MA20:    indicator.MA(closes, 20),
		MA60:    indicator.MA(closes, 60),
		ATR:     indicator.ATR(bars, 14),
		RSI:     indicator.RSI(closes, 14),
		MACD:    macd.MACD,
		Signal:  macd.Signal,
		Delta:   macd.Hist,
		VolMA:   indicator.MA(volumes, 20),
		Lowest2: indicator.Lowest(lows, 2),
		Upper:   make([]float64, n),
		Lower:   make([]float64, n),
	}
	stdev := indicator.StdDev(closes, 20)
	for i, c := range candles {
		// close > ma60 为 na 时 Pine 的三元表达式取 1.8
		mult := 1.8
		if c.Close > ind.MA60[i] {
			mult = 2.2
		}
		ind.Upper[i] = ind.MA20[i] + mult*stdev[i]
		ind.Lower[i] = ind.MA20[i] - mult*stdev[i]
	}
	return ind
}

type bull24xEval struct {
	c     []Candle
	ind   Bull24XIndicators
	stops bool
}

func (e *bull24xEval) Snapshot(i int) map[string]float64 {
	ind := e.ind
	return snapshot(e.c[i], i, map[string][]float64{
		"ma20": ind.MA20, "ma60": ind.MA60, "upper": ind.Upper, "lower": ind.Lower,
		"atr": ind.ATR, "rsi": ind.RSI, "macd": ind.MACD, "signal": ind.Signal, "delta": ind.Delta, "vol_ma": ind.VolMA,
	})
}

func (e *bull24xEval) Entry(i int) Signal {
	c, ind := e.c, e.ind
	k := c[i]
	ma20, ma60 := ind.MA20[i], ind.MA60[i]
	isBull, isBear := k.Close > ma60, k.Close < ma60 // ma60 为 NaN 时两者均为 false

	if isBull && indicator.CrossOver(ind.Delta, zeros(len(ind.Delta)), i) && k.Close > ma20 {
		return Signal{ActionBuy, ReasonBullMomentum, fmt.Sprintf(
			"顺势追涨：收盘 %.2f 站上 MA60(%.2f) 与 MA20(%.2f)，MACD 柱上穿 0 轴(%.2f)",
			k.Close, ma60, ma20, ind.Delta[i])}
	}

	body := math.Abs(k.Close - k.Open)
	lowerWick := math.Min(k.Open, k.Close) - k.Low
	isPinbar := lowerWick > body*2 && k.Low < ind.Lower[i]*1.01
	volConfirmed := k.Volume > ind.VolMA[i]*1.1
	if isBear && isPinbar && volConfirmed && ind.RSI[i] < 35 {
		return Signal{ActionBuy, ReasonBearPinbar, fmt.Sprintf(
			"超跌抄底：收盘 %.2f 在 MA60(%.2f) 下方，Pin Bar(下影 %.2f > 实体 %.2f×2，最低 %.2f < 下轨 %.2f×1.01)，放量(%.2f > 均量 %.2f×1.1)，RSI %.1f < 35",
			k.Close, ma60, lowerWick, body, k.Low, ind.Lower[i], k.Volume, ind.VolMA[i], ind.RSI[i])}
	}
	return Signal{}
}

func (e *bull24xEval) Exit(i int, _ *Position) Signal {
	if i == 0 {
		return Signal{}
	}
	c, ind := e.c, e.ind
	k := c[i]
	ma60 := ind.MA60[i]
	highs := []float64{c[i-1].High, k.High}
	if k.Close < ma60 && indicator.CrossOver(highs, ind.MA20[i-1:i+1], 1) {
		return Signal{ActionSell, ReasonBearRebound, fmt.Sprintf(
			"空头反弹止盈：收盘 %.2f 在 MA60(%.2f) 下方，最高 %.2f 上穿 MA20(%.2f)", k.Close, ma60, k.High, ind.MA20[i])}
	}
	if k.Close > ma60 && indicator.CrossOver(highs, ind.Upper[i-1:i+1], 1) {
		return Signal{ActionSell, ReasonBullUpper, fmt.Sprintf(
			"牛市上轨止盈：收盘 %.2f 在 MA60(%.2f) 上方，最高 %.2f 上穿布林上轨(%.2f)", k.Close, ma60, k.High, ind.Upper[i])}
	}
	return Signal{}
}

// UpdateStops 原脚本意图：盈利超 3% 且收阴线时挂保本止损，否则挂（更新）初始止损
func (e *bull24xEval) UpdateStops(i int, pos *Position) {
	if !e.stops {
		return
	}
	k := e.c[i]
	if k.Close > pos.EntryPrice*1.03 && k.Close < k.Open {
		pos.SetStop(Stop{ReasonBreakeven, pos.EntryPrice * 1.005, "保本离场：盈利超 3%% 后收阴线挂出的保本止损 %.2f 被触发"})
		return
	}
	// 任一为 NaN 时结果为 NaN，即不挂单
	price := math.Min(e.ind.Lowest2[i], pos.EntryPrice-1.2*e.ind.ATR[i])
	pos.SetStop(Stop{ReasonHardStop, price, "止损离场：止损价 %.2f（近 2 日最低与入场价 - 1.2ATR 取低）被触发"})
}

func zeros(n int) []float64 { return make([]float64, n) }
