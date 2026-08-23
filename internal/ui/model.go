package ui

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/furkando/kasa/internal/ai"
	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/portfolio"
	"github.com/furkando/kasa/internal/prices"
	"github.com/furkando/kasa/internal/store"
)

const historyDays = 30

type mode int

const (
	modeDash mode = iota
	modeChat
	modeForm
	modeDetail
)

type chatEntry struct {
	role string // "user" | "assistant" | "error"
	text string
}

type Model struct {
	cfg *config.Config
	st  *store.Store

	// Sender must be set (to Program.Send) before Program.Run so streamed
	// AI deltas can be injected into the update loop.
	Sender func(tea.Msg)
	// Demo marks a randomized sample-data session (kasa demo).
	Demo bool

	quotes      map[string]prices.Quote
	hist        map[string]prices.Series
	sum         portfolio.Summary
	sparksTRY   map[string][]float64
	sparksUSD   map[string][]float64
	totalSpkTRY []float64
	totalSpkUSD []float64
	sparkFrom   time.Time
	sparkTo     time.Time
	baseUSD     bool

	mode      mode
	chatVP    viewport.Model
	chatInput textinput.Model
	chatLog   []chatEntry
	aiChat    *ai.Chat
	aiBusy    bool

	sel        int // selected dashboard row
	form       *form
	confirmDel string // Row.Key pending delete confirmation
	adding     bool   // "n" pressed: choosing new-entity type

	detailKey     string
	detailRange   string // yahoo range: 1mo | 3mo | 1y
	detailHist    prices.AssetHistory
	detailNews    []prices.NewsItem
	detailLoading bool
	detailErr     error
	newsPick      bool // "t" pressed: next digit opens that news item

	sortMode int
	grouped  bool

	spin       spinner.Model
	loading    bool
	err        error
	width      int
	height     int
	lastUpdate time.Time
	countdown  int
}

func New(cfg *config.Config, st *store.Store) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = sSpinner

	ti := textinput.New()
	ti.Placeholder = "portföyün hakkında soru sor…"
	ti.Prompt = "sen › "
	ti.PromptStyle = sKey
	ti.CharLimit = 2000

	return Model{
		cfg: cfg, st: st,
		spin: sp, loading: true,
		chatInput: ti, grouped: true,
		countdown: cfg.RefreshSeconds,
	}
}

type quotesMsg struct {
	q   map[string]prices.Quote
	err error
}
type histMsg struct {
	h   map[string]prices.Series
	err error
}
type tickMsg time.Time
type aiDeltaMsg string
type aiDoneMsg struct{ err error }

func fetchQuotes(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		q, err := prices.FetchQuotes(ctx, cfg)
		return quotesMsg{q, err}
	}
}

func fetchHist(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h, err := prices.FetchHistories(ctx, cfg, historyDays)
		return histMsg{h, err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchQuotes(m.cfg), fetchHist(m.cfg), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.chatVP.Width = m.width - 4
		m.chatVP.Height = m.height - 7
		m.chatInput.Width = m.width - 14
		m.refreshChatVP()
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeChat:
			return m.updateChat(msg)
		case modeForm:
			return m.updateForm(msg)
		case modeDetail:
			return m.updateDetail(msg)
		}
		if m.confirmDel != "" {
			switch msg.String() {
			case "y", "e", "enter":
				m.deleteRow(m.confirmDel)
				m.confirmDel = ""
				if err := config.Save(m.cfg, config.DefaultPath()); err != nil {
					m.err = err
					return m, nil
				}
				m.afterConfigChange()
			default:
				m.confirmDel = ""
			}
			return m, nil
		}
		if m.adding {
			m.adding = false
			switch msg.String() {
			case "c":
				m.form = cashForm("Yeni nakit", nil, "")
			case "h":
				m.form = stockForm("Yeni hisse", nil, "")
			case "v":
				m.form = depositForm("Yeni vadeli", nil, "")
			case "b":
				m.form = loanForm("Yeni alacak/borç", nil, "")
			case "m":
				m.form = propertyForm("Yeni mülk", nil, "")
			case "k":
				m.form = cryptoForm("Yeni kripto", nil, "")
			case "g":
				if m.cfg.Assets.Gold == nil {
					m.form = goldForm(nil)
				}
			}
			if m.form != nil {
				m.mode = modeForm
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, tea.Batch(m.spin.Tick, fetchQuotes(m.cfg), fetchHist(m.cfg))
		case "d", "$":
			m.baseUSD = !m.baseUSD
			return m, nil
		case "a", "i":
			return m.enterChat()
		case "o":
			m.form = settingsForm(m.cfg)
			m.mode = modeForm
			return m, nil
		case "u":
			if m.quotes != nil {
				return m.openDetail("fx")
			}
			return m, nil
		case "s":
			m.cycleSort()
			return m, nil
		case "g":
			m.grouped = !m.grouped
			m.sortRows()
			return m, nil
		case "h":
			if m.sel >= 0 && m.sel < len(m.sum.Rows) {
				key := m.sum.Rows[m.sel].Key
				m.cfg.ToggleHidden(key)
				if err := config.Save(m.cfg, config.DefaultPath()); err != nil {
					m.err = err
					return m, nil
				}
				m.afterConfigChange()
				for i, r := range m.sum.Rows {
					if r.Key == key {
						m.sel = i
					}
				}
			}
			return m, nil
		case "n":
			m.adding = true
			return m, nil
		case "x", "backspace":
			if m.sel >= 0 && m.sel < len(m.sum.Rows) {
				m.confirmDel = m.sum.Rows[m.sel].Key
			}
			return m, nil
		case "up", "k":
			if m.sel > 0 {
				m.sel--
			}
			return m, nil
		case "down", "j":
			if m.sel < len(m.sum.Rows)-1 {
				m.sel++
			}
			return m, nil
		case "enter":
			if m.sel >= 0 && m.sel < len(m.sum.Rows) {
				return m.openDetail(m.sum.Rows[m.sel].Key)
			}
			return m, nil
		case "e":
			if m.sel >= 0 && m.sel < len(m.sum.Rows) {
				if f := m.formFor(m.sum.Rows[m.sel].Key); f != nil {
					m.form = f
					m.mode = modeForm
				}
			}
			return m, nil
		}

	case tickMsg:
		m.countdown--
		if m.countdown <= 0 {
			m.countdown = m.cfg.RefreshSeconds
			return m, tea.Batch(tick(), fetchQuotes(m.cfg))
		}
		return m, tick()

	case quotesMsg:
		m.loading = false
		m.err = msg.err // may be a partial-data warning alongside quotes
		if msg.q != nil {
			m.quotes = msg.q
			m.lastUpdate = time.Now()
			m.recompute()
			m.persist()
		}
		return m, nil

	case histMsg:
		if msg.err == nil {
			m.hist = msg.h
			m.rebuildSparks()
		}
		return m, nil

	case detailMsg:
		if msg.key == m.detailKey && msg.rng == m.detailRange {
			m.detailLoading = false
			m.detailHist = msg.hist
			m.detailNews = msg.news
			m.detailErr = msg.err
		}
		return m, nil

	case aiDeltaMsg:
		if n := len(m.chatLog); n > 0 && m.chatLog[n-1].role == "assistant" {
			m.chatLog[n-1].text += string(msg)
			m.refreshChatVP()
		}
		return m, nil

	case aiDoneMsg:
		m.aiBusy = false
		if msg.err != nil {
			errText := []rune(msg.err.Error())
			if len(errText) > 300 {
				errText = append(errText[:300], '…')
			}
			m.chatLog = append(m.chatLog, chatEntry{"error", "AI hatası: " + string(errText)})
		}
		m.refreshChatVP()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.loading || m.aiBusy || m.detailLoading {
			if m.aiBusy {
				m.refreshChatVP()
			}
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m Model) enterChat() (tea.Model, tea.Cmd) {
	if m.quotes == nil {
		return m, nil // no data yet
	}
	if m.aiChat == nil {
		system := ai.SystemPrompt(m.cfg, m.st, m.quotes, m.hist, time.Now())
		m.aiChat = ai.NewChat(m.cfg, system)
		m.chatLog = append(m.chatLog, chatEntry{"assistant",
			"Portföy verin yüklendi — sorabilirsin. (esc: panele dön)"})
	}
	m.mode = modeChat
	m.chatInput.Focus()
	m.refreshChatVP()
	return m, textinput.Blink
}

func (m Model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeDash
		m.chatInput.Blur()
		return m, nil
	case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.chatVP, cmd = m.chatVP.Update(msg)
		return m, cmd
	case "enter":
		q := strings.TrimSpace(m.chatInput.Value())
		if q == "" || m.aiBusy || m.Sender == nil {
			return m, nil
		}
		m.chatInput.Reset()
		m.chatLog = append(m.chatLog, chatEntry{"user", q}, chatEntry{"assistant", ""})
		m.aiBusy = true
		m.refreshChatVP()
		chat, send := m.aiChat, m.Sender
		askCmd := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			err := chat.Ask(ctx, q, writerFunc(func(p []byte) (int, error) {
				send(aiDeltaMsg(p))
				return len(p), nil
			}))
			return aiDoneMsg{err}
		}
		return m, tea.Batch(m.spin.Tick, askCmd)
	}
	var cmd tea.Cmd
	m.chatInput, cmd = m.chatInput.Update(msg)
	return m, cmd
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// contentWidth is the usable width with a sane floor when the terminal
// size is not (yet) known.
func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 96
	}
	return m.width - 4
}

func (m *Model) afterConfigChange() {
	m.recompute()
	m.persist()
	m.rebuildSparks()
	if m.sel >= len(m.sum.Rows) {
		m.sel = len(m.sum.Rows) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.aiChat != nil && !m.aiBusy {
		m.aiChat.SetSystem(ai.SystemPrompt(m.cfg, m.st, m.quotes, m.hist, time.Now()))
	}
}

var sortNames = []string{"değer", "gün %", "gün ₺", "K/Z", "ad"}

func (m *Model) cycleSort() {
	var selKey string
	if m.sel >= 0 && m.sel < len(m.sum.Rows) {
		selKey = m.sum.Rows[m.sel].Key
	}
	m.sortMode = (m.sortMode + 1) % len(sortNames)
	m.sortRows()
	for i, r := range m.sum.Rows {
		if r.Key == selKey {
			m.sel = i
		}
	}
}

func (m *Model) sortRows() {
	rows := m.sum.Rows
	val := func(v float64) float64 { // NaN sorts last on descending order
		if math.IsNaN(v) {
			return math.Inf(-1)
		}
		return v
	}
	cmp := func(a, b portfolio.Row) bool {
		switch m.sortMode {
		case 1:
			return a.DayPct > b.DayPct
		case 2:
			return a.DayTRY > b.DayTRY
		case 3:
			return val(a.TotalPL) > val(b.TotalPL)
		case 4:
			return strings.ToLower(a.Label) < strings.ToLower(b.Label)
		default:
			return math.Abs(a.ValueTRY) > math.Abs(b.ValueTRY)
		}
	}
	groupVal := map[string]float64{}
	for _, r := range rows {
		if !r.Hidden {
			groupVal[r.Category] += math.Abs(r.ValueTRY)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Hidden != b.Hidden {
			return !a.Hidden // hidden rows sink to the bottom
		}
		if m.grouped && a.Category != b.Category {
			if groupVal[a.Category] != groupVal[b.Category] {
				return groupVal[a.Category] > groupVal[b.Category]
			}
			return a.Category < b.Category
		}
		return cmp(a, b)
	})
}

func (m *Model) recompute() {
	m.sum = portfolio.Compute(m.cfg, m.quotes, time.Now())
	m.sortRows()
}

func (m *Model) rebuildSparks() {
	dates, total, per, fxv := portfolio.Reconstruct(m.cfg, m.hist)
	if len(dates) > 0 {
		m.sparkFrom, m.sparkTo = dates[0], dates[len(dates)-1]
	}
	m.totalSpkTRY = total
	m.sparksTRY = per
	m.totalSpkUSD = divSeries(total, fxv)
	m.sparksUSD = map[string][]float64{}
	for k, v := range per {
		m.sparksUSD[k] = divSeries(v, fxv)
	}
}

func divSeries(vals, by []float64) []float64 {
	if len(vals) != len(by) {
		return nil
	}
	out := make([]float64, len(vals))
	for i, v := range vals {
		if by[i] == 0 {
			return nil
		}
		out[i] = v / by[i]
	}
	return out
}

func (m *Model) persist() {
	if m.sum.TotalTRY == 0 {
		return
	}
	assets := map[string]float64{}
	for _, r := range m.sum.Rows {
		assets[r.Key] = r.ValueTRY
	}
	_ = m.st.Put(store.Snapshot{
		Date:     store.Today(),
		TotalTRY: m.sum.TotalTRY,
		TotalUSD: m.sum.TotalUSD,
		Assets:   assets,
	})
}
