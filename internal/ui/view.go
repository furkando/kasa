package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"github.com/furkando/kasa/internal/prices"
)

var trMonths = [...]string{"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"}
var trDays = [...]string{"Paz", "Pzt", "Sal", "Çar", "Per", "Cum", "Cmt"}

type col struct {
	title string
	w     int
	right bool
	minW  int // hide the column when terminal width < minW
}

var cols = []col{
	{"VARLIK", 14, false, 0},
	{"MİKTAR", 14, true, 100},
	{"FİYAT", 12, true, 0},
	{"GÜN", 8, true, 0},
	{"GÜN ₺", 11, true, 72},
	{"DEĞER", 14, true, 0},
	{"PAY", 7, true, 86},
	{"K/Z ₺", 12, true, 96},
	{"K/Z", 8, true, 110},
	{"30G", 12, false, 122},
}

// --- base-currency helpers -------------------------------------------------

func (m Model) money(v float64) string {
	neg := v < 0
	s := lira(math.Abs(v), 0)
	if m.baseUSD {
		s = dollar(math.Abs(v), 0)
	}
	if neg {
		return "-" + s
	}
	return s
}

func (m Model) signedMoney(v float64) string {
	s := m.money(math.Abs(v))
	if v >= 0 {
		return "+" + s
	}
	return "-" + s
}

func (m Model) baseTotal() float64 {
	if m.baseUSD {
		return m.sum.TotalUSD
	}
	return m.sum.TotalTRY
}

func (m Model) baseDay() float64 {
	if m.baseUSD {
		return m.sum.DayUSD()
	}
	return m.sum.DayTRY
}

func (m Model) paintDay() lipgloss.Style { return paint(m.baseDay()) }

// rowVal / rowDay / rowPL express a row in the active base currency.
func (m Model) rowVal(v float64) float64 {
	if m.baseUSD && m.sum.FX > 0 {
		return v / m.sum.FX
	}
	return v
}

func (m Model) colTitle(t string) string {
	if m.baseUSD {
		return strings.ReplaceAll(t, "₺", "$")
	}
	return t
}

// ---------------------------------------------------------------------------

func (m Model) View() string {
	switch m.mode {
	case modeChat:
		return m.viewChat()
	case modeForm:
		return m.viewForm()
	case modeDetail:
		return m.viewDetail()
	}
	w := m.width
	if w <= 0 {
		w = 100
	}

	if m.quotes == nil {
		body := m.spin.View() + "  fiyatlar alınıyor…"
		if m.err != nil {
			body = sErr.Render("✗ " + m.err.Error())
		}
		return lipgloss.Place(w, m.height, lipgloss.Center, lipgloss.Center, body)
	}

	var b strings.Builder
	pad := lipgloss.NewStyle().Padding(0, 2)

	b.WriteString(m.viewHeader(w - 4))
	b.WriteString("\n\n")
	b.WriteString(m.viewTotals())
	b.WriteString("\n\n")
	if s := m.viewSpark(w - 4); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	b.WriteString(m.viewTable(w - 4))
	b.WriteString("\n\n")
	b.WriteString(m.viewFooter(w - 4))

	return pad.Render(b.String())
}

func (m Model) viewHeader(w int) string {
	left := sBrand.Render("◆ kasa")
	if m.Demo {
		left += sFaint.Render(" · demo (örnek veri)")
	}
	now := time.Now()
	date := fmt.Sprintf("%d %s %d · %s", now.Day(), trMonths[now.Month()-1], now.Year(), trDays[now.Weekday()])
	refresh := fmt.Sprintf("↻ %ds", m.countdown)
	if m.loading {
		refresh = m.spin.View() + " "
	}
	right := sDim.Render(date+"  ") + sFaint.Render(refresh)
	return joinEdges(left, right, w)
}

func (m Model) viewTotals() string {
	total := sTotal.Render(m.money(m.baseTotal()))
	dayPct := pctOf(m.baseDay(), m.baseTotal())
	day := m.paintDay().Bold(true).Render(m.signedMoney(m.baseDay()) + " · " + signedPct(dayPct) + " bugün")
	var alt string
	if m.baseUSD {
		alt = sDim.Render("≈ " + lira(m.sum.TotalTRY, 0))
	} else {
		alt = sDim.Render("≈ " + dollar(m.sum.TotalUSD, 0))
	}
	line := total + "   " + day + "   " + alt
	if fx, ok := m.quotes[prices.KeyUSDTRY]; ok && fx.Price > 0 {
		line += "   " + sFaint.Render("│ $1=") + sDim.Render(lira(fx.Price, 2)) + " " +
			paint(fx.DayChangePct()).Render(signedPct(fx.DayChangePct())) + sFaint.Render(" → u")
	}
	return line
}

func pctOf(delta, total float64) float64 {
	base := total - delta
	if base == 0 {
		return 0
	}
	return delta / base * 100
}

func (m Model) totalSpark() []float64 {
	if m.baseUSD {
		return m.totalSpkUSD
	}
	return m.totalSpkTRY
}

func (m Model) rowSpark(key string) []float64 {
	if m.baseUSD {
		return m.sparksUSD[key]
	}
	return m.sparksTRY[key]
}

func (m Model) compactMoney(v float64) string {
	sym := "₺"
	if m.baseUSD {
		sym = "$"
	}
	abs := math.Abs(v)
	switch {
	case abs >= 1e6:
		return sym + tr(v/1e6, 1) + "M"
	case abs >= 1e3:
		return sym + tr(v/1e3, 0) + "K"
	default:
		return sym + tr(v, 0)
	}
}

const chartHeight = 5

func (m Model) viewSpark(w int) string {
	spk := m.totalSpark()
	if len(spk) < 2 {
		return ""
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range spk {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	first, last := spk[0], spk[len(spk)-1]
	chg := 0.0
	if first > 0 {
		chg = (last/first - 1) * 100
	}

	hiS, midS, loS := m.compactMoney(hi), m.compactMoney((hi+lo)/2), m.compactMoney(lo)
	labelW := lipgloss.Width(hiS)
	for _, s := range []string{midS, loS} {
		if lw := lipgloss.Width(s); lw > labelW {
			labelW = lw
		}
	}

	chartW := w - labelW - 1
	if chartW < 10 {
		return ""
	}
	lines := areaChart(spk, chartW, chartHeight)
	st := paint(chg)

	var b strings.Builder
	for i, ln := range lines {
		lbl, sep := "", "│"
		switch i {
		case 0:
			lbl, sep = hiS, "┤"
		case chartHeight / 2:
			lbl, sep = midS, "┤"
		case chartHeight - 1:
			lbl, sep = loS, "┤"
		}
		b.WriteString(sFaint.Render(fit(lbl, labelW, true)+sep) + st.Render(ln) + "\n")
	}

	dates := ""
	if !m.sparkFrom.IsZero() {
		dates = fmt.Sprintf("%d %s – %d %s",
			m.sparkFrom.Day(), trMonths[m.sparkFrom.Month()-1],
			m.sparkTo.Day(), trMonths[m.sparkTo.Month()-1])
	}
	left := strings.Repeat(" ", labelW+1) + sFaint.Render(dates)
	right := sFaint.Render("son 30 gün ") + st.Render(signedPct(chg))
	b.WriteString(joinEdges(left, right, w))
	return b.String()
}

func (m Model) visibleCols(w int) []col {
	out := make([]col, 0, len(cols))
	for _, c := range cols {
		if c.minW == 0 || w >= c.minW {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) viewTable(w int) string {
	vc := m.visibleCols(w)

	// stretch to the full terminal width: extra space goes to the label
	// column and the spark chart
	baseW := 2 // selection gutter
	for i, c := range vc {
		if i > 0 {
			baseW += 2
		}
		baseW += c.w
	}
	if extra := w - baseW; extra > 0 {
		for i := range vc {
			if vc[i].title == "VARLIK" {
				grow := extra
				if grow > 10 {
					grow = 10
				}
				vc[i].w += grow
				extra -= grow
			}
		}
		for i := range vc {
			if vc[i].title == "30G" && extra > 0 {
				vc[i].w += extra
				extra = 0
			}
		}
		for i := range vc {
			if vc[i].title == "MİKTAR" && extra > 0 {
				grow := extra
				if grow > 6 {
					grow = 6
				}
				vc[i].w += grow
			}
		}
	}
	sparkW := 12
	for _, c := range vc {
		if c.title == "30G" {
			sparkW = c.w
		}
	}

	var b strings.Builder

	sortCols := map[int]string{0: "DEĞER", 1: "GÜN", 2: "GÜN ₺", 3: "K/Z ₺", 4: "VARLIK"}
	b.WriteString("  ")
	for i, c := range vc {
		if i > 0 {
			b.WriteString("  ")
		}
		title := m.colTitle(c.title)
		if sortCols[m.sortMode] == c.title {
			title += "↓"
		}
		b.WriteString(sHeader.Render(fit(title, c.w, c.right)))
	}
	b.WriteString("\n")

	tableW := 0
	for i, c := range vc {
		if i > 0 {
			tableW += 2
		}
		tableW += c.w
	}
	b.WriteString("  " + sFaint.Render(strings.Repeat("─", tableW)))
	b.WriteString("\n")

	type gsum struct{ val, day float64 }
	groups := map[string]*gsum{}
	for _, r := range m.sum.Rows {
		if r.Hidden {
			continue
		}
		g := groups[r.Category]
		if g == nil {
			g = &gsum{}
			groups[r.Category] = g
		}
		g.val += m.rowVal(r.ValueTRY)
		if m.baseUSD {
			g.day += m.sum.RowDayUSD(r)
		} else {
			g.day += r.DayTRY
		}
	}
	prevCat := ""
	hiddenHeader := false
	for ri, r := range m.sum.Rows {
		if r.Hidden && !hiddenHeader {
			hiddenHeader = true
			b.WriteString("\n  " + sFaint.Render(fit("✕ GİZLİ — toplama dahil değil", tableW, false)) + "\n")
		}
		if m.grouped && !r.Hidden && r.Category != prevCat {
			prevCat = r.Category
			g := groups[r.Category]
			pay := 0.0
			if m.sum.TotalTRY != 0 {
				pay = g.val / m.rowVal(m.sum.TotalTRY) * 100
			}
			gcells := map[string]struct {
				text  string
				style lipgloss.Style
			}{
				"VARLIK": {"▾ " + strings.ToUpperSpecial(unicode.TurkishCase, r.Category), sHeader},
				"GÜN ₺":  {m.signedMoney(g.day), paint(g.day)},
				"DEĞER":  {m.money(g.val), sDim},
				"PAY":    {pctPlain(pay), sDim},
			}
			if ri > 0 {
				b.WriteString("\n")
			}
			b.WriteString("  ")
			for i, c := range vc {
				if i > 0 {
					b.WriteString("  ")
				}
				cell, ok := gcells[c.title]
				if !ok {
					b.WriteString(strings.Repeat(" ", c.w))
					continue
				}
				b.WriteString(cell.style.Render(fit(cell.text, c.w, c.right)))
			}
			b.WriteString("\n")
		}
		if ri == m.sel {
			b.WriteString(sKey.Render("▸ "))
		} else {
			b.WriteString("  ")
		}
		weight := 0.0
		if m.sum.TotalTRY > 0 {
			weight = r.ValueTRY / m.sum.TotalTRY * 100
		}
		missing := r.Price == 0

		dayVal := r.DayTRY
		if m.baseUSD {
			dayVal = m.sum.RowDayUSD(r)
		}
		plVal := r.TotalPL
		if m.baseUSD && !math.IsNaN(plVal) {
			plVal = m.rowVal(plVal)
		}

		cells := map[string]struct {
			text  string
			style lipgloss.Style
		}{
			"VARLIK": {r.Label, lipgloss.NewStyle()},
			"MİKTAR": {amountCell(r.Amount, r.Unit), sDim},
			"FİYAT":  {priceCell(r.Price, r.Ccy, r.Unit), sDim},
			"GÜN":    {signedPct(r.DayPct), paint(r.DayPct)},
			"GÜN ₺":  {m.signedMoney(dayVal), paint(dayVal)},
			"DEĞER":  {m.money(m.rowVal(r.ValueTRY)), sTotal},
			"PAY":    {pctPlain(weight), sDim},
			"K/Z ₺":  {m.plCell(plVal), paintNaN(plVal)},
			"K/Z":    {signedPct(r.TotalPct), paintNaN(r.TotalPct)},
			"30G":    {sparkline(m.rowSpark(r.Key), sparkW), sFaint},
		}
		if missing {
			cells["FİYAT"] = cellOf("—", sFaint)
			cells["GÜN"] = cellOf("—", sFaint)
			cells["GÜN ₺"] = cellOf("—", sFaint)
			cells["DEĞER"] = cellOf("veri yok", sErr)
		}
		if r.Hidden {
			cells["PAY"] = cellOf("—", sFaint)
			for k, cell := range cells {
				cells[k] = cellOf(cell.text, sFaint)
			}
		}

		for i, c := range vc {
			if i > 0 {
				b.WriteString("  ")
			}
			cell := cells[c.title]
			b.WriteString(cell.style.Render(fit(cell.text, c.w, c.right)))
		}
		b.WriteString("\n")
	}

	b.WriteString("  " + sFaint.Render(strings.Repeat("─", tableW)))
	b.WriteString("\n")

	totalPL := m.sum.TotalPL
	if m.baseUSD && !math.IsNaN(totalPL) {
		totalPL = m.rowVal(totalPL)
	}
	totals := map[string]struct {
		text  string
		style lipgloss.Style
	}{
		"VARLIK": {"TOPLAM", sHeader},
		"GÜN":    {signedPct(pctOf(m.baseDay(), m.baseTotal())), m.paintDay().Bold(true)},
		"GÜN ₺":  {m.signedMoney(m.baseDay()), m.paintDay().Bold(true)},
		"DEĞER":  {m.money(m.baseTotal()), sTotal},
		"PAY":    {pctPlain(100), sDim},
		"K/Z ₺":  {m.plCell(totalPL), paintNaN(totalPL).Bold(true)},
		"K/Z":    {signedPct(m.sum.TotalPLPct), paintNaN(m.sum.TotalPLPct).Bold(true)},
	}
	b.WriteString("  ")
	for i, c := range vc {
		if i > 0 {
			b.WriteString("  ")
		}
		cell, ok := totals[c.title]
		if !ok {
			b.WriteString(strings.Repeat(" ", c.w))
			continue
		}
		b.WriteString(cell.style.Render(fit(cell.text, c.w, c.right)))
	}
	return b.String()
}

func (m Model) viewFooter(w int) string {
	if m.confirmDel != "" {
		label := m.confirmDel
		for _, r := range m.sum.Rows {
			if r.Key == m.confirmDel {
				label = r.Label
			}
		}
		return sErr.Render("✗ «"+label+"» silinsin mi? ") + sKey.Render("y") + sFaint.Render(" evet · başka tuş iptal")
	}
	if m.adding {
		return sKey.Render("yeni › ") +
			sKey.Render("c") + sFaint.Render(" nakit · ") +
			sKey.Render("h") + sFaint.Render(" hisse · ") +
			sKey.Render("v") + sFaint.Render(" vadeli · ") +
			sKey.Render("b") + sFaint.Render(" alacak/borç · ") +
			sKey.Render("m") + sFaint.Render(" mülk · ") +
			sKey.Render("k") + sFaint.Render(" kripto · ") +
			sKey.Render("g") + sFaint.Render(" altın · ") +
			sFaint.Render("esc iptal")
	}
	left := sKey.Render("enter") + sFaint.Render(" detay · ") +
		sKey.Render("e") + sFaint.Render(" düzenle · ") +
		sKey.Render("n") + sFaint.Render(" yeni · ") +
		sKey.Render("x") + sFaint.Render(" sil · ") +
		sKey.Render("a") + sFaint.Render(" ai · ") +
		sKey.Render("d") + sFaint.Render(" $/₺ · ") +
		sKey.Render("u") + sFaint.Render(" kur · ") +
		sKey.Render("h") + sFaint.Render(" gizle · ") +
		sKey.Render("g") + sFaint.Render(" grup · ") +
		sKey.Render("s") + sFaint.Render(" sıra["+sortNames[m.sortMode]+"] · ") +
		sKey.Render("o") + sFaint.Render(" ayarlar · ") +
		sKey.Render("q") + sFaint.Render(" çık")
	right := ""
	if !m.lastUpdate.IsZero() {
		right = sFaint.Render("güncellendi " + m.lastUpdate.Format("15:04:05"))
	}
	line := joinEdges(left, right, w)
	if m.err != nil {
		line += "\n" + sErr.Render("⚠ "+m.err.Error())
	}
	return line
}

func cellOf(t string, s lipgloss.Style) struct {
	text  string
	style lipgloss.Style
} {
	return struct {
		text  string
		style lipgloss.Style
	}{t, s}
}

func paintNaN(v float64) lipgloss.Style {
	if math.IsNaN(v) {
		return sFaint
	}
	return paint(v)
}

func amountCell(v float64, unit string) string {
	if unit == "" {
		return "—"
	}
	switch unit {
	case "$":
		return "$" + amount(v)
	case "₺":
		return "₺" + amount(v)
	default:
		return amount(v) + " " + unit
	}
}

func priceCell(v float64, ccy, unit string) string {
	if unit == "" || unit == "₺" { // deposits and TL cash have no unit price
		return "—"
	}
	if ccy == "USD" {
		return dollar(v, 2)
	}
	return lira(v, 2)
}

func (m Model) plCell(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	return m.signedMoney(v)
}

func pctPlain(v float64) string {
	return "%" + tr(v, 1)
}

// fit pads (or truncates) s to visual width w.
func fit(s string, w int, right bool) string {
	if w <= 0 {
		return ""
	}
	sw := lipgloss.Width(s)
	if sw > w {
		return truncate(s, w)
	}
	padding := strings.Repeat(" ", w-sw)
	if right {
		return padding + s
	}
	return s + padding
}

func truncate(s string, w int) string {
	if w <= 1 {
		return "…"
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

func joinEdges(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
