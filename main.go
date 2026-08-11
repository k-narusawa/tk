package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/adapter/gh"
	"github.com/k-narusawa/tk/adapter/markdown"
	"github.com/k-narusawa/tk/adapter/tui"
	"github.com/k-narusawa/tk/usecase"
)

func tasksPath() (string, error) {
	if p := os.Getenv("TK_TASKS_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "tasks.md"), nil
}

func aiCommand() string {
	if c := os.Getenv("TK_AI_CMD"); c != "" {
		return c
	}
	return "claude"
}

func run() error {
	path, err := tasksPath()
	if err != nil {
		return err
	}

	inbox := usecase.NewInbox(markdown.NewStore(path), gh.NewPRSource(), gh.NewDetailSource())
	// tasks.md が読めないなら起動を中止する。空リストで起動すると、
	// 書き戻し時に既存の内容を消しかねない。
	if err := inbox.Load(); err != nil {
		return fmt.Errorf("%s を読めない: %w", path, err)
	}

	_, err = tea.NewProgram(tui.New(inbox, aiCommand())).Run()
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
