package openapi

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/app"
	commonapi "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

type OpenAPI struct {
	ctx                      *config.Context
	appService               app.IService
	openapiAuthcodePrefix    string
	openapiAccessTokenPrefix string
	commonService            commonapi.IService
	db                       *DB
	stopSync                 context.CancelFunc
	log.Log
}

func New(ctx *config.Context) *OpenAPI {
	return &OpenAPI{
		Log:                      log.NewTLog("Trade"),
		ctx:                      ctx,
		appService:               app.NewService(ctx),
		openapiAuthcodePrefix:    "openapi:authcodePrefix:",
		openapiAccessTokenPrefix: "openapi:accessTokenPrefix:",
		commonService:            commonapi.NewService(ctx),
		db:                       NewDB(ctx),
	}
}

// Route 路由配置
func (o *OpenAPI) Route(r *wkhttp.WKHttp) {
	// 无需鉴权
	r.GET("/v1/kline/1d", o.getDailyKlines)
	r.GET("/v1/kline/1d/indicators", o.getDailyKlineIndicators)
	r.GET("/v1/strategies", o.getStrategies)
	r.GET("/v1/strategy/backtest", o.getStrategyBacktest)
}

// 单次查询最多返回的日线数（约 8 年）
const maxKlineQueryLimit = 3000

type klineQuery struct {
	symbol    string
	startTime int64
	endTime   int64
}

// parseKlineQuery 解析参数: symbol 交易对(默认BTCUSDT), start_time/end_time 开盘时间范围(毫秒时间戳，闭区间)
func parseKlineQuery(c *wkhttp.Context) (*klineQuery, error) {
	symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	startTime, err := parseMillisQuery(c.Query("start_time"), klineSyncStartTime)
	if err != nil {
		return nil, errors.New("start_time格式错误")
	}
	endTime, err := parseMillisQuery(c.Query("end_time"), time.Now().UnixMilli())
	if err != nil {
		return nil, errors.New("end_time格式错误")
	}
	if startTime > endTime {
		return nil, errors.New("start_time不能大于end_time")
	}
	return &klineQuery{symbol: symbol, startTime: startTime, endTime: endTime}, nil
}

// getDailyKlines 查询时间段内的日K线
func (o *OpenAPI) getDailyKlines(c *wkhttp.Context) {
	q, err := parseKlineQuery(c)
	if err != nil {
		c.ResponseError(err)
		return
	}
	klines, err := o.db.queryKlines(q.symbol, klineInterval1d, q.startTime, q.endTime, maxKlineQueryLimit)
	if err != nil {
		o.Error("查询日K线失败", zap.String("symbol", q.symbol), zap.Error(err))
		c.ResponseError(errors.New("查询日K线失败"))
		return
	}
	if klines == nil {
		klines = make([]*Kline, 0)
	}
	c.Response(klines)
}

// getDailyKlineIndicators 查询时间段内的日K线及技术指标（MA/BOLL/MACD/KDJ）
func (o *OpenAPI) getDailyKlineIndicators(c *wkhttp.Context) {
	q, err := parseKlineQuery(c)
	if err != nil {
		c.ResponseError(err)
		return
	}
	klines, ind, err := o.loadDailyKlinesWithIndicators(q.symbol, q.startTime, q.endTime)
	if err != nil {
		o.Error("查询日K线指标失败", zap.String("symbol", q.symbol), zap.Error(err))
		c.ResponseError(errors.New("查询日K线指标失败"))
		return
	}
	c.Response(toKlineIndicatorResps(klines, ind))
}

func parseMillisQuery(v string, def int64) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
