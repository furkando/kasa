package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/prices"
	"github.com/furkando/kasa/internal/store"
)

func TestChatInputReceivesKeys(t *testing.T) {
	m := New(&config.Config{RefreshSeconds: 30}, &store.Store{})
	m.Sender = func(tea.Msg) {}
	m.quotes = map[string]prices.Quote{prices.KeyUSDTRY: {Price: 47, PrevClose: 47}}
	m.width, m.height = 100, 30

	step := func(msg tea.Msg) {
		var mod tea.Model
		mod, _ = m.Update(msg)
		m = mod.(Model)
	}

	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.mode != modeChat {
		t.Fatalf("expected chat mode after 'a', got %v", m.mode)
	}
	for _, r := range "kac param" {
		step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := m.chatInput.Value(); got != "kac param" {
		t.Fatalf("input value = %q", got)
	}
	step(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.aiBusy {
		t.Fatalf("expected aiBusy after enter")
	}
	if len(m.chatLog) < 2 || m.chatLog[len(m.chatLog)-2].text != "kac param" {
		t.Fatalf("chat log = %+v", m.chatLog)
	}
}
