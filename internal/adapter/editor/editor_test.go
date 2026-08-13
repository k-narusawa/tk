package editor

import (
	"slices"
	"testing"
)

func TestCommandConvertsLineToOneBased(t *testing.T) {
	// ID の行番号は 0 始まり、エディタの +N は 1 始まり。
	c, err := Command("nvim", "/tmp/tasks.md", 0)
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	want := []string{"nvim", "+1", "/tmp/tasks.md"}
	if !slices.Equal(c.Args, want) {
		t.Errorf("Args = %v, want %v", c.Args, want)
	}
}

func TestCommandSplitsEditorFields(t *testing.T) {
	c, err := Command("code -w", "/tmp/tasks.md", 4)
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	want := []string{"code", "-w", "+5", "/tmp/tasks.md"}
	if !slices.Equal(c.Args, want) {
		t.Errorf("Args = %v, want %v", c.Args, want)
	}
}

func TestCommandClampsNegativeLine(t *testing.T) {
	c, err := Command("vi", "/tmp/tasks.md", -3)
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	if c.Args[1] != "+1" {
		t.Errorf("Args[1] = %q, want %q", c.Args[1], "+1")
	}
}

func TestCommandEmptyEditorIsError(t *testing.T) {
	if _, err := Command("   ", "/tmp/tasks.md", 0); err == nil {
		t.Error("空のエディタ指定でエラーにならなかった")
	}
}
