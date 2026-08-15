package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k-narusawa/tk/internal/domain"
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

	cmd, path, err := Command("claude", items)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if cmd.Args[0] != "claude" {
		t.Errorf("Args[0] = %q, want claude", cmd.Args[0])
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("Args = %v, want 2 要素", cmd.Args)
	}
	if path != cmd.Args[1] {
		t.Errorf("path = %q, want cmd.Args[1] = %q", path, cmd.Args[1])
	}
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
	if _, _, err := Command("", nil); err == nil {
		t.Error("空の AI コマンドでエラーが返らなかった")
	}
}

// TK_AI_CMD にフラグを含められること（例: "claude --print"）。
func TestCommandWithArgs(t *testing.T) {
	items := []domain.Item{{ID: domain.TaskID(0), Kind: domain.KindTask, Title: "やること"}}

	cmd, path, err := Command("claude --print", items)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	want := []string{"claude", "--print"}
	if cmd.Args[0] != want[0] || cmd.Args[1] != want[1] {
		t.Errorf("Args[:2] = %v, want %v", cmd.Args[:2], want)
	}
	if len(cmd.Args) != 3 {
		t.Fatalf("Args = %v, want 3 要素", cmd.Args)
	}
}

func reviewPR() domain.Item {
	return domain.Item{
		ID: domain.PRID("app/payment", 412), Kind: domain.KindPR,
		Title: "fix: 決済のnull落ち", Repo: "app/payment", Number: 412,
		URL: "https://github.com/app/payment/pull/412", Role: domain.RoleReview,
	}
}

// プレースホルダを書かないプロンプトでも、末尾のブロックで対象 PR が伝わること。
func TestRenderReviewAppendsTarget(t *testing.T) {
	got := renderReview("この PR をレビューして。\n", reviewPR())

	if !strings.HasPrefix(got, "この PR をレビューして。") {
		t.Errorf("プロンプトが先頭に来ていない:\n%s", got)
	}
	for _, want := range []string{"app/payment", "412", "https://github.com/app/payment/pull/412"} {
		if !strings.Contains(got, want) {
			t.Errorf("対象 PR の %q が含まれない:\n%s", want, got)
		}
	}
}

func TestRenderReviewReplacesPlaceholders(t *testing.T) {
	got := renderReview("対象は {{url}}（{{repo}} #{{number}}）", reviewPR())

	want := "対象は https://github.com/app/payment/pull/412（app/payment #412）"
	if !strings.HasPrefix(got, want) {
		t.Errorf("置換後の先頭行が想定と違う:\n%s", got)
	}
	if strings.Contains(got, "{{") {
		t.Errorf("プレースホルダが残っている:\n%s", got)
	}
}

func TestReviewCommandWritesTempFile(t *testing.T) {
	promptPath := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(promptPath, []byte("観点: エラー処理\n"), 0o600); err != nil {
		t.Fatalf("プロンプトを書けない: %v", err)
	}

	cmd, path, err := ReviewCommand("claude", promptPath, reviewPR())
	if err != nil {
		t.Fatalf("ReviewCommand() error = %v", err)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "claude" {
		t.Fatalf("Args = %v, want [claude <path>]", cmd.Args)
	}
	if path != cmd.Args[1] {
		t.Errorf("path = %q, want cmd.Args[1] = %q", path, cmd.Args[1])
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("一時ファイルを読めない: %v", err)
	}
	for _, want := range []string{"観点: エラー処理", "app/payment"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("一時ファイルに %q が無い:\n%s", want, data)
		}
	}
}

// プロンプトが無いときは、作るべきパスがエラーに出ること。
// これが唯一の「どこに置けばいいか」の案内になる。
func TestReviewCommandMissingPromptMentionsPath(t *testing.T) {
	promptPath := filepath.Join(t.TempDir(), "review.md")

	_, _, err := ReviewCommand("claude", promptPath, reviewPR())
	if err == nil {
		t.Fatal("プロンプトが無いのにエラーが返らない")
	}
	if !strings.Contains(err.Error(), promptPath) {
		t.Errorf("エラーにパスが含まれない: %v", err)
	}
}
