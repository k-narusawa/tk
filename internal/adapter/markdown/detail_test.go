package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDetailDirDropsExtension(t *testing.T) {
	if got := NewDetailStore("/home/u/.config/tk/tasks.md").Dir(); got != "/home/u/.config/tk/tasks" {
		t.Errorf("Dir() = %q, want %q", got, "/home/u/.config/tk/tasks")
	}
}

// 拡張子が無いと「拡張子を落としたパス」がファイル自身と衝突し、
// MkdirAll がファイルにぶつかって失敗する。接尾辞を足して避ける。
func TestDetailDirAvoidsCollisionWhenNoExtension(t *testing.T) {
	got := NewDetailStore("/tmp/tasks").Dir()
	if got == "/tmp/tasks" {
		t.Fatal("拡張子なしのパスで詳細ディレクトリがファイル自身と同じになった")
	}
	if !strings.HasPrefix(got, "/tmp/tasks") {
		t.Errorf("Dir() = %q, want /tmp/tasks で始まるパス", got)
	}
}

// filepath.Ext は ".tasks" を丸ごと拡張子として返す。落とすと親ディレクトリに
// 縮み、詳細ファイルが $HOME に散らばる。
func TestDetailDirHandlesDotfileTasksPath(t *testing.T) {
	got := NewDetailStore("/home/u/.tasks").Dir()
	if got != "/home/u/.tasks.d" {
		t.Errorf("Dir() = %q, want %q", got, "/home/u/.tasks.d")
	}
}

func TestDetailNameSanitizes(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"そのまま", "認証まわりのリファクタ", "認証まわりのリファクタ.md"},
		{"スラッシュを置換", "api/auth を直す", "api-auth を直す.md"},
		{"先頭ドットで隠しファイルにしない", ".hidden", "-hidden.md"},
		{"前後の空白を落とす", "  やること  ", "やること.md"},
		{"タグはタイトルに含まれない", "認証", "認証.md"},
		{"スラッシュだけなら置換結果が残る", "///", "---.md"},
		{"空になったら _ にする", "   ", "_.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailName(tt.title); got != tt.want {
				t.Errorf("detailName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

// ファイル名の上限は 255 バイト。UTF-8 の途中で切ると壊れた文字が残る。
// 先頭に 1 バイト足して切り詰め位置をルート境界からずらし、切り戻しの
// ループを必ず通す。これが無いと、ループを消してもテストが通ってしまう。
func TestDetailNameTruncatesAtRuneBoundary(t *testing.T) {
	got := detailName("x" + strings.Repeat("あ", 200)) // 601 バイト
	if len(got) > 255 {
		t.Errorf("len = %d バイト, want <= 255", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("UTF-8 の途中で切れている: %q", got)
	}
	if !strings.HasSuffix(got, ".md") {
		t.Errorf("拡張子が落ちた: %q", got)
	}
	// 252 バイトちょうどでは切れないはず。切れていたらルート境界を割っている。
	if len(got) == 255 {
		t.Error("ルート境界を割って上限ぴったりで切っている")
	}
}

func TestBodyReadsFile(t *testing.T) {
	dir := t.TempDir()
	d := NewDetailStore(filepath.Join(dir, "tasks.md"))
	if err := os.MkdirAll(d.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "Cookie の SameSite を Lax に\n\nRFC を読み直す\n"
	if err := os.WriteFile(filepath.Join(d.Dir(), "認証.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := d.Body("認証")
	if err != nil {
		t.Fatalf("Body() = %v", err)
	}
	if got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

// 詳細が無いタスクのほうが普通なので、未作成をエラーにしない。
func TestBodyMissingFileIsEmptyNotError(t *testing.T) {
	d := NewDetailStore(filepath.Join(t.TempDir(), "tasks.md"))
	got, err := d.Body("まだ書いていない")
	if err != nil {
		t.Fatalf("Body() = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Body() = %q, want 空文字", got)
	}
}

// エディタは親ディレクトリが無いと保存に失敗するので、先に作っておく。
func TestEditPathCreatesDir(t *testing.T) {
	d := NewDetailStore(filepath.Join(t.TempDir(), "tasks.md"))

	got, err := d.EditPath("api/auth")
	if err != nil {
		t.Fatalf("EditPath() = %v", err)
	}
	want := filepath.Join(d.Dir(), "api-auth.md")
	if got != want {
		t.Errorf("EditPath() = %q, want %q", got, want)
	}
	info, err := os.Stat(d.Dir())
	if err != nil {
		t.Fatalf("詳細ディレクトリが作られていない: %v", err)
	}
	if !info.IsDir() {
		t.Error("詳細ディレクトリがディレクトリでない")
	}
}

// tk は詳細を書かない。EditPath はパスを返すだけで、ファイルは作らない。
func TestEditPathDoesNotCreateFile(t *testing.T) {
	d := NewDetailStore(filepath.Join(t.TempDir(), "tasks.md"))
	p, err := d.EditPath("やること")
	if err != nil {
		t.Fatalf("EditPath() = %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("EditPath がファイルを作った: %v", err)
	}
}
