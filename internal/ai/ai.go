package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os/exec"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/portfolio"
	"github.com/furkando/kasa/internal/prices"
	"github.com/furkando/kasa/internal/store"
)

const defaultModel = "claude-opus-5"

const systemPreamble = `Sen "kasa" adlı kişisel portföy CLI'ının asistanısın. Kullanıcı terminalden seninle kendi birikimleri hakkında konuşuyor.

Aşağıdaki JSON, az önce canlı fiyatlarla hesaplanmış güncel portföy verisi (tutarlar aksi belirtilmedikçe TL). Sorulara bu veriye dayanarak Türkçe, kısa ve net cevap ver; hesap yaptığında hesabı göster. Terminalde okunacağı için markdown başlık/tablo yerine düz metin ve kısa satırlar kullan.

Kişiye özel al/sat/yatırım tavsiyesi verme; kullanıcı tavsiye isterse genel bilgi verebilir ve lisanslı danışman olmadığını kısaca hatırlatırsın.

Not: "gizli": true olan satırlar kullanıcının toplam dışında tuttuğu varlıklardır (örn. kilitli tokenlar); toplam_tl/toplam_usd bunları içermez.

PORTFÖY VERİSİ:
`

// BuildContext fetches live quotes + history and renders the system prompt.
func BuildContext(ctx context.Context, cfg *config.Config, st *store.Store) (string, error) {
	quotes, err := prices.FetchQuotes(ctx, cfg)
	if quotes == nil {
		return "", err
	}
	hist, _ := prices.FetchHistories(ctx, cfg, 30)
	return SystemPrompt(cfg, st, quotes, hist, time.Now()), nil
}

// SystemPrompt renders the chat system prompt from already-fetched data —
// the TUI calls this with the quotes/history it is displaying.
func SystemPrompt(cfg *config.Config, st *store.Store, quotes map[string]prices.Quote, hist map[string]prices.Series, now time.Time) string {
	sum := portfolio.Compute(cfg, quotes, now)

	type row struct {
		Varlik   string  `json:"varlik"`
		Kategori string  `json:"kategori,omitempty"`
		Gizli    bool    `json:"gizli,omitempty"`
		Miktar   float64 `json:"miktar,omitempty"`
		Birim    string  `json:"birim,omitempty"`
		Fiyat    float64 `json:"fiyat,omitempty"`
		FiyatCcy string  `json:"fiyat_kuru,omitempty"`
		DegerTL  float64 `json:"deger_tl"`
		GunPct   float64 `json:"gun_degisim_pct"`
		GunTL    float64 `json:"gun_degisim_tl"`
		PayPct   float64 `json:"portfoy_payi_pct"`
	}
	data := map[string]any{
		"tarih":           now.Format("2006-01-02 15:04"),
		"toplam_tl":       round2(sum.TotalTRY),
		"toplam_usd":      round2(sum.TotalUSD),
		"gun_degisim_tl":  round2(sum.DayTRY),
		"gun_degisim_pct": round2(sum.DayPct),
		"usdtry":          quotes[prices.KeyUSDTRY].Price,
		"gram_altin_tl":   quotes[prices.KeyGram].Price,
	}
	rows := make([]row, 0, len(sum.Rows))
	for _, r := range sum.Rows {
		weight := 0.0
		if sum.TotalTRY > 0 {
			weight = r.ValueTRY / sum.TotalTRY * 100
		}
		rows = append(rows, row{
			Varlik: r.Label, Kategori: r.Category, Gizli: r.Hidden,
			Miktar: round2(r.Amount), Birim: r.Unit,
			Fiyat: round2(r.Price), FiyatCcy: r.Ccy,
			DegerTL: round2(r.ValueTRY), GunPct: round2(r.DayPct),
			GunTL: round2(r.DayTRY), PayPct: round2(weight),
		})
	}
	data["varliklar"] = rows

	var deps []map[string]any
	for _, d := range cfg.Assets.Deposits {
		deps = append(deps, map[string]any{
			"ad": d.Name, "anapara_tl": d.Principal,
			"brut_yillik_faiz": d.GrossRate, "stopaj": d.Stopaj,
			"net_yillik_faiz": math.Round(d.Net()*10000) / 10000,
			"baslangic":       d.Start.Format("2006-01-02"),
			"vade":            d.Maturity.Format("2006-01-02"),
			"guncel_deger_tl": round2(d.Value(now)),
			"vade_sonu_tl":    round2(d.Value(d.Maturity.Time)),
		})
	}
	if deps != nil {
		data["vadeli_detay"] = deps
	}

	var loans []map[string]any
	for _, l := range cfg.Assets.Loans {
		entry := map[string]any{"ad": l.Name, "tur": l.Kind, "para_birimi": l.Currency, "tutar": l.Amount}
		if !l.Due.IsZero() {
			entry["vade"] = l.Due.Format("2006-01-02")
		}
		loans = append(loans, entry)
	}
	if loans != nil {
		data["alacak_borc"] = loans
	}

	if hist != nil {
		if dates, total, _, _ := portfolio.Reconstruct(cfg, hist); len(total) > 0 {
			series := make(map[string]float64, len(total))
			for i, d := range dates {
				series[d.Format("2006-01-02")] = round2(total[i])
			}
			data["son_30_gun_toplam_tl"] = series
		}
	}

	if st != nil {
		if snaps := st.Sorted(); len(snaps) > 0 {
			data["gunluk_kayitlar"] = snaps
		}
	}

	b, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return systemPreamble + "(veri hazırlanamadı)"
	}
	return systemPreamble + string(b)
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

type backend interface {
	ask(ctx context.Context, question string, out io.Writer) error
	setSystem(system string)
}

type Chat struct {
	backend  backend
	Provider string // "claude" | "api" — resolved, for display
}

// NewChat picks a backend: explicit cfg.AI.Provider wins; otherwise the
// claude CLI (Max/Pro subscription, no API credits needed) when installed,
// falling back to the Anthropic API.
func NewChat(cfg *config.Config, system string) *Chat {
	prov := cfg.AI.Provider
	if prov == "" {
		if _, err := exec.LookPath("claude"); err == nil {
			prov = "claude"
		} else {
			prov = "api"
		}
	}
	if prov == "claude" {
		return &Chat{Provider: prov, backend: &claudeBackend{system: system, model: cfg.AI.Model}}
	}
	model := cfg.AI.Model
	if model == "" {
		model = defaultModel
	}
	return &Chat{Provider: "api", backend: &apiBackend{client: anthropic.NewClient(), model: model, system: system}}
}

// Ask streams the answer to out and appends the turn to the conversation.
func (c *Chat) Ask(ctx context.Context, question string, out io.Writer) error {
	return c.backend.ask(ctx, question, out)
}

// SetSystem refreshes the portfolio context (after a config edit).
func (c *Chat) SetSystem(system string) { c.backend.setSystem(system) }

type apiBackend struct {
	client anthropic.Client
	model  string
	system string
	msgs   []anthropic.MessageParam
}

func (c *apiBackend) setSystem(system string) { c.system = system }

func (c *apiBackend) ask(ctx context.Context, question string, out io.Writer) error {
	c.msgs = append(c.msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(question)))

	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: c.system}},
		Messages:  c.msgs,
	})
	msg := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return err
		}
		switch ev := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch d := ev.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				fmt.Fprint(out, d.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		// drop the failed user turn so a retry doesn't double it
		c.msgs = c.msgs[:len(c.msgs)-1]
		return err
	}
	if msg.StopReason == "refusal" {
		fmt.Fprint(out, "(model bu isteği yanıtlamayı reddetti)")
	}
	c.msgs = append(c.msgs, msg.ToParam())
	return nil
}
