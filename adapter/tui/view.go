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
	// 端末幅に収める。ヘルプは固定 64 桁あり、gh の stderr はさらに長くなる。
	// 溢れると折り返してレイアウトが崩れる。
	if m.width > 0 {
		footer = lipgloss.NewStyle().MaxWidth(m.width).Render(footer)
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

// syncDetail は選択中アイテムの内容を viewport に流し込む。
func (m *Model) syncDetail() {
	it, ok := m.selected()
	if !ok {
		m.detail.SetContent("")
		return
	}
	e, loaded := m.details[it.ID]
	m.detail.SetContent(detailText(it, e, loaded))
}

func detailText(it domain.Item, e detailEntry, loaded bool) string {
	if it.Kind == domain.KindTask {
		var b strings.Builder
		b.WriteString(it.Title + "\n\n")
		if it.Tag != "" {
			b.WriteString("tag    : " + it.Tag + "\n")
		}
		state := "未完了"
		if it.Done {
			state = "完了"
		}
		b.WriteString("state  : " + state + "\n")
		return b.String()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s\n\n", it.Number, it.Title)
	fmt.Fprintf(&b, "repo   : %s\n", it.Repo)
	fmt.Fprintf(&b, "role   : %s\n", it.Role)

	if !loaded {
		b.WriteString("\n（詳細を取得中…）\n")
		return b.String()
	}
	if e.err != "" {
		fmt.Fprintf(&b, "\n（詳細を取得できませんでした: %s）\n", e.err)
		return b.String()
	}
	d := e.detail
	if d.CI != "" {
		fmt.Fprintf(&b, "CI     : %s %s\n", ciMark(d.CI), d.CI)
	}
	if d.Reviews != "" {
		fmt.Fprintf(&b, "review : %s\n", d.Reviews)
	}
	fmt.Fprintf(&b, "+%d -%d (%d files)\n", d.Additions, d.Deletions, d.ChangedFiles)
	b.WriteString("\n" + it.URL + "\n")
	return b.String()
}

func ciMark(state string) string {
	switch state {
	case "passing":
		return "✓"
	case "failing":
		return "✗"
	case "pending":
		return "…"
	}
	return " "
}
