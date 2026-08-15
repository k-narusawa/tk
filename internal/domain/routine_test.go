package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRoutines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"箇条書きを1行1件で拾う", "- golang のリリース\n- rust のリリース\n", []string{"golang のリリース", "rust のリリース"}},
		{"見出しや地の文は無視する", "# 監視\n\nメモ\n- golang のリリース\n", []string{"golang のリリース"}},
		{"インデントした行も拾う", "  - golang のリリース\n", []string{"golang のリリース"}},
		{"末尾の空白は落とす", "- golang のリリース   \n", []string{"golang のリリース"}},
		{"名前が空の行は捨てる", "- \n-\n- golang\n", []string{"golang"}},
		{"空ファイル", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := ParseRoutines(strings.Split(tt.input, "\n"))
			got := make([]string, 0, len(items))
			for _, it := range items {
				got = append(got, it.Title)
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRoutines() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRoutinesItem(t *testing.T) {
	items := ParseRoutines([]string{"- golang のリリース"})
	if len(items) != 1 {
		t.Fatalf("件数 = %d, want 1", len(items))
	}
	if items[0].Kind != KindRoutine {
		t.Errorf("Kind = %v, want KindRoutine", items[0].Kind)
	}
	if items[0].ID != RoutineID("golang のリリース") {
		t.Errorf("ID = %q, want %q", items[0].ID, RoutineID("golang のリリース"))
	}
}

// 同じ名前が2行あると ID が衝突する。実行状態も結果ファイルも名前で引くので、
// 2行目は捨てて「同じものが2つ見える」状態を作らない。
func TestParseRoutinesDedupe(t *testing.T) {
	items := ParseRoutines([]string{"- golang", "- rust", "- golang"})
	if len(items) != 2 {
		t.Fatalf("件数 = %d, want 2 (%v)", len(items), items)
	}
	if items[0].Title != "golang" || items[1].Title != "rust" {
		t.Errorf("順序が崩れている: %v", items)
	}
}

// CRLF の tasks.md を扱う以上 routines.md も来る。\r が名前に残ると
// 結果ファイルの名前まで汚れる。
func TestParseRoutinesCRLF(t *testing.T) {
	items := ParseRoutines(strings.Split("- golang のリリース\r\n", "\n"))
	if len(items) != 1 || items[0].Title != "golang のリリース" {
		t.Errorf("ParseRoutines() = %v", items)
	}
}
