package ai

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/k-narusawa/tk/internal/domain"
)

// Render はアイテムを AI CLI に読ませる Markdown にする。
func Render(items []domain.Item) string {
	var b strings.Builder
	b.WriteString("# tk インボックス\n\n")
	for _, it := range items {
		if it.Kind == domain.KindPR {
			fmt.Fprintf(&b, "- PR #%d %s\n", it.Number, it.Title)
			fmt.Fprintf(&b, "  - repo: %s\n", it.Repo)
			fmt.Fprintf(&b, "  - role: %s\n", it.Role)
			fmt.Fprintf(&b, "  - url: %s\n", it.URL)
			continue
		}
		mark := " "
		if it.Done {
			mark = "x"
		}
		fmt.Fprintf(&b, "- [%s] %s", mark, it.Title)
		if it.Tag != "" {
			fmt.Fprintf(&b, " %s", it.Tag)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Command は内容を一時ファイルに書き、$TK_AI_CMD <path> の *exec.Cmd を返す。
// tea.ExecProcess で包むのは adapter/tui の仕事。
func Command(aiCmd string, items []domain.Item) (*exec.Cmd, error) {
	fields := strings.Fields(aiCmd)
	if len(fields) == 0 {
		return nil, errors.New("TK_AI_CMD が空")
	}

	f, err := os.CreateTemp("", "tk-*.md")
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(Render(items)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	return exec.Command(fields[0], append(fields[1:], f.Name())...), nil
}
