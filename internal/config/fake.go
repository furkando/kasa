package config

import (
	"fmt"
	"math/rand/v2"
	"os"
	"time"
)

// Demo builds a randomized, plausible sample portfolio — for sharing
// screenshots without exposing real numbers. Symbols are real so live
// prices still work.
func Demo() *Config {
	rnd := func(lo, hi float64) float64 { return lo + rand.Float64()*(hi-lo) }
	round := func(v, step float64) float64 { return float64(int(v/step)) * step }

	stockPool := []struct {
		sym  string
		cost float64
	}{
		{"AAPL", 210}, {"MSFT", 420}, {"NVDA", 120}, {"VOO", 520},
		{"QQQ", 480}, {"AMZN", 190}, {"THYAO.IS", 260}, {"ASELS.IS", 70},
	}
	rand.Shuffle(len(stockPool), func(i, j int) { stockPool[i], stockPool[j] = stockPool[j], stockPool[i] })
	var stocks []Stock
	for _, p := range stockPool[:3+rand.IntN(2)] {
		stocks = append(stocks, Stock{
			Symbol: p.sym,
			Shares: round(rnd(5, 120), 1),
			Cost:   round(p.cost*rnd(0.7, 1.05), 0.5),
		})
	}

	today := Today()
	start := Date{today.AddDate(0, 0, -rand.IntN(20))}
	dep := Deposit{
		Name: "Vadeli", Principal: round(rnd(250_000, 1_500_000), 50_000),
		GrossRate: round(rnd(0.38, 0.46), 0.005), Stopaj: 0.175,
		Start: start, Maturity: Date{start.AddDate(0, 0, 32)},
	}

	loanKind := "alacak"
	loanName := "Alacak"
	if rand.IntN(2) == 0 {
		loanKind, loanName = "borc", "Borç"
	}

	c := &Config{
		RefreshSeconds: 30,
		Assets: Assets{
			Cash: []Cash{
				{Name: "Banka TL", Currency: "TRY", Amount: round(rnd(40_000, 400_000), 1000)},
				{Name: "USD Nakit", Currency: "USD", Amount: round(rnd(3_000, 40_000), 500),
					APY: 0.03, AsOf: Date{today.AddDate(0, 0, -rand.IntN(60))}},
			},
			Gold:   &Gold{Grams: round(rnd(50, 400), 10), CostTRY: round(rnd(300_000, 2_000_000), 10_000)},
			Stocks: stocks,
			Cryptos: []Crypto{
				{Name: "BTC", Symbol: "BTC", CoinID: "bitcoin",
					Amount: round(rnd(0.05, 1.5), 0.01), CostUSD: round(rnd(40_000, 90_000), 500)},
			},
			Properties: []Property{
				{Name: "Ev", Currency: "TRY", Value: round(rnd(2_000_000, 9_000_000), 250_000), Category: "gayrimenkul"},
			},
			Deposits: []Deposit{dep},
			Loans: []Loan{
				{Name: loanName, Currency: "USD", Amount: round(rnd(2_000, 30_000), 500), Kind: loanKind,
					Due: Date{today.AddDate(0, rand.IntN(6)+1, 0)}},
			},
		},
	}
	// occasionally hide something so the GİZLİ section shows up in demos
	if rand.IntN(2) == 0 {
		c.Hidden = []string{"crypto:BTC"}
	}
	return c
}

// DemoPath writes a demo config to a temp file and returns its path —
// edits made in demo mode land there, never in the real config.
func DemoPath(c *Config) (string, error) {
	path := fmt.Sprintf("%s/kasa-demo-%d.yaml", os.TempDir(), time.Now().UnixNano())
	if err := Save(c, path); err != nil {
		return "", err
	}
	return path, nil
}
