package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"

	"github.com/k-narusawa/tk/internal/domain"
	"github.com/k-narusawa/tk/internal/usecase"
)

// detailEntry は取得済みの詳細と、取得に失敗した場合の理由を持つ。
// マップに無い = まだ取得していない。
type detailEntry struct {
	detail domain.PRDetail
	err    string
}

type tabID int

const (
	tabTask tabID = iota
	tabGitHub
)

type Model struct {
	inbox *usecase.Inbox
	aiCmd string

	items  []domain.Item
	cursor int

	tab         tabID
	otherCursor int // 表示していない側のタブのカーソル位置

	detail  viewport.Model
	details map[domain.ID]detailEntry
	input   textinput.Model
	adding  bool

	errMsg  string
	errIsPR bool // errMsg が PR 取得由来かどうか

	prLoaded bool // Refresh が1度でも完了したか（成否は問わない）

	width, height int
}

func New(inbox *usecase.Inbox, aiCmd string) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "新しいタスク"
	ti.SetWidth(40)

	return Model{
		inbox:   inbox,
		aiCmd:   aiCmd,
		items:   inbox.Tasks(),
		detail:  viewport.New(viewport.WithWidth(40), viewport.WithHeight(20)),
		details: make(map[domain.ID]detailEntry),
		input:   ti,
	}
}

func (m Model) selected() (domain.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return domain.Item{}, false
	}
	return m.items[m.cursor], true
}

// reload は現在のタブの一覧を取り直し、カーソルを範囲内に収める。
func (m *Model) reload() {
	if m.tab == tabGitHub {
		m.items = m.inbox.PRs()
	} else {
		m.items = m.inbox.Tasks()
	}
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}

// switchTab はカーソルを退避してタブを入れ替える。
func (m *Model) switchTab(t tabID) {
	if m.tab == t {
		return
	}
	m.tab, m.cursor, m.otherCursor = t, m.otherCursor, m.cursor
	m.reload()
	m.syncDetail()
}
