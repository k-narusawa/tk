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
		if it.Kind == domain.KindRoutine {
			fmt.Fprintf(&b, "- routine: %s\n", it.Title)
			continue
		}
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

// renderReview はレビュー用プロンプトに対象 PR を埋める。
// プレースホルダを置換したうえで末尾にも対象を足すのは、置換を知らずに
// 書いたプロンプトでも対象が伝わるようにするため。
func renderReview(prompt string, it domain.Item) string {
	r := strings.NewReplacer(
		"{{repo}}", it.Repo,
		"{{number}}", fmt.Sprint(it.Number),
		"{{url}}", it.URL,
	)
	var b strings.Builder
	b.WriteString(strings.TrimRight(r.Replace(prompt), "\n"))
	fmt.Fprintf(&b, "\n\n## 対象 PR\n- repo: %s\n- number: %d\n- url: %s\n", it.Repo, it.Number, it.URL)
	return b.String()
}

// Command は内容を一時ファイルに書き、$TK_AI_CMD <path> の *exec.Cmd を返す。
// tea.ExecProcess で包むのは adapter/tui の仕事。
func Command(aiCmd string, items []domain.Item) (*exec.Cmd, error) {
	return command(aiCmd, Render(items))
}

// ReviewCommand は promptPath のプロンプトと PR の情報を渡す *exec.Cmd を返す。
// プロンプトは毎回読み直す。書き換えたら tk を再起動せずに次の実行から効く。
func ReviewCommand(aiCmd, promptPath string, it domain.Item) (*exec.Cmd, error) {
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("レビュー用プロンプトを読めない（%s に置く）: %w", promptPath, err)
	}
	return command(aiCmd, renderReview(string(prompt), it))
}

// RoutineCommand は routine の指示を非対話で回す *exec.Cmd を返す。
// Command と違って一時ファイルを作らず標準入力に流し込むのは、非対話 CLI
// （claude -p 等）がプロンプトを stdin から受け取るのが素直で、引数長の
// 上限とクォート事故も避けられるため。
//
// sh -c を通すのは、ツール許可の指定に空白が入る（--allowedTools "Bash(gh api:*)"）
// ため。空白で分割するだけでは書けない。
//
// tea.ExecProcess では包まない。TUI を明け渡さずに裏で走らせるので、
// 呼び出し側（adapter/tui）が tea.Cmd の中で Output() を待つ。
func RoutineCommand(routineCmd, prompt string) (*exec.Cmd, error) {
	if strings.TrimSpace(routineCmd) == "" {
		return nil, errors.New("TK_ROUTINE_CMD が空")
	}
	// 指示が無いまま走らせても AI は何も調べられない。実行してから
	// 空の結果を追記するより、押した時点で理由を出すほうが早い。
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("指示が空。e で指示ファイルを書いてください")
	}
	c := exec.Command("sh", "-c", routineCmd)
	c.Stdin = strings.NewReader(prompt)
	return c, nil
}

func command(aiCmd, body string) (*exec.Cmd, error) {
	fields := strings.Fields(aiCmd)
	if len(fields) == 0 {
		return nil, errors.New("TK_AI_CMD が空")
	}

	f, err := os.CreateTemp("", "tk-*.md")
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(body); err != nil {
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
