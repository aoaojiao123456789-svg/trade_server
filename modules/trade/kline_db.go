package openapi

import (
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/gocraft/dbr/v2"
)

// DB 用户db操作
type DB struct {
	session *dbr.Session
	ctx     *config.Context
}

// NewDB NewDB
func NewDB(ctx *config.Context) *DB {
	return &DB{
		session: ctx.DB(),
		ctx:     ctx,
	}
}

// kline_1d
type Kline struct {
	ID               int64   `json:"id"`
	StartTime        int64   `json:"start_time"`
	CloseTime        int64   `json:"close_time"`
	Symbol           string  `json:"symbol"`
	Interval         string  `json:"interval"`
	OpenPrice        float64 `json:"open_price"`
	ClosePrice       float64 `json:"close_price"`
	HighPrice        float64 `json:"high_price"`
	LowPrice         float64 `json:"low_price"`
	BaseVolume       float64 `json:"base_volume"`
	QuoteVolume      float64 `json:"quote_volume"`
	TakerBaseVolume  float64 `json:"taker_base_volume"`
	TakerQuoteVolume float64 `json:"taker_quote_volume"`
	TradeCount       int64   `json:"trade_count"`
}

// queryKlineTimeRange 查询某交易对最早、最后一根K线的开盘时间，没有数据均返回0
func (d *DB) queryKlineTimeRange(symbol, interval string) (first, last int64, err error) {
	var r struct {
		First int64 `db:"first_time"`
		Last  int64 `db:"last_time"`
	}
	_, err = d.session.Select("IFNULL(MIN(start_time),0) AS first_time", "IFNULL(MAX(start_time),0) AS last_time").
		From("kline_1d").Where("symbol=? and `interval`=?", symbol, interval).Load(&r)
	return r.First, r.Last, err
}

// insertKlines 批量写入K线，依赖 (symbol,`interval`,start_time) 唯一索引去重
func (d *DB) insertKlines(klines []*Kline) error {
	if len(klines) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("INSERT IGNORE INTO kline_1d (start_time,close_time,symbol,`interval`,open_price,close_price,high_price,low_price,base_volume,quote_volume,taker_base_volume,taker_quote_volume,trade_count) VALUES ")
	args := make([]interface{}, 0, len(klines)*13)
	for i, k := range klines {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args, k.StartTime, k.CloseTime, k.Symbol, k.Interval, k.OpenPrice, k.ClosePrice, k.HighPrice, k.LowPrice,
			k.BaseVolume, k.QuoteVolume, k.TakerBaseVolume, k.TakerQuoteVolume, k.TradeCount)
	}
	_, err := d.session.InsertBySql(sb.String(), args...).Exec()
	return err
}

func toKline(symbol, interval string, s SpotKline) (*Kline, error) {
	k := &Kline{
		StartTime:  s.OpenTime,
		CloseTime:  s.CloseTime,
		Symbol:     symbol,
		Interval:   interval,
		TradeCount: s.TradeCount,
	}
	fields := []struct {
		dst *float64
		src string
	}{
		{&k.OpenPrice, s.Open},
		{&k.ClosePrice, s.Close},
		{&k.HighPrice, s.High},
		{&k.LowPrice, s.Low},
		{&k.BaseVolume, s.Volume},
		{&k.QuoteVolume, s.QuoteVolume},
		{&k.TakerBaseVolume, s.TakerBaseVolume},
		{&k.TakerQuoteVolume, s.TakerQuoteVolume},
	}
	for _, f := range fields {
		v, err := strconv.ParseFloat(f.src, 64)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return k, nil
}

// queryKlines 按开盘时间范围查询K线，按时间升序
func (d *DB) queryKlines(symbol, interval string, startTime, endTime int64, limit uint64) ([]*Kline, error) {
	var klines []*Kline
	_, err := d.session.Select("*").From("kline_1d").
		Where("symbol=? and `interval`=? and start_time>=? and start_time<=?", symbol, interval, startTime, endTime).
		OrderAsc("start_time").Limit(limit).Load(&klines)
	return klines, err
}
