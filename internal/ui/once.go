package ui

import (
	"context"
	"time"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/prices"
	"github.com/furkando/kasa/internal/store"
)

// RenderOnce fetches everything synchronously and returns a single static
// frame — for `kasa --once` (quick glance / scripting, no alt-screen).
func RenderOnce(cfg *config.Config, st *store.Store, width int, demo bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q, err := prices.FetchQuotes(ctx, cfg)
	if q == nil {
		return "", err
	}
	h, _ := prices.FetchHistories(ctx, cfg, historyDays)

	m := New(cfg, st)
	m.width = width
	m.loading = false
	m.Demo = demo
	m.quotes = q
	m.hist = h
	m.err = err
	m.lastUpdate = time.Now()
	m.recompute()
	m.rebuildSparks()
	m.persist()
	return m.View(), nil
}
