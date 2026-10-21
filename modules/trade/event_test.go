package openapi

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// fakeBinance 模拟币安日线接口：从 startTime 起返回最多 limit 根，包含当天未收盘的一根
func fakeBinance(calls *int) klineFetcher {
	return func(_ context.Context, _, _ string, startTime int64, limit int) ([]SpotKline, error) {
		*calls++
		now := time.Now().UnixMilli()
		var rows []SpotKline
		for t := startTime; t <= now && len(rows) < limit; t += dayMillis {
			p := strconv.FormatInt(t/dayMillis, 10)
			rows = append(rows, SpotKline{
				OpenTime: t, CloseTime: t + dayMillis - 1,
				Open: p, High: p, Low: p, Close: p, Volume: "1", QuoteVolume: "1", TakerBaseVolume: "0", TakerQuoteVolume: "0",
			})
		}
		return rows, nil
	}
}

func collect(t *testing.T, from, until int64) ([]*Kline, int) {
	t.Helper()
	var saved []*Kline
	calls := 0
	err := fetchDailyKlineRange(context.Background(), fakeBinance(&calls), "BTCUSDT", from, until, func(k []*Kline) error {
		saved = append(saved, k...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return saved, calls
}

func todayStart() int64 {
	now := time.Now().UnixMilli()
	return now - now%dayMillis
}

func TestFetchDailyKlineRangeToNow(t *testing.T) {
	// 从 1500 天前开始：需要分两页，且不包含今天未收盘的日线
	from := todayStart() - 1500*dayMillis
	saved, calls := collect(t, from, 0)
	if len(saved) != 1500 || calls != 2 {
		t.Fatalf("saved = %d calls = %d, want 1500 / 2", len(saved), calls)
	}
	for i, k := range saved {
		if k.StartTime != from+int64(i)*dayMillis {
			t.Fatalf("第 %d 根不连续: %d", i, k.StartTime)
		}
	}
	if saved[len(saved)-1].StartTime != todayStart()-dayMillis {
		t.Fatal("最后一根应为昨天")
	}
}

func TestFetchDailyKlineRangeBackfill(t *testing.T) {
	// 向前补齐 [from, until)：不包含 until 当天
	from := todayStart() - 700*dayMillis
	until := todayStart() - 280*dayMillis
	saved, _ := collect(t, from, until)
	if len(saved) != 420 || saved[0].StartTime != from || saved[len(saved)-1].StartTime != until-dayMillis {
		t.Fatalf("saved = %d, first = %d, last = %d", len(saved), saved[0].StartTime, saved[len(saved)-1].StartTime)
	}
}

func TestFetchDailyKlineRangeNothingToDo(t *testing.T) {
	// 已同步到昨天：今天未收盘，不应请求
	if saved, calls := collect(t, todayStart(), 0); len(saved) != 0 || calls != 0 {
		t.Fatalf("saved = %d calls = %d", len(saved), calls)
	}
	// 区间为空
	if saved, calls := collect(t, todayStart()-10*dayMillis, todayStart()-10*dayMillis); len(saved) != 0 || calls != 0 {
		t.Fatalf("saved = %d calls = %d", len(saved), calls)
	}
}
