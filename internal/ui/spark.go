package ui

import "math"

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// areaChart renders vals as a height-row area chart. Each returned line is
// exactly the plot; the caller attaches axis labels.
func areaChart(vals []float64, width, height int) []string {
	if width <= 0 || height <= 0 || len(vals) == 0 {
		return nil
	}
	pts := resample(vals, width)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range pts {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	span := hi - lo
	rows := make([][]rune, height)
	for r := range rows {
		rows[r] = make([]rune, len(pts))
		for c := range rows[r] {
			rows[r][c] = ' '
		}
	}
	for c, v := range pts {
		frac := 0.5
		if span > 0 {
			frac = (v - lo) / span
		}
		// level in eighth-blocks over the full column height; floor so the
		// minimum still draws one thin sliver
		eighths := int(math.Round(frac * float64(height*8)))
		if eighths == 0 {
			eighths = 1
		}
		for r := 0; r < height; r++ {
			rowFromBottom := height - 1 - r
			cellEighths := eighths - rowFromBottom*8
			if cellEighths >= 8 {
				rows[r][c] = '█'
			} else if cellEighths > 0 {
				rows[r][c] = sparkRunes[cellEighths-1]
			}
		}
	}
	out := make([]string, height)
	for r := range rows {
		out[r] = string(rows[r])
	}
	return out
}

// sparkline renders vals into exactly width cells, min-max normalized;
// the series is linearly resampled up or down to fit.
func sparkline(vals []float64, width int) string {
	if width <= 0 || len(vals) == 0 {
		return ""
	}
	pts := resample(vals, width)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range pts {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	out := make([]rune, 0, width)
	for _, v := range pts {
		idx := 0
		if hi > lo {
			idx = int((v - lo) / (hi - lo) * float64(len(sparkRunes)-1))
		} else {
			idx = len(sparkRunes) / 2
		}
		out = append(out, sparkRunes[idx])
	}
	return string(out)
}

func resample(vals []float64, width int) []float64 {
	if len(vals) == width || len(vals) < 2 || width < 2 {
		return vals
	}
	out := make([]float64, width)
	for i := range out {
		pos := float64(i) * float64(len(vals)-1) / float64(width-1)
		j := int(pos)
		frac := pos - float64(j)
		if j+1 < len(vals) {
			out[i] = vals[j]*(1-frac) + vals[j+1]*frac
		} else {
			out[i] = vals[j]
		}
	}
	return out
}
