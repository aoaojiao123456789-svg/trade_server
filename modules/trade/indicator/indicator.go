// Package indicator 技术指标计算（MA、KDJ、BOLL、MACD）
//
// 约定：
//   - 输入序列按时间升序排列（最早的在前）
//   - 输出序列与输入等长、下标一一对应
//   - 数据不足、指标尚未成形的位置为 NaN，可用 IsValid 判断
//
// 公式与通达信/同花顺的常用定义保持一致，个别差异在各函数注释中说明。
package indicator

import "math"

// Bar 计算指标需要的K线字段
type Bar struct {
	High  float64
	Low   float64
	Close float64
}

// IsValid 判断指标值是否有效（非 NaN）
func IsValid(v float64) bool {
	return !math.IsNaN(v)
}

func nanSlice(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}

// ================= MA =================

// MA 简单移动平均：MA(N) = 最近 N 个值之和 / N，前 N-1 个为 NaN
func MA(values []float64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 {
		return out
	}
	var sum float64
	for i, v := range values {
		sum += v
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out[i] = sum / float64(period)
		}
	}
	return out
}

// MAGroup 常用均线组合
type MAGroup struct {
	MA5  []float64
	MA10 []float64
	MA30 []float64
	MA60 []float64
}

// CalcMAGroup 计算 5、10、30、60 日均线
func CalcMAGroup(closes []float64) MAGroup {
	return MAGroup{
		MA5:  MA(closes, 5),
		MA10: MA(closes, 10),
		MA30: MA(closes, 30),
		MA60: MA(closes, 60),
	}
}

// ================= EMA =================

// EMA 指数移动平均：EMA = α*X + (1-α)*EMA[昨]，α = 2/(N+1)
// 与通达信一致，以第一个值作为初始 EMA，因此从第一个位置起就有值；
// 但前若干个值受初始值影响较大，做策略时建议丢弃前 3*N 左右的数据。
func EMA(values []float64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) == 0 {
		return out
	}
	alpha := 2 / float64(period+1)
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

// ================= MACD =================

// MACDResult MACD 计算结果
type MACDResult struct {
	DIF  []float64 // 快线：EMA(close,fast) - EMA(close,slow)
	DEA  []float64 // 慢线：EMA(DIF,signal)
	MACD []float64 // 柱：2*(DIF-DEA)，国内软件口径；币安/TradingView 的柱为 DIF-DEA，即此值的一半
}

// MACD 默认参数 (12,26,9)
func MACD(closes []float64) MACDResult {
	return MACDWithParams(closes, 12, 26, 9)
}

// MACDWithParams 自定义参数的 MACD
func MACDWithParams(closes []float64, fast, slow, signal int) MACDResult {
	emaFast := EMA(closes, fast)
	emaSlow := EMA(closes, slow)
	dif := make([]float64, len(closes))
	for i := range closes {
		dif[i] = emaFast[i] - emaSlow[i]
	}
	dea := EMA(dif, signal)
	macd := make([]float64, len(closes))
	for i := range closes {
		macd[i] = 2 * (dif[i] - dea[i])
	}
	return MACDResult{DIF: dif, DEA: dea, MACD: macd}
}

// ================= BOLL =================

// BOLLResult 布林带计算结果
type BOLLResult struct {
	Upper []float64 // 上轨：MID + k*STD
	Mid   []float64 // 中轨：MA(close,N)
	Lower []float64 // 下轨：MID - k*STD
}

// BOLL 默认参数 (20,2)
func BOLL(closes []float64) BOLLResult {
	return BOLLWithParams(closes, 20, 2)
}

// BOLLWithParams 自定义参数的布林带，前 N-1 个为 NaN
// STD 使用总体标准差（除以 N），与币安/TradingView 一致；
// 通达信的 STD 是样本标准差（除以 N-1），两者带宽略有差异。
func BOLLWithParams(closes []float64, period int, k float64) BOLLResult {
	n := len(closes)
	res := BOLLResult{Upper: nanSlice(n), Mid: MA(closes, period), Lower: nanSlice(n)}
	if period <= 0 {
		return res
	}
	for i := period - 1; i < n; i++ {
		mid := res.Mid[i]
		var variance float64
		for _, v := range closes[i-period+1 : i+1] {
			variance += (v - mid) * (v - mid)
		}
		std := math.Sqrt(variance / float64(period))
		res.Upper[i] = mid + k*std
		res.Lower[i] = mid - k*std
	}
	return res
}

// ================= KDJ =================

// KDJResult KDJ 计算结果
type KDJResult struct {
	K []float64
	D []float64
	J []float64
}

// KDJ 默认参数 (9,3,3)
func KDJ(bars []Bar) KDJResult {
	return KDJWithParams(bars, 9, 3, 3)
}

// KDJWithParams 自定义参数的 KDJ
//
//	RSV = (C - LLV(L,N)) / (HHV(H,N) - LLV(L,N)) * 100
//	K   = SMA(RSV,M1,1) = (1/M1)*RSV + (1-1/M1)*K[昨]
//	D   = SMA(K,M2,1)   = (1/M2)*K   + (1-1/M2)*D[昨]
//	J   = 3K - 2D
//
// K、D 初始值取 50；不足 N 根时用已有的K线计算 HHV/LLV（与通达信一致）；
// 最高价等于最低价时 RSV 沿用上一个值（首根取 50）。
func KDJWithParams(bars []Bar, n, m1, m2 int) KDJResult {
	size := len(bars)
	res := KDJResult{K: nanSlice(size), D: nanSlice(size), J: nanSlice(size)}
	if n <= 0 || m1 <= 0 || m2 <= 0 {
		return res
	}
	prevK, prevD, prevRSV := 50.0, 50.0, 50.0
	for i := range bars {
		start := max(i-n+1, 0)
		hhv, llv := bars[start].High, bars[start].Low
		for _, b := range bars[start+1 : i+1] {
			hhv = math.Max(hhv, b.High)
			llv = math.Min(llv, b.Low)
		}
		rsv := prevRSV
		if hhv > llv {
			rsv = (bars[i].Close - llv) / (hhv - llv) * 100
		}
		k := (rsv + float64(m1-1)*prevK) / float64(m1)
		d := (k + float64(m2-1)*prevD) / float64(m2)
		res.K[i], res.D[i], res.J[i] = k, d, 3*k-2*d
		prevK, prevD, prevRSV = k, d, rsv
	}
	return res
}

// ================= 汇总 =================

// Indicators 一组K线上的全部指标，下标与输入K线一一对应
type Indicators struct {
	MAGroup
	MACD MACDResult
	BOLL BOLLResult
	KDJ  KDJResult
}

// Calc 用默认参数计算全部指标：MA(5/10/30/60)、MACD(12,26,9)、BOLL(20,2)、KDJ(9,3,3)
func Calc(bars []Bar) Indicators {
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	return Indicators{
		MAGroup: CalcMAGroup(closes),
		MACD:    MACD(closes),
		BOLL:    BOLL(closes),
		KDJ:     KDJ(bars),
	}
}
