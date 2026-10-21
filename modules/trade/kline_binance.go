package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const spotKlineURL = "https://api.binance.com/api/v3/klines"

type SpotKline struct {
	OpenTime         int64
	Open             string
	High             string
	Low              string
	Close            string
	Volume           string
	CloseTime        int64
	QuoteVolume      string
	TradeCount       int64
	TakerBaseVolume  string
	TakerQuoteVolume string
}

func FetchSpotKlines(
	ctx context.Context,
	symbol string,
	interval string,
	startTime int64,
	limit int,
) ([]SpotKline, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}

	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)
	params.Set("limit", strconv.Itoa(limit))
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		spotKlineURL+"?"+params.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance HTTP status: %d", resp.StatusCode)
	}

	var rows [][]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}

	result := make([]SpotKline, 0, len(rows))
	for _, row := range rows {
		if len(row) < 12 {
			continue
		}

		var k SpotKline
		if err := json.Unmarshal(row[0], &k.OpenTime); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[1], &k.Open); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[2], &k.High); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[3], &k.Low); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[4], &k.Close); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[5], &k.Volume); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[6], &k.CloseTime); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[7], &k.QuoteVolume); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[8], &k.TradeCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[9], &k.TakerBaseVolume); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[10], &k.TakerQuoteVolume); err != nil {
			return nil, err
		}

		result = append(result, k)
	}

	return result, nil
}

/*ctx := context.Background()

klines, err := binance.FetchSpotKlines(ctx, "BTCUSDT", "1d", 0, 1000)
if err != nil {
panic(err)
}

fmt.Println("获取数量：", len(klines))
if len(klines) > 0 {
fmt.Printf("最新收盘价：%s\n", klines[len(klines)-1].Close)
}*/
