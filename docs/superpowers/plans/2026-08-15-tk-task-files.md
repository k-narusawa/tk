# タスク詳細の1タスク1ファイル化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** タスクの詳細メモを `tasks.md` のインデント行から `~/.config/tk/tasks/<タイトル>.md` へ移し、`e` がそのファイルを直接開くようにする。

**Architecture:** `tasks.md` は一覧（順番・完了・タグ）のインデックスとして書式そのまま残す。詳細はタスクごとの独立した Markdown ファイルで、タイトルをファイル名にして結びつける。詳細ディレクトリは `TK_TASKS_FILE` から導出するので環境変数は増えない。tk は詳細を書かず、読むのと `mkdir` だけを行う。

**Tech Stack:** Go 1.26、標準ライブラリのみ（`os` / `path/filepath` / `unicode/utf8`）。TUI は charm.land の bubbletea v2 / lipgloss v2 / bubbles v2。テストは標準の `testing` のみ。

**Spec:** [docs/superpowers/specs/2026-08-15-tk-task-files-design.md](../specs/2026-08-15-tk-task-files-design.md)

## Global Constraints

- **依存を増やさない。** `go.mod` の require に新しい直接依存を足さない。TOML パーサも SQLite も入れない
- **テストフレームワークを足さない。** 標準の `testing` のみ
- **依存の向きは adapter → usecase → domain の一方向。** `internal/domain` は標準ライブラリのみ、`internal/usecase` は `internal/domain` + 標準ライブラリのみを import する。`internal/usecase` のテストも `internal/adapter/*` を import しない（fake を test ファイル内に書く）
- **`internal/adapter/tui` のテストも `internal/adapter/markdown` を import しない。** fake を `model_test.go` に書く
- **`Parse` → `Render` はバイト一致を維持する。** `tasks.md` の非チェックボックス行は原文のまま保持する
- **bubbletea は v2。** `View() tea.View`、`tea.KeyPressMsg`、スペースキーは `msg.String() == "space"`
- **モジュールパスは `charm.land/...`**（旧 `github.com/charmbracelet/...` ではない）
- **各タスクの終わりで `go build ./... && go test ./... && go vet ./...` が通ること。** 途中でビルドが壊れる順序にしない
- 環境変数の既定値: `TK_TASKS_FILE` = `~/.config/tk/tasks.md`、`TK_AI_CMD` = `claude`、`TK_EDITOR` = `$VISUAL` → `$EDITOR` → `vi`
- コミットメッセージは Conventional Commits（`type(scope): 日本語の要約`）。句点は付けない

---

## File Structure

| ファイル | 責務 | タスク |
|---|---|---|
| `internal/adapter/markdown/detail.go` | **新規。** 詳細ファイルの読み込み、ファイル名 sanitize、詳細ディレクトリの導出と作成 | 1 |
| `internal/adapter/markdown/detail_test.go` | **新規。** 上のテスト | 1 |
| `internal/usecase/port.go` | `TaskDetailStore` ポートを追加 | 2 |
| `internal/usecase/inbox.go` | `Inbox` に詳細ストアを持たせ、`Body` / `DetailPath` を追加 | 2 |
| `internal/usecase/inbox_test.go` | `fakeDetailStore` 追加、`NewInbox` 呼び出しの更新 | 2 |
| `internal/adapter/editor/editor.go` | `Command` から `line` 引数を落とす | 3 |
| `internal/adapter/editor/editor_test.go` | 行ジャンプのテストを削除、引数のテストを書き直す | 3 |
| `internal/adapter/tui/update.go` | `editExec` が詳細ファイルを開く | 4 |
| `internal/adapter/tui/view.go` | `syncDetail` がファイルから本文を読む、`detailText` が本文を引数で受ける | 4 |
| `internal/adapter/tui/model_test.go` | `fakeDetailStore` 追加、詳細まわりのテスト書き直し | 4 |
| `internal/domain/tasklist.go` | `bodyAt` / `dedent` / `task.body` を削除、`Add` の挿入位置を単純化 | 5 |
| `internal/domain/item.go` | `Item.Body` を削除 | 5 |
| `internal/domain/tasklist_test.go` | 詳細パースのテストを削除、`Add` のテストを更新 | 5 |
| `main.go` | 既定パスの変更と詳細ストアの配線 | 2, 6 |
| `README.md` | 保存形式・移行手順・キー説明の更新 | 7 |
| `docs/known-issues.md` | 設計判断の追記 | 7 |

`detail.go` を既存の `store.go` と分けるのは、`Store` が `mtime` / `size` の状態を持ち並行呼び出しが安全でないのに対し、`DetailStore` は状態を持たず読むだけだから。混ぜると `Store` の制約が `DetailStore` にも広がる。

---

### Task 1: `markdown.DetailStore`（詳細ファイルの読み込み）

**Files:**
- Create: `internal/adapter/markdown/detail.go`
- Test: `internal/adapter/markdown/detail_test.go`

**Interfaces:**
- Consumes: なし（このタスクが最初）
- Produces:
  - `func NewDetailStore(tasksFile string) *DetailStore`
  - `func (d *DetailStore) Dir() string`
  - `func (d *DetailStore) Body(title string) (string, error)`
  - `func (d *DetailStore) EditPath(title string) (string, error)`

- [ ] **Step 1: 詳細ディレクトリの導出テストを書く**

`internal/adapter/markdown/detail_test.go` を新規作成:

```go
package markdown

import (
	"strings"
	"testing"
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
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/adapter/markdown/ -run TestDetailDir -v`
Expected: FAIL（`undefined: NewDetailStore` でコンパイルエラー）

- [ ] **Step 3: `NewDetailStore` と `Dir` を実装する**

`internal/adapter/markdown/detail.go` を新規作成:

```go
package markdown

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// DetailStore はタスク1件ぶんの詳細ファイルを読む。Store と違って
// mtime/size の状態を持たないので、並行に呼んでも安全。
type DetailStore struct{ dir string }

// NewDetailStore は tasks.md のパスから詳細ディレクトリを決める。
// 専用の環境変数を増やさないための導出。
//
//	~/.config/tk/tasks.md → ~/.config/tk/tasks/
//	/tmp/t.md             → /tmp/t/
//
// 拡張子が無いと落とした結果がファイル自身と衝突するので、その場合だけ
// ".d" を足す。衝突したままだと MkdirAll がファイルにぶつかって失敗する。
func NewDetailStore(tasksFile string) *DetailStore {
	dir := strings.TrimSuffix(tasksFile, filepath.Ext(tasksFile))
	if dir == tasksFile {
		dir += ".d"
	}
	return &DetailStore{dir: dir}
}

func (d *DetailStore) Dir() string { return d.dir }
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/adapter/markdown/ -run TestDetailDir -v`
Expected: PASS（2件）

- [ ] **Step 5: ファイル名 sanitize のテストを書く**

`internal/adapter/markdown/detail_test.go` に追記:

```go
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
func TestDetailNameTruncatesAtRuneBoundary(t *testing.T) {
	got := detailName(strings.Repeat("あ", 200)) // 600 バイト
	if len(got) > 255 {
		t.Errorf("len = %d バイト, want <= 255", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("UTF-8 の途中で切れている: %q", got)
	}
	if !strings.HasSuffix(got, ".md") {
		t.Errorf("拡張子が落ちた: %q", got)
	}
}
```

`detail_test.go` の import に `"unicode/utf8"` を足す。

- [ ] **Step 6: テストが落ちることを確認する**

Run: `go test ./internal/adapter/markdown/ -run TestDetailName -v`
Expected: FAIL（`undefined: detailName`）

- [ ] **Step 7: `detailName` を実装する**

`internal/adapter/markdown/detail.go` に追記:

```go
// maxNameBytes はファイル名の上限。macOS / Linux の 255 バイト。
const maxNameBytes = 255

// detailName はタスクのタイトルを詳細ファイルの名前に変換する。
// タイトルがそのまま読めることを優先し、使えない文字だけを潰す。
func detailName(title string) string {
	s := strings.TrimSpace(title)
	s = strings.NewReplacer("/", "-", "\x00", "-").Replace(s)
	s = strings.TrimSpace(s)
	// 先頭のドットは隠しファイルになるので潰す。ls で見えないと存在を忘れる。
	if strings.HasPrefix(s, ".") {
		s = "-" + s[1:]
	}
	s = truncateBytes(s, maxNameBytes-len(".md"))
	if s == "" {
		s = "_"
	}
	return s + ".md"
}

// truncateBytes は UTF-8 のルート境界を割らずに n バイト以内へ切り詰める。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
```

- [ ] **Step 8: テストが通ることを確認する**

Run: `go test ./internal/adapter/markdown/ -run TestDetailName -v`
Expected: PASS（サブテスト7件 + 切り詰め1件）

- [ ] **Step 9: `Body` と `EditPath` のテストを書く**

`internal/adapter/markdown/detail_test.go` の import に `"os"` と `"path/filepath"` を足したうえで追記:

```go
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
```

- [ ] **Step 10: テストが落ちることを確認する**

Run: `go test ./internal/adapter/markdown/ -run 'TestBody|TestEditPath' -v`
Expected: FAIL（`d.Body undefined` / `d.EditPath undefined`）

- [ ] **Step 11: `Body` と `EditPath` を実装する**

`internal/adapter/markdown/detail.go` に追記:

```go
// Body は詳細の本文。ファイルが無ければ空文字を返す。詳細を持たない
// タスクのほうが普通なので、未作成をエラーにしない。
func (d *DetailStore) Body(title string) (string, error) {
	data, err := os.ReadFile(filepath.Join(d.dir, detailName(title)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EditPath は e が開くパス。tk は詳細を書かないのでファイルは作らないが、
// 親ディレクトリだけは作る。エディタは親が無いと保存に失敗する。
func (d *DetailStore) EditPath(title string) (string, error) {
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(d.dir, detailName(title)), nil
}
```

- [ ] **Step 12: テストが通ることを確認する**

Run: `go test ./internal/adapter/markdown/ -v`
Expected: PASS（既存の `store_test.go` も含めて全部）

- [ ] **Step 13: ビルドと vet を確認する**

Run: `go build ./... && go vet ./...`
Expected: 出力なし

- [ ] **Step 14: コミット**

```bash
git add internal/adapter/markdown/detail.go internal/adapter/markdown/detail_test.go
git commit -m "feat(markdown): タスク詳細ファイルを読む DetailStore を追加"
```

---

### Task 2: usecase のポートと `Inbox.Body` / `Inbox.DetailPath`

**Files:**
- Modify: `internal/usecase/port.go`
- Modify: `internal/usecase/inbox.go:11-28`（`Inbox` 構造体と `NewInbox`）
- Modify: `internal/usecase/inbox_test.go`（`NewInbox` の全呼び出し）
- Modify: `internal/adapter/tui/model_test.go`（`NewInbox` の全呼び出し）
- Modify: `main.go:51`

**Interfaces:**
- Consumes: Task 1 の `markdown.NewDetailStore(tasksFile) *markdown.DetailStore`、`(*DetailStore).Body(title string) (string, error)`、`(*DetailStore).EditPath(title string) (string, error)`
- Produces:
  - `type TaskDetailStore interface { Body(title string) (string, error); EditPath(title string) (string, error) }`
  - `func NewInbox(store TaskStore, prs PRSource, details PRDetailSource, taskDetails TaskDetailStore) *Inbox`（**第4引数が増える**）
  - `func (i *Inbox) Body(id domain.ID) (string, error)`
  - `func (i *Inbox) DetailPath(id domain.ID) (string, error)`

- [ ] **Step 1: 既存の `NewInbox` 呼び出しを機械的に更新する**

**新しいテストより先にここをやる。** 逆にすると、下の `perl` が新しいテストの
4引数の呼び出しにも第5引数を足してしまう。

既存の呼び出しはすべて1行で `)` で終わっているので、まとめて置換する:

```bash
perl -pi -e 's/NewInbox\((.*)\)$/NewInbox($1, \&fakeDetailStore{})/' internal/usecase/inbox_test.go
perl -pi -e 's/usecase\.NewInbox\((.*)\)$/usecase.NewInbox($1, \&fakeDetailStore{})/' internal/adapter/tui/model_test.go
```

`internal/usecase/inbox_test.go` の先頭付近（他の fake の並び）と、
`internal/adapter/tui/model_test.go` の `fakeStore` の定義の直後に、
**同じ内容**を追記する（どちらのテストも `internal/adapter/markdown` を
import しないので、fake を二重に持つ）:

```go
// fakeDetailStore は usecase.TaskDetailStore を満たす。タイトル → 本文の
// マップで、ファイルシステムを使わずに詳細の受け渡しを検証する。
type fakeDetailStore struct {
	bodies map[string]string
	err    error
}

func (f *fakeDetailStore) Body(title string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.bodies[title], nil
}

func (f *fakeDetailStore) EditPath(title string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "/tmp/tk-test/" + title + ".md", nil
}
```

`main.go:51` を手で直す:

```go
	store := markdown.NewStore(path)
	inbox := usecase.NewInbox(store, gh.NewPRSource(), gh.NewDetailSource(), markdown.NewDetailStore(path))
```

この時点ではまだ `NewInbox` が3引数なのでビルドは通らない。Step 4-5 で通る。

- [ ] **Step 2: 失敗するテストを書く**

`internal/usecase/inbox_test.go` の末尾に追記:

```go
func TestBodyReadsDetailOfSelectedTask(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 認証まわりのリファクタ @today\n- [ ] 別のタスク\n")}
	details := &fakeDetailStore{bodies: map[string]string{"認証まわりのリファクタ": "Cookie の SameSite を Lax に"}}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, details)
	if err := in.Load(); err != nil {
		t.Fatal(err)
	}

	got, err := in.Body(domain.TaskID(0))
	if err != nil {
		t.Fatalf("Body() = %v", err)
	}
	if got != "Cookie の SameSite を Lax に" {
		t.Errorf("Body() = %q, want %q", got, "Cookie の SameSite を Lax に")
	}
}

// タグはファイル名に含めない。タグを付け替えても詳細が迷子にならないため。
func TestDetailPathUsesTitleWithoutTag(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 認証 @today\n")}
	details := &fakeDetailStore{}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, details)
	if err := in.Load(); err != nil {
		t.Fatal(err)
	}

	got, err := in.DetailPath(domain.TaskID(0))
	if err != nil {
		t.Fatalf("DetailPath() = %v", err)
	}
	if got != "/tmp/tk-test/認証.md" {
		t.Errorf("DetailPath() = %q, want %q（タグが混ざっている）", got, "/tmp/tk-test/認証.md")
	}
}

// 存在しない ID で詳細を開こうとしたら、空パスを返さずエラーにする。
// 空パスのままエディタを起動すると、意図しない場所に保存されうる。
func TestDetailPathUnknownIDIsError(t *testing.T) {
	in := NewInbox(&fakeStore{list: taskList("- [ ] やること\n")}, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{})
	if err := in.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := in.DetailPath(domain.TaskID(99)); err == nil {
		t.Error("存在しない ID なのにエラーにならなかった")
	}
}
```

- [ ] **Step 3: テストが落ちることを確認する**

Run: `go test ./internal/usecase/ -run 'TestBodyReads|TestDetailPath' -v`
Expected: FAIL（`too many arguments in call to NewInbox` でコンパイルエラー）

- [ ] **Step 4: ポートを追加する**

`internal/usecase/port.go` の `TaskStore` の直後に追記:

```go
// TaskDetailStore はタスク1件ぶんの詳細ファイル。adapter/markdown が実装する。
// 詳細は tasks.md の中ではなく、タイトルを名前にした独立したファイルに置く。
type TaskDetailStore interface {
	// Body は詳細の本文。ファイルが無ければ空文字を返し、エラーにしない。
	Body(title string) (string, error)
	// EditPath は e が開くパス。親ディレクトリが無ければ作る。
	EditPath(title string) (string, error)
}
```

- [ ] **Step 5: `Inbox` に詳細ストアを持たせる**

`internal/usecase/inbox.go` の構造体に1フィールド足す:

```go
type Inbox struct {
	store       TaskStore
	prs         PRSource
	details     PRDetailSource
	taskDetails TaskDetailStore

	mu      sync.Mutex
	tasks   domain.TaskList
	prItems []domain.Item
	cache   map[domain.ID]domain.PRDetail
}

func NewInbox(store TaskStore, prs PRSource, details PRDetailSource, taskDetails TaskDetailStore) *Inbox {
	return &Inbox{
		store:       store,
		prs:         prs,
		details:     details,
		taskDetails: taskDetails,
		cache:       make(map[domain.ID]domain.PRDetail),
	}
}
```

同じファイルの末尾に追記:

```go
// title は id に対応するタスクのタイトル。詳細ファイルの名前の元になる。
func (i *Inbox) title(id domain.ID) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, it := range i.tasks.Items() {
		if it.ID == id {
			return it.Title, true
		}
	}
	return "", false
}

// Body は選択中タスクの詳細。カーソルが動くたびに呼ばれるが、ローカルの
// 数 KB のファイルを1本読むだけなのでキャッシュしない。キャッシュしない
// ことで、エディタで書き換えた内容が次に選んだ瞬間に反映される。
func (i *Inbox) Body(id domain.ID) (string, error) {
	title, ok := i.title(id)
	if !ok {
		return "", nil
	}
	return i.taskDetails.Body(title)
}

// DetailPath は e が開く詳細ファイルのパス。存在しない ID はエラーにする。
// 空パスを返すとエディタが意図しない場所を開いてしまう。
func (i *Inbox) DetailPath(id domain.ID) (string, error) {
	title, ok := i.title(id)
	if !ok {
		return "", fmt.Errorf("タスクが見つからない: %s", id)
	}
	return i.taskDetails.EditPath(title)
}
```

（`fmt` は `inbox.go` で既に import 済み。）

- [ ] **Step 6: 置換結果を目視する**

Run: `git diff && go build ./...`
Expected: ビルドが通る。通らない行があれば Step 1 の `perl` が空振りしているので手で直す。`perl` は行末が `)` の呼び出ししか拾わないので、複数行に分かれた呼び出しがあれば目視で見つける

- [ ] **Step 7: テストが通ることを確認する**

Run: `go test ./... && go vet ./...`
Expected: PASS（全パッケージ）

- [ ] **Step 8: コミット**

```bash
git add internal/usecase internal/adapter/tui/model_test.go main.go
git commit -m "feat(usecase): 詳細ファイルのポートと Inbox.Body/DetailPath を追加"
```

---

### Task 3: `editor.Command` から行ジャンプを落とす

**Files:**
- Modify: `internal/adapter/editor/editor.go`
- Modify: `internal/adapter/editor/editor_test.go`（全面書き換え）
- Modify: `internal/adapter/tui/update.go:52`（呼び出し側をコンパイルさせるため）

**Interfaces:**
- Consumes: なし
- Produces: `func Command(editorCmd, path string) (*exec.Cmd, error)`（**`line` 引数が消える**）

- [ ] **Step 1: テストを書き換える**

`internal/adapter/editor/editor_test.go` を全面的に置き換える:

```go
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
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/adapter/editor/ -v`
Expected: FAIL（`not enough arguments in call to Command`）

- [ ] **Step 3: `Command` から `line` を落とす**

`internal/adapter/editor/editor.go` を置き換える:

```go
// Package editor はタスクの詳細を書くためのエディタ起動コマンドを組み立てる。
package editor

import (
	"errors"
	"os/exec"
	"strings"
)

// Command は詳細ファイルを開く *exec.Cmd を返す。1タスク1ファイルなので
// 行ジャンプは要らない。"+N" を渡さないぶん、vi 系以外のエディタでも
// 引数がそのまま通る。tea.ExecProcess で包むのは adapter/tui の仕事。
func Command(editorCmd, path string) (*exec.Cmd, error) {
	fields := strings.Fields(editorCmd)
	if len(fields) == 0 {
		return nil, errors.New("エディタが指定されていない")
	}
	return exec.Command(fields[0], append(fields[1:], path)...), nil
}
```

- [ ] **Step 4: 呼び出し側を暫定で直す**

`internal/adapter/tui/update.go` の `editExec` 内、`editor.Command(m.cfg.EditorCmd, m.cfg.TasksFile, line)` を次に変える（詳細ファイルへの切り替えは Task 4）:

```go
	c, err := editor.Command(m.cfg.EditorCmd, m.cfg.TasksFile)
```

直前の `line, ok := it.ID.TaskLine()` ブロックが未使用になるので削除する:

```go
func (m Model) editExec() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.Kind != domain.KindTask {
		return nil
	}
	c, err := editor.Command(m.cfg.EditorCmd, m.cfg.TasksFile)
	if err != nil {
		return func() tea.Msg { return editDoneMsg{err: err} }
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{err: err}
	})
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./... && go vet ./...`
Expected: PASS（全パッケージ）

- [ ] **Step 6: コミット**

```bash
git add internal/adapter/editor internal/adapter/tui/update.go
git commit -m "refactor(editor): 行ジャンプ引数を落として詳細ファイルを丸ごと開く"
```

---

### Task 4: TUI が詳細ファイルを読み書きする

**Files:**
- Modify: `internal/adapter/tui/update.go`（`editExec`）
- Modify: `internal/adapter/tui/view.go`（`syncDetail` と `detailText`）
- Modify: `internal/adapter/tui/model_test.go`

**Interfaces:**
- Consumes: Task 2 の `(*usecase.Inbox).Body(id domain.ID) (string, error)` と `(*usecase.Inbox).DetailPath(id domain.ID) (string, error)`、Task 3 の `editor.Command(editorCmd, path string)`
- Produces: `func detailText(it domain.Item, body string, e detailEntry, loaded bool) string`（**`body` 引数が増える**）

- [ ] **Step 1: 失敗するテストを書く**

`internal/adapter/tui/model_test.go` の `TestDetailShowsTaskBody` を置き換え、後ろに3件足す:

```go
func TestDetailShowsTaskBody(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	details := &fakeDetailStore{bodies: map[string]string{"やること": "詳細のメモ"}}
	inbox := usecase.NewInbox(store, &fakePRs{}, &fakeDetails{}, details)
	if err := inbox.Load(); err != nil {
		t.Fatal(err)
	}
	m := New(inbox, Config{})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	if !strings.Contains(m.View().Content, "詳細のメモ") {
		t.Errorf("詳細ペインにメモが出ていない:\n%s", m.View().Content)
	}
}

// 詳細ファイルが読めないときは黙って空にせず、右ペインに理由を出す。
// 空表示だと「詳細を書いていない」と見分けが付かない。
func TestDetailShowsReadError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	details := &fakeDetailStore{err: errors.New("permission denied")}
	inbox := usecase.NewInbox(store, &fakePRs{}, &fakeDetails{}, details)
	if err := inbox.Load(); err != nil {
		t.Fatal(err)
	}
	m := New(inbox, Config{})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	if !strings.Contains(m.View().Content, "permission denied") {
		t.Errorf("読み込みエラーが右ペインに出ていない:\n%s", m.View().Content)
	}
}

// e は tasks.md ではなく、そのタスクの詳細ファイルを開く。
func TestEKeyOpensDetailFileNotTasksFile(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatal(err)
	}
	m := New(inbox, Config{EditorCmd: "true", TasksFile: "/tmp/tasks.md"})

	path, err := inbox.DetailPath(domain.TaskID(0))
	if err != nil {
		t.Fatal(err)
	}
	if path == "/tmp/tasks.md" {
		t.Fatal("詳細ファイルのパスが tasks.md と同じ")
	}

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if cmd == nil {
		t.Fatal("e を押しても cmd が nil")
	}
}

// 詳細ファイルのパスが取れないときも、黙って何もしないのではなくエラーを出す。
func TestEKeyWithDetailPathErrorSurfacesError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	details := &fakeDetailStore{err: errors.New("mkdir: read-only file system")}
	inbox := usecase.NewInbox(store, &fakePRs{}, &fakeDetails{}, details)
	if err := inbox.Load(); err != nil {
		t.Fatal(err)
	}
	m := New(inbox, Config{EditorCmd: "true"})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if cmd == nil {
		t.Fatal("パスが取れなくても cmd が nil であってはならない")
	}
	done, ok := cmd().(editDoneMsg)
	if !ok {
		t.Fatalf("cmd() が返したのは %T, want editDoneMsg", cmd())
	}
	if done.err == nil {
		t.Error("パスが取れないのに editDoneMsg.err が nil")
	}
}
```

同じファイルの `TestEditDoneReloadsTasks` から `Body` の検証を落とす（詳細はもう `Item` に載らない）:

```go
// エディタで書いた詳細を取り込むため、閉じたら必ず読み直す。
func TestEditDoneReloadsTasks(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)
	before := store.loadCalls

	got, _ := m.Update(editDoneMsg{})
	m = got.(Model)

	if store.loadCalls == before {
		t.Fatal("エディタを閉じても tasks.md を読み直していない")
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/adapter/tui/ -run 'TestDetailShows|TestEKey' -v`
Expected: FAIL（`TestDetailShowsTaskBody` は「詳細ペインにメモが出ていない」、`TestDetailShowsReadError` は「読み込みエラーが右ペインに出ていない」）

- [ ] **Step 3: `syncDetail` と `detailText` を書き換える**

`internal/adapter/tui/view.go` の `syncDetail` / `detailText` を置き換える:

```go
// syncDetail は選択中アイテムの内容を viewport に流し込む。タスクの詳細は
// 選ばれるたびにファイルから読む。キャッシュしないので、エディタで
// 書き換えた内容が次に選んだ瞬間に反映される。
func (m *Model) syncDetail() {
	it, ok := m.selected()
	if !ok {
		m.detail.SetContent("")
		return
	}
	if it.Kind == domain.KindTask {
		body, err := m.inbox.Body(it.ID)
		if err != nil {
			// 空表示にすると「詳細を書いていない」と見分けが付かない。
			body = "（詳細を読めませんでした: " + err.Error() + "）"
		}
		m.detail.SetContent(detailText(it, body, detailEntry{}, false))
		return
	}
	e, loaded := m.details[it.ID]
	m.detail.SetContent(detailText(it, "", e, loaded))
}

func detailText(it domain.Item, body string, e detailEntry, loaded bool) string {
	if it.Kind == domain.KindTask {
		var b strings.Builder
		b.WriteString(it.Title + "\n\n")
		if it.Tag != "" {
			b.WriteString("tag    : " + it.Tag + "\n")
		}
		state := "未完了"
		if it.Done {
			state = "完了"
		}
		b.WriteString("state  : " + state + "\n")
		if body != "" {
			b.WriteString("\n" + strings.TrimRight(body, "\n") + "\n")
		}
		return b.String()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s\n\n", it.Number, it.Title)
	fmt.Fprintf(&b, "repo   : %s\n", it.Repo)
	fmt.Fprintf(&b, "role   : %s\n", it.Role)

	if !loaded {
		b.WriteString("\n（詳細を取得中…）\n")
		return b.String()
	}
	if e.err != "" {
		fmt.Fprintf(&b, "\n（詳細を取得できませんでした: %s）\n", e.err)
		return b.String()
	}
	d := e.detail
	if d.CI != "" {
		fmt.Fprintf(&b, "CI     : %s %s\n", ciMark(d.CI), d.CI)
	}
	if d.Reviews != "" {
		fmt.Fprintf(&b, "review : %s\n", d.Reviews)
	}
	fmt.Fprintf(&b, "+%d -%d (%d files)\n", d.Additions, d.Deletions, d.ChangedFiles)
	b.WriteString("\n" + it.URL + "\n")
	return b.String()
}
```

ファイルから読んだ本文は末尾に改行を持つので `strings.TrimRight` する。付けたままだと枠の中で1行ぶん余る。

- [ ] **Step 4: `editExec` が詳細ファイルを開くようにする**

`internal/adapter/tui/update.go` の `editExec` を置き換える:

```go
// editExec は選択中タスクの詳細ファイルをエディタで開く tea.Cmd を作る。
// tk は詳細を書き込まない。編集はエディタに任せ、閉じたら tasks.md を
// 読み直す（タイトルや完了状態が変わっているかもしれないため）。
func (m Model) editExec() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.Kind != domain.KindTask {
		return nil
	}
	fail := func(err error) tea.Cmd {
		return func() tea.Msg { return editDoneMsg{err: err} }
	}
	path, err := m.inbox.DetailPath(it.ID)
	if err != nil {
		return fail(err)
	}
	c, err := editor.Command(m.cfg.EditorCmd, path)
	if err != nil {
		return fail(err)
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{err: err}
	})
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/adapter/tui/ -v`
Expected: PASS

- [ ] **Step 6: 全体のテストと競合検出**

Run: `go test -race ./... && go vet ./...`
Expected: PASS（`-race` は TUI と `Refresh` の並行処理があるので必ず通す）

- [ ] **Step 7: コミット**

```bash
git add internal/adapter/tui
git commit -m "feat(tui): 詳細をファイルから読み e で詳細ファイルを開く"
```

---

### Task 5: domain からインデント詳細のパースを削除

**Files:**
- Modify: `internal/domain/item.go`（`Item.Body` を削除）
- Modify: `internal/domain/tasklist.go`（`bodyAt` / `dedent` / `task.body` / `Add`）
- Modify: `internal/domain/tasklist_test.go`

**Interfaces:**
- Consumes: なし（このタスクの時点で `Item.Body` の参照は全部消えている）
- Produces: `Item` から `Body` フィールドが消える。`Parse` / `Render` / `Toggle` / `Add` / `Items` のシグネチャは変わらない

- [ ] **Step 1: テストを更新する**

`internal/domain/tasklist_test.go` から `TestParseCollectsBody` を**削除**する。

`TestParseNestedCheckboxIsTaskNotBody` から `Body` の検証を落として名前を変える:

```go
// インデントされたチェックボックスは独立したタスクとして数える。
func TestParseNestedCheckboxIsSeparateTask(t *testing.T) {
	src := "- [ ] 親\n  - [ ] 子\n"
	items := Parse(lines(src)).Items()
	if len(items) != 2 {
		t.Fatalf("Items の件数 = %d, want 2", len(items))
	}
}
```

`TestAddInsertsAfterLastTaskBody` を、新しい挿入規則に合わせて書き換える:

```go
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
```

`TestParseRenderRoundTripWithBody` は**残す**（名前だけ変える）。インデント行が原文のまま保たれることは引き続き守る不変条件:

```go
// インデント行はもう詳細として解釈しないが、原文のまま保持する。
func TestParseRenderRoundTripWithIndentedLines(t *testing.T) {
	got := strings.Join(Parse(lines(withBody)).Render(), "\n")
	if got != withBody {
		t.Errorf("round-trip が一致しない\n--- got:\n%q\n--- want:\n%q", got, withBody)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/domain/ -run 'TestAddInsertsAfterLastCheckboxLine' -v`
Expected: FAIL（`Add の挿入位置が違う`。今は `メモ2` の後ろに入る）

- [ ] **Step 3: `Item.Body` を削除する**

`internal/domain/item.go` の `Item` から `Body` フィールドの行を消す:

```go
type Item struct {
	ID    ID
	Kind  Kind
	Title string

	// KindTask のみ
	Done bool
	Tag  string // "@today" など

	// KindPR のみ
	Repo   string
	Number int
	URL    string
	Role   Role
}
```

- [ ] **Step 4: `bodyAt` / `dedent` / `task.body` を削除する**

`internal/domain/tasklist.go` から次を消す:

- `task` 構造体の `body []string` フィールドと、その上のコメント
- 関数 `bodyAt` とその doc コメント一式
- 関数 `dedent` とその doc コメント一式

`Parse` の `t.tasks = append(...)` から `body: bodyAt(lines, i),` の行を消す。

`Items` から `Body: dedent(tk.body),` の行を消す。

- [ ] **Step 5: `Add` の挿入位置を単純化する**

`internal/domain/tasklist.go` の `Add` の冒頭を置き換える:

```go
// Add は最後のチェックボックス行の直後に挿入する。末尾に追記すると
// "## メモ" のような後続セクションの下に紛れ込んでしまうため。
func (t TaskList) Add(title string) TaskList {
	at := -1
	if n := len(t.tasks); n > 0 {
		at = t.tasks[n-1].line
	}
	if at < 0 {
		// チェックボックスが無いなら最後の非空行の直後
		for i := len(t.lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(t.lines[i]) != "" {
				at = i
				break
			}
		}
	}
	// 以下は変更なし
```

`t.tasks` は `Parse` が行順に append するので、末尾の要素が最後のチェックボックス行。

- [ ] **Step 6: テストが通ることを確認する**

Run: `go test ./internal/domain/ -v`
Expected: PASS（`TestParseRenderRoundTrip` を含む全部）

- [ ] **Step 7: 全体のテストとビルド**

Run: `go build ./... && go test -race ./... && go vet ./...`
Expected: PASS。`Item.Body` の参照が残っていればここでコンパイルエラーになる

- [ ] **Step 8: コミット**

```bash
git add internal/domain
git commit -m "refactor(domain): インデント行の詳細パースを削除"
```

---

### Task 6: 既定の保存先を `~/.config/tk/` へ移す

**Files:**
- Modify: `main.go:16-25`（`tasksPath`）

**Interfaces:**
- Consumes: Task 2 で配線済みの `markdown.NewDetailStore(path)`
- Produces: なし（`main.go` は最上位）

- [ ] **Step 1: 既定パスを変える**

`main.go` の `tasksPath` を置き換える:

```go
// tasksPath は一覧ファイルのパス。詳細ファイルはここから導出するので、
// 環境変数は TK_TASKS_FILE の1つだけで済む。
// 既定を ~/.config/tk/ に置くのは、設定とデータを1か所にまとめるため。
// dotfiles リポジトリで ~/.config を管理している場合は .gitignore が要る。
func tasksPath() (string, error) {
	if p := os.Getenv("TK_TASKS_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tk", "tasks.md"), nil
}
```

- [ ] **Step 2: ビルドとテストを確認する**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 3: 手元で動作を確認する**

Run:

```bash
rm -rf /tmp/tk-manual && mkdir -p /tmp/tk-manual
printf -- '- [ ] テストタスク @today\n- [ ] 詳細なし\n' > /tmp/tk-manual/tasks.md
TK_TASKS_FILE=/tmp/tk-manual/tasks.md TK_EDITOR=vi go run .
```

確認すること:

1. 起動して「テストタスク」が一覧に出る
2. `e` を押すと `vi` が空のファイルを開く。何か書いて `:wq` する
3. 右ペインに書いた内容が出る
4. `ls /tmp/tk-manual/tasks/` に `テストタスク.md` がある
5. `j` で「詳細なし」に移ると右ペインの本文が空になる
6. `k` で戻すと本文が再び出る（キャッシュしていないので毎回読み直す）
7. `q` で終了

- [ ] **Step 4: 後片付けとコミット**

```bash
rm -rf /tmp/tk-manual
git add main.go
git commit -m "feat: タスクの既定の保存先を ~/.config/tk/ へ移す"
```

---

### Task 7: ドキュメントを更新する

**Files:**
- Modify: `README.md`
- Modify: `docs/known-issues.md`

**Interfaces:**
- Consumes: Task 1〜6 の最終形
- Produces: なし

- [ ] **Step 1: README の冒頭とキー表を直す**

`README.md` の4行目を置き換える:

```markdown
タスクは `~/.config/tk/tasks.md`（Markdown のチェックボックス）に保存する。各タスクの詳細は `~/.config/tk/tasks/<タイトル>.md` に1ファイルずつ置く。どちらも Neovim で直接編集してよい。GitHub へのアクセスは `gh` CLI をサブプロセスで叩く。認証情報は tk 自身が一切扱わない。
```

キー表の `e` の行を置き換える:

```markdown
| `e` | 選択中タスクの詳細ファイルを `$EDITOR` で開く（タスクペインのみ）。閉じると `tasks.md` を読み直す |
```

環境変数の表の `TK_TASKS_FILE` の行を置き換える:

```markdown
| `TK_TASKS_FILE` | `~/.config/tk/tasks.md` | タスク一覧の保存先。詳細ディレクトリはここから導出する（`tasks.md` → `tasks/`） |
```

開発コマンドの1行を置き換える:

```sh
TK_TASKS_FILE=/tmp/t.md go run .   # 一覧も詳細も /tmp/t/ に隔離して試す
```

- [ ] **Step 2: README の「タスクの詳細」節を書き換える**

`## タスクの詳細` 節の本文（`チェックボックス行の直後に続くインデント行が…` から `…それ以外のエディタでは効かない。` まで）を、次で丸ごと置き換える:

````markdown
詳細はタスクごとの独立したファイルに置く。ファイル名がタスクのタイトルそのもの。

```
~/.config/tk/
  tasks.md                          一覧（順番・完了・タグ）
  tasks/
    認証まわりのリファクタ.md          詳細（本文だけ）
```

`tasks.md`:

```markdown
- [ ] 認証まわりのリファクタ @today
- [x] 別のタスク
```

`tasks/認証まわりのリファクタ.md`:

```markdown
Cookie の SameSite を Lax に

RFC を読み直すこと
```

詳細ファイルの中身に tk が課す規則はない。frontmatter も要らない。書いた Markdown がそのまま右ペインに出る。

書くのは `e`（そのタスクの詳細ファイルが `$EDITOR` で開く）か、直接ファイルを編集する。**tk 自身は詳細を書き込まない。** ファイルが無ければエディタが新規作成する。tk がやるのは `tasks/` ディレクトリを作ることだけ。

いくつか決めごとがある。

- ファイル名はタイトルから作る。`/` は `-` に、先頭の `.` は `-` に置換し、255 バイトで切り詰める。**タグは含めない**ので、タグを付け替えても詳細は迷子にならない
- 同じタイトルのタスクが2つあれば、同じ詳細ファイルを見る
- **tk は詳細ファイルを削除しない。** `tasks.md` から行を消しても詳細は残る。「完了して消した」のか「一時的にコメントアウトした」のかを区別できないので、書いたメモを勝手に消さない。溜まったら手で消す
- 詳細はキャッシュしない。カーソルが動くたびに読み直すので、エディタで書き換えれば次に選んだ瞬間に反映される
````

- [ ] **Step 3: README に移行手順の節を足す**

`## タスクの詳細` 節の直後に足す:

````markdown
## 0.x からの移行

保存先が `~/tasks.md` から `~/.config/tk/tasks.md` に変わった。自動移行はしないので手で移す。

```sh
mkdir -p ~/.config/tk
mv ~/tasks.md ~/.config/tk/tasks.md
```

`~/tasks.md` を使い続けたい場合は `TK_TASKS_FILE=~/tasks.md` を指定する。

以前はチェックボックス行の直後のインデント行が詳細だった。この解釈はやめたので、既存のインデント行は右ペインに出なくなる（`tasks.md` からは消えない。tk は非チェックボックス行を原文のまま保つ）。右ペインに出したければ `~/.config/tk/tasks/<タイトル>.md` に手で移す。

`~/.config` を dotfiles リポジトリで管理している場合、タスクの中身がリポジトリに入る。`.gitignore` に `tk/` を足しておくこと。
````

- [ ] **Step 4: README の層の表を直す**

`internal/adapter/markdown` の行を置き換える:

```markdown
| `internal/adapter/markdown` | `tasks.md` の読み書き（tmp + `os.Rename` で atomic）と、詳細ファイルの読み込み | |
```

「変更するときに踏み外しやすい点」の `tasks.md` の項目の直後に1項目足す:

```markdown
- **詳細ファイルは tk が書かない。** 読むのと親ディレクトリの `mkdir` だけ。書き込みを足すと、エディタで開いている最中の外部変更と競合する。`Store` が `mtime` / `size` で守っているのと同じ問題を、詳細ファイルぶん抱え込むことになる。
```

- [ ] **Step 5: known-issues に設計判断を追記する**

`docs/known-issues.md` の「意識的に採用した設計判断」の表に3行足す:

```markdown
| タスクの詳細は `tasks.md` の中ではなく1タスク1ファイル | インデント行を詳細とする書式は、覚えていないと拾われず「エディタで書いたのに出ない」が起きた。ファイル名 = タイトルなら行がずれても対応が壊れない |
| 完了状態を SQLite で持たない | Neovim でタスクを完了にできなくなる。ファイルを外でリネーム・削除すると DB 行が孤児になり、起動のたびに突き合わせが要る。完了日時や履歴が必要になったら、**ファイルから再生成できるインデックス**として足す |
| tk は詳細ファイルを削除しない | `tasks.md` から行が消えたのが「完了して消した」のか「一時的にコメントアウトした」のか区別できない。書いたメモを勝手に消すほうが害が大きい |
```

「使ってみて気になったら考えること」に2項目足す:

```markdown
- **同じタイトルのタスクが2つあると同じ詳細ファイルを見る。** 区別するために ID やサフィックスを入れると、ファイル名から人が読めなくなる。実害が出たら考える
- **`Add` は最後のチェックボックス行の直後に入れる。** その下にインデントされた自由記述があると、その上に割り込む形になる。詳細が別ファイルになった今、インデント行を避ける必要は無くなったのでこの形にした
```

- [ ] **Step 6: ドキュメント一覧に本 spec を足す**

`README.md` の `### ドキュメント` の箇条書きに1行足す（`2026-08-13-tk-task-detail-design.md` の直後）:

```markdown
- [docs/superpowers/specs/2026-08-15-tk-task-files-design.md](docs/superpowers/specs/2026-08-15-tk-task-files-design.md) — 詳細を1タスク1ファイルに分離。SQLite を採らなかった理由
```

- [ ] **Step 7: リンク切れがないか確認する**

Run: `grep -o '](docs/[^)]*)' README.md | tr -d '](' | sed 's/)$//' | xargs ls`
Expected: すべてのパスが存在する（エラー出力なし）

- [ ] **Step 8: 最終確認**

Run: `go build ./... && go test -race ./... && go vet ./...`
Expected: PASS

- [ ] **Step 9: コミット**

```bash
git add README.md docs/known-issues.md
git commit -m "docs: 詳細ファイル方式と ~/.config/tk への移行手順を反映"
```

---

## 完了条件

- `go test -race ./...` と `go vet ./...` が通る
- `~/.config/tk/tasks.md` が既定の保存先になっている
- `e` でそのタスクの詳細ファイルが開き、書いた内容が右ペインに出る
- `grep -rn "bodyAt\|dedent\|Item.Body\|\.Body\b" internal/domain/` が何も返さない
- `go.mod` の `require` ブロックに新しい直接依存が増えていない
