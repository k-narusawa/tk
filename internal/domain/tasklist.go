package domain

import (
	"regexp"
	"slices"
	"strings"
)

var checkboxRe = regexp.MustCompile(`^(\s*)- \[([ xX])\] (.*)$`)

type task struct {
	line  int // lines へのインデックス
	done  bool
	title string
	tag   string
}

// TaskList は tasks.md の全行と、そこから解釈したタスクを対で持つ。
type TaskList struct {
	lines []string
	tasks []task
}

func Parse(lines []string) TaskList {
	t := TaskList{lines: slices.Clone(lines)}
	for i, line := range lines {
		m := checkboxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		title, tag := splitTag(strings.TrimSpace(m[3]))
		t.tasks = append(t.tasks, task{
			line:  i,
			done:  m[2] == "x" || m[2] == "X",
			title: title,
			tag:   tag,
		})
	}
	return t
}

// splitTag は末尾の " @xxx" をタグとして切り出す。
func splitTag(s string) (title, tag string) {
	i := strings.LastIndex(s, " @")
	if i < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:i]), s[i+1:]
}

func (t TaskList) Items() []Item {
	items := make([]Item, 0, len(t.tasks))
	for _, tk := range t.tasks {
		items = append(items, Item{
			ID:    TaskID(tk.line),
			Kind:  KindTask,
			Title: tk.title,
			Done:  tk.done,
			Tag:   tk.tag,
		})
	}
	return items
}

func (t TaskList) Render() []string { return slices.Clone(t.lines) }

// Toggle は対象行のチェックボックス1文字だけを差し替えた新しい TaskList を返す。
// 元を書き換えないので、保存に失敗しても状態が壊れない。
func (t TaskList) Toggle(id ID) TaskList {
	for _, tk := range t.tasks {
		if TaskID(tk.line) != id {
			continue
		}
		mark := "x"
		if tk.done {
			mark = " "
		}
		lines := slices.Clone(t.lines)
		m := checkboxRe.FindStringSubmatchIndex(lines[tk.line])
		// m[4]:m[5] はチェックボックスの中身1文字。前後は原文のまま残す。
		lines[tk.line] = lines[tk.line][:m[4]] + mark + lines[tk.line][m[5]:]
		return Parse(lines)
	}
	return t
}

// Add は最後のチェックボックス行の直後に挿入する。末尾に追記すると
// "## メモ" のような後続セクションの下に紛れ込んでしまうため。
// ただし、旧形式（チェックボックス行に続くインデント行を詳細として書いて
// いたファイル）からアップグレードした直後のユーザーはその形のまま残って
// いる。直後に割り込むと続くインデント行が新タスクの下にぶら下がる形に
// 化けてしまうので、そのインデント行の直後まで飛ばす。
func (t TaskList) Add(title string) TaskList {
	at := -1
	if n := len(t.tasks); n > 0 {
		at = addAnchor(t.lines, t.tasks[n-1].line)
	}
	if at < 0 {
		// チェックボックスが無いなら最後の非空行の直後
		for i := len(t.lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(t.lines[i]) != "" {
				at = i
				break
			}
		}
	}

	newLine := "- [ ] " + title
	if at >= 0 && strings.HasSuffix(t.lines[at], "\r") {
		newLine += "\r"
	}

	lines := make([]string, 0, len(t.lines)+1)
	lines = append(lines, t.lines[:at+1]...)
	lines = append(lines, newLine)
	lines = append(lines, t.lines[at+1:]...)
	return Parse(lines)
}

// addAnchor は checkboxLine に続くインデント行を「そのタスクの続き」とみなし
// て飛ばした先の行番号を返す。非インデントの行（次のチェックボックスや見出し）
// か入力の終端で止まる。空行そのものは挿入位置に含めない。次のインデント行へ
// 続くかどうかを空行の時点では判断できず、"## メモ" の前の空行の連続に紛れ
// 込ませないため。
func addAnchor(lines []string, checkboxLine int) int {
	at := checkboxLine
	for i := checkboxLine + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		at = i
	}
	return at
}
