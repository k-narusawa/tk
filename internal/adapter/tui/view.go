package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/k-narusawa/tk/internal/domain"
)

const help = " h/l:ペイン j/k:移動 space:完了 n:追加 enter:開く d:diff a/A:AI r:更新 R:再読込 q:終了"

var paneNames = [paneCount]string{"タスク", "GitHub"}

// layout は端末サイズから各枠の寸法（枠線込み）を決める。View と
// WindowSizeMsg の両方が同じ値を要るので、計算はここ1箇所に置く。
type layout struct {
	left, right        int // 左カラム / 右ペインの幅
	body               int // 右ペイン、および左カラム全体の高さ
	focused, collapsed int // 左カラム2枠の高さ（合計 = body）
}

func newLayout(width, height int) layout {
	var l layout
	l.left = max(1, width*40/100)
	l.right = max(1, width-l.left)
	// 潰れた枠はタイトル（枠の上辺）と空の1行だけ。box.Height は最小値なので、
	// 空文字を描いても1行は残る。フォーカス中が残りの高さを全部使う。
	l.collapsed = 3
	// フッタ1行を除いた残り全部を本文が使う。下限は両方の枠が枠線として
	// 成立する高さ。これを割る端末では溢れるので、View が本文を削る。
	l.body = max(2*l.collapsed, height-1)
	l.focused = l.body - l.collapsed
	return l
}

func (m Model) View() tea.View {
	l := newLayout(m.width, m.height)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, m.paneView(l, paneTasks), m.paneView(l, paneGitHub)),
		m.detailView(l),
	)

	footer := help
	if m.errMsg != "" {
		footer = " " + m.errMsg
	}
	if m.adding {
		footer = m.input.View()
	}
	// 端末幅に収める。ヘルプは固定 70 桁前後あり、gh の stderr はさらに
	// 長くなる。溢れると折り返してレイアウトが崩れる。
	if m.width > 0 {
		footer = lipgloss.NewStyle().MaxWidth(m.width).Render(footer)
	}
	// 枠が2つ縦に並ぶ下限（7行）を割る端末では、本文を削ってでもフッタを
	// 残す。フッタが押し出されると保存失敗や gh のエラーが見えなくなる。
	lines := append(strings.Split(body, "\n"), footer)
	if m.height > 0 && len(lines) > m.height {
		lines = append(lines[:m.height-1], footer)
	}

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	if m.adding {
		v.Cursor = m.input.Cursor()
	}
	return v
}

// focusColor はフォーカス中の枠の色。ANSI の 2 番を指すので、実際の緑は
// 端末のテーマに従う。
var (
	focusColor = lipgloss.Color("2")
	paneBorder = lipgloss.RoundedBorder()
)

// paneView は左カラムの枠を1つ描く。フォーカスしていない側は中身を出さず、
// 件数だけをタイトルに載せる（中が見えないので、件数が無いと PR が来て
// いるかどうか分からなくなる）。
func (m Model) paneView(l layout, p paneID) string {
	box := lipgloss.NewStyle().Border(paneBorder).Width(l.left)
	border := lipgloss.NewStyle()
	body, height := "", l.collapsed
	if p == m.focus {
		box = box.BorderForeground(focusColor)
		border = border.Foreground(focusColor)
		body, height = m.listView(l.left-2, l.focused-2), l.focused
	}
	// 幅が足りないときは件数を落としてでも名前を残す。どちらのペインかが
	// 分からなくなるほうが困る。
	return withTitle(box.Height(height).Render(body), border, m.paneTitle(p), paneNames[p])
}

// detailView は右の詳細ペイン。高さは左カラム2枠の合計と一致する。
func (m Model) detailView(l layout) string {
	return lipgloss.NewStyle().Border(paneBorder).Width(l.right).Height(l.body).Render(m.detail.View())
}

// paneTitle は枠に載せる名前と件数。取得前の GitHub を "(0)" と出すと
// 「PR なし」と見分けが付かないので、そこだけ伏せる。
func (m Model) paneTitle(p paneID) string {
	if p == paneGitHub && !m.prLoaded {
		return paneNames[p] + " (…)"
	}
	return fmt.Sprintf("%s (%d)", paneNames[p], m.itemCount(p))
}

// withTitle は枠の1行目をタイトル付きの上辺（╭─タスク (4)───╮）に差し替える。
// lipgloss v2 に枠タイトルの API が無いのでここだけ自前で組む。
// titles は長いものから順に渡す。収まらなければ次を試し、どれも入らなければ
// 無題にする。狭い端末で枠が壊れるより無題のほうがまし。
//
// 幅は描画済みの箱から測る。Style.Width は下限であって、狭いところでは
// 指定より広い箱が返ってくるため。
func withTitle(box string, style lipgloss.Style, titles ...string) string {
	top, rest, found := strings.Cut(box, "\n")
	if !found {
		return box
	}
	w := lipgloss.Width(top)
	if w < 3 {
		return box // 枠の体をなしていない。触らない
	}

	title, fill := "", w-3
	for _, c := range titles {
		if n := w - 3 - lipgloss.Width(c); n >= 0 {
			title, fill = c, n
			break
		}
	}
	line := paneBorder.TopLeft + paneBorder.Top + title + strings.Repeat(paneBorder.Top, fill) + paneBorder.TopRight
	return style.Render(line) + "\n" + rest
}

// listView は一覧のうち高さ rows に収まる範囲だけを描画する。box.Height は
// 最小値であってクランプではないので、全件を無条件に出すと箱が端末を超えて
// 伸び、フッタが画面外に押し出される（保存失敗などのエラーが見えなくなる）。
// カーソルが窓の下端を超えたら追従してスクロールする。
// 幅 w を超える行は切り詰める。折り返させると1件が2行になり、同じ理由で
// 枠が伸びる（PR のタイトルは平気で枠幅を超える）。
func (m Model) listView(w, rows int) string {
	clamp := lipgloss.NewStyle().MaxWidth(max(1, w))
	if len(m.items) == 0 {
		return clamp.Render(m.emptyLabel())
	}
	rows = max(1, rows)
	start := 0
	if m.cursor >= rows {
		start = m.cursor - rows + 1
	}
	end := min(len(m.items), start+rows)

	// 末尾に改行を付けない。付けると rows 行ぶんの一覧が rows+1 行になり、
	// 枠が1行ぶん膨らんで下のペインを押し出す。
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		lines = append(lines, clamp.Render(cursor+itemLabel(m.items[i])))
	}
	return strings.Join(lines, "\n")
}

// emptyLabel は GitHub ペインでだけ状態を出す。起動直後は gh の取得が
// 終わるまで必ず空になるので、無表示だと故障と区別できない。
// タスクの空はユーザーが知っている状態なので何も出さない。
func (m Model) emptyLabel() string {
	if m.focus != paneGitHub {
		return ""
	}
	if !m.prLoaded {
		return "（PR を取得中…）"
	}
	return "（PR なし）"
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
