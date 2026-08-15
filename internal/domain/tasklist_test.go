package domain

import (
	"strings"
	"testing"
)

// sample は見出し・空行・自由記述・完了済みタスクを全部含む。
const sample = `# 仕事

- [ ] 設計レビューの返信 @today
- [ ] 1on1の準備 @2026-08-12
- [x] 決済バグの再現手順を書く

## メモ
ここは自由記述。tk は解釈しないが、消しもしない。
`

func lines(s string) []string { return strings.Split(s, "\n") }

func TestParseRenderRoundTrip(t *testing.T) {
	got := strings.Join(Parse(lines(sample)).Render(), "\n")
	if got != sample {
		t.Errorf("round-trip が一致しない\n--- got:\n%q\n--- want:\n%q", got, sample)
	}
}

func TestParseItems(t *testing.T) {
	items := Parse(lines(sample)).Items()
	if len(items) != 3 {
		t.Fatalf("Items の件数 = %d, want 3", len(items))
	}

	want := []Item{
		{ID: TaskID(2), Kind: KindTask, Title: "設計レビューの返信", Tag: "@today"},
		{ID: TaskID(3), Kind: KindTask, Title: "1on1の準備", Tag: "@2026-08-12"},
		{ID: TaskID(4), Kind: KindTask, Title: "決済バグの再現手順を書く", Done: true},
	}
	for i, w := range want {
		if items[i] != w {
			t.Errorf("items[%d] = %+v, want %+v", i, items[i], w)
		}
	}
}

func TestToggleOnlyTouchesTargetLine(t *testing.T) {
	// 2行目（未完了）を完了にする
	got := strings.Join(Parse(lines(sample)).Toggle(TaskID(2)).Render(), "\n")
	want := strings.Replace(sample, "- [ ] 設計レビューの返信 @today", "- [x] 設計レビューの返信 @today", 1)
	if got != want {
		t.Errorf("Toggle の結果が違う\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

func TestToggleBackToIncomplete(t *testing.T) {
	got := strings.Join(Parse(lines(sample)).Toggle(TaskID(4)).Render(), "\n")
	want := strings.Replace(sample, "- [x] 決済バグの再現手順を書く", "- [ ] 決済バグの再現手順を書く", 1)
	if got != want {
		t.Errorf("Toggle（完了→未完了）の結果が違う\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

func TestToggleUnknownIDIsNoop(t *testing.T) {
	got := strings.Join(Parse(lines(sample)).Toggle(TaskID(999)).Render(), "\n")
	if got != sample {
		t.Errorf("存在しない ID で内容が変わった\n--- got:\n%q", got)
	}
}

// Add は「最後のチェックボックス行の直後」に入れる。
// 末尾に追記すると ## メモ の下に紛れ込むため。
func TestAddInsertsAfterLastCheckbox(t *testing.T) {
	got := strings.Join(Parse(lines(sample)).Add("新しいタスク").Render(), "\n")
	want := strings.Replace(sample,
		"- [x] 決済バグの再現手順を書く\n",
		"- [x] 決済バグの再現手順を書く\n- [ ] 新しいタスク\n", 1)
	if got != want {
		t.Errorf("Add の挿入位置が違う\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

// CRLF で統一されたファイルに Add すると、挿入行も CRLF に揃える。
func TestAddToCRLFFilePreservesLineEnding(t *testing.T) {
	src := "# 仕事\r\n- [ ] A\r\n"
	got := strings.Join(Parse(lines(src)).Add("新規").Render(), "\n")
	want := "# 仕事\r\n- [ ] A\r\n- [ ] 新規\r\n"
	if got != want {
		t.Errorf("CRLF ファイルへの Add で改行コードが揃わない\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

func TestAddToFileWithoutCheckbox(t *testing.T) {
	src := "# 仕事\n\n## メモ\n自由記述\n"
	got := strings.Join(Parse(lines(src)).Add("最初のタスク").Render(), "\n")
	want := "# 仕事\n\n## メモ\n自由記述\n- [ ] 最初のタスク\n"
	if got != want {
		t.Errorf("チェックボックス無しファイルへの Add\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

func TestAddToEmptyFileKeepsTrailingNewline(t *testing.T) {
	got := strings.Join(Parse(lines("")).Add("最初のタスク").Render(), "\n")
	want := "- [ ] 最初のタスク\n"
	if got != want {
		t.Errorf("空ファイルへの Add\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

// インデントされたチェックボックスも、余分な空白も原文のまま保つ。
func TestTogglePreservesIndentAndSpacing(t *testing.T) {
	src := "  - [ ] ネストしたタスク  \n"
	got := strings.Join(Parse(lines(src)).Toggle(TaskID(0)).Render(), "\n")
	want := "  - [x] ネストしたタスク  \n"
	if got != want {
		t.Errorf("インデント・末尾空白が壊れた\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

// 詳細付きのタスクを含むサンプル。空行を挟んだ2段落と、詳細を持たないタスク。
const withBody = `# 仕事

- [ ] 認証リファクタ @today
  - Cookie の SameSite を Lax に

  RFC を読み直す
- [ ] 詳細なしのタスク

## メモ
自由記述
`

// インデント行はもう詳細として解釈しないが、原文のまま保持する。
func TestParseRenderRoundTripWithIndentedLines(t *testing.T) {
	got := strings.Join(Parse(lines(withBody)).Render(), "\n")
	if got != withBody {
		t.Errorf("round-trip が一致しない\n--- got:\n%q\n--- want:\n%q", got, withBody)
	}
}

// インデントされたチェックボックスは独立したタスクとして数える。
func TestParseNestedCheckboxIsSeparateTask(t *testing.T) {
	src := "- [ ] 親\n  - [ ] 子\n"
	items := Parse(lines(src)).Items()
	if len(items) != 2 {
		t.Fatalf("Items の件数 = %d, want 2", len(items))
	}
}

// 詳細は別ファイルになったので、Add は最後のチェックボックス行の直後に入る。
// インデント行はもう詳細ではなく、ただの自由記述として原文のまま残る。
func TestAddInsertsAfterLastCheckboxLine(t *testing.T) {
	src := "- [ ] A\n  メモ1\n  メモ2\n"
	got := strings.Join(Parse(lines(src)).Add("B").Render(), "\n")
	want := "- [ ] A\n- [ ] B\n  メモ1\n  メモ2\n"
	if got != want {
		t.Errorf("Add の挿入位置が違う\n--- got:\n%q\n--- want:\n%q", got, want)
	}
}

func TestParseIgnoresNonCheckboxLines(t *testing.T) {
	src := "- ふつうの箇条書き\n- [] 閉じ括弧の形が違う\n* [ ] アスタリスク\n"
	if n := len(Parse(lines(src)).Items()); n != 0 {
		t.Errorf("チェックボックス以外を拾った: %d 件", n)
	}
}
