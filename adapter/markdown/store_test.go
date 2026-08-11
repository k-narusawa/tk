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
