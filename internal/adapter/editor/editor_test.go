package editor

import (
	"slices"
	"testing"
)

func TestCommandPassesPath(t *testing.T) {
	c, err := Command("nvim", "/tmp/tk/tasks/認証.md")
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	want := []string{"nvim", "/tmp/tk/tasks/認証.md"}
	if !slices.Equal(c.Args, want) {
		t.Errorf("Args = %v, want %v", c.Args, want)
	}
}

func TestCommandSplitsEditorFields(t *testing.T) {
	c, err := Command("code -w", "/tmp/tk/tasks/認証.md")
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	want := []string{"code", "-w", "/tmp/tk/tasks/認証.md"}
	if !slices.Equal(c.Args, want) {
		t.Errorf("Args = %v, want %v", c.Args, want)
	}
}

// 詳細ファイルを丸ごと開くので +N を渡さない。vi 系以外のエディタでも
// 引数が正しく解釈される。
func TestCommandHasNoLineJump(t *testing.T) {
	c, err := Command("vi", "/tmp/tk/tasks/認証.md")
	if err != nil {
		t.Fatalf("Command() = %v", err)
	}
	for _, a := range c.Args {
		if len(a) > 1 && a[0] == '+' {
			t.Errorf("行ジャンプ引数が残っている: %q", a)
		}
	}
}

func TestCommandEmptyEditorIsError(t *testing.T) {
	if _, err := Command("   ", "/tmp/tk/tasks/認証.md"); err == nil {
		t.Error("空のエディタ指定でエラーにならなかった")
	}
}
