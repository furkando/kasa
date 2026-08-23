package ui

import (
	"fmt"
	"math"
	"strings"
)

// tr formats 1234567.89 as "1.234.567,89" (Turkish grouping).
func tr(v float64, decimals int) string {
	neg := v < 0
	v = math.Abs(v)
	s := fmt.Sprintf("%.*f", decimals, v)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if decimals > 0 {
		out += "," + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func lira(v float64, decimals int) string   { return "₺" + tr(v, decimals) }
func dollar(v float64, decimals int) string { return "$" + tr(v, decimals) }

// signedPct renders Turkish style: "+%0,48".
func signedPct(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	sign := "+"
	if v < 0 {
		sign = "-"
	}
	return sign + "%" + tr(math.Abs(v), 2)
}

// amount trims trailing zeros: 150 -> "150", 10.5 -> "10,5".
func amount(v float64) string {
	if v == math.Trunc(v) {
		return tr(v, 0)
	}
	return tr(v, 2)
}
