package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/k-narusawa/tk/domain"
)

const help = " j/k:移動 space:完了 n:追加 enter:開く d:diff a:AI r:更新 q:終了"

func (m Model) View() tea.View {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	left := m.width * 40 / 100
	right := m.width - left - 4
	inner := max(1, m.height-4)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		box.Width(max(1, left)).Height(inner).Render(m.listView()),
		box.Width(max(1, right)).Height(inner).Render(m.detail.View()),
	)

	footer := help
	if m.errMsg != "" {
		footer = " " + m.errMsg
	}
	if m.adding {
		footer = m.input.View()
	}

	v := tea.NewView(body + "\n" + footer)
	v.AltScreen = true
	if m.adding {
		v.Cursor = m.input.Cursor()
	}
	return v
}

func (m Model) listView() string {
	var b strings.Builder
	for i, it := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		b.WriteString(cursor + itemLabel(it) + "\n")
	}
	return b.String()
}

func itemLabel(it domain.Item) string {
	if it.Kind == domain.KindPR {
		return fmt.Sprintf("#%d %s", it.Number, it.Title)
	}
	mark := "□ "
	if it.Done {
		mark = "▣ "
	}
	return mark + it.Title
}
