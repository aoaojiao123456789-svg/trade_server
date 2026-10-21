// Package strategy 日线交易策略及回测引擎
//
// 结构：
//   - engine.go：通用类型、策略接口、注册表、逐根K线撮合引擎（成交、手续费、止损、收益统计）
//   - 每个策略一个文件，只负责计算指标并给出买卖信号，新增策略后在 registry 中注册即可
//
// 撮合规则与 TradingView 默认一致：每根K线收盘时计算信号，市价单在下一根K线开盘价成交；
// 止损单在下一根K线内触发，开盘即低于止损价时按开盘价成交。只做多、满仓进出，持仓时不再买入，
// 买卖双边按手续费率扣费。
package strategy

import (
	"fmt"
	"math"
	"strings"
)

// DefaultFeeRate 默认手续费率，币安现货普通用户买卖各 0.1%
const DefaultFeeRate = 0.001

// Action 操作类型
type Action string

const (
	ActionNone Action = ""
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

// ReasonCode 买卖原因代码，由各策略定义，便于统计分析
type ReasonCode string

// Market 策略适用的市场环境
type Market string

const (
	MarketBull  Market = "bull"  // 牛市（趋势上涨）
	MarketRange Market = "range" // 震荡市
)

// MarketLabels 市场环境中文名
var MarketLabels = map[Market]string{
	MarketBull:  "牛市",
	MarketRange: "震荡市",
}

// Candle 策略需要的K线字段
type Candle struct {
	Time   int64 // 开盘时间（毫秒）
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// Params 回测参数
type Params struct {
	FeeRate float64         // 单边手续费率
	Options map[string]bool // 策略开关，键见 Info.Options
}

// Option 读取开关，未设置时取策略定义的默认值
func (p Params) Option(s Strategy, key string) bool {
	if v, ok := p.Options[key]; ok {
		return v
	}
	for _, o := range s.Info().Options {
		if o.Key == key {
			return o.Default
		}
	}
	return false
}

// OptionInfo 策略开关说明
type OptionInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

// IndicatorInfo 信号快照中指标字段的说明
type IndicatorInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Info 策略元信息
type Info struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Market       Market                `json:"market"`
	MarketLabel  string                `json:"market_label"`
	Source       string                `json:"source"`      // 来源（原始脚本文件）
	Description  string                `json:"description"` // 一句话介绍
	Notes        []string              `json:"notes"`       // 复刻说明、与原脚本的差异等
	Options      []OptionInfo          `json:"options"`
	ReasonLabels map[ReasonCode]string `json:"reason_labels"`
	Indicators   []IndicatorInfo       `json:"indicators"`  // 信号快照字段
	WarmupBars   int                   `json:"warmup_bars"` // 指标预热所需K线数
}

// Rules 策略规则说明（随参数变化）
type Rules struct {
	Indicators string `json:"indicators"`
	Buy        string `json:"buy"`
	Sell       string `json:"sell"`
	Execution  string `json:"execution"`
}

// Strategy 一个交易策略
type Strategy interface {
	Info() Info
	Rules(p Params) Rules
	// Prepare 在全部K线（含预热数据）上计算指标，返回逐根K线判断信号的 Evaluator
	Prepare(candles []Candle, p Params) Evaluator
}

// Evaluator 在第 i 根K线收盘时判断信号
type Evaluator interface {
	// Entry 空仓时的买入信号
	Entry(i int) Signal
	// Exit 持仓时的卖出信号（下一根开盘市价全平）
	Exit(i int, pos *Position) Signal
	// Snapshot 第 i 根K线的价格与指标，跳过数据不足的指标
	Snapshot(i int) map[string]float64
}

// StopUpdater 需要挂止损单的策略实现此接口：持仓且无卖出信号时，在收盘后更新 pos.Stops
type StopUpdater interface {
	UpdateStops(i int, pos *Position)
}

// Signal 某根K线收盘时产生的信号
type Signal struct {
	Action Action
	Code   ReasonCode
	Reason string
}

// Stop 止损单，价格为 NaN 表示未挂单；多个止损单同时存在时价高者先触发
type Stop struct {
	Code   ReasonCode
	Price  float64
	Reason string // 触发时的说明，%.2f 处填入止损价
}

// Position 当前持仓
type Position struct {
	EntryIndex int
	EntryPrice float64
	Quantity   float64 // 扣除买入手续费后实际持有数量
	Cost       float64 // 买入花费的资金（含手续费）
	Stops      map[ReasonCode]Stop
}

// SetStop 挂出或更新止损单
func (p *Position) SetStop(s Stop) {
	if p.Stops == nil {
		p.Stops = map[ReasonCode]Stop{}
	}
	p.Stops[s.Code] = s
}

func (p *Position) activeStop() (Stop, bool) {
	var best Stop
	found := false
	for _, s := range p.Stops {
		if math.IsNaN(s.Price) {
			continue
		}
		if !found || s.Price > best.Price {
			best, found = s, true
		}
	}
	return best, found
}

// Trade 一条成交记录
type Trade struct {
	Action     Action
	Time       int64   // 成交K线的开盘时间
	SignalTime int64   // 产生信号的K线开盘时间（市价单为前一根，止损单为成交当根）
	Price      float64 // 成交价
	Quantity   float64 // 成交数量（买入为扣费后到手数量）
	Amount     float64 // 资金变动：买入为花费资金（含手续费），卖出为扣费后到手资金
	Fee        float64
	ReasonCode ReasonCode
	Reason     string
	Indicators map[string]float64 // 信号K线的价格与指标
	// 以下仅卖出记录有效
	Profit     float64 // 本笔净收益 = 卖出到手资金 - 买入花费资金
	ProfitRate float64 // 本笔净收益率
}

// PendingSignal 最后一根K线收盘产生、将在下一根K线开盘执行的信号
type PendingSignal struct {
	Action     Action
	Code       ReasonCode
	Reason     string
	SignalTime int64
	Indicators map[string]float64
}

// Summary 回测汇总，收益均已扣除手续费
type Summary struct {
	InitialCapital  float64
	FeeRate         float64
	TotalFee        float64
	FinalCapital    float64 // 已平仓后的资金（不含未平仓浮动盈亏）
	TotalProfit     float64
	TotalReturnRate float64
	TradeCount      int
	WinCount        int
	WinRate         float64
	// 区间结束时仍持仓：按最后收盘价卖出（扣卖出手续费）估算
	Holding            bool
	FloatingProfit     float64
	FloatingProfitRate float64
	Equity             float64
	EquityReturnRate   float64
}

// Result 回测结果
type Result struct {
	Trades  []Trade
	Summary Summary
	Pending *PendingSignal
}

// ================= 注册表 =================

// DefaultID 默认策略
const DefaultID = "bull_24x"

var registry = []Strategy{
	Bull24X{},
	RangeEMAMACD{},
}

// List 全部策略的元信息
func List() []Info {
	infos := make([]Info, len(registry))
	for i, s := range registry {
		infos[i] = s.Info()
	}
	return infos
}

// Get 按 ID 获取策略
func Get(id string) (Strategy, bool) {
	for _, s := range registry {
		if s.Info().ID == id {
			return s, true
		}
	}
	return nil, false
}

// ExecutionRule 通用的成交规则说明
func ExecutionRule(feeRate float64) string {
	fee := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", feeRate*100), "0"), ".")
	return fmt.Sprintf("每天收盘时判断信号，次日开盘价满仓成交（与 TradingView 一致）；只做多，持仓未卖出前不再买入；买卖各收 %s%% 手续费，收益均为扣费后净值；暂不计滑点", fee)
}

// ================= 撮合引擎 =================

type order struct {
	sig        Signal
	signalTime int64
	snapshot   map[string]float64
}

// Backtest 用策略回测；candles 含指标预热数据，从 startIndex 开始交易
func Backtest(s Strategy, candles []Candle, startIndex int, initialCapital float64, p Params) Result {
	return Simulate(s.Prepare(candles, p), candles, startIndex, initialCapital, p.FeeRate)
}

// Simulate 逐根K线模拟交易
func Simulate(ev Evaluator, candles []Candle, startIndex int, initialCapital, feeRate float64) Result {
	res := Result{Trades: []Trade{}}
	s := &res.Summary
	cash := initialCapital
	var pos *Position
	var pending *order
	stops, _ := ev.(StopUpdater)

	buy := func(i int, price float64, o *order) {
		fee := cash * feeRate
		pos = &Position{EntryIndex: i, EntryPrice: price, Quantity: (cash - fee) / price, Cost: cash}
		res.Trades = append(res.Trades, Trade{
			Action: ActionBuy, Time: candles[i].Time, SignalTime: o.signalTime, Price: price,
			Quantity: pos.Quantity, Amount: cash, Fee: fee,
			ReasonCode: o.sig.Code, Reason: o.sig.Reason, Indicators: o.snapshot,
		})
		s.TotalFee += fee
		cash = 0
	}
	sell := func(i int, price float64, o *order) {
		gross := pos.Quantity * price
		fee := gross * feeRate
		proceeds := gross - fee
		t := Trade{
			Action: ActionSell, Time: candles[i].Time, SignalTime: o.signalTime, Price: price,
			Quantity: pos.Quantity, Amount: proceeds, Fee: fee,
			ReasonCode: o.sig.Code, Reason: o.sig.Reason, Indicators: o.snapshot,
			Profit: proceeds - pos.Cost, ProfitRate: proceeds/pos.Cost - 1,
		}
		res.Trades = append(res.Trades, t)
		s.TotalFee += fee
		s.TradeCount++
		if t.Profit > 0 {
			s.WinCount++
		}
		cash = proceeds
		pos = nil
	}

	for i := startIndex; i < len(candles); i++ {
		c := candles[i]

		// 1. 上一根收盘产生的市价单，按本根开盘价成交
		if pending != nil {
			if pending.sig.Action == ActionBuy && pos == nil {
				buy(i, c.Open, pending)
			} else if pending.sig.Action == ActionSell && pos != nil {
				sell(i, c.Open, pending)
			}
			pending = nil
		}

		// 2. 盘中检查止损单（上一根收盘时挂出）
		if pos != nil {
			if stop, ok := pos.activeStop(); ok && c.Low <= stop.Price {
				price := math.Min(stop.Price, c.Open) // 跳空低开按开盘价成交
				sig := Signal{ActionSell, stop.Code, fmt.Sprintf(stop.Reason, stop.Price)}
				sell(i, price, &order{sig: sig, signalTime: c.Time, snapshot: ev.Snapshot(i)})
			}
		}

		// 3. 本根收盘计算信号
		var sig Signal
		if pos == nil {
			sig = ev.Entry(i)
		} else {
			sig = ev.Exit(i, pos)
			if sig.Action == ActionNone && stops != nil {
				stops.UpdateStops(i, pos)
			}
		}
		if sig.Action != ActionNone {
			pending = &order{sig: sig, signalTime: c.Time, snapshot: ev.Snapshot(i)}
		}
	}

	if pending != nil {
		res.Pending = &PendingSignal{
			Action: pending.sig.Action, Code: pending.sig.Code, Reason: pending.sig.Reason,
			SignalTime: pending.signalTime, Indicators: pending.snapshot,
		}
	}

	s.InitialCapital = initialCapital
	s.FeeRate = feeRate
	s.FinalCapital = cash
	s.Equity = cash
	if pos != nil && len(candles) > 0 {
		last := candles[len(candles)-1].Close
		liquidation := pos.Quantity * last * (1 - feeRate)
		s.Holding = true
		s.FinalCapital = pos.Cost
		s.FloatingProfit = liquidation - pos.Cost
		s.FloatingProfitRate = liquidation/pos.Cost - 1
		s.Equity = liquidation
	}
	s.TotalProfit = s.FinalCapital - initialCapital
	if initialCapital > 0 {
		s.TotalReturnRate = s.FinalCapital/initialCapital - 1
		s.EquityReturnRate = s.Equity/initialCapital - 1
	}
	if s.TradeCount > 0 {
		s.WinRate = float64(s.WinCount) / float64(s.TradeCount)
	}
	return res
}

// snapshot 生成信号快照的辅助函数：价格字段 + 有效的指标值
func snapshot(c Candle, i int, series map[string][]float64) map[string]float64 {
	m := map[string]float64{
		"open": c.Open, "high": c.High, "low": c.Low, "close": c.Close, "volume": c.Volume,
	}
	for k, s := range series {
		if i < len(s) && !math.IsNaN(s[i]) {
			m[k] = s[i]
		}
	}
	return m
}
