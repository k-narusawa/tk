package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/k-narusawa/tk/domain"
)

func TestRenderTask(t *testing.T) {
	items := []domain.Item{
		{ID: domain.TaskID(2), Kind: domain.KindTask, Title: "設計レビューの返信", Tag: "@today"},
	}
	got := Render(items)
	for _, want := range []string{"設計レビューの返信", "@today"} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() に %q が含まれない:\n%s", want, got)
		}
	}
}

func TestRenderPR(t *testing.T) {
	items := []domain.Item{
		{
			ID: domain.PRID("app/payment", 412), Kind: domain.KindPR,
			Title: "fix: 決済のnull落ち", Repo: "app/payment", Number: 412,
			URL: "https://github.com/app/payment/pull/412", Role: domain.RoleReview,
		},
	}
	got := Render(items)
	for _, want := range []string{"#412", "fix: 決済のnull落ち", "app/payment", "https://github.com/app/payment/pull/412"} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() に %q が含まれない:\n%s", want, got)
		}
	}
}

func TestRenderMultiple(t *testing.T) {
	items := []domain.Item{
		{ID: domain.TaskID(0), Kind: domain.KindTask, Title: "ひとつめ"},
		{ID: domain.TaskID(1), Kind: domain.KindTask, Title: "ふたつめ", Done: true},
	}
	got := Render(items)
	if !strings.Contains(got, "ひとつめ") || !strings.Contains(got, "ふたつめ") {
		t.Errorf("複数アイテムが出ていない:\n%s", got)
	}
}

// Command は一時ファイルに書き出し、そのパスを引数にした exec.Cmd を返す。
func TestCommandWritesTempFile(t *testing.T) {
	items := []domain.Item{{ID: domain.TaskID(0), Kind: domain.KindTask, Title: "やること"}}

	cmd, err := Command("claude", items)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if cmd.Args[0] != "claude" {
		t.Errorf("Args[0] = %q, want claude", cmd.Args[0])
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("Args = %v, want 2 要素", cmd.Args)
	}

	path := cmd.Args[1]
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("一時ファイルを読めない: %v", err)
	}
	if !strings.Contains(string(data), "やること") {
		t.Errorf("一時ファイルの内容 = %q", data)
	}
}

func TestCommandEmptyAICmd(t *testing.T) {
	if _, err := Command("", nil); err == nil {
		t.Error("空の AI コマンドでエラーが返らなかった")
	}
}
