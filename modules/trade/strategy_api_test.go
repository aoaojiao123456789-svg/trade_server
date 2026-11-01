package openapi

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/trade/strategy"
)

// sineKlines 长短周期正弦叠加的日线，便于产生买卖信号（震荡市策略在长波上涨途中靠短波触发 EMA 金叉）
func sineKlines(n int) []*Kline {
	klines := make([]*Kline, n)
	for i := range klines {
		p := 100 + 15*math.Sin(float64(i)/20) + 5*math.Sin(float64(i)/3) + float64(i)*0.05
		klines[i] = &Kline{StartTime: int64(i) * dayMillis, Symbol: "BTCUSDT", OpenPrice: p - 0.5,
			HighPrice: p + 2, LowPrice: p - 2, ClosePrice: p, BaseVolume: 10 + float64(i%7)}
	}
	return klines
}

func TestRunBacktestAllStrategies(t *testing.T) {
	klines := sineKlines(800)
	for _, info := range strategy.List() {
		resp, err := RunBacktest(klines, 320, BacktestRequest{
			StrategyID: info.ID, Symbol: "BTCUSDT", Capital: 1000, FeeRate: 0.001,
			Options: map[string]bool{"unknown_option": true},
		})
		if err != nil {
			t.Fatalf("%s: %v", info.ID, err)
		}
		if resp.StrategyID != info.ID || resp.Rules.Buy == "" || resp.MarketLabel == "" || len(resp.ReasonLabels) == 0 {
			t.Errorf("%s: 元信息缺失 %+v", info.ID, resp)
		}
		if _, ok := resp.Options["unknown_option"]; ok {
			t.Errorf("%s: 不应接受策略未定义的开关", info.ID)
		}
		if len(resp.Trades) == 0 {
			t.Errorf("%s: 长短周期叠加的行情应产生交易", info.ID)
		}
		for _, tr := range resp.Trades {
			if _, ok := resp.ReasonLabels[tr.ReasonCode]; !ok {
				t.Errorf("%s: 原因 %s 缺少中文标签", info.ID, tr.ReasonCode)
			}
		}

		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("%s: %v", info.ID, err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		days := 0
		for _, seg := range resp.Regimes {
			days += seg.Days
			if seg.Label == "" {
				t.Errorf("%s: 市场阶段缺少中文标签 %+v", info.ID, seg)
			}
		}
		if days != len(klines)-320 {
			t.Errorf("%s: 市场阶段应覆盖整个区间，实际 %d 天", info.ID, days)
		}
		for _, key := range []string{"strategy_id", "market", "rules", "options", "reason_labels", "indicator_labels", "trades", "summary", "pending_signal", "notes", "regimes"} {
			if _, ok := m[key]; !ok {
				t.Errorf("%s: JSON 缺少字段 %s", info.ID, key)
			}
		}
	}
}

func TestRunBacktestOptionsAndUnknownStrategy(t *testing.T) {
	klines := sineKlines(400)
	resp, err := RunBacktest(klines, 150, BacktestRequest{
		StrategyID: "bull_24x", Capital: 1000, Options: map[string]bool{strategy.OptionProtectiveStops: true},
	})
	if err != nil || !resp.Options[strategy.OptionProtectiveStops] {
		t.Fatalf("开关应生效: %v %+v", err, resp.Options)
	}
	if _, err := RunBacktest(klines, 150, BacktestRequest{StrategyID: "not_exist", Capital: 1000}); err == nil {
		t.Fatal("不存在的策略应报错")
	}
}
