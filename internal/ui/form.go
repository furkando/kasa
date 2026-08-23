package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/furkando/kasa/internal/config"
)

type fieldKind int

const (
	fText fieldKind = iota
	fNumber
	fPercent // typed as "41" or "17,5"; stored /100
	fDate    // 2006-01-02
	fChoice  // enter cycles choices
)

type formField struct {
	label    string
	kind     fieldKind
	value    string
	choices  []string
	choiceIx int
	optional bool
	help     string
}

type form struct {
	title   string
	fields  []formField
	sel     int
	editing bool
	input   textinput.Model
	err     string
	// apply validates parsed values and mutates cfg; the model then saves.
	apply func(m *Model, f *form) error
}

// --- field value parsing ---------------------------------------------------

func (f formField) text() string {
	if f.kind == fChoice {
		return f.choices[f.choiceIx]
	}
	return strings.TrimSpace(f.value)
}

func (f formField) number() (float64, error) {
	s := strings.ReplaceAll(f.text(), ",", ".")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		if f.optional {
			return 0, nil
		}
		return 0, fmt.Errorf("%s boş olamaz", f.label)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s: geçersiz sayı %q", f.label, f.text())
	}
	return v, nil
}

func (f formField) percent() (float64, error) {
	v, err := f.number()
	if err != nil {
		return 0, err
	}
	return v / 100, nil
}

func (f formField) date() (config.Date, error) {
	s := f.text()
	if s == "" {
		if f.optional {
			return config.Date{}, nil
		}
		return config.Date{}, fmt.Errorf("%s boş olamaz", f.label)
	}
	d, err := config.ParseDate(s)
	if err != nil {
		return config.Date{}, fmt.Errorf("%s: %v", f.label, err)
	}
	return d, nil
}

func fmtNum(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func fmtPct(v float64) string { return fmtNum(v * 100) }

func fmtDate(d config.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.Format("2006-01-02")
}

// --- forms per entity ------------------------------------------------------

func cashForm(title string, cur *config.Cash, origName string) *form {
	ccyIx := 0
	if cur != nil && cur.Currency == "USD" {
		ccyIx = 1
	}
	f := &form{title: title, fields: []formField{
		{label: "ad", kind: fText},
		{label: "para birimi", kind: fChoice, choices: []string{"TRY", "USD"}, choiceIx: ccyIx},
		{label: "tutar", kind: fNumber},
		{label: "yıllık getiri (%)", kind: fPercent, optional: true, help: "örn. Midas USD için 3"},
		{label: "maliyet (₺)", kind: fNumber, optional: true, help: "toplam K/Z için"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Name
		f.fields[2].value = fmtNum(cur.Amount)
		f.fields[3].value = fmtPct(cur.APY)
		f.fields[4].value = fmtNum(cur.CostTRY)
	}
	f.apply = func(m *Model, f *form) error {
		name := f.fields[0].text()
		if name == "" {
			return fmt.Errorf("ad boş olamaz")
		}
		amount, err := f.fields[2].number()
		if err != nil {
			return err
		}
		apy, err := f.fields[3].percent()
		if err != nil {
			return err
		}
		cost, err := f.fields[4].number()
		if err != nil {
			return err
		}
		next := config.Cash{Name: name, Currency: f.fields[1].text(), Amount: amount, APY: apy, CostTRY: cost}
		cash := m.cfg.Assets.Cash
		if origName == "" { // new
			for _, c := range cash {
				if c.Name == name {
					return fmt.Errorf("%q zaten var", name)
				}
			}
			if apy > 0 {
				next.AsOf = config.Today()
			}
			m.cfg.Assets.Cash = append(cash, next)
			return nil
		}
		for i := range cash {
			if cash[i].Name == origName {
				next.AsOf = cash[i].AsOf
				// restart accrual when the balance or the yield changes
				if apy > 0 && (cash[i].Amount != amount || cash[i].APY != apy) {
					next.AsOf = config.Today()
				}
				cash[i] = next
				return nil
			}
		}
		return fmt.Errorf("%q bulunamadı", origName)
	}
	return f
}

func goldForm(cur *config.Gold) *form {
	f := &form{title: "Altın", fields: []formField{
		{label: "gram", kind: fNumber},
		{label: "maliyet (₺)", kind: fNumber, optional: true},
	}}
	if cur != nil {
		f.fields[0].value = fmtNum(cur.Grams)
		f.fields[1].value = fmtNum(cur.CostTRY)
	}
	f.apply = func(m *Model, f *form) error {
		grams, err := f.fields[0].number()
		if err != nil {
			return err
		}
		cost, err := f.fields[1].number()
		if err != nil {
			return err
		}
		m.cfg.Assets.Gold = &config.Gold{Grams: grams, CostTRY: cost}
		return nil
	}
	return f
}

func stockForm(title string, cur *config.Stock, origSymbol string) *form {
	f := &form{title: title, fields: []formField{
		{label: "sembol", kind: fText, help: "BIST: THYAO.IS · ABD: AAPL"},
		{label: "adet", kind: fNumber},
		{label: "maliyet/adet", kind: fNumber, optional: true, help: "hissenin kendi kurunda"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Symbol
		f.fields[1].value = fmtNum(cur.Shares)
		f.fields[2].value = fmtNum(cur.Cost)
	}
	f.apply = func(m *Model, f *form) error {
		sym := strings.ToUpper(f.fields[0].text())
		if sym == "" {
			return fmt.Errorf("sembol boş olamaz")
		}
		shares, err := f.fields[1].number()
		if err != nil {
			return err
		}
		cost, err := f.fields[2].number()
		if err != nil {
			return err
		}
		next := config.Stock{Symbol: sym, Shares: shares, Cost: cost}
		stocks := m.cfg.Assets.Stocks
		if origSymbol == "" {
			for _, s := range stocks {
				if s.Symbol == sym {
					return fmt.Errorf("%s zaten var", sym)
				}
			}
			m.cfg.Assets.Stocks = append(stocks, next)
			return nil
		}
		for i := range stocks {
			if stocks[i].Symbol == origSymbol {
				stocks[i] = next
				return nil
			}
		}
		return fmt.Errorf("%s bulunamadı", origSymbol)
	}
	return f
}

func depositForm(title string, cur *config.Deposit, origName string) *form {
	f := &form{title: title, fields: []formField{
		{label: "ad", kind: fText},
		{label: "anapara (₺)", kind: fNumber},
		{label: "brüt yıllık faiz (%)", kind: fPercent, help: "bankanın söylediği oran, örn. 41"},
		{label: "stopaj (%)", kind: fPercent, optional: true, help: "TL mevduatta genelde 17,5"},
		{label: "başlangıç", kind: fDate, help: "2026-08-17"},
		{label: "vade", kind: fDate, help: "2026-09-18"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Name
		f.fields[1].value = fmtNum(cur.Principal)
		if cur.GrossRate > 0 {
			f.fields[2].value = fmtPct(cur.GrossRate)
			f.fields[3].value = fmtPct(cur.Stopaj)
		} else { // legacy net-only config: show it as gross with no stopaj
			f.fields[2].value = fmtPct(cur.NetRate)
		}
		f.fields[4].value = fmtDate(cur.Start)
		f.fields[5].value = fmtDate(cur.Maturity)
	} else {
		f.fields[3].value = "17,5"
		f.fields[4].value = fmtDate(config.Today())
	}
	f.apply = func(m *Model, f *form) error {
		name := f.fields[0].text()
		if name == "" {
			return fmt.Errorf("ad boş olamaz")
		}
		principal, err := f.fields[1].number()
		if err != nil {
			return err
		}
		gross, err := f.fields[2].percent()
		if err != nil {
			return err
		}
		if gross <= 0 {
			return fmt.Errorf("brüt faiz 0'dan büyük olmalı")
		}
		stopaj, err := f.fields[3].percent()
		if err != nil {
			return err
		}
		start, err := f.fields[4].date()
		if err != nil {
			return err
		}
		maturity, err := f.fields[5].date()
		if err != nil {
			return err
		}
		if !maturity.After(start.Time) {
			return fmt.Errorf("vade başlangıçtan sonra olmalı")
		}
		next := config.Deposit{Name: name, Principal: principal, GrossRate: gross, Stopaj: stopaj, Start: start, Maturity: maturity}
		deps := m.cfg.Assets.Deposits
		if origName == "" {
			for _, d := range deps {
				if d.Name == name {
					return fmt.Errorf("%q zaten var", name)
				}
			}
			m.cfg.Assets.Deposits = append(deps, next)
			return nil
		}
		for i := range deps {
			if deps[i].Name == origName {
				deps[i] = next
				return nil
			}
		}
		return fmt.Errorf("%q bulunamadı", origName)
	}
	return f
}

func loanForm(title string, cur *config.Loan, origName string) *form {
	kindIx, ccyIx := 0, 0
	if cur != nil {
		if cur.Kind == "borc" {
			kindIx = 1
		}
		if cur.Currency == "USD" {
			ccyIx = 1
		}
	}
	f := &form{title: title, fields: []formField{
		{label: "ad", kind: fText},
		{label: "tür", kind: fChoice, choices: []string{"alacak", "borç"}, choiceIx: kindIx,
			help: "alacak = sana ödenecek · borç = senin ödeyeceğin"},
		{label: "para birimi", kind: fChoice, choices: []string{"TRY", "USD"}, choiceIx: ccyIx},
		{label: "tutar", kind: fNumber},
		{label: "vade", kind: fDate, optional: true, help: "ops., örn. 2026-12-01"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Name
		f.fields[3].value = fmtNum(cur.Amount)
		f.fields[4].value = fmtDate(cur.Due)
	}
	f.apply = func(m *Model, f *form) error {
		name := f.fields[0].text()
		if name == "" {
			return fmt.Errorf("ad boş olamaz")
		}
		amount, err := f.fields[3].number()
		if err != nil {
			return err
		}
		if amount <= 0 {
			return fmt.Errorf("tutar 0'dan büyük olmalı")
		}
		due, err := f.fields[4].date()
		if err != nil {
			return err
		}
		kind := "alacak"
		if f.fields[1].choiceIx == 1 {
			kind = "borc"
		}
		next := config.Loan{Name: name, Currency: f.fields[2].text(), Amount: amount, Kind: kind, Due: due}
		loans := m.cfg.Assets.Loans
		if origName == "" {
			for _, l := range loans {
				if l.Name == name {
					return fmt.Errorf("%q zaten var", name)
				}
			}
			m.cfg.Assets.Loans = append(loans, next)
			return nil
		}
		for i := range loans {
			if loans[i].Name == origName {
				loans[i] = next
				return nil
			}
		}
		return fmt.Errorf("%q bulunamadı", origName)
	}
	return f
}

func propertyForm(title string, cur *config.Property, origName string) *form {
	ccyIx := 0
	if cur != nil && cur.Currency == "USD" {
		ccyIx = 1
	}
	f := &form{title: title, fields: []formField{
		{label: "ad", kind: fText, help: "Ev, Ofis, Araba…"},
		{label: "para birimi", kind: fChoice, choices: []string{"TRY", "USD"}, choiceIx: ccyIx},
		{label: "değer", kind: fNumber, help: "güncel tahmini değer"},
		{label: "alış fiyatı", kind: fNumber, optional: true, help: "K/Z için, aynı kurda"},
		{label: "kategori", kind: fText, optional: true, help: "boş = gayrimenkul · örn. araç"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Name
		f.fields[2].value = fmtNum(cur.Value)
		f.fields[3].value = fmtNum(cur.Cost)
		f.fields[4].value = cur.Category
	}
	f.apply = func(m *Model, f *form) error {
		name := f.fields[0].text()
		if name == "" {
			return fmt.Errorf("ad boş olamaz")
		}
		value, err := f.fields[2].number()
		if err != nil {
			return err
		}
		if value <= 0 {
			return fmt.Errorf("değer 0'dan büyük olmalı")
		}
		cost, err := f.fields[3].number()
		if err != nil {
			return err
		}
		next := config.Property{Name: name, Currency: f.fields[1].text(), Value: value, Cost: cost, Category: f.fields[4].text()}
		props := m.cfg.Assets.Properties
		if origName == "" {
			for _, p := range props {
				if p.Name == name {
					return fmt.Errorf("%q zaten var", name)
				}
			}
			m.cfg.Assets.Properties = append(props, next)
			return nil
		}
		for i := range props {
			if props[i].Name == origName {
				props[i] = next
				return nil
			}
		}
		return fmt.Errorf("%q bulunamadı", origName)
	}
	return f
}

func cryptoForm(title string, cur *config.Crypto, origName string) *form {
	f := &form{title: title, fields: []formField{
		{label: "ad", kind: fText, help: "örn. LIT"},
		{label: "sembol", kind: fText, optional: true, help: "birim gösterimi, örn. LIT"},
		{label: "coingecko id", kind: fText, help: "coingecko.com'daki id — örn. bitcoin, lighter"},
		{label: "adet", kind: fNumber},
		{label: "ort. maliyet ($)", kind: fNumber, optional: true},
		{label: "not", kind: fText, optional: true, help: "örn. kilit/vesting bilgisi"},
	}}
	if cur != nil {
		f.fields[0].value = cur.Name
		f.fields[1].value = cur.Symbol
		f.fields[2].value = cur.CoinID
		f.fields[3].value = fmtNum(cur.Amount)
		f.fields[4].value = fmtNum(cur.CostUSD)
		f.fields[5].value = cur.Note
	}
	f.apply = func(m *Model, f *form) error {
		name := f.fields[0].text()
		if name == "" {
			return fmt.Errorf("ad boş olamaz")
		}
		id := strings.ToLower(f.fields[2].text())
		if id == "" {
			return fmt.Errorf("coingecko id boş olamaz")
		}
		amount, err := f.fields[3].number()
		if err != nil {
			return err
		}
		cost, err := f.fields[4].number()
		if err != nil {
			return err
		}
		next := config.Crypto{Name: name, Symbol: strings.ToUpper(f.fields[1].text()), CoinID: id,
			Amount: amount, CostUSD: cost, Note: f.fields[5].text()}
		crs := m.cfg.Assets.Cryptos
		if origName == "" {
			for _, c := range crs {
				if c.Name == name {
					return fmt.Errorf("%q zaten var", name)
				}
			}
			m.cfg.Assets.Cryptos = append(crs, next)
			return nil
		}
		for i := range crs {
			if crs[i].Name == origName {
				crs[i] = next
				return nil
			}
		}
		return fmt.Errorf("%q bulunamadı", origName)
	}
	return f
}

func settingsForm(cfg *config.Config) *form {
	provIx := 0
	switch cfg.AI.Provider {
	case "claude":
		provIx = 1
	case "api":
		provIx = 2
	}
	f := &form{title: "Ayarlar", fields: []formField{
		{label: "yenileme (sn)", kind: fNumber, value: strconv.Itoa(cfg.RefreshSeconds)},
		{label: "ai sağlayıcı", kind: fChoice, choices: []string{"otomatik", "claude", "api"}, choiceIx: provIx,
			help: "claude = Max/Pro aboneliği · api = ANTHROPIC_API_KEY"},
		{label: "ai model", kind: fText, value: cfg.AI.Model, optional: true, help: "boş = varsayılan"},
	}}
	f.apply = func(m *Model, f *form) error {
		secs, err := f.fields[0].number()
		if err != nil {
			return err
		}
		if secs < 5 {
			return fmt.Errorf("yenileme en az 5 sn olmalı")
		}
		m.cfg.RefreshSeconds = int(secs)
		prov := f.fields[1].text()
		if prov == "otomatik" {
			prov = ""
		}
		m.cfg.AI.Provider = prov
		m.cfg.AI.Model = f.fields[2].text()
		return nil
	}
	return f
}

// formFor builds the edit form for a dashboard row key.
func (m *Model) formFor(rowKey string) *form {
	switch {
	case rowKey == "gold":
		return goldForm(m.cfg.Assets.Gold)
	case strings.HasPrefix(rowKey, "cash:"):
		name := strings.TrimPrefix(rowKey, "cash:")
		for i := range m.cfg.Assets.Cash {
			if m.cfg.Assets.Cash[i].Name == name {
				return cashForm("Nakit · "+name, &m.cfg.Assets.Cash[i], name)
			}
		}
	case strings.HasPrefix(rowKey, "vadeli:"):
		name := strings.TrimPrefix(rowKey, "vadeli:")
		for i := range m.cfg.Assets.Deposits {
			if m.cfg.Assets.Deposits[i].Name == name {
				return depositForm("Vadeli · "+name, &m.cfg.Assets.Deposits[i], name)
			}
		}
	case strings.HasPrefix(rowKey, "loan:"):
		name := strings.TrimPrefix(rowKey, "loan:")
		for i := range m.cfg.Assets.Loans {
			if m.cfg.Assets.Loans[i].Name == name {
				return loanForm("Alacak/Borç · "+name, &m.cfg.Assets.Loans[i], name)
			}
		}
	case strings.HasPrefix(rowKey, "prop:"):
		name := strings.TrimPrefix(rowKey, "prop:")
		for i := range m.cfg.Assets.Properties {
			if m.cfg.Assets.Properties[i].Name == name {
				return propertyForm("Mülk · "+name, &m.cfg.Assets.Properties[i], name)
			}
		}
	case strings.HasPrefix(rowKey, "crypto:"):
		name := strings.TrimPrefix(rowKey, "crypto:")
		for i := range m.cfg.Assets.Cryptos {
			if m.cfg.Assets.Cryptos[i].Name == name {
				return cryptoForm("Kripto · "+name, &m.cfg.Assets.Cryptos[i], name)
			}
		}
	default:
		for i := range m.cfg.Assets.Stocks {
			if m.cfg.Assets.Stocks[i].Symbol == rowKey {
				return stockForm("Hisse · "+rowKey, &m.cfg.Assets.Stocks[i], rowKey)
			}
		}
	}
	return nil
}

// deleteRow removes the asset behind a dashboard row key from cfg.
func (m *Model) deleteRow(rowKey string) {
	switch {
	case rowKey == "gold":
		m.cfg.Assets.Gold = nil
	case strings.HasPrefix(rowKey, "cash:"):
		name := strings.TrimPrefix(rowKey, "cash:")
		m.cfg.Assets.Cash = deleteBy(m.cfg.Assets.Cash, func(c config.Cash) bool { return c.Name == name })
	case strings.HasPrefix(rowKey, "vadeli:"):
		name := strings.TrimPrefix(rowKey, "vadeli:")
		m.cfg.Assets.Deposits = deleteBy(m.cfg.Assets.Deposits, func(d config.Deposit) bool { return d.Name == name })
	case strings.HasPrefix(rowKey, "loan:"):
		name := strings.TrimPrefix(rowKey, "loan:")
		m.cfg.Assets.Loans = deleteBy(m.cfg.Assets.Loans, func(l config.Loan) bool { return l.Name == name })
	case strings.HasPrefix(rowKey, "prop:"):
		name := strings.TrimPrefix(rowKey, "prop:")
		m.cfg.Assets.Properties = deleteBy(m.cfg.Assets.Properties, func(p config.Property) bool { return p.Name == name })
	case strings.HasPrefix(rowKey, "crypto:"):
		name := strings.TrimPrefix(rowKey, "crypto:")
		m.cfg.Assets.Cryptos = deleteBy(m.cfg.Assets.Cryptos, func(c config.Crypto) bool { return c.Name == name })
	default:
		m.cfg.Assets.Stocks = deleteBy(m.cfg.Assets.Stocks, func(s config.Stock) bool { return s.Symbol == rowKey })
	}
}

func deleteBy[T any](s []T, match func(T) bool) []T {
	out := s[:0]
	for _, v := range s {
		if !match(v) {
			out = append(out, v)
		}
	}
	return out
}

// --- form update & view ----------------------------------------------------

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	if f.editing {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			f.editing = false
			return m, nil
		case "enter":
			f.fields[f.sel].value = f.input.Value()
			f.editing = false
			f.err = ""
			if f.sel < len(f.fields)-1 {
				f.sel++
			}
			return m, nil
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeDash
		m.form = nil
		return m, nil
	case "up", "k":
		if f.sel > 0 {
			f.sel--
		}
		return m, nil
	case "down", "j", "tab":
		if f.sel < len(f.fields)-1 {
			f.sel++
		}
		return m, nil
	case "enter":
		fl := &f.fields[f.sel]
		if fl.kind == fChoice {
			fl.choiceIx = (fl.choiceIx + 1) % len(fl.choices)
			f.err = ""
			return m, nil
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 60
		ti.Width = 30
		ti.SetValue(fl.value)
		ti.CursorEnd()
		ti.Focus()
		f.input = ti
		f.editing = true
		return m, textinput.Blink
	case "s":
		if err := f.apply(&m, f); err != nil {
			f.err = err.Error()
			return m, nil
		}
		if err := config.Save(m.cfg, config.DefaultPath()); err != nil {
			f.err = err.Error()
			return m, nil
		}
		m.mode = modeDash
		m.form = nil
		m.afterConfigChange()
		return m, tea.Batch(m.spin.Tick, fetchQuotes(m.cfg), fetchHist(m.cfg))
	}
	return m, nil
}

func (m Model) viewForm() string {
	f := m.form
	w := m.contentWidth()

	left := sBrand.Render("◆ kasa · ") + sTotal.Render(f.title)
	right := sKey.Render("s") + sFaint.Render(" kaydet · ") + sKey.Render("esc") + sFaint.Render(" vazgeç")
	var b strings.Builder
	b.WriteString(joinEdges(left, right, w))
	b.WriteString("\n\n")

	labelW := 0
	for _, fl := range f.fields {
		if lw := lipgloss.Width(fl.label); lw > labelW {
			labelW = lw
		}
	}
	for i, fl := range f.fields {
		marker := "  "
		if i == f.sel {
			marker = sKey.Render("▸ ")
		}
		label := sDim.Render(fit(fl.label, labelW+2, false))
		var val string
		switch {
		case i == f.sel && f.editing:
			val = f.input.View()
		case fl.kind == fChoice:
			val = sTotal.Render(fl.choices[fl.choiceIx]) + sFaint.Render("  (enter değiştirir)")
		case fl.text() == "":
			val = sFaint.Render("—")
		default:
			val = sTotal.Render(fl.text())
		}
		line := marker + label + val
		if i == f.sel && fl.help != "" && !f.editing {
			line += sFaint.Render("   " + fl.help)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + sFaint.Render("↑↓ alan · enter düzenle/değiştir · s kaydet"))
	if f.err != "" {
		b.WriteString("\n" + sErr.Render("⚠ "+f.err))
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
