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
	paneRoutine
	paneCount
)

// routineState は routine 1件の実行状態。マップに無い = このセッションで
// まだ走らせていない。
type routineState int

// iota + 1 から始めるのは、ゼロ値（マップに無い＝未実行）を状態と
// 取り違えないため。
const (
	routineRunning routineState = iota + 1
	routineOK
	routineNG
)

// Config は main.go が環境変数から決める設定。文字列が並ぶので、
// 位置引数だと取り違えてもコンパイルが通ってしまう。
type Config struct {
	AICmd      string // $TK_AI_CMD
	RoutineCmd string // $TK_ROUTINE_CMD
	EditorCmd  string // $TK_EDITOR / $VISUAL / $EDITOR
	// ReviewPromptPath は v が読むレビュー用プロンプト。tasks.md と同じ
	// ディレクトリの review.md を main.go が導出する。
	ReviewPromptPath string
}

type Model struct {
	inbox *usecase.Inbox
	cfg   Config

	items  []domain.Item
	cursor int

	focus paneID
	// panes は全ペインの一覧。フォーカスしていないペインも件数をタイトルに
	// 出すので、フォーカス中のぶん（= items）も含めて全部持つ。
	panes [paneCount][]domain.Item
	// cursors はフォーカスを離れたペインのカーソル位置。戻ってきたときに
	// 同じ行に居るようにする。フォーカス中のぶんは cursor が持つ。
	cursors [paneCount]int

	detail  viewport.Model
	details map[domain.ID]detailEntry
	input   textinput.Model
	adding  bool

	errMsg  string
	errIsPR bool // errMsg が PR 取得由来かどうか

	prLoaded bool // Refresh が1度でも完了したか（成否は問わない）

	// routines は起動中に走らせた routine の状態。プロセスの生死は tk が
	// 抱えているので、終了したら道連れになる（永続化しても意味がない）。
	routines map[domain.ID]routineState
	// quitWarned は実行中の routine があるときの q を1度だけ握り潰したか。
	// q 以外のキーで戻す（間を置いた2回目の q は、また警告から始まる）。
	quitWarned bool

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

	detail := viewport.New(viewport.WithWidth(40), viewport.WithHeight(20))
	// 詳細は任意サイズの Markdown ファイルになった。h/l はペイン切替に使うため
	// 横スクロールする手段が無く、折り返さないと幅を超えた分が二度と見えない。
	detail.SoftWrap = true

	m := Model{
		inbox:    inbox,
		cfg:      cfg,
		detail:   detail,
		details:  make(map[domain.ID]detailEntry),
		routines: make(map[domain.ID]routineState),
		input:    ti,
	}
	m.reload()
	return m
}

func (m Model) selected() (domain.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return domain.Item{}, false
	}
	return m.items[m.cursor], true
}

// reload は全ペインの一覧を取り直し、カーソルを範囲内に収める。
// 潰れているペインも件数をタイトルに出すので、全部いる。
func (m *Model) reload() {
	m.panes = [paneCount][]domain.Item{
		paneTasks:   m.inbox.Tasks(),
		paneGitHub:  m.inbox.PRs(),
		paneRoutine: m.inbox.Routines(),
	}
	m.items = m.panes[m.focus]
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}

// moveFocus はカーソルを退避して delta 個先のペインへフォーカスを移す。
// ペインが2つだった頃は h も l も「もう一方」で足りたが、3つになると
// 一方向にしか回らないのは戻るのに2回押すことになる。l で次、h で前。
func (m *Model) moveFocus(delta int) {
	m.cursors[m.focus] = m.cursor
	m.focus = (m.focus + paneID(delta) + paneCount) % paneCount
	m.cursor = m.cursors[m.focus]
	m.reload()
	m.syncDetail()
}

// itemCount は指定ペインの件数。フォーカスしていないペインは件数しか
// 使わない（中身は描かない）。
func (m Model) itemCount(p paneID) int { return len(m.panes[p]) }

// runningRoutines は実行中の routine の数。q を握り潰すかどうかの判断に使う。
func (m Model) runningRoutines() int {
	n := 0
	for _, s := range m.routines {
		if s == routineRunning {
			n++
		}
	}
	return n
}
