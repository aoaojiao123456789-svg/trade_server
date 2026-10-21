package openapi

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/strategy"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

const (
	defaultBacktestCapital = 10000
	maxBacktestFeeRate     = 0.01
)

type backtestTradeResp struct {
	Action     strategy.Action     `json:"action"`      // buy / sell
	Time       int64               `json:"time"`        // 成交K线的开盘时间（毫秒）
	SignalTime int64               `json:"signal_time"` // 产生信号的K线开盘时间：市价单为成交前一天收盘，止损单为成交当天
	Price      float64             `json:"price"`       // 成交价：市价单为成交当天开盘价，止损单为止损价或跳空开盘价
	Quantity   float64             `json:"quantity"`
	Amount     float64             `json:"amount"` // 买入为花费资金（含手续费），卖出为扣费后到手资金
	Fee        float64             `json:"fee"`
	ReasonCode strategy.ReasonCode `json:"reason_code"`
	Reason     string              `json:"reason"`
	Indicators map[string]float64  `json:"indicators"`  // 信号K线的价格与策略指标
	Profit     *float64            `json:"profit"`      // 本笔净收益（已扣双边手续费），仅卖出有值
	ProfitRate *float64            `json:"profit_rate"` // 本笔净收益率，仅卖出有值
}

type backtestSummaryResp struct {
	InitialCapital     float64 `json:"initial_capital"`
	FeeRate            float64 `json:"fee_rate"`
	TotalFee           float64 `json:"total_fee"`
	FinalCapital       float64 `json:"final_capital"`
	TotalProfit        float64 `json:"total_profit"`
	TotalReturnRate    float64 `json:"total_return_rate"`
	TradeCount         int     `json:"trade_count"`
	WinCount           int     `json:"win_count"`
	WinRate            float64 `json:"win_rate"`
	Holding            bool    `json:"holding"`
	FloatingProfit     float64 `json:"floating_profit"`
	FloatingProfitRate float64 `json:"floating_profit_rate"`
	Equity             float64 `json:"equity"`
	EquityReturnRate   float64 `json:"equity_return_rate"`
}

// 区间最后一天收盘产生、将在下一天开盘执行的信号
type pendingSignalResp struct {
	Action     strategy.Action     `json:"action"`
	ReasonCode strategy.ReasonCode `json:"reason_code"`
	Reason     string              `json:"reason"`
	SignalTime int64               `json:"signal_time"`
	Indicators map[string]float64  `json:"indicators"`
}

// BacktestResponse 回测接口返回
type BacktestResponse struct {
	StrategyID      string                         `json:"strategy_id"`
	Strategy        string                         `json:"strategy"`
	Market          strategy.Market                `json:"market"`
	MarketLabel     string                         `json:"market_label"`
	Notes           []string                       `json:"notes"`
	Rules           strategy.Rules                 `json:"rules"`
	Options         map[string]bool                `json:"options"` // 本次回测各开关的取值
	ReasonLabels    map[strategy.ReasonCode]string `json:"reason_labels"`
	IndicatorLabels []strategy.IndicatorInfo       `json:"indicator_labels"`
	Symbol          string                         `json:"symbol"`
	StartTime       int64                          `json:"start_time"`
	EndTime         int64                          `json:"end_time"`
	Trades          []*backtestTradeResp           `json:"trades"`
	Summary         backtestSummaryResp            `json:"summary"`
	PendingSignal   *pendingSignalResp             `json:"pending_signal"`
}

// BacktestRequest 回测参数
type BacktestRequest struct {
	StrategyID string
	Symbol     string
	StartTime  int64
	EndTime    int64
	Capital    float64
	FeeRate    float64
	Options    map[string]bool
}

// getStrategies 策略列表
func (o *OpenAPI) getStrategies(c *wkhttp.Context) {
	c.Response(strategy.List())
}

// getStrategyBacktest 用策略回测时间段内的日线，返回每笔买卖记录与汇总
// 参数: strategy 策略ID(默认 bull_24x)，symbol/start_time/end_time 同K线接口，capital 初始资金(默认10000)，
// fee_rate 单边手续费率(默认0.001)，options 开启的策略开关，逗号分隔（见 /v1/strategies 的 options）
func (o *OpenAPI) getStrategyBacktest(c *wkhttp.Context) {
	q, err := parseKlineQuery(c)
	if err != nil {
		c.ResponseError(err)
		return
	}
	req := BacktestRequest{
		StrategyID: strings.TrimSpace(c.Query("strategy")),
		Symbol:     q.symbol,
		StartTime:  q.startTime,
		EndTime:    q.endTime,
		Capital:    defaultBacktestCapital,
		FeeRate:    strategy.DefaultFeeRate,
		Options:    map[string]bool{},
	}
	if req.StrategyID == "" {
		req.StrategyID = strategy.DefaultID
	}
	s, ok := strategy.Get(req.StrategyID)
	if !ok {
		c.ResponseError(errors.New("策略不存在"))
		return
	}
	if v := strings.TrimSpace(c.Query("capital")); v != "" {
		req.Capital, err = strconv.ParseFloat(v, 64)
		if err != nil || req.Capital <= 0 {
			c.ResponseError(errors.New("capital必须为正数"))
			return
		}
	}
	if v := strings.TrimSpace(c.Query("fee_rate")); v != "" {
		req.FeeRate, err = strconv.ParseFloat(v, 64)
		if err != nil || req.FeeRate < 0 || req.FeeRate > maxBacktestFeeRate {
			c.ResponseError(errors.New("fee_rate需在0到0.01之间"))
			return
		}
	}
	for _, key := range strings.Split(c.Query("options"), ",") {
		if key = strings.TrimSpace(key); key != "" {
			req.Options[key] = true
		}
	}

	// 策略指标在含预热数据的完整K线上计算，从区间内第一根开始交易
	klines, start, err := o.loadDailyKlinesWithWarmup(q.symbol, q.startTime, q.endTime, s.Info().WarmupBars)
	if err != nil {
		o.Error("策略回测查询K线失败", zap.String("symbol", q.symbol), zap.Error(err))
		c.ResponseError(errors.New("策略回测失败"))
		return
	}
	resp, err := RunBacktest(klines, start, req)
	if err != nil {
		c.ResponseError(err)
		return
	}
	c.Response(resp)
}

// RunBacktest 在已加载的日线上运行回测；klines 按时间升序且含预热数据，从下标 start 开始交易
func RunBacktest(klines []*Kline, start int, req BacktestRequest) (*BacktestResponse, error) {
	s, ok := strategy.Get(req.StrategyID)
	if !ok {
		return nil, fmt.Errorf("策略 %s 不存在", req.StrategyID)
	}
	info := s.Info()
	params := strategy.Params{FeeRate: req.FeeRate, Options: map[string]bool{}}
	for _, opt := range info.Options {
		params.Options[opt.Key] = req.Options[opt.Key] // 只接受策略定义的开关
	}

	candles := make([]strategy.Candle, len(klines))
	for i, k := range klines {
		candles[i] = strategy.Candle{
			Time: k.StartTime, Open: k.OpenPrice, High: k.HighPrice, Low: k.LowPrice, Close: k.ClosePrice, Volume: k.BaseVolume,
		}
	}
	result := strategy.Backtest(s, candles, start, req.Capital, params)

	resp := &BacktestResponse{
		StrategyID:      info.ID,
		Strategy:        info.Name,
		Market:          info.Market,
		MarketLabel:     info.MarketLabel,
		Notes:           info.Notes,
		Rules:           s.Rules(params),
		Options:         params.Options,
		ReasonLabels:    info.ReasonLabels,
		IndicatorLabels: info.Indicators,
		Symbol:          req.Symbol,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Trades:          make([]*backtestTradeResp, len(result.Trades)),
		Summary:         backtestSummaryResp(result.Summary),
	}
	for i, t := range result.Trades {
		tr := &backtestTradeResp{
			Action:     t.Action,
			Time:       t.Time,
			SignalTime: t.SignalTime,
			Price:      t.Price,
			Quantity:   t.Quantity,
			Amount:     t.Amount,
			Fee:        t.Fee,
			ReasonCode: t.ReasonCode,
			Reason:     t.Reason,
			Indicators: t.Indicators,
		}
		if t.Action == strategy.ActionSell {
			profit, rate := t.Profit, t.ProfitRate
			tr.Profit, tr.ProfitRate = &profit, &rate
		}
		resp.Trades[i] = tr
	}
	if p := result.Pending; p != nil {
		resp.PendingSignal = &pendingSignalResp{
			Action: p.Action, ReasonCode: p.Code, Reason: p.Reason, SignalTime: p.SignalTime, Indicators: p.Indicators,
		}
	}
	return resp, nil
}
