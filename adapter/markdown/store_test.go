package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "# 仕事\n\n- [ ] やること @today\n\n## メモ\n自由記述\n"

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	list, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := s.Save(list); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sample {
		t.Errorf("round-trip でファイルが変わった\n--- got:\n%q\n--- want:\n%q", got, sample)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nope.md"))
	list, err := s.Load()
	if err != nil {
		t.Fatalf("存在しないファイルでエラー: %v", err)
	}
	if n := len(list.Items()); n != 0 {
		t.Errorf("Items() = %d 件, want 0", n)
	}
}

func TestSaveCreatesFileWithTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.md")
	s := NewStore(path)
	list, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(list.Add("最初のタスク")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "- [ ] 最初のタスク\n" {
		t.Errorf("新規作成の内容 = %q", got)
	}
}

// 一時ファイルを残さない
func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.md")
	s := NewStore(path)
	list, _ := s.Load()
	if err := s.Save(list.Add("x")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tk-") {
			t.Errorf("一時ファイルが残っている: %s", e.Name())
		}
	}
}

// Load 後に外部でファイルが書き換えられたら、Save はそれを検知して拒否し、
// 外部の変更内容を上書きしない。mtime だけに頼るとファイルシステムの解像度
// によっては直後の書き込みで mtime が変わらないことがあるため、外部の書き込み
// はサイズも変えて mtime に依存せず検知できるようにする。
func TestSaveRejectsExternalModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	list, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	external := sample + "外部エディタで追記した行\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Save(list.Add("tk 側で追加したタスク")); err == nil {
		t.Fatal("外部変更後の Save がエラーを返さなかった")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != external {
		t.Errorf("Save 失敗後のファイル内容が external と違う\n--- got:\n%q\n--- want:\n%q", got, external)
	}

	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tk-") {
			t.Errorf("Save 失敗後も一時ファイルが残っている: %s", e.Name())
		}
	}
}

// 連続で Save しても2回目が誤って「外部変更」判定されないこと
// （1回目の Save 後に憶えている mtime/size を更新しているか）。
func TestSaveTwiceSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	list, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := s.Save(list.Add("ひとつめ")); err != nil {
		t.Fatalf("1回目の Save() error = %v", err)
	}
	if err := s.Save(list.Add("ひとつめ").Add("ふたつめ")); err != nil {
		t.Fatalf("2回目の Save() error = %v", err)
	}
}

// ファイルが存在しない状態からの Save（初回作成）は競合とみなさない。
func TestSaveSucceedsWhenFileMissing(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "tasks.md"))
	list, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := s.Save(list.Add("最初のタスク")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

// tasks.md がシンボリックリンクなら、Save 後もリンクのままでリンク先の
// 実体が更新されること（os.Rename でリンクごと通常ファイルに置き換わらない）。
func TestSavePreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-tasks.md")
	link := filepath.Join(dir, "tasks.md")
	if err := os.WriteFile(real, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	s := NewStore(link)
	list, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := s.Save(list.Add("シンボリックリンク経由")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Save 後、tasks.md がシンボリックリンクでなくなった")
	}

	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "シンボリックリンク経由") {
		t.Errorf("リンク先の実体に新しい内容が反映されていない: %q", got)
	}
}

// Save が既存ファイルのパーミッションを 0600 に落としてしまわないこと。
func TestSavePreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	list, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(list); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("Save() 後のパーミッション = %v, want 0644", info.Mode().Perm())
	}
}
