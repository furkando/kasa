package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) refreshChatVP() {
	if m.chatVP.Width <= 0 {
		return
	}
	wrap := lipgloss.NewStyle().Width(m.chatVP.Width)
	var b strings.Builder
	for i, e := range m.chatLog {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch e.role {
		case "user":
			b.WriteString(wrap.Render(sKey.Render("sen › ") + e.text))
		case "error":
			b.WriteString(wrap.Render(sErr.Render(e.text)))
		default:
			text := e.text
			if text == "" && m.aiBusy {
				text = m.spin.View() + " düşünüyor…"
			}
			b.WriteString(wrap.Render(text))
		}
	}
	m.chatVP.SetContent(b.String())
	m.chatVP.GotoBottom()
}

func (m Model) viewChat() string {
	w := m.contentWidth()

	left := sBrand.Render("◆ kasa ai")
	right := sDim.Render(m.money(m.baseTotal()) + "  " + m.paintDay().Render(m.signedMoney(m.baseDay())))
	header := joinEdges(left, right, w)

	input := m.chatInput.View()
	footer := sKey.Render("esc") + sFaint.Render(" panel · ") +
		sKey.Render("enter") + sFaint.Render(" gönder · ") +
		sKey.Render("↑↓") + sFaint.Render(" kaydır")

	body := header + "\n\n" + m.chatVP.View() + "\n\n" + input + "\n" + footer
	return lipgloss.NewStyle().Padding(0, 2).Render(body)
}
