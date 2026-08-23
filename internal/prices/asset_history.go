package prices

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkando/kasa/internal/config"
)

// AssetHistory is one dashboard row's daily value history, in both bases.
type AssetHistory struct {
	Dates []time.Time
	TRY   []float64
	USD   []float64
}

// FetchAssetHistory builds the value series for a row key ("gold",
// "cash:<name>", "vadeli:<name>" or a stock symbol). rng is a Yahoo range
// ("1mo", "3mo", "1y"); deposits ignore it and project start→maturity.
func FetchAssetHistory(ctx context.Context, cfg *config.Config, key, rng string) (AssetHistory, error) {
	fx, err := yahooSeries(ctx, symUSDTRY, rng)
	if err != nil {
		return AssetHistory{}, err
	}
	fxAt := lookupFn(fx)

	build := func(dates []time.Time, try []float64) AssetHistory {
		h := AssetHistory{Dates: dates, TRY: try}
		h.USD = make([]float64, len(try))
		for i, d := range dates {
			if r := fxAt(d); r > 0 {
				h.USD[i] = try[i] / r
			}
		}
		return h
	}

	switch {
	case key == "fx": // USD/TRY rate itself
		return AssetHistory{Dates: fx.Dates, TRY: fx.Closes, USD: fx.Closes}, nil

	case key == "gold":
		g := cfg.Assets.Gold
		if g == nil {
			return AssetHistory{}, fmt.Errorf("altın tanımlı değil")
		}
		oz, err := yahooSeries(ctx, symGoldOz, rng)
		if err != nil {
			return AssetHistory{}, err
		}
		try := make([]float64, len(oz.Closes))
		for i, d := range oz.Dates {
			try[i] = g.Grams * oz.Closes[i] * fxAt(d) / ozToGram
		}
		return build(oz.Dates, try), nil

	case strings.HasPrefix(key, "cash:"):
		name := strings.TrimPrefix(key, "cash:")
		for _, c := range cfg.Assets.Cash {
			if c.Name != name {
				continue
			}
			try := make([]float64, len(fx.Dates))
			for i, d := range fx.Dates {
				units := c.Units(d)
				if c.Currency == "USD" {
					try[i] = units * fx.Closes[i]
				} else {
					try[i] = units
				}
			}
			return build(fx.Dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", name)

	case strings.HasPrefix(key, "prop:"):
		name := strings.TrimPrefix(key, "prop:")
		for _, pr := range cfg.Assets.Properties {
			if pr.Name != name {
				continue
			}
			try := make([]float64, len(fx.Dates))
			for i := range fx.Dates {
				if pr.Currency == "USD" {
					try[i] = pr.Value * fx.Closes[i]
				} else {
					try[i] = pr.Value
				}
			}
			return build(fx.Dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", name)

	case strings.HasPrefix(key, "crypto:"):
		name := strings.TrimPrefix(key, "crypto:")
		for _, cr := range cfg.Assets.Cryptos {
			if cr.Name != name {
				continue
			}
			s, err := coingeckoSeries(ctx, cr.CoinID, rangeDays(rng))
			if err != nil {
				return AssetHistory{}, err
			}
			try := make([]float64, len(s.Closes))
			for i, d := range s.Dates {
				try[i] = cr.Amount * s.Closes[i] * fxAt(d)
			}
			return build(s.Dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", name)

	case strings.HasPrefix(key, "loan:"):
		name := strings.TrimPrefix(key, "loan:")
		for _, l := range cfg.Assets.Loans {
			if l.Name != name {
				continue
			}
			try := make([]float64, len(fx.Dates))
			for i := range fx.Dates {
				v := l.Amount * l.Sign()
				if l.Currency == "USD" {
					v *= fx.Closes[i]
				}
				try[i] = v
			}
			return build(fx.Dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", name)

	case strings.HasPrefix(key, "vadeli:"):
		name := strings.TrimPrefix(key, "vadeli:")
		for _, dep := range cfg.Assets.Deposits {
			if dep.Name != name {
				continue
			}
			var dates []time.Time
			var try []float64
			for d := dep.Start.Time; !d.After(dep.Maturity.Time); d = d.AddDate(0, 0, 1) {
				dates = append(dates, d)
				try = append(try, dep.Value(d))
				if len(dates) > 800 { // guard absurd terms
					break
				}
			}
			return build(dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", name)

	default: // stock symbol
		for _, st := range cfg.Assets.Stocks {
			if st.Symbol != key {
				continue
			}
			s, err := yahooSeries(ctx, key, rng)
			if err != nil {
				return AssetHistory{}, err
			}
			try := make([]float64, len(s.Closes))
			for i, d := range s.Dates {
				v := st.Shares * s.Closes[i]
				if s.Currency == "USD" {
					v *= fxAt(d)
				}
				try[i] = v
			}
			return build(s.Dates, try), nil
		}
		return AssetHistory{}, fmt.Errorf("%q bulunamadı", key)
	}
}

// lookupFn forward-fills a series' closes over arbitrary dates.
func lookupFn(s Series) func(time.Time) float64 {
	return func(t time.Time) float64 {
		if len(s.Closes) == 0 {
			return 0
		}
		last := s.Closes[0]
		for i, d := range s.Dates {
			if d.After(t) {
				break
			}
			last = s.Closes[i]
		}
		return last
	}
}
