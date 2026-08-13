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
	body  []string // line の直後に続く詳細行（原文のまま）
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
			body:  bodyAt(lines, i),
		})
	}
	return t
}

// bodyAt はチェックボックス行 at に続く詳細行を集める。インデントされた行と
// 空行が続く限りが詳細で、非インデントの非空行（"## メモ" など）か次の
// チェックボックス行で終わる。末尾の空行は含めない。
// インデントされたチェックボックス行は詳細ではなく独立したタスクなので、
// そこで打ち切る。
func bodyAt(lines []string, at int) []string {
	end := at + 1
	for ; end < len(lines); end++ {
		l := strings.TrimRight(lines[end], " \t\r")
		if l == "" {
			continue
		}
		if checkboxRe.MatchString(lines[end]) || !strings.ContainsAny(l[:1], " \t") {
			break
		}
	}
	for end > at+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[at+1 : end]
}

// dedent は非空行に共通する最小のインデントだけを剥がす。枠の中で二重に
// インデントされて見えるのを防ぎつつ、メモの中の入れ子は保つ。
func dedent(lines []string) string {
	n := -1
	for _, l := range lines {
		t := strings.TrimLeft(l, " \t")
		if t == "" {
			continue
		}
		if w := len(l) - len(t); n < 0 || w < n {
			n = w
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		s := strings.TrimRight(l, " \t\r")
		if n > 0 && len(s) >= n {
			s = s[n:]
		}
		out[i] = s
	}
	return strings.Join(out, "\n")
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
			Body:  dedent(tk.body),
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

// Add は最後のタスク（詳細行があればその末尾）の直後に挿入する。末尾に
// 追記すると "## メモ" のような後続セクションの下に紛れ込んでしまい、
// チェックボックス行の直後に入れると詳細の途中に割り込んでしまうため。
func (t TaskList) Add(title string) TaskList {
	at := -1
	for _, tk := range t.tasks {
		at = max(at, tk.line+len(tk.body))
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
