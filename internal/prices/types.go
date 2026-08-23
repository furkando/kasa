package prices

import "time"

type Quote struct {
	Symbol    string
	Name      string
	Price     float64
	PrevClose float64
	Currency  string // "TRY" or "USD"
	At        time.Time
}

func (q Quote) DayChangePct() float64 {
	if q.PrevClose == 0 {
		return 0
	}
	return (q.Price - q.PrevClose) / q.PrevClose * 100
}

// Series is a daily close history, oldest first.
type Series struct {
	Symbol   string
	Currency string
	Dates    []time.Time
	Closes   []float64
}
