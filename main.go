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
// routineCommand は routine を裏で回す非対話 CLI。TK_AI_CMD と分けるのは、
// あちらが対話起動（画面を明け渡して人が読む）なのに対し、こちらは終了を
// 待って標準出力を拾う必要があるため。課金体系が変わったら環境変数だけで
// 他の CLI に乗り換えられる。
//
// --allowedTools を既定に含めるのは、非対話では承認ダイアログを出せず、
// 指定しないと調べ物のツールが全部拒否されて「取得できませんでした」だけが
// 結果ファイルに追記されるため。読み取り系だけに絞る。
func routineCommand() string {
	if c := os.Getenv("TK_ROUTINE_CMD"); c != "" {
		return c
	}
	return `claude -p --allowedTools "WebSearch,WebFetch,Bash(gh api:*)"`
}

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

	// routines.md は tasks.md の隣。専用の環境変数を足さなくても
	// TK_TASKS_FILE を移せば一緒に付いてくる（review.md と同じ導出）。
	routinesPath := filepath.Join(filepath.Dir(path), "routines.md")

	inbox := usecase.NewInbox(
		markdown.NewStore(path),
		gh.NewPRSource(),
		gh.NewDetailSource(),
		markdown.NewDetailStore(path),
		markdown.NewRoutineStore(routinesPath),
	)
	// tasks.md が読めないなら起動を中止する。空リストで起動すると、
	// 書き戻し時に既存の内容を消しかねない。
	if err := inbox.Load(); err != nil {
		return fmt.Errorf("%s を読めない: %w", path, err)
	}

	cfg := tui.Config{
		AICmd:      aiCommand(),
		RoutineCmd: routineCommand(),
		EditorCmd:  editorCommand(),
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
