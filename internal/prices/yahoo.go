package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	KeyUSDTRY = "USDTRY"
	KeyGram   = "GRAM"

	symUSDTRY = "TRY=X"
	symGoldOz = "GC=F"
	ozToGram  = 31.1034768
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"
)

var yahooHosts = []string{"query1.finance.yahoo.com", "query2.finance.yahoo.com"}

var httpClient = &http.Client{Timeout: 10 * time.Second}

type yahooResp struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"previousClose"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				ShortName          string  `json:"shortName"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
				Adjclose []struct {
					Adjclose []*float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func yahooChart(ctx context.Context, symbol, rng string) (*yahooResp, error) {
	var lastErr error
	for _, host := range yahooHosts {
		url := fmt.Sprintf("https://%s/v8/finance/chart/%s?range=%s&interval=1d", host, symbol, rng)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var out yahooResp
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", symbol, err)
			continue
		}
		if out.Chart.Error != nil {
			return nil, fmt.Errorf("%s: yahoo: %s", symbol, out.Chart.Error.Description)
		}
		if len(out.Chart.Result) == 0 {
			lastErr = fmt.Errorf("%s: yahoo: empty result", symbol)
			continue
		}
		return &out, nil
	}
	return nil, lastErr
}

func yahooQuote(ctx context.Context, symbol string) (Quote, error) {
	r, err := yahooChart(ctx, symbol, "1d")
	if err != nil {
		return Quote{}, err
	}
	m := r.Chart.Result[0].Meta
	prev := m.PreviousClose
	if prev == 0 {
		prev = m.ChartPreviousClose
	}
	return Quote{
		Symbol:    symbol,
		Name:      m.ShortName,
		Price:     m.RegularMarketPrice,
		PrevClose: prev,
		Currency:  m.Currency,
		At:        time.Now(),
	}, nil
}

func yahooSeries(ctx context.Context, symbol, rng string) (Series, error) {
	r, err := yahooChart(ctx, symbol, rng)
	if err != nil {
		return Series{}, err
	}
	res := r.Chart.Result[0]
	closes := res.Indicators.Quote[0].Close
	// Prefer adjusted closes for equities (splits/dividends).
	if len(res.Indicators.Adjclose) > 0 && len(res.Indicators.Adjclose[0].Adjclose) == len(closes) {
		closes = res.Indicators.Adjclose[0].Adjclose
	}
	s := Series{Symbol: symbol, Currency: res.Meta.Currency}
	for i, ts := range res.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue // halted/missing bars come back as null
		}
		s.Dates = append(s.Dates, time.Unix(ts, 0))
		s.Closes = append(s.Closes, *closes[i])
	}
	if len(s.Closes) == 0 {
		return s, fmt.Errorf("%s: yahoo: no closes", symbol)
	}
	return s, nil
}
