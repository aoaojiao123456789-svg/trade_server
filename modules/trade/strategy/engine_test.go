package strategy

import (
	"math"
	"testing"
)

// fakeEval 按预设在指定K线给出信号，用于单独测试撮合引擎
type fakeEval struct {
	entries map[int]bool
	exits   map[int]bool
	stop    func(i int, pos *Position)
}

func (f *fakeEval) Entry(i int) Signal {
	if f.entries[i] {
		return Signal{ActionBuy, "b", "buy"}
	}
	return Signal{}
}

func (f *fakeEval) Exit(i int, _ *Position) Signal {
	if f.exits[i] {
		return Signal{ActionSell, "s", "sell"}
	}
	return Signal{}
}

func (f *fakeEval) Snapshot(i int) map[string]float64 { return map[string]float64{"i": float64(i)} }

func (f *fakeEval) UpdateStops(i int, pos *Position) {
	if f.stop != nil {
		f.stop(i, pos)
	}
}

// bars 构造开盘价依次为 opens 的K线，收盘 = 开盘，最高/最低 = 开盘 ±1
func bars(opens ...float64) []Candle {
	c := make([]Candle, len(opens))
	for i, o := range opens {
		c[i] = Candle{Time: int64(i), Open: o, High: o + 1, Low: o - 1, Close: o}
	}
	return c
}

func TestEngineFillsAtNextOpen(t *testing.T) {
	ev := &fakeEval{entries: map[int]bool{0: true}, exits: map[int]bool{2: true}}
	res := Simulate(ev, bars(100, 101, 110, 120, 130), 0, 1000, 0.001)
	if len(res.Trades) != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}
	buy, sell := res.Trades[0], res.Trades[1]
	if buy.Time != 1 || buy.SignalTime != 0 || buy.Price != 101 || buy.Indicators["i"] != 0 {
		t.Errorf("buy = %+v", buy)
	}
	if sell.Time != 3 || sell.SignalTime != 2 || sell.Price != 120 {
		t.Errorf("sell = %+v", sell)
	}
	// 买入扣 1，到手 999/101；卖出 120 再扣 0.1%
	want := 999.0 / 101 * 120 * 0.999
	if math.Abs(sell.Amount-want) > 1e-9 || math.Abs(res.Summary.TotalProfit-(want-1000)) > 1e-9 {
		t.Errorf("sell = %+v summary = %+v", sell, res.Summary)
	}
	if math.Abs(res.Summary.TotalFee-(1+999.0/101*120*0.001)) > 1e-9 || res.Summary.WinRate != 1 {
		t.Errorf("summary = %+v", res.Summary)
	}
}

func TestEngineNoPyramidingAndPending(t *testing.T) {
	// 持仓中的买入信号忽略；最后一根的卖出信号为待执行
	ev := &fakeEval{entries: map[int]bool{0: true, 2: true}, exits: map[int]bool{3: true}}
	res := Simulate(ev, bars(100, 100, 100, 100), 0, 1000, 0)
	if len(res.Trades) != 1 || !res.Summary.Holding {
		t.Fatalf("trades = %+v", res.Trades)
	}
	if res.Pending == nil || res.Pending.Action != ActionSell || res.Pending.SignalTime != 3 {
		t.Fatalf("pending = %+v", res.Pending)
	}
}

func TestEngineStartIndex(t *testing.T) {
	ev := &fakeEval{entries: map[int]bool{1: true}}
	if res := Simulate(ev, bars(100, 100, 100, 100), 2, 1000, 0); len(res.Trades) != 0 {
		t.Fatalf("预热期不应交易: %+v", res.Trades)
	}
}

func TestEngineStops(t *testing.T) {
	stop := func(price float64) func(int, *Position) {
		return func(_ int, pos *Position) {
			pos.SetStop(Stop{"low", price - 5, "低止损 %.2f"})
			pos.SetStop(Stop{"high", price, "高止损 %.2f"})
			pos.SetStop(Stop{"none", math.NaN(), "未挂单"})
		}
	}
	// 第1根开盘 100 买入，收盘挂止损 95/90；第2根最低 94 触发价高的 95
	c := bars(100, 100, 98, 98)
	c[2].Low = 94
	res := Simulate(&fakeEval{entries: map[int]bool{0: true}, stop: stop(95)}, c, 0, 1000, 0)
	if len(res.Trades) != 2 || res.Trades[1].Price != 95 || res.Trades[1].ReasonCode != "high" ||
		res.Trades[1].Reason != "高止损 95.00" || res.Trades[1].SignalTime != 2 {
		t.Fatalf("trades = %+v", res.Trades)
	}

	// 跳空低开到 80，按开盘价成交
	c = bars(100, 100, 80, 80)
	res = Simulate(&fakeEval{entries: map[int]bool{0: true}, stop: stop(95)}, c, 0, 1000, 0)
	if len(res.Trades) != 2 || res.Trades[1].Price != 80 {
		t.Fatalf("trades = %+v", res.Trades)
	}
}

func TestEngineHoldingAtEnd(t *testing.T) {
	// 第1根开盘 100 买入，最后收盘 120，按扣卖出手续费估算浮盈
	res := Simulate(&fakeEval{entries: map[int]bool{0: true}}, bars(100, 100, 120), 0, 1000, 0.001)
	s := res.Summary
	equity := 999.0 / 100 * 120 * 0.999
	if !s.Holding || s.FinalCapital != 1000 || math.Abs(s.Equity-equity) > 1e-9 || math.Abs(s.FloatingProfit-(equity-1000)) > 1e-9 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestRegistry(t *testing.T) {
	infos := List()
	if len(infos) < 2 {
		t.Fatalf("至少应有牛市、震荡市两个策略: %+v", infos)
	}
	markets := map[Market]bool{}
	for _, info := range infos {
		s, ok := Get(info.ID)
		if !ok || s.Info().ID != info.ID {
			t.Errorf("Get(%s) 失败", info.ID)
		}
		if info.Name == "" || info.MarketLabel == "" || info.WarmupBars <= 0 || len(info.ReasonLabels) == 0 || len(info.Indicators) == 0 {
			t.Errorf("策略 %s 元信息不完整: %+v", info.ID, info)
		}
		markets[info.Market] = true
	}
	if !markets[MarketBull] || !markets[MarketRange] {
		t.Errorf("markets = %v", markets)
	}
	if _, ok := Get(DefaultID); !ok {
		t.Error("默认策略不存在")
	}
	if _, ok := Get("not_exist"); ok {
		t.Error("不存在的策略应返回 false")
	}
}

func TestParamsOption(t *testing.T) {
	s := Bull24X{}
	if (Params{}).Option(s, OptionProtectiveStops) {
		t.Error("默认应关闭")
	}
	if !(Params{Options: map[string]bool{OptionProtectiveStops: true}}).Option(s, OptionProtectiveStops) {
		t.Error("应读取传入的开关")
	}
	if ExecutionRule(0.00075) == ExecutionRule(0.001) {
		t.Error("成交说明应包含费率")
	}
}
