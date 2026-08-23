package portfolio

import (
	"math"
	"strings"
	"time"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/prices"
)

// Well-known quote keys, alongside plain stock symbols.
const (
	KeyUSDTRY = prices.KeyUSDTRY
	KeyGram   = prices.KeyGram
)

type Row struct {
	Key      string
	Label    string
	Amount   float64
	Unit     string  // "$", "gr", "ad", ""
	Price    float64 // unit price in native currency
	Ccy      string
	ValueTRY float64
	DayPct   float64 // native-currency day move (accrual pct for deposits)
	DayTRY   float64 // TRY day change including FX effect
	TotalPL  float64 // TRY, NaN when no cost basis
	TotalPct float64 // NaN when no cost basis
	Category string  // nakit | kripto | hisse | altın | vadeli | gayrimenkul | alacak/borç | …
	Hidden   bool    // excluded from every total
	Spark    []float64
}

type Summary struct {
	Rows       []Row
	TotalTRY   float64
	TotalUSD   float64
	DayTRY     float64
	DayPct     float64
	TotalPL    float64 // sum over rows with a known cost basis; NaN if none
	TotalPLPct float64
	FX         float64 // USDTRY now
	FXPrev     float64 // USDTRY previous close
	At         time.Time
}

// DayUSD is the day change of the whole portfolio measured in USD
// (an fx rally lifts DayTRY while barely moving DayUSD).
func (s Summary) DayUSD() float64 { return dayUSD(s.TotalTRY, s.DayTRY, s.FX, s.FXPrev) }

// RowDayUSD is r's day change in USD terms.
func (s Summary) RowDayUSD(r Row) float64 { return dayUSD(r.ValueTRY, r.DayTRY, s.FX, s.FXPrev) }

func dayUSD(valTRY, dayTRY, fx, fxPrev float64) float64 {
	if fx == 0 || fxPrev == 0 {
		return 0
	}
	return valTRY/fx - (valTRY-dayTRY)/fxPrev
}

// Compute values the whole portfolio. quotes must contain KeyUSDTRY, and
// KeyGram / stock symbols for the assets present in cfg.
func Compute(cfg *config.Config, quotes map[string]prices.Quote, now time.Time) Summary {
	s := Summary{At: now}
	fx := quotes[KeyUSDTRY]
	nan := math.NaN()

	knownCost := 0.0
	add := func(r Row, costTRY float64) {
		r.Hidden = cfg.IsHidden(r.Key)
		s.Rows = append(s.Rows, r)
		if r.Hidden {
			return
		}
		s.TotalTRY += r.ValueTRY
		s.DayTRY += r.DayTRY
		knownCost += costTRY
	}

	for _, c := range cfg.Assets.Cash {
		units := c.Units(now)
		unitsPrev := c.Units(now.Add(-24 * time.Hour))
		var val, prev, price float64
		unit := "₺"
		dayPct := 0.0
		if c.Currency == "USD" {
			val = units * fx.Price
			prev = unitsPrev * fx.PrevClose
			price = fx.Price
			unit = "$"
			dayPct = fx.DayChangePct()
		} else {
			val, prev, price = units, unitsPrev, 1
		}
		if c.APY > 0 {
			dayPct += c.APY / 365 * 100
		}
		cat := c.Category
		if cat == "" {
			cat = "nakit"
		}
		r := Row{
			Key: "cash:" + c.Name, Label: c.Name, Amount: units, Unit: unit,
			Price: price, Ccy: "TRY", ValueTRY: val,
			DayPct: dayPct, DayTRY: val - prev,
			TotalPL: nan, TotalPct: nan, Category: cat,
		}
		if c.CostTRY > 0 {
			r.TotalPL = val - c.CostTRY
			r.TotalPct = r.TotalPL / c.CostTRY * 100
		}
		add(r, c.CostTRY)
	}

	if a := cfg.Assets.Gold; a != nil && a.Grams > 0 {
		g := quotes[KeyGram]
		val := a.Grams * g.Price
		r := Row{
			Key: "gold", Label: "Altın", Amount: a.Grams, Unit: "gr", Category: "altın",
			Price: g.Price, Ccy: "TRY", ValueTRY: val,
			DayPct: g.DayChangePct(), DayTRY: val - a.Grams*g.PrevClose,
			TotalPL: nan, TotalPct: nan,
		}
		if a.CostTRY > 0 {
			r.TotalPL = val - a.CostTRY
			r.TotalPct = r.TotalPL / a.CostTRY * 100
		}
		add(r, a.CostTRY)
	}

	for _, l := range cfg.Assets.Loans {
		sign := l.Sign()
		var val, prev, price float64
		unit := "₺"
		dayPct := 0.0
		if l.Currency == "USD" {
			val = l.Amount * fx.Price * sign
			prev = l.Amount * fx.PrevClose * sign
			price = fx.Price
			unit = "$"
			dayPct = fx.DayChangePct() * sign
		} else {
			val, prev, price = l.Amount*sign, l.Amount*sign, 1
		}
		add(Row{
			Key: "loan:" + l.Name, Label: l.Name, Amount: l.Amount, Unit: unit,
			Price: price, Ccy: "TRY", ValueTRY: val,
			DayPct: dayPct, DayTRY: val - prev,
			TotalPL: nan, TotalPct: nan, Category: "alacak/borç",
		}, 0)
	}

	for _, p := range cfg.Assets.Properties {
		cat := p.Category
		if cat == "" {
			cat = "gayrimenkul"
		}
		var val, prev float64
		dayPct := 0.0
		if p.Currency == "USD" {
			val = p.Value * fx.Price
			prev = p.Value * fx.PrevClose
			dayPct = fx.DayChangePct()
		} else {
			val, prev = p.Value, p.Value
		}
		r := Row{
			Key: "prop:" + p.Name, Label: p.Name,
			Price: val, Ccy: "TRY", ValueTRY: val,
			DayPct: dayPct, DayTRY: val - prev,
			TotalPL: nan, TotalPct: nan, Category: cat,
		}
		costTRY := 0.0
		if p.Cost > 0 {
			costTRY = p.Cost
			if p.Currency == "USD" {
				costTRY = p.Cost * fx.Price
			}
			r.TotalPL = val - costTRY
			r.TotalPct = pct(r.TotalPL, costTRY)
		}
		add(r, costTRY)
	}

	for _, cr := range cfg.Assets.Cryptos {
		q := quotes["crypto:"+cr.Name]
		val := cr.Amount * q.Price * fx.Price
		prev := cr.Amount * q.PrevClose * fx.PrevClose
		unit := cr.Symbol
		if unit == "" {
			unit = "adet"
		}
		r := Row{
			Key: "crypto:" + cr.Name, Label: cr.Name, Amount: cr.Amount, Unit: unit,
			Price: q.Price, Ccy: "USD", ValueTRY: val,
			DayPct: q.DayChangePct(), DayTRY: val - prev,
			TotalPL: nan, TotalPct: nan, Category: "kripto",
		}
		costTRY := 0.0
		if cr.CostUSD > 0 {
			costTRY = cr.Amount * cr.CostUSD * fx.Price
			r.TotalPL = val - costTRY
			r.TotalPct = pct(r.TotalPL, costTRY)
		}
		add(r, costTRY)
	}

	for _, st := range cfg.Assets.Stocks {
		q := quotes[st.Symbol]
		toTRY, prevToTRY := 1.0, 1.0
		if q.Currency == "USD" {
			toTRY, prevToTRY = fx.Price, fx.PrevClose
		}
		val := st.Shares * q.Price * toTRY
		prev := st.Shares * q.PrevClose * prevToTRY
		r := Row{
			Key: st.Symbol, Label: strings.TrimSuffix(st.Symbol, ".IS"), Amount: st.Shares, Unit: "ad", Category: "hisse",
			Price: q.Price, Ccy: q.Currency, ValueTRY: val,
			DayPct: q.DayChangePct(), DayTRY: val - prev,
			TotalPL: nan, TotalPct: nan,
		}
		costTRY := 0.0
		if st.Cost > 0 {
			costTRY = st.Shares * st.Cost * toTRY
			r.TotalPL = val - costTRY
			r.TotalPct = r.TotalPL / costTRY * 100
		}
		add(r, costTRY)
	}

	for _, d := range cfg.Assets.Deposits {
		val := d.Value(now)
		acc := d.DailyAccrual(now)
		r := Row{
			Key: "vadeli:" + d.Name, Label: d.Name, Category: "vadeli",
			Price: val, Ccy: "TRY", ValueTRY: val,
			DayPct: pct(acc, val-acc), DayTRY: acc,
			TotalPL: val - d.Principal, TotalPct: pct(val-d.Principal, d.Principal),
		}
		add(r, d.Principal)
	}

	s.FX, s.FXPrev = fx.Price, fx.PrevClose
	if fx.Price > 0 {
		s.TotalUSD = s.TotalTRY / fx.Price
	}
	s.DayPct = pct(s.DayTRY, s.TotalTRY-s.DayTRY)
	s.TotalPL, s.TotalPLPct = nan, nan
	if knownCost > 0 {
		pl := 0.0
		for _, r := range s.Rows {
			if !math.IsNaN(r.TotalPL) {
				pl += r.TotalPL
			}
		}
		s.TotalPL = pl
		s.TotalPLPct = pl / knownCost * 100
	}
	return s
}

func pct(delta, base float64) float64 {
	if base == 0 {
		return 0
	}
	return delta / base * 100
}
