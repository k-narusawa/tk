package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"

	"github.com/k-narusawa/tk/domain"
	"github.com/k-narusawa/tk/usecase"
)

// detailEntry は取得済みの詳細と、取得に失敗した場合の理由を持つ。
// マップに無い = まだ取得していない。
type detailEntry struct {
	detail domain.PRDetail
	err    string
}

type Model struct {
	inbox *usecase.Inbox

	items  []domain.Item
	cursor int

	detail  viewport.Model
	details map[domain.ID]detailEntry
	input   textinput.Model
	adding  bool

	errMsg  string
	errIsPR bool // errMsg が PR 取得由来かどうか

	width, height int
}

func New(inbox *usecase.Inbox) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "新しいタスク"
	ti.SetWidth(40)

	return Model{
		inbox:   inbox,
		items:   inbox.Items(),
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

// reload は usecase から一覧を取り直し、カーソルを範囲内に収める。
func (m *Model) reload() {
	m.items = m.inbox.Items()
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}
