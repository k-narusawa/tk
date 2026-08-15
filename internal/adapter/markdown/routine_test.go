package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRoutineStore(t *testing.T, body string) (*RoutineStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "routines.md")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return NewRoutineStore(path), dir
}

func TestRoutineStoreList(t *testing.T) {
	s, _ := newRoutineStore(t, "# 監視\n\n- golang のリリース\n- rust のリリース\n")
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "golang のリリース" {
		t.Fatalf("List() = %v", items)
	}
}

// routines.md がまだ無いのは普通の状態。起動を止めない。
func TestRoutineStoreListMissing(t *testing.T) {
	s, _ := newRoutineStore(t, "")
	items, err := s.List()
	if err != nil {
		t.Fatalf("ファイルが無いだけでエラーにしている: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("List() = %v, want 空", items)
	}
}

// 指示ファイルの置き場所はタスクの詳細と同じ導出。routines.md → routines/
func TestRoutineStoreDir(t *testing.T) {
	s, dir := newRoutineStore(t, "")
	if want := filepath.Join(dir, "routines"); s.Dir() != want {
		t.Errorf("Dir() = %q, want %q", s.Dir(), want)
	}
}

func TestRoutineStoreEditPath(t *testing.T) {
	s, dir := newRoutineStore(t, "")
	got, err := s.EditPath("golang のリリース")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "routines", "golang のリリース.md"); got != want {
		t.Errorf("EditPath() = %q, want %q", got, want)
	}
}

func TestRoutineStoreAppendResult(t *testing.T) {
	s, dir := newRoutineStore(t, "")
	if err := s.AppendResult("golang", "1回目\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendResult("golang", "2回目\n"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Result("golang")
	if err != nil {
		t.Fatal(err)
	}
	// 上書きすると「前回から何が変わったか」が消えるので追記であること。
	if !strings.Contains(got, "1回目") || !strings.Contains(got, "2回目") {
		t.Errorf("追記されていない:\n%s", got)
	}
	if strings.Index(got, "1回目") > strings.Index(got, "2回目") {
		t.Errorf("古い結果が後ろに来ていない:\n%s", got)
	}
	// いつの結果か分からないと、監視の記録として使えない。
	if strings.Count(got, "## ") != 2 {
		t.Errorf("実行ごとの日時見出しが無い:\n%s", got)
	}

	// 指示ファイルと同じディレクトリに、名前を揃えて置く。
	if _, err := os.Stat(filepath.Join(dir, "routines", "golang.result.md")); err != nil {
		t.Errorf("結果ファイルの置き場所が違う: %v", err)
	}
}

// 一度も実行していない routine は結果が無いのが普通。エラーにしない。
func TestRoutineStoreResultMissing(t *testing.T) {
	s, _ := newRoutineStore(t, "")
	got, err := s.Result("golang")
	if err != nil || got != "" {
		t.Errorf("Result() = %q, %v, want \"\", nil", got, err)
	}
}

// 名前に / が入っても指示ファイルと結果ファイルは同じ変換規則で並ぶ。
// ばらけると片方だけ迷子になる。
func TestRoutineStoreNameWithSlash(t *testing.T) {
	s, dir := newRoutineStore(t, "")
	if err := s.AppendResult("golang/go のリリース", "本文"); err != nil {
		t.Fatal(err)
	}
	edit, err := s.EditPath("golang/go のリリース")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "routines", "golang-go のリリース.md"); edit != want {
		t.Errorf("EditPath() = %q, want %q", edit, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "routines", "golang-go のリリース.result.md")); err != nil {
		t.Errorf("結果ファイルが指示ファイルと揃っていない: %v", err)
	}
}

// 結果ファイルは指示ファイルとして拾われてはいけない。".result.md" で
// 終わる名前の routine を書かれても、指示と結果が同じファイルを指さないこと。
func TestRoutineResultNameDiffersFromDetail(t *testing.T) {
	if detailName("golang") == resultName("golang") {
		t.Fatal("指示と結果が同じファイル名になっている")
	}
}
