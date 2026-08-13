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
	// 下限は両方の枠が潰れずに描ける高さ。これを割る端末では溢れるが、
	// 潰れた枠より小さくすると枠線そのものが崩れる。
	l.body = max(2*l.collapsed, height-4)
	l.focused = l.body - l.collapsed
	return l
}

func (m Model) View() tea.View {
	l := newLayout(m.width, m.height)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, m.paneView(l, paneTasks), m.paneView(l, paneGitHub)),
		box.Width(l.right).Height(l.body).Render(m.detail.View()),
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

	v := tea.NewView(body + "\n" + footer)
	v.AltScreen = true
	if m.adding {
		v.Cursor = m.input.Cursor()
	}
	return v
}

// focusColor はフォーカス中の枠の色。ANSI の 2 番を指すので、実際の緑は
// 端末のテーマに従う。
var focusColor = lipgloss.Color("2")

// paneView は左カラムの枠を1つ描く。フォーカスしていない側は中身を出さず、
// 件数だけをタイトルに載せる（中が見えないので、件数が無いと PR が来て
// いるかどうか分からなくなる）。
func (m Model) paneView(l layout, p paneID) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(l.left)
	border := lipgloss.NewStyle()
	body, height := "", l.collapsed
	if p == m.focus {
		box = box.BorderForeground(focusColor)
		border = border.Foreground(focusColor)
		body, height = m.listView(l.left-2, l.focused-2), l.focused
	}

	title := fmt.Sprintf("%s (%d)", paneNames[p], len(m.paneItems(p)))
	return withTitle(box.Height(height).Render(body), border.Render(topBorder(l.left, title)))
}

// withTitle は枠の1行目をタイトル付きの上辺に差し替える。lipgloss v2 に
// 枠タイトルの API が無いので上辺だけ自前で作る。色を付けると1行目が
// エスケープ列を含むため、描画済みの文字列は切り貼りしない。
func withTitle(box, top string) string {
	_, rest, found := strings.Cut(box, "\n")
	if !found {
		return top
	}
	return top + "\n" + rest
}

// topBorder は ╭─タスク (4)────╮ を組み立てる。width は枠線を含む外寸。
func topBorder(width int, title string) string {
	// ╭ ─ title … ╮ が収まらない幅ならタイトルを諦める。狭い端末で枠が
	// 壊れるより無題のほうがまし。
	if fill := width - 3 - lipgloss.Width(title); fill >= 0 {
		return "╭─" + title + strings.Repeat("─", fill) + "╮"
	}
	return "╭" + strings.Repeat("─", max(0, width-2)) + "╮"
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
