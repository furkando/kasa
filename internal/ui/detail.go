package ui

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/portfolio"
	"github.com/furkando/kasa/internal/prices"
)

type detailMsg struct {
	key  string
	rng  string
	hist prices.AssetHistory
	news []prices.NewsItem
	err  error
}

var rangeNames = map[string]string{"1mo": "1 ay", "3mo": "3 ay", "1y": "1 yıl"}

func (m Model) openDetail(key string) (tea.Model, tea.Cmd) {
	m.mode = modeDetail
	m.detailKey = key
	if m.detailRange == "" {
		m.detailRange = "1mo"
	}
	return m.fetchDetail()
}

// fetchNewsFor picks the right source per asset: Yahoo (relatedTickers-
// filtered) for stocks, Turkish Google News for kur/altın, English Google
// News for crypto, nothing for cash/deposits/loans/properties.
func fetchNewsFor(ctx context.Context, cfg *config.Config, key string) []prices.NewsItem {
	switch {
	case key == "fx":
		n, _ := prices.FetchNewsRSS(ctx, "dolar TL kuru", "tr", 4)
		return n
	case key == "gold":
		n, _ := prices.FetchNewsRSS(ctx, "gram altın fiyat", "tr", 4)
		return n
	case strings.HasPrefix(key, "crypto:"):
		name := strings.TrimPrefix(key, "crypto:")
		for _, cr := range cfg.Assets.Cryptos {
			if cr.Name == name {
				q := fmt.Sprintf("%q crypto %s token", cr.CoinID, cr.Symbol)
				n, _ := prices.FetchNewsRSS(ctx, q, "en", 4)
				return n
			}
		}
		return nil
	case strings.HasPrefix(key, "cash:"), strings.HasPrefix(key, "vadeli:"),
		strings.HasPrefix(key, "loan:"), strings.HasPrefix(key, "prop:"):
		return nil
	default: // stock symbol
		n, _ := prices.FetchNews(ctx, key, key, 4)
		return n
	}
}

func (m Model) fetchDetail() (tea.Model, tea.Cmd) {
	m.detailLoading = true
	m.detailErr = nil
	m.detailNews = nil
	key, rng, cfg := m.detailKey, m.detailRange, m.cfg
	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		h, err := prices.FetchAssetHistory(ctx, cfg, key, rng)
		news := fetchNewsFor(ctx, cfg, key) // best effort
		return detailMsg{key, rng, h, news, err}
	})
}

func openURL(link string) {
	if link == "" {
		return
	}
	bin := "open" // macOS
	if _, err := exec.LookPath(bin); err != nil {
		bin = "xdg-open"
	}
	_ = exec.Command(bin, link).Start()
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.newsPick {
		m.newsPick = false
		k := msg.String()
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if idx := int(k[0] - '1'); idx < len(m.detailNews) {
				openURL(m.detailNews[idx].Link)
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.mode = modeDash
		return m, nil
	case "d", "$":
		m.baseUSD = !m.baseUSD
		return m, nil
	case "u":
		if m.detailKey != "fx" {
			return m.openDetail("fx")
		}
		return m, nil
	case "e", "enter":
		if f := m.formFor(m.detailKey); f != nil {
			m.form = f
			m.mode = modeForm
		}
		return m, nil
	case "t":
		if len(m.detailNews) > 0 {
			m.newsPick = true
		}
		return m, nil
	case "1", "3", "y":
		if strings.HasPrefix(m.detailKey, "vadeli:") {
			return m, nil // deposits always show the full term
		}
		rng := map[string]string{"1": "1mo", "3": "3mo", "y": "1y"}[msg.String()]
		if rng == m.detailRange {
			return m, nil
		}
		m.detailRange = rng
		return m.fetchDetail()
	case "up", "k", "down", "j":
		// browse neighbouring assets without leaving the detail view
		if m.detailKey == "fx" {
			return m, nil
		}
		if msg.String() == "up" || msg.String() == "k" {
			if m.sel > 0 {
				m.sel--
			}
		} else if m.sel < len(m.sum.Rows)-1 {
			m.sel++
		}
		if m.sel >= 0 && m.sel < len(m.sum.Rows) && m.sum.Rows[m.sel].Key != m.detailKey {
			return m.openDetail(m.sum.Rows[m.sel].Key)
		}
		return m, nil
	}
	return m, nil
}

// changeSince returns the % change from the last value ≥days before the end.
func changeSince(dates []time.Time, vals []float64, days int) (float64, bool) {
	if len(vals) < 2 {
		return 0, false
	}
	end := dates[len(dates)-1]
	cutoff := end.AddDate(0, 0, -days)
	base := vals[0]
	found := false
	for i, d := range dates {
		if d.After(cutoff) {
			break
		}
		base = vals[i]
		found = true
	}
	if days >= int(end.Sub(dates[0]).Hours()/24) {
		base, found = vals[0], true // window longer than the series: use full span
	}
	if !found || base == 0 {
		return 0, false
	}
	return (vals[len(vals)-1]/base - 1) * 100, true
}

// dualChange renders "+%1,2 ₺ · +%0,4 $" for a lookback window.
func (m Model) dualChange(days int) (string, bool) {
	h := m.detailHist
	cTry, ok1 := changeSince(h.Dates, h.TRY, days)
	cUsd, ok2 := changeSince(h.Dates, h.USD, days)
	if !ok1 || !ok2 {
		return "", false
	}
	return paint(cTry).Render(signedPct(cTry)+" ₺") + sFaint.Render(" · ") +
		paint(cUsd).Render(signedPct(cUsd)+" $"), true
}

func (m Model) detailRow() (portfolio.Row, bool) {
	for _, r := range m.sum.Rows {
		if r.Key == m.detailKey {
			return r, true
		}
	}
	return portfolio.Row{}, false
}

func (m Model) viewDetail() string {
	w := m.contentWidth()
	isFX := m.detailKey == "fx"

	var row portfolio.Row
	var title string
	if isFX {
		title = "USD/TRY"
	} else {
		r, ok := m.detailRow()
		if !ok {
			return lipgloss.NewStyle().Padding(1, 2).Render(sErr.Render("varlık bulunamadı — esc"))
		}
		row, title = r, r.Label
	}

	var b strings.Builder

	// header: title + range switch + keys
	left := sBrand.Render("◆ kasa · ") + sTotal.Render(title)
	var rangeHint string
	if strings.HasPrefix(m.detailKey, "vadeli:") {
		rangeHint = sFaint.Render("dönem: başlangıç → vade")
	} else {
		parts := make([]string, 0, 3)
		for _, r := range []string{"1mo", "3mo", "1y"} {
			key := map[string]string{"1mo": "1", "3mo": "3", "1y": "y"}[r]
			lbl := " " + rangeNames[r]
			if r == m.detailRange {
				parts = append(parts, sKey.Render(key)+sTotal.Render(lbl+" ✓"))
			} else {
				parts = append(parts, sKey.Render(key)+sFaint.Render(lbl))
			}
		}
		rangeHint = strings.Join(parts, sFaint.Render(" · "))
	}
	keys := sKey.Render("d") + sFaint.Render(" $/₺ · ")
	if !isFX {
		keys += sKey.Render("u") + sFaint.Render(" kur · ") + sKey.Render("e") + sFaint.Render(" düzenle · ")
	}
	if len(m.detailNews) > 0 {
		keys += sKey.Render("t") + sFaint.Render(" haber aç · ")
	}
	keys += sKey.Render("esc")
	right := rangeHint + sFaint.Render("   ") + keys
	b.WriteString(joinEdges(left, right, w))
	b.WriteString("\n\n")

	// value line
	if isFX {
		fx := m.quotes[prices.KeyUSDTRY]
		b.WriteString(sTotal.Render("$1 = "+lira(fx.Price, 4)) + "   " +
			paint(fx.DayChangePct()).Bold(true).Render(signedPct(fx.DayChangePct())+" bugün") + "   " +
			sDim.Render("önceki kapanış "+lira(fx.PrevClose, 4)))
	} else {
		val := m.rowVal(row.ValueTRY)
		day := row.DayTRY
		if m.baseUSD {
			day = m.sum.RowDayUSD(row)
		}
		var alt string
		if m.baseUSD {
			alt = sDim.Render("≈ " + lira(row.ValueTRY, 0))
		} else if m.sum.FX > 0 {
			alt = sDim.Render("≈ " + dollar(row.ValueTRY/m.sum.FX, 0))
		}
		b.WriteString(sTotal.Render(m.money(val)) + "   " +
			paint(day).Bold(true).Render(m.signedMoney(day)+" · "+signedPct(row.DayPct)+" bugün") + "   " + alt)
	}
	b.WriteString("\n\n")

	// chart
	vals := m.detailHist.TRY
	labelFn := m.compactMoney
	if isFX {
		labelFn = func(v float64) string { return tr(v, 2) }
	} else if m.baseUSD {
		vals = m.detailHist.USD
	}
	switch {
	case m.detailLoading:
		b.WriteString(m.spin.View() + "  grafik yükleniyor…\n")
	case m.detailErr != nil:
		b.WriteString(sErr.Render("⚠ "+m.detailErr.Error()) + "\n")
	case len(vals) >= 2:
		b.WriteString(m.renderDetailChart(vals, w, labelFn))
	default:
		b.WriteString(sFaint.Render("grafik için veri yok") + "\n")
	}
	b.WriteString("\n")

	// stats
	b.WriteString(m.detailStats(row, isFX, w))

	// news
	if n := m.viewNews(w); n != "" {
		b.WriteString("\n" + n)
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

const detailChartHeight = 7

func (m Model) renderDetailChart(vals []float64, w int, labelFn func(float64) string) string {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range vals {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	hiS, midS, loS := labelFn(hi), labelFn((hi+lo)/2), labelFn(lo)
	labelW := 0
	for _, s := range []string{hiS, midS, loS} {
		if lw := lipgloss.Width(s); lw > labelW {
			labelW = lw
		}
	}
	chartW := w - labelW - 1
	if chartW < 10 {
		return ""
	}
	first, last := vals[0], vals[len(vals)-1]
	chg := 0.0
	if first > 0 {
		chg = (last/first - 1) * 100
	}
	st := paint(chg)

	var b strings.Builder
	for i, ln := range areaChart(vals, chartW, detailChartHeight) {
		lbl, sep := "", "│"
		switch i {
		case 0:
			lbl, sep = hiS, "┤"
		case detailChartHeight / 2:
			lbl, sep = midS, "┤"
		case detailChartHeight - 1:
			lbl, sep = loS, "┤"
		}
		b.WriteString(sFaint.Render(fit(lbl, labelW, true)+sep) + st.Render(ln) + "\n")
	}
	dates := ""
	if n := len(m.detailHist.Dates); n > 0 {
		from, to := m.detailHist.Dates[0], m.detailHist.Dates[n-1]
		dates = fmt.Sprintf("%d %s – %d %s", from.Day(), trMonths[from.Month()-1], to.Day(), trMonths[to.Month()-1])
	}
	left := strings.Repeat(" ", labelW+1) + sFaint.Render(dates)
	right := sFaint.Render("dönem ") + st.Render(signedPct(chg))
	b.WriteString(joinEdges(left, right, w) + "\n")
	return b.String()
}

func (m Model) detailStats(row portfolio.Row, isFX bool, w int) string {
	type kv struct{ k, v string }
	var stats []kv
	now := time.Now()

	if isFX {
		if len(m.detailHist.TRY) >= 2 {
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, v := range m.detailHist.TRY {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
			stats = append(stats, kv{"dönem min", lira(lo, 2)}, kv{"dönem max", lira(hi, 2)})
			for _, win := range []struct {
				lbl  string
				days int
			}{{"7 gün", 7}, {"30 gün", 30}} {
				if c, ok := changeSince(m.detailHist.Dates, m.detailHist.TRY, win.days); ok {
					stats = append(stats, kv{win.lbl, paint(c).Render(signedPct(c))})
				}
			}
		}
	} else {
		if row.Unit != "" && row.Unit != "₺" {
			stats = append(stats, kv{"miktar", amountCell(row.Amount, row.Unit)})
			stats = append(stats, kv{"fiyat", priceCell(row.Price, row.Ccy, row.Unit)})
		} else if row.Unit == "₺" {
			stats = append(stats, kv{"bakiye", amountCell(row.Amount, row.Unit)})
		}
		if m.sum.TotalTRY > 0 {
			stats = append(stats, kv{"portföy payı", pctPlain(row.ValueTRY / m.sum.TotalTRY * 100)})
		}
		if !m.detailLoading && m.detailErr == nil {
			if s, ok := m.dualChange(7); ok {
				stats = append(stats, kv{"7 gün", s})
			}
			if s, ok := m.dualChange(30); ok {
				stats = append(stats, kv{"30 gün", s})
			}
		}
		if !math.IsNaN(row.TotalPL) {
			pl := row.TotalPL
			if m.baseUSD {
				pl = m.rowVal(pl)
			}
			stats = append(stats, kv{"toplam K/Z", m.plCell(pl) + " (" + signedPct(row.TotalPct) + ")"})
		}

		switch {
		case strings.HasPrefix(m.detailKey, "vadeli:"):
			name := strings.TrimPrefix(m.detailKey, "vadeli:")
			for _, d := range m.cfg.Assets.Deposits {
				if d.Name != name {
					continue
				}
				stats = append(stats,
					kv{"anapara", lira(d.Principal, 0)},
					kv{"faiz", fmt.Sprintf("brüt %%%s · stopaj %%%s → net %%%s",
						tr(d.GrossRate*100, 1), tr(d.Stopaj*100, 1), tr(d.Net()*100, 2))},
					kv{"dönem", d.Start.Format("02.01.2006") + " → " + d.Maturity.Format("02.01.2006")},
					kv{"işleyen faiz", lira(d.Value(now)-d.Principal, 0)},
					kv{"günlük tahakkuk", lira(d.DailyAccrual(now), 0)},
					kv{"vade sonu", lira(d.Value(d.Maturity.Time), 0)},
				)
				if left := int(d.Maturity.Sub(now).Hours() / 24); left >= 0 {
					stats = append(stats, kv{"kalan", fmt.Sprintf("%d gün", left)})
				}
			}
		case strings.HasPrefix(m.detailKey, "loan:"):
			name := strings.TrimPrefix(m.detailKey, "loan:")
			for _, l := range m.cfg.Assets.Loans {
				if l.Name != name {
					continue
				}
				kind := "alacak (sana ödenecek)"
				if l.Kind == "borc" {
					kind = "borç (senin ödeyeceğin)"
				}
				unit := "₺"
				if l.Currency == "USD" {
					unit = "$"
				}
				stats = append(stats,
					kv{"tür", kind},
					kv{"tutar", unit + tr(l.Amount, 0)},
				)
				if !l.Due.IsZero() {
					stats = append(stats, kv{"vade", l.Due.Format("02.01.2006")})
					if left := int(l.Due.Sub(now).Hours() / 24); left >= 0 {
						stats = append(stats, kv{"kalan", fmt.Sprintf("%d gün", left)})
					} else {
						stats = append(stats, kv{"kalan", sErr.Render("vadesi geçmiş")})
					}
				}
			}
		case m.detailKey != "gold" && row.Unit == "ad":
			for _, st := range m.cfg.Assets.Stocks {
				if st.Symbol != m.detailKey || st.Cost == 0 {
					continue
				}
				cur := "₺"
				if row.Ccy == "USD" {
					cur = "$"
				}
				stats = append(stats, kv{"ort. maliyet", cur + tr(st.Cost, 2) + "/ad"})
			}
		case strings.HasPrefix(m.detailKey, "crypto:"):
			name := strings.TrimPrefix(m.detailKey, "crypto:")
			for _, cr := range m.cfg.Assets.Cryptos {
				if cr.Name != name {
					continue
				}
				if cr.CostUSD > 0 {
					stats = append(stats, kv{"ort. maliyet", "$" + tr(cr.CostUSD, 2)})
				}
				if cr.Note != "" {
					stats = append(stats, kv{"not", cr.Note})
				}
			}
		case strings.HasPrefix(m.detailKey, "prop:"):
			name := strings.TrimPrefix(m.detailKey, "prop:")
			for _, pr := range m.cfg.Assets.Properties {
				if pr.Name != name {
					continue
				}
				unit := "₺"
				if pr.Currency == "USD" {
					unit = "$"
				}
				stats = append(stats, kv{"değer", unit + tr(pr.Value, 0)})
				if pr.Cost > 0 {
					stats = append(stats, kv{"alış", unit + tr(pr.Cost, 0)})
				}
			}
		case strings.HasPrefix(m.detailKey, "cash:"):
			name := strings.TrimPrefix(m.detailKey, "cash:")
			for _, c := range m.cfg.Assets.Cash {
				if c.Name != name || c.APY == 0 {
					continue
				}
				accrued := c.Units(now) - c.Amount
				unit := "₺"
				if c.Currency == "USD" {
					unit = "$"
				}
				stats = append(stats,
					kv{"yıllık getiri", "%" + tr(c.APY*100, 2)},
					kv{"tahakkuk", fmt.Sprintf("%s%s (%s'den beri)", unit, tr(accrued, 2), c.AsOf.Format("02.01.2006"))},
				)
			}
		}
	}

	// two-column layout
	colW := 0
	for _, s := range stats {
		if lw := lipgloss.Width(s.k); lw > colW {
			colW = lw
		}
	}
	cell := func(s kv) string { return sDim.Render(fit(s.k, colW+2, false)) + sTotal.Render(s.v) }
	half := (w - 4) / 2
	var b strings.Builder
	for i := 0; i < len(stats); i += 2 {
		line := cell(stats[i])
		if i+1 < len(stats) {
			line = fit(line, half, false) + cell(stats[i+1])
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "az önce"
	case d < 24*time.Hour:
		return fmt.Sprintf("%ds önce", int(d.Hours()))
	default:
		return fmt.Sprintf("%dg önce", int(d.Hours()/24))
	}
}

func (m Model) viewNews(w int) string {
	if len(m.detailNews) == 0 {
		return ""
	}
	timeW, pubW := 0, 0
	for _, n := range m.detailNews {
		if l := lipgloss.Width(relTime(n.At)); l > timeW {
			timeW = l
		}
		if l := lipgloss.Width(n.Publisher); l > pubW {
			pubW = l
		}
	}
	if pubW > 18 {
		pubW = 18
	}
	metaW := timeW + 2 + pubW + 3
	titleW := w - metaW - 3 // 3: numara öneki
	if titleW < 24 {        // terminal too narrow for columns: simple fallback
		var b strings.Builder
		b.WriteString(sFaint.Render(strings.Repeat("─", min(w, 40))) + "\n")
		for _, n := range m.detailNews {
			b.WriteString(lipgloss.NewStyle().Width(w).Render(sFaint.Render(relTime(n.At)+" · "+n.Publisher+" — ")+n.Title) + "\n")
		}
		return b.String()
	}

	var b strings.Builder
	header := strings.Repeat("─", min(w-14, 40)) + " haberler"
	if m.newsPick {
		header = strings.Repeat("─", min(w-30, 40)) + " " + "hangisi? "
		b.WriteString(sFaint.Render(header) + sKey.Render("1-"+fmt.Sprint(len(m.detailNews))) + sFaint.Render(" · esc") + "\n")
	} else {
		b.WriteString(sFaint.Render(header) + "\n")
	}
	indent := strings.Repeat(" ", metaW+3)
	for ni, n := range m.detailNews {
		num := sKey.Render(fmt.Sprintf("%d", ni+1)) + sFaint.Render("  ")
		meta := num + sFaint.Render(fit(relTime(n.At), timeW, true)+"  "+fit(n.Publisher, pubW, false)) + "   "
		wrapped := lipgloss.NewStyle().Width(titleW).Render(n.Title)
		for i, line := range strings.Split(wrapped, "\n") {
			if i == 0 {
				b.WriteString(meta + line + "\n")
			} else {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	return b.String()
}
