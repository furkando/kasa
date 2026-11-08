package ui

import (
	"strings"
	"testing"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/portfolio"
)

func rowByKey(t *testing.T, m Model, key string) portfolio.Row {
	t.Helper()
	for _, r := range m.sum.Rows {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("row %q not found", key)
	return portfolio.Row{}
}

func TestEntryPrice(t *testing.T) {
	m, cfg, _ := newTestModel(t)
	cfg.Assets.Gold.CostTRY = 1_500_000 // 250 gr
	cfg.Assets.Cash = []config.Cash{
		{Name: "USD", Currency: "USD", Amount: 1000, CostTRY: 40_000},
		{Name: "USD2", Currency: "USD", Amount: 1000},
		{Name: "TL", Currency: "TRY", Amount: 1000},
	}
	cfg.Assets.Properties = []config.Property{
		{Name: "Ev", Currency: "USD", Value: 100, Cost: 80},
		{Name: "Arsa", Currency: "TRY", Value: 100},
	}
	cfg.Assets.Cryptos = []config.Crypto{{Name: "BTC", Symbol: "BTC", Amount: 1, CostUSD: 60_000}}
	cfg.Assets.Loans = []config.Loan{{Name: "ali", Currency: "USD", Amount: 5, Kind: "alacak"}}
	m.recompute()

	cases := map[string]string{
		"gold":             "₺6.000,00/gr",
		"cash:USD":         "₺40,00/$",
		"cash:USD2":        "— (e ile girilir)",
		"cash:TL":          "",
		"prop:Ev":          "$80",
		"prop:Arsa":        "— (e ile girilir)",
		"crypto:BTC":       "$60.000,00",
		"loan:ali":         "",
		"vadeli:İş Vadeli": "",
	}
	for key, want := range cases {
		if got := m.entryCell(rowByKey(t, m, key)); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

func TestDetailPageShowsGoldEntryPrice(t *testing.T) {
	m, cfg, _ := newTestModel(t)
	cfg.Assets.Gold.CostTRY = 1_500_000
	m.recompute()
	m.detailKey = "gold"

	out := m.detailStats(rowByKey(t, m, "gold"), false, 100)
	if !strings.Contains(out, "giriş fiyatı") || !strings.Contains(out, "₺6.000,00/gr") {
		t.Fatalf("entry price missing:\n%s", out)
	}
}
