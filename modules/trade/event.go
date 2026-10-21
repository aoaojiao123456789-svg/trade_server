package openapi

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	klineInterval1d = "1d"
	dayMillis       = int64(24 * time.Hour / time.Millisecond)
	klineFetchLimit = 1000
)

// klineSyncStartTime 日线数据起始时间（币安日线按 UTC 0 点切分），也是查询接口的默认开始时间。
// 调早后下次同步会自动向前补齐。注意各策略需要指标预热（最长为震荡市策略约 300 天），
// 回测开始日期距离该时间太近时，开头一段会因指标未成形而没有信号
var klineSyncStartTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// 需要同步日线的交易对
var klineSymbols = []string{"BTCUSDT"}

func (o *OpenAPI) syncApiData() {
	ctx, cancel := context.WithCancel(context.Background())
	o.stopSync = cancel
	go o.startApiTimer(ctx, time.Minute)
}

func (o *OpenAPI) stopApiData() {
	if o.stopSync != nil {
		o.stopSync()
	}
}

// 独立定时器：单协程顺序执行，不会出现并发重入
func (o *OpenAPI) startApiTimer(ctx context.Context, delayTime time.Duration) {
	ticker := time.NewTicker(delayTime)
	defer ticker.Stop()

	// 启动时立即补一次数据
	o.syncDailyKlines(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.syncDailyKlines(ctx)
		}
	}
}

func (o *OpenAPI) syncDailyKlines(ctx context.Context) {
	for _, symbol := range klineSymbols {
		if err := o.syncSymbolDailyKlines(ctx, symbol); err != nil {
			o.Error("同步日线数据失败", zap.String("symbol", symbol), zap.Error(err))
		}
	}
}

// syncSymbolDailyKlines 补齐某交易对的日线：
//   - 库中最早一根晚于起始时间时，向前补齐 [起始时间, 最早一根)
//   - 从库中最后一根的下一天向后补齐到最近一根已收盘的日线
func (o *OpenAPI) syncSymbolDailyKlines(ctx context.Context, symbol string) error {
	first, last, err := o.db.queryKlineTimeRange(symbol, klineInterval1d)
	if err != nil {
		return err
	}
	if last == 0 {
		return o.syncDailyKlineRange(ctx, symbol, klineSyncStartTime, 0)
	}
	if first > klineSyncStartTime {
		if err := o.syncDailyKlineRange(ctx, symbol, klineSyncStartTime, first); err != nil {
			return err
		}
	}
	return o.syncDailyKlineRange(ctx, symbol, last+dayMillis, 0)
}

// syncDailyKlineRange 拉取开盘时间在 [from, until) 内已收盘的日线并入库，until 为 0 表示不限
func (o *OpenAPI) syncDailyKlineRange(ctx context.Context, symbol string, from, until int64) error {
	return fetchDailyKlineRange(ctx, FetchSpotKlines, symbol, from, until, func(klines []*Kline) error {
		if err := o.db.insertKlines(klines); err != nil {
			return err
		}
		o.Info("同步日线数据",
			zap.String("symbol", symbol),
			zap.Int("count", len(klines)),
			zap.String("from", time.UnixMilli(klines[0].StartTime).UTC().Format("2006-01-02")),
			zap.String("to", time.UnixMilli(klines[len(klines)-1].StartTime).UTC().Format("2006-01-02")),
		)
		return nil
	})
}

type klineFetcher func(ctx context.Context, symbol, interval string, startTime int64, limit int) ([]SpotKline, error)

// fetchDailyKlineRange 从 from 开始分页拉取日线，过滤出开盘时间在 [from, until) 内且已收盘的交给 save
func fetchDailyKlineRange(ctx context.Context, fetch klineFetcher, symbol string, from, until int64, save func([]*Kline) error) error {
	next := from
	for {
		nowMillis := time.Now().UnixMilli()
		// 只保存已收盘的日线，当天未收盘的跳过
		if next+dayMillis > nowMillis || (until > 0 && next >= until) {
			return nil
		}

		rows, err := fetch(ctx, symbol, klineInterval1d, next, klineFetchLimit)
		if err != nil {
			return err
		}

		klines := make([]*Kline, 0, len(rows))
		for _, row := range rows {
			if row.OpenTime < next || row.CloseTime >= nowMillis || (until > 0 && row.OpenTime >= until) {
				continue
			}
			k, err := toKline(symbol, klineInterval1d, row)
			if err != nil {
				return err
			}
			klines = append(klines, k)
		}
		if len(klines) == 0 {
			return nil
		}
		if err := save(klines); err != nil {
			return err
		}

		next = klines[len(klines)-1].StartTime + dayMillis
		if len(rows) < klineFetchLimit {
			return nil
		}
	}
}
