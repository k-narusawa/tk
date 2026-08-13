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

type paneID int

const (
	paneTasks paneID = iota
	paneGitHub
	paneCount
)

type Model struct {
	inbox *usecase.Inbox
	aiCmd string

	items  []domain.Item
	cursor int

	focus       paneID
	otherItems  []domain.Item // フォーカスしていない側のペインの一覧
	otherCursor int           // フォーカスしていない側のペインのカーソル位置

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
		inbox:      inbox,
		aiCmd:      aiCmd,
		items:      inbox.Tasks(),
		otherItems: inbox.PRs(),
		detail:     viewport.New(viewport.WithWidth(40), viewport.WithHeight(20)),
		details:    make(map[domain.ID]detailEntry),
		input:      ti,
	}
}

func (m Model) selected() (domain.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return domain.Item{}, false
	}
	return m.items[m.cursor], true
}

// reload は両ペインの一覧を取り直し、カーソルを範囲内に収める。
// 潰れている側も件数をタイトルに出すので、両方いる。
func (m *Model) reload() {
	tasks, prs := m.inbox.Tasks(), m.inbox.PRs()
	if m.focus == paneGitHub {
		m.items, m.otherItems = prs, tasks
	} else {
		m.items, m.otherItems = tasks, prs
	}
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}

// focusPane はカーソルを退避してフォーカスを移す。
func (m *Model) focusPane(p paneID) {
	if m.focus == p {
		return
	}
	m.focus, m.cursor, m.otherCursor = p, m.otherCursor, m.cursor
	m.reload()
	m.syncDetail()
}

// paneItems は指定ペインの一覧を返す。潰れた枠の件数表示に使う。
func (m Model) paneItems(p paneID) []domain.Item {
	if p == m.focus {
		return m.items
	}
	return m.otherItems
}
