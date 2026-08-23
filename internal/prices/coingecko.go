package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// coingeckoQuotes fetches USD spot + 24h change for a set of CoinGecko ids.
func coingeckoQuotes(ctx context.Context, ids []string) (map[string]Quote, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	u := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true",
		url.QueryEscape(strings.Join(ids, ",")))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw map[string]struct {
		USD    float64 `json:"usd"`
		Change float64 `json:"usd_24h_change"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make(map[string]Quote, len(raw))
	for id, v := range raw {
		if v.USD == 0 {
			continue
		}
		out[id] = Quote{
			Symbol: id, Price: v.USD,
			PrevClose: v.USD / (1 + v.Change/100),
			Currency:  "USD", At: time.Now(),
		}
	}
	return out, nil
}

// coingeckoSeries fetches daily USD closes for a coin id.
func coingeckoSeries(ctx context.Context, id string, days int) (Series, error) {
	u := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s/market_chart?vs_currency=usd&days=%d&interval=daily",
		url.PathEscape(id), days)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Series{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Series{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		Prices [][2]float64 `json:"prices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Series{}, err
	}
	s := Series{Symbol: id, Currency: "USD"}
	for _, p := range raw.Prices {
		s.Dates = append(s.Dates, time.UnixMilli(int64(p[0])))
		s.Closes = append(s.Closes, p[1])
	}
	if len(s.Closes) == 0 {
		return s, fmt.Errorf("coingecko %s: veri yok", id)
	}
	return s, nil
}

func rangeDays(rng string) int {
	switch rng {
	case "3mo":
		return 90
	case "1y":
		return 365
	default:
		return 30
	}
}
