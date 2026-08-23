package portfolio

import (
	"sort"
	"time"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/prices"
)

// Reconstruct builds a daily TRY value history of the *current* holdings over
// the union of dates in hist, forward-filling gaps (BIST and US holidays differ).
// Returns the total series, per-row series keyed like Compute's Row.Key, and the
// USDTRY close per date (for re-basing the series into USD).
func Reconstruct(cfg *config.Config, hist map[string]prices.Series) (dates []time.Time, total []float64, perAsset map[string][]float64, fxVals []float64) {
	dateSet := map[string]time.Time{}
	for _, s := range hist {
		for _, d := range s.Dates {
			day := d.Truncate(24 * time.Hour)
			dateSet[day.Format("2006-01-02")] = day
		}
	}
	if len(dateSet) == 0 {
		return nil, nil, nil, nil
	}
	for _, d := range dateSet {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	lookup := func(sym string) func(time.Time) float64 {
		s, ok := hist[sym]
		if !ok || len(s.Closes) == 0 {
			return func(time.Time) float64 { return 0 }
		}
		return func(t time.Time) float64 {
			last := 0.0
			for i, d := range s.Dates {
				if d.After(t) {
					break
				}
				last = s.Closes[i]
			}
			if last == 0 {
				last = s.Closes[0]
			}
			return last
		}
	}

	fx := lookup(KeyUSDTRY)
	gram := lookup(KeyGram)
	stock := map[string]func(time.Time) float64{}
	for _, st := range cfg.Assets.Stocks {
		stock[st.Symbol] = lookup(st.Symbol)
	}

	perAsset = map[string][]float64{}
	appendVal := func(key string, v float64) { perAsset[key] = append(perAsset[key], v) }
	crypto := map[string]func(time.Time) float64{}
	for _, cr := range cfg.Assets.Cryptos {
		crypto[cr.Name] = lookup("crypto:" + cr.Name)
	}

	total = make([]float64, 0, len(dates))
	for _, d := range dates {
		sum := 0.0
		fxVals = append(fxVals, fx(d))
		for _, c := range cfg.Assets.Cash {
			v := c.Units(d)
			if c.Currency == "USD" {
				v *= fx(d)
			}
			appendVal("cash:"+c.Name, v)
			if !cfg.IsHidden("cash:" + c.Name) {
				sum += v
			}
		}
		if a := cfg.Assets.Gold; a != nil && a.Grams > 0 {
			v := a.Grams * gram(d)
			appendVal("gold", v)
			if !cfg.IsHidden("gold") {
				sum += v
			}
		}
		for _, l := range cfg.Assets.Loans {
			v := l.Amount * l.Sign()
			if l.Currency == "USD" {
				v *= fx(d)
			}
			appendVal("loan:"+l.Name, v)
			if !cfg.IsHidden("loan:" + l.Name) {
				sum += v
			}
		}
		for _, st := range cfg.Assets.Stocks {
			v := st.Shares * stock[st.Symbol](d)
			if cur := hist[st.Symbol].Currency; cur == "USD" {
				v *= fx(d)
			}
			appendVal(st.Symbol, v)
			if !cfg.IsHidden(st.Symbol) {
				sum += v
			}
		}
		for _, dep := range cfg.Assets.Deposits {
			v := dep.Value(d)
			appendVal("vadeli:"+dep.Name, v)
			if !cfg.IsHidden("vadeli:" + dep.Name) {
				sum += v
			}
		}
		for _, p := range cfg.Assets.Properties {
			v := p.Value
			if p.Currency == "USD" {
				v *= fx(d)
			}
			appendVal("prop:"+p.Name, v)
			if !cfg.IsHidden("prop:" + p.Name) {
				sum += v
			}
		}
		for _, cr := range cfg.Assets.Cryptos {
			v := cr.Amount * crypto[cr.Name](d) * fx(d)
			appendVal("crypto:"+cr.Name, v)
			if !cfg.IsHidden("crypto:" + cr.Name) {
				sum += v
			}
		}
		total = append(total, sum)
	}
	return dates, total, perAsset, fxVals
}
