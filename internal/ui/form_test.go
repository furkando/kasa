package ui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/portfolio"
	"github.com/furkando/kasa/internal/prices"
	"github.com/furkando/kasa/internal/store"
)

const formTestYAML = `refresh_seconds: 30
assets:
  gold:
    grams: 250
  deposits:
    - name: İş Vadeli
      principal: 750000
      gross_rate: 0.38
      stopaj: 0.175
      start: 2026-08-17
      maturity: 2026-09-18
`

func newTestModel(t *testing.T) (Model, *config.Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portfolio.yaml")
	if err := os.WriteFile(path, []byte(formTestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KASA_CONFIG", path)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg, &store.Store{})
	m.Sender = func(tea.Msg) {}
	m.quotes = map[string]prices.Quote{
		prices.KeyUSDTRY: {Price: 47, PrevClose: 47},
		prices.KeyGram:   {Price: 6700, PrevClose: 6700},
	}
	m.width, m.height = 100, 30
	m.recompute()
	return m, cfg, path
}

func stepper(t *testing.T, m *Model) func(tea.Msg) {
	t.Helper()
	return func(msg tea.Msg) {
		mod, _ := m.Update(msg)
		*m = mod.(Model)
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(step func(tea.Msg), s string) {
	for _, r := range s {
		step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// selectRow moves the dashboard selection to the row with the given key.
func selectRow(t *testing.T, m *Model, step func(tea.Msg), rowKey string) {
	t.Helper()
	for i, r := range m.sum.Rows {
		if r.Key == rowKey {
			for m.sel < i {
				step(key("down"))
			}
			return
		}
	}
	t.Fatalf("row %q not found", rowKey)
}

func TestDepositRateEdit(t *testing.T) {
	m, cfg, path := newTestModel(t)
	step := stepper(t, &m)

	selectRow(t, &m, step, "vadeli:İş Vadeli")
	step(key("e"))
	if m.mode != modeForm {
		t.Fatal("expected form mode")
	}
	// fields: ad(0) anapara(1) brüt(2) stopaj(3) başlangıç(4) vade(5)
	step(key("down"))
	step(key("down")) // brüt faiz
	step(key("enter"))
	m.form.input.SetValue("45")
	step(key("enter"))
	step(key("s"))
	if m.mode != modeDash {
		t.Fatalf("form did not save: %s", m.form.err)
	}
	d := cfg.Assets.Deposits[0]
	if d.GrossRate != 0.45 || d.Stopaj != 0.175 {
		t.Fatalf("rates = %v / %v", d.GrossRate, d.Stopaj)
	}
	if net := d.Net(); math.Abs(net-0.45*0.825) > 1e-9 {
		t.Fatalf("net = %v", net)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "gross_rate: 0.45") {
		t.Fatalf("yaml not updated:\n%s", b)
	}
}

func TestAddAndDeleteStock(t *testing.T) {
	m, cfg, path := newTestModel(t)
	step := stepper(t, &m)

	// add: n → h → symbol msft → adet 10 → save
	step(key("n"))
	step(key("h"))
	if m.mode != modeForm {
		t.Fatal("expected stock form")
	}
	step(key("enter"))
	typeText(step, "msft")
	step(key("enter")) // commit symbol, advance to adet
	step(key("enter")) // edit adet
	typeText(step, "10")
	step(key("enter"))
	step(key("s"))
	if m.mode != modeDash {
		t.Fatalf("form did not save: %s", m.form.err)
	}
	if len(cfg.Assets.Stocks) != 1 || cfg.Assets.Stocks[0].Symbol != "MSFT" || cfg.Assets.Stocks[0].Shares != 10 {
		t.Fatalf("stocks = %+v", cfg.Assets.Stocks)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "MSFT") {
		t.Fatalf("yaml missing MSFT:\n%s", b)
	}

	// delete it: select row, x, y
	m.quotes["MSFT"] = prices.Quote{Price: 500, PrevClose: 500, Currency: "USD"}
	m.recompute()
	selectRow(t, &m, step, "MSFT")
	step(key("x"))
	if m.confirmDel != "MSFT" {
		t.Fatalf("confirmDel = %q", m.confirmDel)
	}
	step(key("y"))
	if len(cfg.Assets.Stocks) != 0 {
		t.Fatalf("stock not deleted: %+v", cfg.Assets.Stocks)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "MSFT") {
		t.Fatalf("yaml still has MSFT:\n%s", b)
	}
}

func TestSettingsForm(t *testing.T) {
	m, cfg, _ := newTestModel(t)
	step := stepper(t, &m)

	step(key("o"))
	if m.mode != modeForm {
		t.Fatal("expected settings form")
	}
	step(key("down"))  // ai sağlayıcı
	step(key("enter")) // otomatik → claude
	step(key("s"))
	if m.mode != modeDash {
		t.Fatalf("settings did not save: %s", m.form.err)
	}
	if cfg.AI.Provider != "claude" {
		t.Fatalf("provider = %q", cfg.AI.Provider)
	}
}

func TestLegacyNetRateStillWorks(t *testing.T) {
	d := config.Deposit{NetRate: 0.3382}
	if d.Net() != 0.3382 {
		t.Fatalf("legacy net = %v", d.Net())
	}
}

func TestSortCycle(t *testing.T) {
	m, _, _ := newTestModel(t)
	step := stepper(t, &m)

	// gold falls today (-1.5%), deposit flat → "gün %" sort should put deposit first
	m.quotes[prices.KeyGram] = prices.Quote{Price: 6700, PrevClose: 6800}
	m.recompute()
	if m.sum.Rows[0].Key != "gold" { // default: by value, gold is biggest
		t.Fatalf("default sort broke: %s", m.sum.Rows[0].Key)
	}
	selectRow(t, &m, step, "vadeli:İş Vadeli")
	step(key("g")) // flat view: sort applies across all rows
	step(key("s")) // → gün %
	if m.sortMode != 1 {
		t.Fatalf("sortMode = %d", m.sortMode)
	}
	if m.sum.Rows[0].Key != "vadeli:İş Vadeli" {
		t.Fatalf("gün%% sort order wrong: %s first", m.sum.Rows[0].Key)
	}
	// selection follows the asset, not the index
	if m.sum.Rows[m.sel].Key != "vadeli:İş Vadeli" {
		t.Fatalf("selection lost after sort: %s", m.sum.Rows[m.sel].Key)
	}
}

func TestLoanAddAndSign(t *testing.T) {
	m, cfg, path := newTestModel(t)
	step := stepper(t, &m)
	totalBefore := m.sum.TotalTRY

	// n → b → ad "Borcum" → tür borç → USD → tutar 20000 → save
	step(key("n"))
	step(key("b"))
	if m.mode != modeForm {
		t.Fatal("expected loan form")
	}
	step(key("enter"))
	typeText(step, "Borcum")
	step(key("enter")) // commit ad → tür
	step(key("enter")) // alacak → borç
	step(key("down"))  // para birimi
	step(key("enter")) // TRY → USD
	step(key("down"))  // tutar
	step(key("enter"))
	typeText(step, "20000")
	step(key("enter"))
	step(key("s"))
	if m.mode != modeDash {
		t.Fatalf("loan form did not save: %s", m.form.err)
	}
	l := cfg.Assets.Loans[0]
	if l.Kind != "borc" || l.Currency != "USD" || l.Amount != 20000 {
		t.Fatalf("loan = %+v", l)
	}
	// borç subtracts: 20000 × 47 = 940000 TL
	if diff := totalBefore - m.sum.TotalTRY; math.Abs(diff-940000) > 1 {
		t.Fatalf("total diff = %v", diff)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "kind: borc") {
		t.Fatalf("yaml missing loan:\n%s", b)
	}

	// receivable variant adds
	cfg.Assets.Loans[0].Kind = "alacak"
	m.recompute()
	if math.Abs(m.sum.TotalTRY-(totalBefore+940000)) > 1 {
		t.Fatalf("alacak total = %v", m.sum.TotalTRY)
	}
}

func TestHiddenExcludedFromTotals(t *testing.T) {
	m, cfg, _ := newTestModel(t)
	step := stepper(t, &m)
	totalBefore := m.sum.TotalTRY
	goldVal := 250.0 * 6700

	selectRow(t, &m, step, "gold")
	step(key("h"))
	if !cfg.IsHidden("gold") {
		t.Fatal("gold not hidden in cfg")
	}
	if math.Abs(m.sum.TotalTRY-(totalBefore-goldVal)) > 1 {
		t.Fatalf("hidden gold still in total: %v", m.sum.TotalTRY)
	}
	// row still listed, flagged hidden, at the bottom
	last := m.sum.Rows[len(m.sum.Rows)-1]
	if last.Key != "gold" || !last.Hidden {
		t.Fatalf("hidden row placement: %+v", last)
	}
	// toggle back
	selectRow(t, &m, step, "gold")
	step(key("h"))
	if cfg.IsHidden("gold") || math.Abs(m.sum.TotalTRY-totalBefore) > 1 {
		t.Fatalf("unhide failed: %v", m.sum.TotalTRY)
	}
}

func TestPropertyAndCryptoRows(t *testing.T) {
	m, cfg, _ := newTestModel(t)
	cfg.Assets.Properties = []config.Property{
		{Name: "Ev", Currency: "TRY", Value: 3000000},
		{Name: "Ofis", Currency: "USD", Value: 150000, Category: "gayrimenkul"},
	}
	cfg.Assets.Cryptos = []config.Crypto{{Name: "LIT", Symbol: "LIT", CoinID: "lighter", Amount: 100, CostUSD: 2}}
	m.quotes["crypto:LIT"] = prices.Quote{Price: 3.3, PrevClose: 3.0, Currency: "USD"}
	m.recompute()

	byKey := map[string]portfolio.Row{}
	for _, r := range m.sum.Rows {
		byKey[r.Key] = r
	}
	if v := byKey["prop:Ev"].ValueTRY; v != 3000000 {
		t.Fatalf("ev = %v", v)
	}
	if v := byKey["prop:Ofis"].ValueTRY; math.Abs(v-150000*47) > 1 {
		t.Fatalf("ofis = %v", v)
	}
	if c := byKey["prop:Ofis"].Category; c != "gayrimenkul" {
		t.Fatalf("kategori = %q", c)
	}
	lit := byKey["crypto:LIT"]
	if math.Abs(lit.ValueTRY-100*3.3*47) > 1 || lit.Category != "kripto" {
		t.Fatalf("lit = %+v", lit)
	}
	if math.Abs(lit.TotalPL-(100*3.3*47-100*2*47)) > 1 {
		t.Fatalf("lit K/Z = %v", lit.TotalPL)
	}
}
