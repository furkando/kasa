package prices

import (
	"context"
	"sync"
	"time"

	"github.com/furkando/kasa/internal/config"
)

// FetchQuotes pulls live quotes for everything cfg needs, in parallel.
// USDTRY is always fetched (needed for totals); gram gold falls back to
// GC=F × USDTRY when truncgil is down.
func FetchQuotes(ctx context.Context, cfg *config.Config) (map[string]Quote, error) {
	out := map[string]Quote{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	run := func(key string, f func(context.Context) (Quote, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, err := f(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			q.Symbol = key
			out[key] = q
		}()
	}

	run(KeyUSDTRY, func(ctx context.Context) (Quote, error) { return yahooQuote(ctx, symUSDTRY) })

	if g := cfg.Assets.Gold; g != nil && g.Grams > 0 {
		run(KeyGram, gramGoldQuote)
	}
	for _, st := range cfg.Assets.Stocks {
		sym := st.Symbol
		run(sym, func(ctx context.Context) (Quote, error) { return yahooQuote(ctx, sym) })
	}
	if len(cfg.Assets.Cryptos) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := make([]string, 0, len(cfg.Assets.Cryptos))
			for _, cr := range cfg.Assets.Cryptos {
				ids = append(ids, cr.CoinID)
			}
			byID, err := coingeckoQuotes(ctx, ids)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			for _, cr := range cfg.Assets.Cryptos {
				if q, ok := byID[cr.CoinID]; ok {
					q.Symbol = "crypto:" + cr.Name
					out["crypto:"+cr.Name] = q
				}
			}
		}()
	}
	wg.Wait()

	// Gold fallback via COMEX futures if truncgil failed but FX succeeded.
	if g := cfg.Assets.Gold; g != nil && g.Grams > 0 {
		if _, ok := out[KeyGram]; !ok {
			if fx, ok := out[KeyUSDTRY]; ok {
				if oz, err := yahooQuote(ctx, symGoldOz); err == nil {
					out[KeyGram] = Quote{
						Symbol: KeyGram, Name: "Gram Altın (GC=F)",
						Price:     oz.Price * fx.Price / ozToGram,
						PrevClose: oz.PrevClose * fx.PrevClose / ozToGram,
						Currency:  "TRY", At: time.Now(),
					}
					firstErr = nil
				}
			}
		}
	}

	if _, ok := out[KeyUSDTRY]; !ok {
		return nil, firstErr
	}
	// Partial data beats no data: only fail hard when FX itself is missing;
	// otherwise pass the first error up as a warning next to the data.
	return out, firstErr
}

// FetchHistories pulls ~days of daily closes for sparklines. Gram gold history
// is derived from GC=F × USDTRY (consistent basis ⇒ accurate daily returns,
// absolute level ~1–2% off Turkish spot — fine for charts).
func FetchHistories(ctx context.Context, cfg *config.Config, days int) (map[string]Series, error) {
	rng := "1mo"
	if days > 31 {
		rng = "3mo"
	}

	out := map[string]Series{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	run := func(key, symbol string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := yahooSeries(ctx, symbol, rng)
			if err != nil {
				return
			}
			s.Symbol = key
			mu.Lock()
			out[key] = s
			mu.Unlock()
		}()
	}

	run(KeyUSDTRY, symUSDTRY)
	if g := cfg.Assets.Gold; g != nil && g.Grams > 0 {
		run("GOLD_OZ", symGoldOz)
	}
	for _, st := range cfg.Assets.Stocks {
		run(st.Symbol, st.Symbol)
	}
	for _, cr := range cfg.Assets.Cryptos {
		cr := cr
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := coingeckoSeries(ctx, cr.CoinID, days)
			if err != nil {
				return
			}
			s.Symbol = "crypto:" + cr.Name
			mu.Lock()
			out["crypto:"+cr.Name] = s
			mu.Unlock()
		}()
	}
	wg.Wait()

	if oz, ok := out["GOLD_OZ"]; ok {
		if fx, ok := out[KeyUSDTRY]; ok {
			out[KeyGram] = deriveGramSeries(oz, fx)
		}
		delete(out, "GOLD_OZ")
	}
	return out, nil
}

func deriveGramSeries(oz, fx Series) Series {
	fxAt := func(t time.Time) float64 {
		last := fx.Closes[0]
		for i, d := range fx.Dates {
			if d.After(t) {
				break
			}
			last = fx.Closes[i]
		}
		return last
	}
	s := Series{Symbol: KeyGram, Currency: "TRY"}
	for i, d := range oz.Dates {
		s.Dates = append(s.Dates, d)
		s.Closes = append(s.Closes, oz.Closes[i]*fxAt(d)/ozToGram)
	}
	return s
}
