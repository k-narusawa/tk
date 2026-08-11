package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/adapter/ai"
	"github.com/k-narusawa/tk/adapter/gh"
	"github.com/k-narusawa/tk/domain"
)

type prLoadedMsg struct{ err error }

type detailLoadedMsg struct {
	id     domain.ID
	detail domain.PRDetail
	err    error
}

type execDoneMsg struct{ err error }

// aiExec は選択アイテムを AI CLI に渡す tea.Cmd を作る。tea.ExecProcess で
// ターミナルを明け渡す必要があるので、ここで包む（ai パッケージは包まない）。
func (m Model) aiExec(items []domain.Item) tea.Cmd {
	c, err := ai.Command(m.aiCmd, items)
	if err != nil {
		return func() tea.Msg { return execDoneMsg{err: err} }
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.refreshCmd(), m.detailCmd()) }

// refreshCmd は gh 呼び出しを tea.Cmd に包む。usecase は同期のままで、
// 非同期にするかどうかは TUI 側が決める。
func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		return prLoadedMsg{err: m.inbox.Refresh(context.Background())}
	}
}

// detailCmd は選択中の PR の詳細を取る。取得済みなら何もしない。
// 起動時に全 PR ぶん叩かないのが狙い。
func (m Model) detailCmd() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.Kind != domain.KindPR {
		return nil
	}
	if _, done := m.details[it.ID]; done {
		return nil
	}
	id := it.ID
	return func() tea.Msg {
		d, err := m.inbox.Detail(context.Background(), id)
		return detailLoadedMsg{id: id, detail: d, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		left := m.width * 40 / 100
		right := m.width - left
		inner := max(1, m.height-4)
		// 枠線の内側に収める。box.Width/Height は枠を含む寸法なので、
		// viewport には -2 した値を渡さないと溢れて箱が膨らむ。
		m.detail.SetWidth(max(1, right-2))
		m.detail.SetHeight(max(1, inner-2))
		m.input.SetWidth(max(1, m.width-4))
		m.syncDetail()
		return m, nil

	case prLoadedMsg:
		if msg.err != nil {
			m.errMsg, m.errIsPR = msg.err.Error(), true
		} else if m.errIsPR {
			// PR 取得のエラーだけを消す。保存失敗などは残す。
			m.errMsg, m.errIsPR = "", false
		}
		m.reload()
		m.syncDetail()
		return m, m.detailCmd()

	case detailLoadedMsg:
		if msg.err != nil {
			m.errMsg, m.errIsPR = msg.err.Error(), true
			m.details[msg.id] = detailEntry{err: msg.err.Error()}
		} else {
			m.details[msg.id] = detailEntry{detail: msg.detail}
		}
		m.syncDetail()
		return m, nil

	case execDoneMsg:
		// 成功しても errMsg は消さない。保存失敗など無関係なエラーを
		// 握り潰さないため（prLoadedMsg の errIsPR ガードと同じ理由）。
		if msg.err != nil {
			m.errMsg, m.errIsPR = msg.err.Error(), false
		}
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
	case "ctrl+c":
		return m, tea.Quit

	case "enter":
		title := strings.TrimSpace(m.input.Value())
		m.adding = false
		m.input.Reset()
		m.input.Blur()
		if title == "" {
			return m, nil
		}
		if err := m.inbox.Add(title); err != nil {
			m.errMsg, m.errIsPR = err.Error(), false
			return m, nil
		}
		m.errMsg, m.errIsPR = "", false
		m.reload()
		m.syncDetail()
		return m, m.detailCmd()

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
		m.syncDetail()
		return m, m.detailCmd()

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m.syncDetail()
		return m, m.detailCmd()

	case "space":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindTask {
			return m, nil
		}
		if err := m.inbox.Toggle(it.ID); err != nil {
			m.errMsg, m.errIsPR = err.Error(), false
			return m, nil
		}
		m.errMsg, m.errIsPR = "", false
		m.reload()
		m.syncDetail()
		return m, m.detailCmd()

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

	case "R":
		if err := m.inbox.Load(); err != nil {
			m.errMsg, m.errIsPR = err.Error(), false
			return m, nil
		}
		m.errMsg, m.errIsPR = "", false
		m.reload()
		m.syncDetail()
		return m, m.detailCmd()

	case "enter":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindPR {
			return m, nil
		}
		repo, number := it.Repo, it.Number
		// ブラウザを開くだけなので TUI を畳まない
		return m, func() tea.Msg { return execDoneMsg{err: gh.RunWeb(repo, number)} }

	case "d":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindPR {
			return m, nil
		}
		return m, tea.ExecProcess(gh.DiffCommand(it.Repo, it.Number), func(err error) tea.Msg {
			return execDoneMsg{err: err}
		})

	case "a":
		it, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, m.aiExec([]domain.Item{it})

	case "A":
		return m, m.aiExec(m.items)
	}
	return m, nil
}
