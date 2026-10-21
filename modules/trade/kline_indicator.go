package openapi

import (
	"math"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/indicator"
)

// 指标需要预热：MA60 要 60 根，EMA 类（MACD）初始值影响约 3*26 根
const indicatorWarmupDays = 120

// KlinesToBars Kline 转为指标计算输入，klines 需按时间升序
func KlinesToBars(klines []*Kline) []indicator.Bar {
	bars := make([]indicator.Bar, len(klines))
	for i, k := range klines {
		bars[i] = indicator.Bar{High: k.HighPrice, Low: k.LowPrice, Close: k.ClosePrice}
	}
	return bars
}

// loadDailyKlinesWithWarmup 查询 [startTime,endTime] 的日线，并向前多取 warmupDays 天用于指标预热；
// 返回含预热数据的K线，以及区间内第一根K线的下标
func (o *OpenAPI) loadDailyKlinesWithWarmup(symbol string, startTime, endTime int64, warmupDays int) ([]*Kline, int, error) {
	klines, err := o.db.queryKlines(symbol, klineInterval1d, startTime-int64(warmupDays)*dayMillis, endTime, uint64(maxKlineQueryLimit+warmupDays))
	if err != nil {
		return nil, 0, err
	}
	start := 0
	for start < len(klines) && klines[start].StartTime < startTime {
		start++
	}
	return klines, start, nil
}

// loadDailyKlinesWithIndicators 查询 [startTime,endTime] 的日线并计算指标
// 会额外向前多取 indicatorWarmupDays 天用于预热，返回结果已裁掉预热部分，
// 返回的 klines 与指标序列下标一一对应
func (o *OpenAPI) loadDailyKlinesWithIndicators(symbol string, startTime, endTime int64) ([]*Kline, indicator.Indicators, error) {
	klines, skip, err := o.loadDailyKlinesWithWarmup(symbol, startTime, endTime, indicatorWarmupDays)
	if err != nil {
		return nil, indicator.Indicators{}, err
	}
	ind := indicator.Calc(KlinesToBars(klines))
	return klines[skip:], sliceIndicators(ind, skip), nil
}

func sliceIndicators(ind indicator.Indicators, from int) indicator.Indicators {
	return indicator.Indicators{
		MAGroup: indicator.MAGroup{
			MA5:  ind.MA5[from:],
			MA10: ind.MA10[from:],
			MA30: ind.MA30[from:],
			MA60: ind.MA60[from:],
		},
		MACD: indicator.MACDResult{DIF: ind.MACD.DIF[from:], DEA: ind.MACD.DEA[from:], MACD: ind.MACD.MACD[from:]},
		BOLL: indicator.BOLLResult{Upper: ind.BOLL.Upper[from:], Mid: ind.BOLL.Mid[from:], Lower: ind.BOLL.Lower[from:]},
		KDJ:  indicator.KDJResult{K: ind.KDJ.K[from:], D: ind.KDJ.D[from:], J: ind.KDJ.J[from:]},
	}
}

// klineIndicatorResp 接口返回：K线字段 + 指标，指标未成形时为 null
type klineIndicatorResp struct {
	*Kline
	MA5       *float64 `json:"ma5"`
	MA10      *float64 `json:"ma10"`
	MA30      *float64 `json:"ma30"`
	MA60      *float64 `json:"ma60"`
	BollUpper *float64 `json:"boll_upper"`
	BollMid   *float64 `json:"boll_mid"`
	BollLower *float64 `json:"boll_lower"`
	DIF       *float64 `json:"dif"`
	DEA       *float64 `json:"dea"`
	MACD      *float64 `json:"macd"`
	K         *float64 `json:"k"`
	D         *float64 `json:"d"`
	J         *float64 `json:"j"`
}

func toKlineIndicatorResps(klines []*Kline, ind indicator.Indicators) []*klineIndicatorResp {
	resps := make([]*klineIndicatorResp, len(klines))
	for i, k := range klines {
		resps[i] = &klineIndicatorResp{
			Kline:     k,
			MA5:       validOrNil(ind.MA5[i]),
			MA10:      validOrNil(ind.MA10[i]),
			MA30:      validOrNil(ind.MA30[i]),
			MA60:      validOrNil(ind.MA60[i]),
			BollUpper: validOrNil(ind.BOLL.Upper[i]),
			BollMid:   validOrNil(ind.BOLL.Mid[i]),
			BollLower: validOrNil(ind.BOLL.Lower[i]),
			DIF:       validOrNil(ind.MACD.DIF[i]),
			DEA:       validOrNil(ind.MACD.DEA[i]),
			MACD:      validOrNil(ind.MACD.MACD[i]),
			K:         validOrNil(ind.KDJ.K[i]),
			D:         validOrNil(ind.KDJ.D[i]),
			J:         validOrNil(ind.KDJ.J[i]),
		}
	}
	return resps
}

func validOrNil(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}
