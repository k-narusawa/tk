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

// Config は main.go が環境変数から決める設定。文字列が並ぶので、
// 位置引数だと取り違えてもコンパイルが通ってしまう。
type Config struct {
	AICmd     string // $TK_AI_CMD
	EditorCmd string // $TK_EDITOR / $VISUAL / $EDITOR
}

type Model struct {
	inbox *usecase.Inbox
	cfg   Config

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

	// refreshing は Refresh が in-flight かどうか。r 連打で複数の Refresh が
	// 並行して走ると、後に投げた方が先に届くとは限らず、新しい結果を
	// 古い結果で上書きしうる（到着順の非保証）。in-flight 中は r を無視する。
	refreshing bool

	width, height int
}

func New(inbox *usecase.Inbox, cfg Config) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "新しいタスク"
	ti.SetWidth(40)

	return Model{
		inbox:      inbox,
		cfg:        cfg,
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

// toggleFocus はカーソルを退避してもう一方のペインへフォーカスを移す。
// ペインは2つなので、方向は要らない。
func (m *Model) toggleFocus() {
	m.focus, m.cursor, m.otherCursor = (m.focus+1)%paneCount, m.otherCursor, m.cursor
	m.reload()
	m.syncDetail()
}

// itemCount は指定ペインの件数。フォーカスしていない側は件数しか使わない
// （中身は描かない）。
func (m Model) itemCount(p paneID) int {
	if p == m.focus {
		return len(m.items)
	}
	return len(m.otherItems)
}
