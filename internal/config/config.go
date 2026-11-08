package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Display base currency; TRY is the primary, USD shown as secondary.
	RefreshSeconds int      `yaml:"refresh_seconds"`
	Assets         Assets   `yaml:"assets"`
	Hidden         []string `yaml:"hidden,omitempty"`    // row keys excluded from totals
	Collapsed      []string `yaml:"collapsed,omitempty"` // categories folded in the grouped view
	AI             AI       `yaml:"ai,omitempty"`
}

func (c *Config) IsCollapsed(category string) bool {
	for _, k := range c.Collapsed {
		if k == category {
			return true
		}
	}
	return false
}

func (c *Config) ToggleCollapsed(category string) {
	for i, k := range c.Collapsed {
		if k == category {
			c.Collapsed = append(c.Collapsed[:i], c.Collapsed[i+1:]...)
			return
		}
	}
	c.Collapsed = append(c.Collapsed, category)
}

func (c *Config) IsHidden(rowKey string) bool {
	for _, k := range c.Hidden {
		if k == rowKey {
			return true
		}
	}
	return false
}

func (c *Config) ToggleHidden(rowKey string) {
	for i, k := range c.Hidden {
		if k == rowKey {
			c.Hidden = append(c.Hidden[:i], c.Hidden[i+1:]...)
			return
		}
	}
	c.Hidden = append(c.Hidden, rowKey)
}

type AI struct {
	Model    string `yaml:"model,omitempty"`    // api default: claude-opus-5; claude cli: user's default
	Provider string `yaml:"provider,omitempty"` // "claude" (cli, Max/Pro) | "api"; empty = auto
}

type Assets struct {
	Cash       []Cash     `yaml:"cash,omitempty"`
	Gold       *Gold      `yaml:"gold,omitempty"`
	Stocks     []Stock    `yaml:"stocks,omitempty"`
	Deposits   []Deposit  `yaml:"deposits,omitempty"`
	Loans      []Loan     `yaml:"loans,omitempty"`
	Properties []Property `yaml:"properties,omitempty"`
	Cryptos    []Crypto   `yaml:"cryptos,omitempty"`
}

// Property is a manually-valued fixed asset: house, office, car…
type Property struct {
	Name     string  `yaml:"name"`
	Currency string  `yaml:"currency"` // "TRY" | "USD"
	Value    float64 `yaml:"value"`
	Cost     float64 `yaml:"cost,omitempty"`     // purchase price, same currency
	Category string  `yaml:"category,omitempty"` // default "gayrimenkul"; e.g. "araç"
}

// Crypto is a coin priced live via CoinGecko (USD).
type Crypto struct {
	Name    string  `yaml:"name"`
	Symbol  string  `yaml:"symbol"`       // display unit, e.g. LIT
	CoinID  string  `yaml:"coingecko_id"` // e.g. "lighter", "bitcoin"
	Amount  float64 `yaml:"amount"`
	CostUSD float64 `yaml:"cost,omitempty"` // avg cost per unit, USD
	Note    string  `yaml:"note,omitempty"` // e.g. lock/vesting info
}

// Loan is money owed to you (alacak, adds to net worth) or by you
// (borc, subtracts). TL or USD.
type Loan struct {
	Name     string  `yaml:"name"`
	Currency string  `yaml:"currency"` // "TRY" | "USD"
	Amount   float64 `yaml:"amount"`   // always positive; Kind carries the sign
	Kind     string  `yaml:"kind"`     // "alacak" | "borc"
	Due      Date    `yaml:"due,omitempty"`
}

// Sign is +1 for receivables, -1 for debts.
func (l Loan) Sign() float64 {
	if l.Kind == "borc" {
		return -1
	}
	return 1
}

// Date is a yaml-friendly day-precision time ("2006-01-02").
type Date struct{ time.Time }

func Today() Date {
	y, m, d := time.Now().Date()
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.Local)}
}

func (d *Date) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("geçersiz tarih %q (örn. 2026-09-18)", s)
}

func (d Date) MarshalYAML() (any, error) { return d.Format("2006-01-02"), nil }

// ParseDate parses "2006-01-02" in local time.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return Date{}, fmt.Errorf("geçersiz tarih %q (örn. 2026-09-18)", s)
	}
	return Date{t}, nil
}

// Cash is a currency balance at some venue (bank TL, USD, USDC ≈ USD).
type Cash struct {
	Name     string  `yaml:"name"`
	Currency string  `yaml:"currency"` // "TRY" | "USD"
	Amount   float64 `yaml:"amount"`
	APY      float64 `yaml:"apy,omitempty"`   // optional yearly yield (e.g. Midas USD 0.03)
	AsOf     Date    `yaml:"as_of,omitempty"` // date Amount was recorded; APY accrues from here
	CostTRY  float64 `yaml:"cost_try,omitempty"`
	Category string  `yaml:"category,omitempty"` // default "nakit"; e.g. "kripto" for USDC
}

// Units returns Amount grown by simple APY since AsOf.
func (c Cash) Units(t time.Time) float64 {
	if c.APY == 0 || c.AsOf.IsZero() || t.Before(c.AsOf.Time) {
		return c.Amount
	}
	days := t.Sub(c.AsOf.Time).Hours() / 24
	return c.Amount * (1 + c.APY*days/365)
}

type Gold struct {
	Grams   float64 `yaml:"grams"`
	CostTRY float64 `yaml:"cost_try,omitempty"`
}

type Stock struct {
	Symbol string  `yaml:"symbol"` // Yahoo symbol: THYAO.IS, AAPL, ...
	Shares float64 `yaml:"shares"`
	Cost   float64 `yaml:"cost,omitempty"` // avg cost per share in the stock's own currency
}

type Deposit struct {
	Name      string  `yaml:"name"`
	Principal float64 `yaml:"principal"`             // TRY
	GrossRate float64 `yaml:"gross_rate,omitempty"`  // yearly simple rate before tax, 0.41 = %41
	Stopaj    float64 `yaml:"stopaj,omitempty"`      // withholding tax, 0.175 = %17,5
	NetRate   float64 `yaml:"annual_rate,omitempty"` // legacy: net rate directly
	Start     Date    `yaml:"start"`
	Maturity  Date    `yaml:"maturity"`
}

// Net is the effective yearly simple rate after stopaj.
func (d Deposit) Net() float64 {
	if d.GrossRate > 0 {
		return d.GrossRate * (1 - d.Stopaj)
	}
	return d.NetRate
}

// Value returns accrued value at t using simple interest, capped at maturity.
func (d Deposit) Value(t time.Time) float64 {
	end := t
	if end.After(d.Maturity.Time) {
		end = d.Maturity.Time
	}
	if end.Before(d.Start.Time) {
		return d.Principal
	}
	days := end.Sub(d.Start.Time).Hours() / 24
	return d.Principal * (1 + d.Net()*days/365)
}

// DailyAccrual is the TRY earned per day while the deposit is active.
func (d Deposit) DailyAccrual(t time.Time) float64 {
	if t.Before(d.Start.Time) || t.After(d.Maturity.Time) {
		return 0
	}
	return d.Principal * d.Net() / 365
}

func DefaultPath() string {
	if p := os.Getenv("KASA_CONFIG"); p != "" {
		return p
	}
	if _, err := os.Stat("portfolio.yaml"); err == nil {
		return "portfolio.yaml"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kasa", "portfolio.yaml")
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.RefreshSeconds <= 0 {
		c.RefreshSeconds = 30
	}
	return &c, nil
}

const saveHeader = `# kasa portföyü — bu dosya kasa TUI'sinden yönetilir
# (↑↓ seç · enter düzenle · n yeni · x sil · o ayarlar)
`

// Save writes the whole config atomically. The file is fully regenerated —
// kasa's TUI is the editing surface, the yaml is just the store.
func Save(c *Config, path string) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".portfolio-*.yaml")
	if err != nil {
		return err
	}
	if _, err := f.Write(append([]byte(saveHeader+"\n"), b...)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	f.Close()
	return os.Rename(f.Name(), path)
}
