package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/internal/adapter/ai"
	"github.com/k-narusawa/tk/internal/adapter/editor"
	"github.com/k-narusawa/tk/internal/adapter/gh"
	"github.com/k-narusawa/tk/internal/domain"
)

type prLoadedMsg struct{ err error }

type detailLoadedMsg struct {
	id     domain.ID
	detail domain.PRDetail
	err    error
}

type execDoneMsg struct{ err error }

// routineDoneMsg は裏で走らせた routine が終わったことを伝える。
type routineDoneMsg struct {
	id  domain.ID
	err error
}

// editDoneMsg はエディタが閉じたことを伝える。execDoneMsg と分けているのは、
// エディタの後だけ tasks.md を読み直す必要があるため。
type editDoneMsg struct{ err error }

// aiExec は選択アイテムを AI CLI に渡す tea.Cmd を作る。tea.ExecProcess で
// ターミナルを明け渡す必要があるので、ここで包む（ai パッケージは包まない）。
func (m Model) aiExec(items []domain.Item) tea.Cmd {
	c, err := ai.Command(m.cfg.AICmd, items)
	if err != nil {
		return func() tea.Msg { return execDoneMsg{err: err} }
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// reviewExec は PR を review.md のプロンプト付きで AI CLI に渡す tea.Cmd を作る。
func (m Model) reviewExec(it domain.Item) tea.Cmd {
	c, err := ai.ReviewCommand(m.cfg.AICmd, m.cfg.ReviewPromptPath, it)
	if err != nil {
		return func() tea.Msg { return execDoneMsg{err: err} }
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// routineExec は選択中 routine を裏で回す tea.Cmd を作る。tea.ExecProcess で
// 包まないので TUI は畳まれず、走っている間も操作を続けられる。
//
// tk が終われば子プロセスも道連れになる。終了前に確認を挟むのは updateList の
// 役目（ここでは扱わない）。
func (m Model) routineExec(it domain.Item) tea.Cmd {
	id, routineCmd := it.ID, m.cfg.RoutineCmd
	inbox := m.inbox
	return func() tea.Msg {
		prompt, err := inbox.RoutinePrompt(id)
		if err != nil {
			return routineDoneMsg{id: id, err: err}
		}
		c, err := ai.RoutineCommand(routineCmd, prompt)
		if err != nil {
			return routineDoneMsg{id: id, err: err}
		}
		// Output は stderr を ExitError に詰めてくれる。非対話 CLI の
		// 失敗理由（未ログイン等）は stderr にしか出ないので、拾わないと
		// 「✗ だが理由が分からない」で終わる。
		out, err := c.Output()
		if err != nil {
			return routineDoneMsg{id: id, err: routineError(err)}
		}
		return routineDoneMsg{id: id, err: inbox.SaveRoutineResult(id, string(out))}
	}
}

// routineError は exec の失敗に stderr の1行目を足す。
func routineError(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		line, _, _ := strings.Cut(strings.TrimSpace(string(ee.Stderr)), "\n")
		if line != "" {
			return fmt.Errorf("%w: %s", err, line)
		}
	}
	return err
}

// editorCommand は選択中タスクの詳細ファイルを開く *exec.Cmd を組み立てる。
// tea.ExecProcess で包む前に切り出してあるのは、どのパスを開こうとしているかを
// TUI を起動せずにテストするため（adapter/editor と同じ理由）。
func (m Model) editorCommand() (*exec.Cmd, error) {
	it, ok := m.selected()
	if !ok {
		return nil, nil
	}
	var path string
	var err error
	switch it.Kind {
	case domain.KindTask:
		path, err = m.inbox.DetailPath(it.ID)
	case domain.KindRoutine:
		// routine で開くのは指示ファイル。結果ファイルは tk が書くもので、
		// 人が編集するものではない。
		path, err = m.inbox.RoutinePath(it.ID)
	default:
		return nil, nil // PR には開くファイルが無い
	}
	if err != nil {
		return nil, err
	}
	return editor.Command(m.cfg.EditorCmd, path)
}

// editExec は選択中タスクの詳細ファイルをエディタで開く tea.Cmd を作る。
// tk は詳細を書き込まない。編集はエディタに任せ、閉じたら tasks.md を
// 読み直す（タイトルや完了状態が変わっているかもしれないため）。
func (m Model) editExec() tea.Cmd {
	c, err := m.editorCommand()
	if err != nil {
		return func() tea.Msg { return editDoneMsg{err: err} }
	}
	if c == nil {
		return nil // タスク以外を選んでいる。編集する対象が無い
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{err: err}
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
		l := newLayout(m.width, m.height)
		// 枠線の内側に収める。box.Width/Height は枠を含む寸法なので、
		// viewport には -2 した値を渡さないと溢れて箱が膨らむ。
		m.detail.SetWidth(max(1, l.right-2))
		m.detail.SetHeight(max(1, l.body-2))
		m.input.SetWidth(max(1, m.width-4))
		m.syncDetail()
		return m, nil

	case prLoadedMsg:
		m.prLoaded = true
		m.refreshing = false
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

	case routineDoneMsg:
		if msg.err != nil {
			m.routines[msg.id] = routineNG
			m.errMsg, m.errIsPR = msg.err.Error(), false
		} else {
			m.routines[msg.id] = routineOK
		}
		// 結果ファイルが増えたので、その routine を選んだままなら描き直す。
		m.syncDetail()
		return m, nil

	case execDoneMsg:
		// 成功しても errMsg は消さない。保存失敗など無関係なエラーを
		// 握り潰さないため（prLoadedMsg の errIsPR ガードと同じ理由）。
		if msg.err != nil {
			m.errMsg, m.errIsPR = msg.err.Error(), false
		}
		return m, nil

	case editDoneMsg:
		// エディタが異常終了しても保存はされているかもしれないので、
		// まず読み直してから失敗を伝える。
		next, cmd := m.reloadTasks()
		if msg.err != nil {
			mm := next.(Model)
			mm.errMsg, mm.errIsPR = msg.err.Error(), false
			return mm, cmd
		}
		return next, cmd

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
	// q 以外を押したら警告を出し直す状態に戻す。連打でなければ、間に何か
	// 挟んだ2回目の q はもう一度止まる。
	if msg.String() != "q" {
		m.quitWarned = false
	}

	switch msg.String() {
	// ctrl+c は握り潰さない。逃げ道は常に残す。
	case "ctrl+c":
		return m, tea.Quit

	case "q":
		// 実行中の routine は tk と一緒に死ぬ。一度だけ止めて知らせる。
		if n := m.runningRoutines(); n > 0 && !m.quitWarned {
			m.quitWarned = true
			m.errMsg, m.errIsPR = fmt.Sprintf("routine 実行中 %d 件。もう一度 q で終了", n), false
			return m, nil
		}
		return m, tea.Quit

	case "l":
		m.moveFocus(1)
		return m, m.detailCmd()

	case "h":
		m.moveFocus(-1)
		return m, m.detailCmd()

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
		if m.focus != paneTasks {
			return m, nil
		}
		m.adding = true
		return m, m.input.Focus()

	case "ctrl+d":
		m.detail.HalfPageDown()
		return m, nil

	case "ctrl+u":
		m.detail.HalfPageUp()
		return m, nil

	case "r":
		if m.focus == paneTasks {
			return m.reloadTasks()
		}
		if m.refreshing {
			// in-flight 中の連打は無視する。ここで新たに Refresh を
			// 走らせると、複数の Refresh の到着順が保証されず、新しい
			// 結果を古い結果で上書きしうる。
			return m, nil
		}
		m.refreshing = true
		return m, m.refreshCmd()

	case "R":
		return m.reloadTasks()

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

	case "v":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindPR {
			return m, nil
		}
		return m, m.reviewExec(it)

	case "x":
		it, ok := m.selected()
		if !ok || it.Kind != domain.KindRoutine {
			return m, nil
		}
		// 同じ routine の二重起動は防ぐ。結果ファイルへの追記が混ざるうえ、
		// 同じことを2回 AI に聞くだけで得るものが無い。
		if m.routines[it.ID] == routineRunning {
			return m, nil
		}
		m.routines[it.ID] = routineRunning
		// PR 取得のエラーはここでは消さない。routine を走らせても PR の
		// 状況は変わらないので、消すと直ったように見えてしまう。
		if !m.errIsPR {
			m.errMsg = ""
		}
		return m, m.routineExec(it)

	case "e":
		return m, m.editExec()

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

// reloadTasks は tasks.md を読み直す。外部エディタでの変更を取り込むためのもの。
func (m Model) reloadTasks() (tea.Model, tea.Cmd) {
	if err := m.inbox.Load(); err != nil {
		m.errMsg, m.errIsPR = err.Error(), false
		return m, nil
	}
	// PR 取得のエラーはここでは消さない。tasks.md を読み直しても
	// PR の状況は変わらないので、消すと直ったように見えてしまう。
	if !m.errIsPR {
		m.errMsg = ""
	}
	m.reload()
	m.syncDetail()
	return m, m.detailCmd()
}
