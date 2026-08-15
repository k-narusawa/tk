package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/internal/adapter/gh"
	"github.com/k-narusawa/tk/internal/adapter/markdown"
	"github.com/k-narusawa/tk/internal/adapter/tui"
	"github.com/k-narusawa/tk/internal/usecase"
)

// tasksPath は一覧ファイルのパス。詳細ファイルはここから導出するので、
// 環境変数は TK_TASKS_FILE の1つだけで済む。
// 既定を ~/.config/tk/ に置くのは、設定とデータを1か所にまとめるため。
// dotfiles リポジトリで ~/.config を管理している場合は .gitignore が要る。
func tasksPath() (string, error) {
	if p := os.Getenv("TK_TASKS_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tk", "tasks.md"), nil
}

func aiCommand() string {
	if c := os.Getenv("TK_AI_CMD"); c != "" {
		return c
	}
	return "claude"
}

// editorCommand は詳細メモを書くエディタ。TK_EDITOR を先頭に置くのは
// 「tk のときだけ別のものを使いたい」逃げ道を残すため。
func editorCommand() string {
	for _, k := range []string{"TK_EDITOR", "VISUAL", "EDITOR"} {
		if c := os.Getenv(k); c != "" {
			return c
		}
	}
	return "vi"
}

func run() error {
	path, err := tasksPath()
	if err != nil {
		return err
	}

	inbox := usecase.NewInbox(markdown.NewStore(path), gh.NewPRSource(), gh.NewDetailSource(), markdown.NewDetailStore(path))
	// tasks.md が読めないなら起動を中止する。空リストで起動すると、
	// 書き戻し時に既存の内容を消しかねない。
	if err := inbox.Load(); err != nil {
		return fmt.Errorf("%s を読めない: %w", path, err)
	}

	cfg := tui.Config{
		AICmd:     aiCommand(),
		EditorCmd: editorCommand(),
		// tasks.md と同じディレクトリに置く。専用の環境変数を足さなくても
		// TK_TASKS_FILE を移せば一緒に付いてくる。
		ReviewPromptPath: filepath.Join(filepath.Dir(path), "review.md"),
	}
	_, err = tea.NewProgram(tui.New(inbox, cfg)).Run()
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
