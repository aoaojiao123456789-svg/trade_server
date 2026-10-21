package indicator

import "math"

// 本文件为 TradingView Pine Script (ta.*) 口径的指标实现，用于复刻 TradingView 策略。
// 与通达信口径的主要差异：
//   - EMA 以前 N 个值的简单平均作为初始值（ta.ema），而非第一个值
//   - ATR、RSI 使用 Wilder 平滑 RMA（ta.rma），同样以 SMA 作为初始值
//   - MACD 柱 = MACD线 - 信号线（不乘 2）
// 输入中的 NaN 视为数据不足，输出在数据不足的位置为 NaN。

// PineEMA ta.ema：alpha = 2/(N+1)，以首个完整窗口的 SMA 为初始值
func PineEMA(values []float64, period int) []float64 {
	return seededSmooth(values, period, 2/float64(period+1))
}

// RMA ta.rma（Wilder 平滑）：alpha = 1/N，以首个完整窗口的 SMA 为初始值
func RMA(values []float64, period int) []float64 {
	return seededSmooth(values, period, 1/float64(period))
}

// seededSmooth 跳过开头的 NaN，从第一个连续 N 个有效值的 SMA 开始做指数平滑
func seededSmooth(values []float64, period int, alpha float64) []float64 {
	out := nanSlice(len(values))
	if period <= 0 {
		return out
	}
	var sum float64
	count := 0
	for i, v := range values {
		if math.IsNaN(v) {
			// 数据不足阶段遇到 NaN 重新计数；已成形后不会再出现 NaN
			sum, count = 0, 0
			continue
		}
		if count < period {
			sum += v
			count++
			if count == period {
				out[i] = sum / float64(period)
			}
			continue
		}
		out[i] = alpha*v + (1-alpha)*out[i-1]
	}
	return out
}

// StdDev ta.stdev(src, N)：总体标准差（biased=true，除以 N），前 N-1 个为 NaN
func StdDev(values []float64, period int) []float64 {
	out := nanSlice(len(values))
	mean := MA(values, period)
	for i := period - 1; i < len(values) && period > 0; i++ {
		var variance float64
		for _, v := range values[i-period+1 : i+1] {
			variance += (v - mean[i]) * (v - mean[i])
		}
		out[i] = math.Sqrt(variance / float64(period))
	}
	return out
}

// OHLCV 计算 TradingView 指标需要的K线字段
type OHLCV struct {
	Open, High, Low, Close, Volume float64
}

// ATR ta.atr(N) = RMA(真实波幅, N)；首根K线的真实波幅为 最高-最低
func ATR(bars []OHLCV, period int) []float64 {
	tr := make([]float64, len(bars))
	for i, b := range bars {
		if i == 0 {
			tr[i] = b.High - b.Low
			continue
		}
		prev := bars[i-1].Close
		tr[i] = math.Max(b.High-b.Low, math.Max(math.Abs(b.High-prev), math.Abs(b.Low-prev)))
	}
	return RMA(tr, period)
}

// RSI ta.rsi(src, N)：涨跌幅分别做 RMA，RSI = 100 - 100/(1+涨/跌)
func RSI(values []float64, period int) []float64 {
	n := len(values)
	up, down := nanSlice(n), nanSlice(n)
	for i := 1; i < n; i++ {
		ch := values[i] - values[i-1]
		up[i], down[i] = math.Max(ch, 0), math.Max(-ch, 0)
	}
	upRMA, downRMA := RMA(up, period), RMA(down, period)
	out := nanSlice(n)
	for i := range out {
		u, d := upRMA[i], downRMA[i]
		switch {
		case math.IsNaN(u) || math.IsNaN(d):
		case d == 0:
			out[i] = 100
		case u == 0:
			out[i] = 0
		default:
			out[i] = 100 - 100/(1+u/d)
		}
	}
	return out
}

// PineMACDResult ta.macd 的三个返回值
type PineMACDResult struct {
	MACD   []float64 // EMA(fast) - EMA(slow)
	Signal []float64 // EMA(MACD, signal)
	Hist   []float64 // MACD - Signal
}

// PineMACD ta.macd(src, fast, slow, signal)
func PineMACD(values []float64, fast, slow, signal int) PineMACDResult {
	emaFast, emaSlow := PineEMA(values, fast), PineEMA(values, slow)
	macd := make([]float64, len(values))
	for i := range values {
		macd[i] = emaFast[i] - emaSlow[i] // 任一为 NaN 结果即为 NaN
	}
	sig := PineEMA(macd, signal)
	hist := make([]float64, len(values))
	for i := range values {
		hist[i] = macd[i] - sig[i]
	}
	return PineMACDResult{MACD: macd, Signal: sig, Hist: hist}
}

// Lowest ta.lowest(src, N)：最近 N 个值的最小值，前 N-1 个为 NaN
func Lowest(values []float64, period int) []float64 {
	out := nanSlice(len(values))
	for i := period - 1; i < len(values) && period > 0; i++ {
		m := values[i-period+1]
		for _, v := range values[i-period+2 : i+1] {
			m = math.Min(m, v)
		}
		out[i] = m
	}
	return out
}

// CrossOver ta.crossover(a, b)：当前 a > b 且上一根 a <= b；任一值为 NaN 时为 false
func CrossOver(a, b []float64, i int) bool {
	if i <= 0 {
		return false
	}
	for _, v := range []float64{a[i], b[i], a[i-1], b[i-1]} {
		if math.IsNaN(v) {
			return false
		}
	}
	return a[i] > b[i] && a[i-1] <= b[i-1]
}

// CrossUnder ta.crossunder(a, b)：当前 a < b 且上一根 a >= b；任一值为 NaN 时为 false
func CrossUnder(a, b []float64, i int) bool {
	return CrossOver(b, a, i)
}
