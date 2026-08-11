package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/domain"
)

type prLoadedMsg struct{ err error }

func (m Model) Init() tea.Cmd { return m.refreshCmd() }

// refreshCmd は gh 呼び出しを tea.Cmd に包む。usecase は同期のままで、
// 非同期にするかどうかは TUI 側が決める。
func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		return prLoadedMsg{err: m.inbox.Refresh(context.Background())}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		left := m.width * 40 / 100
		m.detail.SetWidth(max(1, m.width-left-4))
		m.detail.SetHeight(max(1, m.height-4))
		m.input.SetWidth(max(1, m.width-4))
		return m, nil

	case prLoadedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else {
			m.errMsg = ""
		}
		m.reload()
		return m, nil

	case tea.KeyPressMsg:
		if m.adding {
			return m.updateAdding(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m Model) updateAdding(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		m.adding = false
		m.input.Reset()
		m.input.Blur()
		if title == "" {
			return m, nil
		}
		if err := m.inbox.Add(title); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.reload()
		return m, nil

	case "esc":
		m.adding = false
		m.input.Reset()
		m.input.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "space":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindTask {
			return m, nil
		}
		if err := m.inbox.Toggle(it.ID); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.reload()
		return m, nil

	case "n":
		m.adding = true
		return m, m.input.Focus()

	case "ctrl+d":
		m.detail.HalfPageDown()
		return m, nil

	case "ctrl+u":
		m.detail.HalfPageUp()
		return m, nil

	case "r":
		return m, m.refreshCmd()
	}
	return m, nil
}
