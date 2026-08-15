package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/k-narusawa/tk/internal/domain"
	"github.com/k-narusawa/tk/internal/usecase"
)

type fakeStore struct {
	list      domain.TaskList
	saved     []domain.TaskList
	saveErr   error
	loadCalls int
}

func (f *fakeStore) Load() (domain.TaskList, error) {
	f.loadCalls++
	return f.list, nil
}

func (f *fakeStore) Save(t domain.TaskList) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, t)
	f.list = t
	return nil
}

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

// fakePRs は role ごとに1件返す。r キーや起動時の非同期取り込みを
// 実プロセスなしで検証するためのもの。
type fakePRs struct {
	items map[domain.Role]domain.Item
	err   error
}

func (f *fakePRs) Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []domain.Item{f.items[role]}, nil
}

// fakeDetails は usecase.PRDetailSource を満たす。Detail() の戻り値を固定できる。
type fakeDetails struct {
	detail domain.PRDetail
	err    error
	calls  int
}

func (f *fakeDetails) Detail(ctx context.Context, repo string, number int) (domain.PRDetail, error) {
	f.calls++
	return f.detail, f.err
}

func taskList(s string) domain.TaskList { return domain.Parse(strings.Split(s, "\n")) }

// prPair は review/mine 双方に1件ずつ PR を持つ fakePRs を作る。
// SortPRs の並びでは review が先に来るので items[0] が review 側になる。
func prPair() *fakePRs {
	return &fakePRs{items: map[domain.Role]domain.Item{
		domain.RoleReview: {ID: domain.PRID("a/x", 1), Kind: domain.KindPR, Repo: "a/x", Number: 1, Title: "fix", Role: domain.RoleReview},
		domain.RoleMine:   {ID: domain.PRID("a/y", 2), Kind: domain.KindPR, Repo: "a/y", Number: 2, Title: "feat", Role: domain.RoleMine},
	}}
}

func newTestModel(t *testing.T, store *fakeStore) Model {
	t.Helper()
	inbox := usecase.NewInbox(store, nil, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return New(inbox, Config{AICmd: "claude"})
}

func TestWindowSizeRendersNonEmptyView(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	v := m.View()
	if v.Content == "" {
		t.Error("View().Content が空")
	}
}

func TestJKMovesCursor(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] 一\n- [ ] 二\n- [ ] 三\n")})

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Fatalf("j 後の cursor = %d, want 1", m.cursor)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = got.(Model)
	if m.cursor != 2 {
		t.Fatalf("j 後の cursor = %d, want 2", m.cursor)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Fatalf("k 後の cursor = %d, want 1", m.cursor)
	}
}

func TestSpaceTogglesTask(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)

	if len(store.saved) == 0 {
		t.Fatal("Save が呼ばれていない")
	}
	last := store.saved[len(store.saved)-1]
	rendered := strings.Join(last.Render(), "\n")
	if !strings.Contains(rendered, "- [x]") {
		t.Errorf("保存内容に - [x] が無い: %q", rendered)
	}
}

func TestNAddsTaskViaTextinput(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)
	if !m.adding {
		t.Fatal("n 後に adding が true になっていない")
	}

	for _, r := range "新規タスク" {
		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = got.(Model)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = got.(Model)

	if m.adding {
		t.Error("enter 後も adding が true のまま")
	}
	if len(store.saved) == 0 {
		t.Fatal("Save が呼ばれていない")
	}
	rendered := strings.Join(store.saved[len(store.saved)-1].Render(), "\n")
	if !strings.Contains(rendered, "新規タスク") {
		t.Errorf("保存内容に新規タスクが無い: %q", rendered)
	}
}

// 空白だけのタイトルは、余計な "- [ ] " 行を作るので保存しない。
func TestNRejectsWhitespaceOnlyTitle(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)

	for _, r := range "   " {
		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = got.(Model)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = got.(Model)

	if m.adding {
		t.Error("enter 後も adding が true のまま")
	}
	if len(store.saved) != 0 {
		t.Errorf("空白のみのタイトルで Save が呼ばれた: %+v", store.saved)
	}
}

// 起動直後、最初の WindowSizeMsg が来る前に bubbletea は View() を呼ぶ。
// このとき width/height は 0 で、right = width-left は 0 になる。
func TestViewBeforeWindowSize(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	if m.width != 0 || m.height != 0 {
		t.Fatalf("前提が崩れている: width=%d height=%d", m.width, m.height)
	}
	v := m.View() // panic しないこと
	if v.Content == "" {
		t.Error("View().Content が空")
	}
}

// 極端に小さい端末でも落ちないこと
func TestViewTinyTerminal(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	for _, size := range [][2]int{{1, 1}, {2, 2}, {5, 3}, {0, 30}, {100, 0}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		if v := mm.View(); v.Content == "" {
			t.Errorf("size %v で View().Content が空", size)
		}
	}
}

func TestQReturnsQuitCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if cmd == nil {
		t.Fatal("q で cmd が nil")
	}
	if msg := cmd(); msg == nil {
		t.Error("q の cmd を実行した結果が nil")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("q の cmd を実行した結果 = %T, want tea.QuitMsg", msg)
	}
}

// Init() が返す cmd を実行して PR が一覧に反映されることを確認する。
// TTY が無い環境では実際に起動して確認できないので、これがその代わり。
func TestInitFetchesPRsAsync(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	prs := &fakePRs{items: map[domain.Role]domain.Item{
		domain.RoleReview: {ID: domain.PRID("a/x", 1), Kind: domain.KindPR, Repo: "a/x", Number: 1, Role: domain.RoleReview},
		domain.RoleMine:   {ID: domain.PRID("a/y", 2), Kind: domain.KindPR, Repo: "a/y", Number: 2, Role: domain.RoleMine},
	}}
	inbox := usecase.NewInbox(store, prs, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() の cmd が nil")
	}

	msg := cmd()
	loaded, ok := msg.(prLoadedMsg)
	if !ok {
		t.Fatalf("Init() の cmd が返したのは %T, want prLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("prLoadedMsg.err = %v, want nil", loaded.err)
	}

	got, _ := m.Update(loaded)
	m = got.(Model)

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)

	found := false
	for _, it := range m.items {
		if it.ID == domain.PRID("a/x", 1) {
			found = true
		}
	}
	if !found {
		t.Errorf("PR がリストに反映されていない: %+v", m.items)
	}
}

// PR 取得の失敗はエラー表示に留まり、タスクの一覧には影響しないこと。
func TestInitPRFailureKeepsTasksIntact(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	wantErr := errors.New("gh: not logged in")
	inbox := usecase.NewInbox(store, &fakePRs{err: wantErr}, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	msg := m.Init()()
	loaded, ok := msg.(prLoadedMsg)
	if !ok {
		t.Fatalf("Init() の cmd が返したのは %T, want prLoadedMsg", msg)
	}
	if loaded.err == nil {
		t.Fatal("prLoadedMsg.err が nil, want error")
	}

	got, _ := m.Update(loaded)
	m = got.(Model)

	if m.errMsg == "" {
		t.Error("PR 取得失敗が errMsg に反映されていない")
	}

	found := false
	for _, it := range m.items {
		if it.Kind == domain.KindTask && it.Title == "やること" {
			found = true
		}
	}
	if !found {
		t.Error("PR 取得失敗でタスクが失われた")
	}
}

// 保存失敗のエラーは、その後の PR 取得成功で消えてはいけない。
// 別々の原因のエラーを取得成功が一括で握り潰す、が今回の実バグ。
func TestSuccessfulPRRefreshKeepsSaveError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	inbox := usecase.NewInbox(store, &fakePRs{}, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("保存失敗が errMsg に反映されていない")
	}
	saveErrMsg := m.errMsg

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)

	if m.errMsg != saveErrMsg {
		t.Errorf("PR 取得成功で保存エラーが消えた: errMsg = %q, want %q", m.errMsg, saveErrMsg)
	}
}

// PR 取得エラー自体は、次の取得成功で消えること。
func TestSuccessfulPRRefreshClearsPRError(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, _ := m.Update(prLoadedMsg{err: errors.New("gh: not logged in")})
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("PR 取得失敗が errMsg に反映されていない")
	}

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)

	if m.errMsg != "" {
		t.Errorf("PR 取得成功後も errMsg が残っている: %q", m.errMsg)
	}
}

// タスクにカーソルが乗っているときは detailCmd() が何もしないこと。
func TestDetailCmdNilForTaskItem(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	it, ok := m.selected()
	if !ok || it.Kind != domain.KindTask {
		t.Fatalf("selected() = %+v, %v, want タスク", it, ok)
	}
	if cmd := m.detailCmd(); cmd != nil {
		t.Error("detailCmd() がタスク選択中に nil を返さなかった")
	}
}

// PR にカーソルを合わせたら detailCmd() が取得コマンドを返し、実行すると
// detailLoadedMsg が届く。それを Update に渡すと View() に CI 状態が出る。
func TestDetailCmdFetchesSelectedPRAndUpdatesView(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	details := &fakeDetails{detail: domain.PRDetail{CI: "passing", Additions: 10, Deletions: 2, ChangedFiles: 3}}
	m := prModelWith(t, store, details, "claude")

	it, ok := m.selected()
	if !ok || it.Kind != domain.KindPR {
		t.Fatalf("selected() = %+v, %v, want PR", it, ok)
	}

	cmd := m.detailCmd()
	if cmd == nil {
		t.Fatal("detailCmd() が nil、PR 選択中なのに取得しない")
	}

	msg := cmd()
	loaded, ok := msg.(detailLoadedMsg)
	if !ok {
		t.Fatalf("detailCmd() の結果は %T, want detailLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("detailLoadedMsg.err = %v, want nil", loaded.err)
	}
	if loaded.detail.CI != "passing" {
		t.Errorf("detailLoadedMsg.detail.CI = %q, want passing", loaded.detail.CI)
	}

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	got, _ = m.Update(loaded)
	m = got.(Model)

	if content := m.View().Content; !strings.Contains(content, "passing") {
		t.Errorf("View().Content に CI 状態が出ていない: %q", content)
	}
}

// 同じ PR が m.details にすでにあるなら detailCmd() は再取得しない。
// カーソルを動かすたびに gh を叩き直さないための、いちばん重要な保証。
func TestDetailCmdNilWhenAlreadyFetched(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")

	it, ok := m.selected()
	if !ok || it.Kind != domain.KindPR {
		t.Fatalf("selected() = %+v, %v, want PR", it, ok)
	}
	m.details[it.ID] = detailEntry{detail: domain.PRDetail{CI: "passing"}}

	if cmd := m.detailCmd(); cmd != nil {
		t.Error("detailCmd() が取得済みの PR に対して nil を返さなかった")
	}
}

// PRDetailSource が失敗したら errMsg に反映されるが、一覧は失われないこと。
func TestDetailLoadedMsgErrorKeepsItemsIntact(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	wantErr := errors.New("gh: rate limited")
	m := prModelWith(t, store, &fakeDetails{err: wantErr}, "claude")
	wantItemCount := len(m.items)
	// GitHub ペインなので、prModelWith が選択済みの PR がそのままカーソルにある。

	cmd := m.detailCmd()
	if cmd == nil {
		t.Fatal("detailCmd() が nil")
	}
	loaded, ok := cmd().(detailLoadedMsg)
	if !ok {
		t.Fatal("detailCmd() の結果が detailLoadedMsg でない")
	}
	if loaded.err == nil {
		t.Fatal("detailLoadedMsg.err が nil, want error")
	}

	got, _ := m.Update(loaded)
	m = got.(Model)

	if m.errMsg == "" {
		t.Error("PR 詳細取得の失敗が errMsg に反映されていない")
	}
	if len(m.items) != wantItemCount {
		t.Errorf("詳細取得の失敗で一覧が変わった: len = %d, want %d", len(m.items), wantItemCount)
	}
}

// 詳細ペインは「未取得 / 取得成功 / 取得失敗」の3状態を区別する。
// 取得失敗時に「取得中」のまま固まってはいけない。
func TestDetailPaneThreeStates(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	if len(m.items) < 2 || m.items[0].Kind != domain.KindPR || m.items[1].Kind != domain.KindPR {
		t.Fatalf("前提が崩れている: PR が2件必要 items = %+v", m.items)
	}
	first, second := m.items[0], m.items[1]

	// 未取得
	if content := m.View().Content; !strings.Contains(content, "取得中") {
		t.Errorf("未取得なのに「取得中」が出ていない: %q", content)
	}

	// 取得成功
	got, _ = m.Update(detailLoadedMsg{id: first.ID, detail: domain.PRDetail{CI: "passing"}})
	m = got.(Model)
	content := m.View().Content
	if strings.Contains(content, "取得中") {
		t.Errorf("成功後も「取得中」が残っている: %q", content)
	}
	if !strings.Contains(content, "passing") {
		t.Errorf("成功後に CI 状態が出ていない: %q", content)
	}

	// カーソルを未取得の2件目に移す
	m.cursor = 1
	m.syncDetail()
	if content := m.View().Content; !strings.Contains(content, "取得中") {
		t.Errorf("未取得の2件目で「取得中」が出ていない: %q", content)
	}

	// 取得失敗
	got, _ = m.Update(detailLoadedMsg{id: second.ID, err: errors.New("gh: rate limited")})
	m = got.(Model)
	content = m.View().Content
	if strings.Contains(content, "取得中") {
		t.Errorf("失敗後も「取得中」が残っている: %q", content)
	}
	if !strings.Contains(content, "取得できませんでした") {
		t.Errorf("失敗が詳細ペインに反映されていない: %q", content)
	}
}

// 成功したのに CI・レビュー・差分ファイル数がすべて空/ゼロの PRDetail は、
// 旧ロジック（フィールドの空っぽさで「未取得」を推測する）だと「取得中」に
// 誤判定される。ロードの成否そのもので判定しなければならない。
func TestDetailPaneZeroValueSuccessIsNotPending(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	it, ok := m.selected()
	if !ok || it.Kind != domain.KindPR {
		t.Fatalf("selected() = %+v, %v, want PR", it, ok)
	}

	got, _ = m.Update(detailLoadedMsg{id: it.ID, detail: domain.PRDetail{}})
	m = got.(Model)

	if content := m.View().Content; strings.Contains(content, "取得中") {
		t.Errorf("全フィールドゼロの成功レスポンスが「取得中」と誤判定された: %q", content)
	}
}

// space によるトグルでタスクが並び順を変える（完了タスクは末尾に回る）とき、
// カーソルは同位置に留まり繰り上がった次のタスクを指す（ユーザー承認済みの挙動）。
// 詳細ペインはその「今カーソルが指しているタスク」に追従しなければならない。
func TestSpaceKeepsDetailPaneInSyncWithCursor(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 一つ目\n- [ ] 二つ目\n")}
	m := newTestModel(t, store)
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)

	it, ok := m.selected()
	if !ok || it.Title != "二つ目" {
		t.Fatalf("トグル後のカーソル位置 = %+v, %v, want 二つ目", it, ok)
	}
	// m.View().Content には左の一覧も含まれ、そちらには常に両方のタイトルが
	// 出るため、詳細ペイン単体（m.detail.View()）で確認する。
	content := m.detail.View()
	if !strings.Contains(content, "二つ目") {
		t.Errorf("詳細ペインがカーソルに追従していない（二つ目が出ていない）: %q", content)
	}
	if strings.Contains(content, "一つ目") {
		t.Errorf("詳細ペインに完了させた一つ目がまだ表示されている: %q", content)
	}
}

// n → enter でタスクを追加したとき、未完了タスクは並び順で先頭に来る。
// 元々カーソル0が完了済みタスクを指していた場合、追加後は新規タスクを
// 指すことになるので、詳細ペインもそれに追従しなければならない。
func TestAddKeepsDetailPaneInSyncWithCursor(t *testing.T) {
	store := &fakeStore{list: taskList("- [x] 完了済み\n")}
	m := newTestModel(t, store)
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	it, ok := m.selected()
	if !ok || it.Title != "完了済み" {
		t.Fatalf("前提が崩れている: selected() = %+v, %v", it, ok)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)
	for _, r := range "新規" {
		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = got.(Model)
	}
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = got.(Model)

	it, ok = m.selected()
	if !ok || it.Title != "新規" {
		t.Fatalf("追加後のカーソル位置 = %+v, %v, want 新規", it, ok)
	}
	// m.View().Content には左の一覧も含まれ、そちらには常に両方のタイトルが
	// 出るため、詳細ペイン単体（m.detail.View()）で確認する。
	content := m.detail.View()
	if !strings.Contains(content, "新規") {
		t.Errorf("詳細ペインがカーソルに追従していない（新規が出ていない）: %q", content)
	}
	if strings.Contains(content, "完了済み") {
		t.Errorf("詳細ペインに古い選択（完了済み）がまだ表示されている: %q", content)
	}
}

// 左カラム（2枠の合計）と右の詳細ペインの高さが一致すること。ズレると
// 枠の下端が食い違って端末で一目で分かる。
//
// レンダリング結果から枠の閉じ位置を探す形では検出できない。左カラムが
// 伸びた場合、最終行は「左の下端 ++ 右の下端」のままで、どちらの目印も
// 同じ行に居続けるため。枠を直接測る。
func TestPanesHaveEqualHeight(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "- [ ] item%d\n", i)
	}
	m := prModelWith(t, &fakeStore{list: taskList(b.String())}, &fakeDetails{}, "claude")

	for _, size := range [][2]int{{100, 30}, {80, 24}, {120, 40}, {60, 15}} {
		for _, focus := range []paneID{paneTasks, paneGitHub} {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			mm := updated.(Model)
			if mm.focus != focus {
				got, _ := mm.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
				mm = got.(Model)
			}

			l := newLayout(size[0], size[1])
			left := lipgloss.Height(mm.paneView(l, paneTasks)) + lipgloss.Height(mm.paneView(l, paneGitHub))
			right := lipgloss.Height(mm.detailView(l))
			if left != right {
				t.Errorf("size %v focus=%d: 左カラム %d 行, 右ペイン %d 行", size, focus, left, right)
			}

			for i, ln := range strings.Split(mm.View().Content, "\n") {
				if w := lipgloss.Width(ln); w > size[0] {
					t.Errorf("size %v: %d行目が端末幅を超えている（幅=%d, want <=%d）: %q",
						size, i, w, size[0], ansi.Strip(ln))
				}
			}
		}
	}
}

// 一覧が端末に収まらない件数でも、レンダリング全体の行数が端末の高さを
// 超えないこと。超えるとフッタ（エラー表示）が画面外に押し出される。
func TestListDoesNotOverflowTerminalHeight(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "- [ ] item%d\n", i)
	}
	m := newTestModel(t, &fakeStore{list: taskList(b.String())})

	for _, size := range [][2]int{{100, 20}, {80, 24}, {60, 15}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		lines := strings.Split(mm.View().Content, "\n")
		if len(lines) > size[1] {
			t.Errorf("size %v: レンダリング行数 = %d, want <= %d", size, len(lines), size[1])
		}
	}
}

// 一覧が窓に収まらない件数のとき、カーソルを下まで送るとその行が
// レンダリングに含まれ続けること（窓が追従してスクロールする）。
func TestListScrollsCursorIntoView(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "- [ ] item%d\n", i)
	}
	m := newTestModel(t, &fakeStore{list: taskList(b.String())})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	mm := updated.(Model)

	for i := 0; i < 39; i++ {
		got, _ := mm.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
		mm = got.(Model)
	}
	if mm.cursor != 39 {
		t.Fatalf("cursor = %d, want 39", mm.cursor)
	}
	if !strings.Contains(mm.View().Content, "item39") {
		t.Errorf("窓が追従せず、末尾のカーソル行が表示に含まれていない: %q", mm.View().Content)
	}
}

// ctrl+d / ctrl+u は詳細ペイン（bubbles/viewport）への半ページ送りを
// そのまま委譲するだけの1行だが、その委譲自体にテストが無かった。
func TestCtrlDCtrlUScrollDetailPane(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = got.(Model)

	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	m.detail.SetContent(b.String())
	if m.detail.YOffset() != 0 {
		t.Fatalf("前提が崩れている: YOffset = %d, want 0", m.detail.YOffset())
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	m = got.(Model)
	afterDown := m.detail.YOffset()
	if afterDown <= 0 {
		t.Errorf("ctrl+d 後の YOffset = %d, want > 0", afterDown)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	m = got.(Model)
	if afterUp := m.detail.YOffset(); afterUp >= afterDown {
		t.Errorf("ctrl+u 後の YOffset = %d, want < %d（ctrl+d 直後）", afterUp, afterDown)
	}
}

// gh の 401 のような長いエラー（93桁前後、JSON を含む）がフッターに出ても
// 端末幅を超えないこと。help の固定文言よりこちらの方が実際に長くなる。
func TestLongErrorInFooterDoesNotOverflow(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	longErr := errors.New(`gh: HTTP 401: Bad credentials (https://api.github.com/graphql) {"message":"Bad credentials","documentation_url":"https://docs.github.com/graphql"}`)

	got, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	m = got.(Model)
	got, _ = m.Update(prLoadedMsg{err: longErr})
	m = got.(Model)

	for i, ln := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(ln); w > 60 {
			t.Errorf("%d行目が端末幅を超えている（幅=%d, want <=60）: %q", i, w, ln)
		}
	}
}

// prModelWith は PR を取得済みで GitHub ペインにフォーカスした Model を作る。
// New() はタスクペインから始まるので `l` を通す。取得完了を Model に伝える
// のは prLoadedMsg なので、実際の起動と同じくそれも流す。
func prModelWith(t *testing.T, store *fakeStore, details *fakeDetails, aiCmd string) Model {
	t.Helper()
	inbox := usecase.NewInbox(store, prPair(), details, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: aiCmd})
	got, _ := m.Update(prLoadedMsg{})
	m = got.(Model)
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	it, ok := m.selected()
	if !ok || it.Kind != domain.KindPR {
		t.Fatalf("前提が崩れている: selected() = %+v, %v, want PR", it, ok)
	}
	return m
}

func prModel(t *testing.T, aiCmd string) Model {
	t.Helper()
	return prModelWith(t, &fakeStore{list: taskList("")}, &fakeDetails{}, aiCmd)
}

func TestHLKeysMoveFocus(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	if m.focus != paneTasks {
		t.Fatalf("起動時の focus = %d, want paneTasks", m.focus)
	}
	for _, it := range m.items {
		if it.Kind != domain.KindTask {
			t.Errorf("タスクペインに PR が出ている: %+v", it)
		}
	}

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	if m.focus != paneGitHub {
		t.Fatalf("l の後の focus = %d, want paneGitHub", m.focus)
	}
	if len(m.items) != 2 {
		t.Fatalf("GitHub ペインの件数 = %d, want 2", len(m.items))
	}
	for _, it := range m.items {
		if it.Kind != domain.KindPR {
			t.Errorf("GitHub ペインにタスクが出ている: %+v", it)
		}
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	if m.focus != paneTasks {
		t.Fatalf("h の後の focus = %d, want paneTasks", m.focus)
	}
	if len(m.items) != 1 {
		t.Fatalf("タスクペインの件数 = %d, want 1", len(m.items))
	}
}

// ペインは2つなので h も l も「もう一方」へ移る。押し続けても
// どちらかに張り付かないこと。
func TestPaneFocusWrapsWithBothKeys(t *testing.T) {
	for _, key := range []rune{'h', 'l'} {
		m := newTestModel(t, &fakeStore{list: taskList("- [ ] 一\n")})

		got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: key, Text: string(key)}))
		m = got.(Model)
		if m.focus != paneGitHub {
			t.Errorf("%c の後の focus = %d, want paneGitHub", key, m.focus)
		}

		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: key, Text: string(key)}))
		m = got.(Model)
		if m.focus != paneTasks {
			t.Errorf("%c を2回押した後の focus = %d, want paneTasks", key, m.focus)
		}
	}
}

// カーソル位置はペインごとに保たれる。往復しても元の行に戻ること。
func TestCursorIsPerPane(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 一\n- [ ] 二\n- [ ] 三\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	// タスクペインで3行目へ
	for i := 0; i < 2; i++ {
		got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
		m = got.(Model)
	}
	if m.cursor != 2 {
		t.Fatalf("タスクペインの cursor = %d, want 2", m.cursor)
	}

	// GitHub ペインへ移ると先頭から始まる
	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	if m.cursor != 0 {
		t.Fatalf("GitHub ペイン初回の cursor = %d, want 0", m.cursor)
	}

	// GitHub ペインで2行目へ
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Fatalf("GitHub ペインの cursor = %d, want 1", m.cursor)
	}

	// タスクペインに戻ると3行目
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	if m.cursor != 2 {
		t.Errorf("タスクペインに戻った cursor = %d, want 2", m.cursor)
	}

	// もう一度 GitHub ペインへ行くと2行目
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Errorf("GitHub ペインに戻った cursor = %d, want 1", m.cursor)
	}
}

// 保持していたカーソル位置が、戻ってきたときに件数を超えていても
// 落ちないこと（PR が減る、タスクが消えるのは普通に起きる）。
func TestPerPaneCursorClampsWhenListShrinks(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 一\n- [ ] 二\n- [ ] 三\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	for i := 0; i < 2; i++ {
		got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
		m = got.(Model)
	}
	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)

	// 裏でタスクが1件に減る
	store.list = taskList("- [ ] 一\n")
	if err := m.inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	if m.cursor != 0 {
		t.Errorf("縮んだ一覧に戻った cursor = %d, want 0", m.cursor)
	}
	if _, ok := m.selected(); !ok {
		t.Error("selected() が false、カーソルが範囲外のまま")
	}
}

// A に渡るのはフォーカス中のペインのアイテムだけ。aiExec は m.items をそのまま
// 渡すので、m.items がフォーカス中ペインに閉じていることで担保する。
func TestShiftAScopeIsFocusedPane(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")

	for _, it := range m.items {
		if it.Kind != domain.KindPR {
			t.Errorf("GitHub ペインの m.items にタスクが混ざっている: %+v", it)
		}
	}

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A"}))
	if cmd == nil {
		t.Error("GitHub ペインの A が cmd を返さない")
	}
}

// フォーカスを移したら右ペインが新しい選択に追従すること。
// 追従しないと、タスクの詳細を出したまま PR 一覧を見ることになる。
func TestFocusMoveKeepsDetailPaneInSync(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	if !strings.Contains(m.detail.View(), "やること") {
		t.Fatalf("タスクペインの右ペインにタスクが出ていない: %q", m.detail.View())
	}

	got, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	if !strings.Contains(m.detail.View(), "fix") {
		t.Errorf("GitHub ペインに切り替えても右ペインがタスクのまま: %q", m.detail.View())
	}
	if cmd == nil {
		t.Error("PR を選択したのに詳細取得の cmd が返っていない")
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	if !strings.Contains(m.detail.View(), "やること") {
		t.Errorf("タスクペインに戻っても右ペインが PR のまま: %q", m.detail.View())
	}
}

// d/enter が返す Cmd は tea.ExecProcess で実プロセスを起動するので、
// ここでは cmd() を呼ばず nil かどうかだけを見る。
func TestDKeyOnPRReturnsCmd(t *testing.T) {
	m := prModel(t, "claude")
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	if cmd == nil {
		t.Fatal("PR で d を押しても cmd が nil")
	}
}

func TestDKeyOnTaskReturnsNil(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	if cmd != nil {
		t.Error("タスクで d を押したのに cmd が nil でない")
	}
}

func TestEnterKeyOnPRReturnsCmd(t *testing.T) {
	m := prModel(t, "claude")
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("PR で enter を押しても cmd が nil")
	}
}

func TestEnterKeyOnTaskReturnsNil(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Error("タスクで enter を押したのに cmd が nil でない")
	}
}

func TestAKeyReturnsCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	if cmd == nil {
		t.Fatal("a で cmd が nil")
	}
}

// TK_AI_CMD が空でも、a は cmd を返してエラーを表に出さなければならない
// （黙って何もしないのはダメ）。この cmd は実プロセスを起動する前に
// エラーで返るので、唯一実行して確認できるケース。
func TestAKeyWithEmptyAICmdSurfacesError(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	m.cfg.AICmd = ""

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	if cmd == nil {
		t.Fatal("aiCmd が空でも cmd が nil であってはならない（エラーを出せない）")
	}

	msg := cmd()
	done, ok := msg.(execDoneMsg)
	if !ok {
		t.Fatalf("cmd() が返したのは %T, want execDoneMsg", msg)
	}
	if done.err == nil {
		t.Error("aiCmd が空なのに execDoneMsg.err が nil")
	}
}

func TestShiftAKeyReturnsCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A"}))
	if cmd == nil {
		t.Fatal("A で cmd が nil")
	}
}

// 外部プロセスのエラーは errIsPR=false で記録され、PR 取得成功で
// 消えてはいけない。保存失敗のエラーを取得成功が握り潰していた過去のバグ
// (TestSuccessfulPRRefreshKeepsSaveError) と同じ形の回帰を防ぐ。
func TestSuccessfulPRRefreshKeepsExecError(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, _ := m.Update(execDoneMsg{err: errors.New("exit status 1")})
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("外部プロセスのエラーが errMsg に反映されていない")
	}
	if m.errIsPR {
		t.Error("外部プロセスのエラーなのに errIsPR が true")
	}
	execErrMsg := m.errMsg

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)

	if m.errMsg != execErrMsg {
		t.Errorf("PR 取得成功で外部プロセスのエラーが消えた: errMsg = %q, want %q", m.errMsg, execErrMsg)
	}
}

// R キーは Load() を経由してファイルを読み直す。外部エディタでの変更を
// 取り込むための再読み込みキー。カーソルが PR を指す状態にして、返る cmd
// （m.detailCmd()）が nil にならないケースで検証する。
func TestShiftRReReadsThroughStore(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	before := store.loadCalls

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'R', Text: "R"}))
	if cmd == nil {
		t.Fatal("R で cmd が nil")
	}
	if store.loadCalls != before+1 {
		t.Errorf("Load() の呼び出し回数 = %d, want %d", store.loadCalls, before+1)
	}
}

// 逆方向: 外部プロセスの成功は、無関係な保存失敗のエラーを消してはいけない。
// 外部プロセスには「成功」以外に報告することがないので、成功時は errMsg に
// 触れないのが正しい（prLoadedMsg が errIsPR でガードするのと同じ理由）。
func TestSuccessfulExecKeepsSaveError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("保存失敗が errMsg に反映されていない")
	}
	saveErrMsg := m.errMsg

	got, _ = m.Update(execDoneMsg{err: nil})
	m = got.(Model)

	if m.errMsg != saveErrMsg {
		t.Errorf("外部プロセスの成功で保存エラーが消えた: errMsg = %q, want %q", m.errMsg, saveErrMsg)
	}
}

// タスクペインの r は tasks.md を読み直す。PR の再取得ではない。
// 見ているペインと関係ない更新をしても、何も起きていないように見える。
func TestRKeyOnTaskPaneReloadsTasks(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)
	before := store.loadCalls

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)

	if store.loadCalls != before+1 {
		t.Errorf("タスクペインの r で Load() の呼び出し回数 = %d, want %d", store.loadCalls, before+1)
	}
}

// タスクペインの r で tasks.md を読み直しても、PR 取得のエラーは消さない。
// 消すと「直ったように見えて何も直っていない」状態になる。
func TestReloadTasksKeepsPRError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)

	got, _ := m.Update(prLoadedMsg{err: errors.New("gh: not logged in")})
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("PR 取得失敗が errMsg に反映されていない")
	}
	prErrMsg := m.errMsg

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)

	if m.errMsg != prErrMsg {
		t.Errorf("タスクペインの r で PR のエラーが消えた: errMsg = %q, want %q", m.errMsg, prErrMsg)
	}
}

// 一方、保存失敗など PR 由来でないエラーは r で読み直せば消える。
func TestReloadTasksClearsNonPRError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)
	if m.errMsg == "" {
		t.Fatal("保存失敗が errMsg に反映されていない")
	}

	store.saveErr = nil
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)

	if m.errMsg != "" {
		t.Errorf("r で保存エラーが消えていない: errMsg = %q", m.errMsg)
	}
}

// GitHub ペインの r は今まで通り PR を再取得する。
func TestRKeyOnGitHubPaneRefreshesPRs(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	before := store.loadCalls

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if cmd == nil {
		t.Fatal("GitHub ペインの r で cmd が nil")
	}
	if _, ok := cmd().(prLoadedMsg); !ok {
		t.Errorf("GitHub ペインの r が返した cmd は PR 再取得ではない")
	}
	if store.loadCalls != before {
		t.Errorf("GitHub ペインの r で tasks.md を読み直した: loadCalls = %d, want %d", store.loadCalls, before)
	}
}

// GitHub ペインで r を連打しても、前の Refresh が in-flight の間は無視される。
// 連打で複数の Refresh が並行して走ると、後に投げた方が先に届くとは限らず、
// 新しい結果を古い結果で上書きしうる（到着順の非保証）ため。
func TestRKeyIgnoredWhileRefreshInFlight(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")

	got, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)
	if cmd == nil {
		t.Fatal("最初の r で cmd が nil")
	}
	if !m.refreshing {
		t.Fatal("最初の r 後に refreshing が true になっていない")
	}

	got, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)
	if cmd != nil {
		t.Error("in-flight 中の連打で cmd が返った（2つ目の Refresh が走ってしまう）")
	}

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)
	if m.refreshing {
		t.Error("prLoadedMsg 到着後も refreshing が true のまま")
	}

	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if cmd == nil {
		t.Error("Refresh 完了後の r で cmd が nil、再度更新できない")
	}
}

// Refresh が in-flight の間はフッタに「更新中」を出す。r 連打を無視するのを
// 無反応と誤解されないようにするため。
func TestFooterShowsRefreshingIndicator(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	m = got.(Model)

	if !strings.Contains(m.View().Content, "更新中") {
		t.Errorf("in-flight 中に「更新中」がフッタに出ていない: %q", m.View().Content)
	}

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)

	if strings.Contains(m.View().Content, "更新中") {
		t.Errorf("Refresh 完了後も「更新中」が残っている: %q", m.View().Content)
	}
}

// R はどちらのペインにいても tasks.md を読み直す。
func TestShiftRReloadsTasksFromGitHubPane(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")
	before := store.loadCalls

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'R', Text: "R"}))
	m = got.(Model)

	if store.loadCalls != before+1 {
		t.Errorf("GitHub ペインの R で Load() の呼び出し回数 = %d, want %d", store.loadCalls, before+1)
	}
}

// 両ペインの枠に名前と件数が出ること。潰れている側は中身が見えないので、
// 件数が出ないと PR が来ているかどうか分からなくなる。
func TestPaneTitlesShowNameAndCount(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})
	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	// 取得が終わるまでは件数を伏せる。潰れた枠は中身が見えないので、
	// 取得中の "(0)" が「PR なし」と区別できない。
	if content := m.View().Content; !strings.Contains(content, "GitHub (…)") {
		t.Errorf("取得前の GitHub 枠が件数を出している: %q", content)
	}

	got, _ = m.Update(prLoadedMsg{})
	m = got.(Model)

	content := m.View().Content
	if !strings.Contains(content, "タスク (1)") {
		t.Errorf("タスク枠のタイトルに件数が出ていない: %q", content)
	}
	if !strings.Contains(content, "GitHub (2)") {
		t.Errorf("潰れた GitHub 枠のタイトルに件数が出ていない: %q", content)
	}
}

// フォーカス中の枠だけに色が付くこと。潰れているだけだと、どちらを
// 操作しているかが一覧の中身からしか分からない。
func TestFocusedPaneBorderIsColored(t *testing.T) {
	m := prModelWith(t, &fakeStore{list: taskList("- [ ] やること\n")}, &fakeDetails{}, "claude")

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	got, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	// 枠の上辺はタイトルを含むので、色の有無をタイトルの直前で見る。
	lines := strings.Split(m.View().Content, "\n")
	var taskTop, githubTop string
	for _, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "タスク (1)") {
			taskTop = ln
		}
		if strings.Contains(ansi.Strip(ln), "GitHub (2)") {
			githubTop = ln
		}
	}
	if taskTop == "" || githubTop == "" {
		t.Fatalf("枠のタイトル行が見つからない: %q", m.View().Content)
	}
	if ansi.Strip(taskTop) == taskTop {
		t.Errorf("フォーカス中のタスク枠に色が付いていない: %q", taskTop)
	}
	// 上辺は自前で組んでいるので、そこだけ見ても箱本体の枠色は分からない。
	// フォーカス中の枠の下端（左カラムで最初に閉じる行）も見る。
	for _, ln := range lines {
		if strings.HasPrefix(ansi.Strip(ln), "╰") {
			if ansi.Strip(ln) == ln {
				t.Errorf("フォーカス中の枠の上辺以外に色が付いていない: %q", ln)
			}
			break
		}
	}
	if s := strings.SplitN(githubTop, "╭─GitHub", 2)[0]; ansi.Strip(s) != s {
		t.Errorf("フォーカスしていない GitHub 枠に色が付いている: %q", githubTop)
	}
}

// フォーカスを移すと2枠の高さが入れ替わり、それでも左カラム全体の高さは
// 変わらないこと（右ペインとの整合が崩れない）。
func TestAccordionSwapsPaneHeights(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "- [ ] item%d\n", i)
	}
	m := prModelWith(t, &fakeStore{list: taskList(b.String())}, &fakeDetails{}, "claude")

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	if strings.Contains(m.View().Content, "item9") {
		t.Errorf("潰れているはずのタスク枠に中身が出ている: %q", m.View().Content)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = got.(Model)
	content := m.View().Content
	if !strings.Contains(content, "item9") {
		t.Errorf("フォーカスしたタスク枠が広がっていない: %q", content)
	}
	if strings.Contains(content, "#1 fix") {
		t.Errorf("潰れたはずの GitHub 枠に中身が出ている: %q", content)
	}
}

// 端末の高さを使い切ること。余らせると、アコーディオンで広げたペインに
// 出せるはずの行を捨てることになる。
func TestRenderUsesFullTerminalHeight(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "- [ ] item%d\n", i)
	}
	m := newTestModel(t, &fakeStore{list: taskList(b.String())})

	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 15}, {40, 10}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		if n := len(strings.Split(mm.View().Content, "\n")); n != size[1] {
			t.Errorf("size %v: レンダリング行数 = %d, want %d", size, n, size[1])
		}
	}
}

// 枠が2つ入らない高さでは本文を削ってでもフッタを残すこと。フッタが
// 押し出されると、保存失敗や gh のエラーが見えなくなる。
func TestFooterSurvivesTinyTerminal(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	got, _ := m.Update(prLoadedMsg{err: errors.New("gh: not logged in")})
	m = got.(Model)

	for h := 1; h <= 8; h++ {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: h})
		mm := updated.(Model)
		lines := strings.Split(mm.View().Content, "\n")
		if len(lines) > h {
			t.Errorf("height %d: レンダリング行数 = %d, want <= %d", h, len(lines), h)
		}
		if last := lines[len(lines)-1]; !strings.Contains(last, "not logged in") {
			t.Errorf("height %d: 最終行がフッタでない: %q", h, last)
		}
	}
}

// 狭い端末でも幅・高さを超えないこと。
func TestPanesFitNarrowTerminal(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 15}, {20, 8}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		lines := strings.Split(mm.View().Content, "\n")
		if len(lines) > size[1] {
			t.Errorf("size %v: 行数 = %d, want <= %d", size, len(lines), size[1])
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w > size[0] {
				t.Errorf("size %v: %d行目が幅を超えている（幅=%d）: %q", size, i, w, ln)
			}
		}
	}
}

// 件数が入らない幅でも、ペインの名前だけは枠に残ること。どちらのペインを
// 見ているのか分からなくなるほうが困る。
func TestPaneTitleDropsCountBeforeName(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	m = got.(Model)

	content := ansi.Strip(m.View().Content)
	if strings.Contains(content, "タスク (1)") {
		t.Errorf("幅 30 に収まらない件数付きタイトルが出ている:\n%s", content)
	}
	for _, name := range []string{"タスク", "GitHub"} {
		if !strings.Contains(content, "─"+name) {
			t.Errorf("%s の枠に名前が残っていない:\n%s", name, content)
		}
	}
}

// GitHub ペインで n を押してもタスク追加モードに入らないこと。
// 入ってしまうと、見えないペインにタスクが増える。
func TestNKeyDisabledOnGitHubPane(t *testing.T) {
	m := prModel(t, "claude")

	got, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)

	if m.adding {
		t.Error("GitHub ペインで n を押して追加モードに入った")
	}
	if cmd != nil {
		t.Error("GitHub ペインの n が cmd を返した")
	}
}

// GitHub ペインで space を押しても保存が走らないこと。
func TestSpaceDisabledOnGitHubPane(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := prModelWith(t, store, &fakeDetails{}, "claude")

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)

	if len(store.saved) != 0 {
		t.Errorf("GitHub ペインの space で Save が呼ばれた: %+v", store.saved)
	}
	if m.errMsg != "" {
		t.Errorf("GitHub ペインの space でエラーが出た: %q", m.errMsg)
	}
}

// タスクペインでは n が今まで通り効くこと。
func TestNKeyStillWorksOnTaskPane(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)

	if !m.adding {
		t.Error("タスクペインで n を押しても追加モードに入らない")
	}
	if cmd == nil {
		t.Error("タスクペインの n が cmd を返さない（Focus されていない）")
	}
}

// 起動直後の GitHub ペインは gh の取得待ちで必ず空になる。何も出さないと
// 故障と区別できないので、取得中と0件を書き分ける。
func TestGitHubPaneEmptyStates(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, &fakePRs{}, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)

	if !strings.Contains(m.View().Content, "取得中") {
		t.Errorf("取得前の GitHub ペインに取得中の表示が無い:\n%s", m.View().Content)
	}

	got, _ = m.Update(prLoadedMsg{err: nil})
	m = got.(Model)

	content := m.View().Content
	if strings.Contains(content, "取得中") {
		t.Errorf("取得完了後も取得中のまま:\n%s", content)
	}
	if !strings.Contains(content, "PR なし") {
		t.Errorf("0件の表示が無い:\n%s", content)
	}
}

// 取得が失敗した場合も「取得中」で止めない。フッタにエラーが出るので、
// 一覧は「なし」で矛盾しない。
func TestGitHubPaneEmptyAfterFetchError(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	inbox := usecase.NewInbox(store, &fakePRs{err: errors.New("gh: not logged in")}, nil, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)
	got, _ = m.Update(prLoadedMsg{err: errors.New("gh: not logged in")})
	m = got.(Model)

	if strings.Contains(m.View().Content, "取得中") {
		t.Errorf("取得失敗後も取得中のまま:\n%s", m.View().Content)
	}
}

// GitHub ペインを開いたまま PR 取得が完了したとき、一覧が入れ替わり
// 「PR なし」「PR を取得中」のどちらも出ないこと。
// 以前の TestGitHubTabWithPRsShowsNoPlaceholder は strings.Contains(content,
// "取得中") という広すぎる一致で右ペインの「（詳細を取得中…）」を拾って
// 誤検知したため削除された。同じ轍を踏まないよう、検索文字列は
// 右ペインの表示と衝突しない「PR なし」「PR を取得中」に絞る。
func TestGitHubPaneFillsWithPRsOnLoad(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox, Config{AICmd: "claude"})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)
	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	m = got.(Model)

	// refreshCmd() の cmd を実行して、実際に inbox に PR を取り込ませる
	// （prLoadedMsg を直接組み立てただけでは inbox.PRs() は空のまま）。
	loaded := m.refreshCmd()().(prLoadedMsg)
	got, _ = m.Update(loaded)
	m = got.(Model)

	if len(m.items) != 2 {
		t.Fatalf("GitHub ペインの件数 = %d, want 2", len(m.items))
	}
	content := m.View().Content
	if strings.Contains(content, "PR なし") {
		t.Errorf("PR があるのに「PR なし」が出ている:\n%s", content)
	}
	if strings.Contains(content, "PR を取得中") {
		t.Errorf("PR があるのに「PR を取得中」が出ている:\n%s", content)
	}
}

// タスクペインが空のときは何も出さない。ユーザーが知っている状態であり、
// 非同期の取得も挟まらない。
func TestTaskPaneEmptyShowsNothing(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("")})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	content := m.View().Content
	if strings.Contains(content, "取得中") || strings.Contains(content, "なし") {
		t.Errorf("タスクペインの空表示に文言が出ている:\n%s", content)
	}
}

func TestEKeyOnTaskReturnsCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	m.cfg.EditorCmd = "true"

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if cmd == nil {
		t.Fatal("タスクで e を押しても cmd が nil")
	}
}

// PR には編集する行が無いので、e は何もしない。
func TestEKeyOnPRReturnsNil(t *testing.T) {
	m := prModel(t, "claude")
	m.cfg.EditorCmd = "true"

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if cmd != nil {
		t.Error("PR で e を押したのに cmd が nil でない")
	}
}

// エディタが指定できないときも、黙って何もしないのではなくエラーを出す。
func TestEKeyWithEmptyEditorSurfacesError(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})
	m.cfg.EditorCmd = ""

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if cmd == nil {
		t.Fatal("エディタが空でも cmd が nil であってはならない（エラーを出せない）")
	}
	done, ok := cmd().(editDoneMsg)
	if !ok {
		t.Fatalf("cmd() が返したのは %T, want editDoneMsg", cmd())
	}
	if done.err == nil {
		t.Error("エディタが空なのに editDoneMsg.err が nil")
	}
}

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

// 異常終了でも保存済みかもしれないので読み直し、そのうえでエラーを残す。
func TestEditDoneWithErrorStillReloadsAndShowsError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)
	before := store.loadCalls

	got, _ := m.Update(editDoneMsg{err: errors.New("exit status 1")})
	m = got.(Model)

	if store.loadCalls == before {
		t.Error("エディタが異常終了したときに読み直していない")
	}
	if m.errMsg == "" {
		t.Error("エディタのエラーが errMsg に出ていない")
	}
}

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
// e が開くのは tasks.md ではなく、そのタスクの詳細ファイル。
// argv を直接見る。cmd が非 nil なだけでは、どのファイルを開こうとして
// いるか分からず、tasks.md に戻る退行を検出できない。
func TestEKeyOpensDetailFileNotTasksFile(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	inbox := usecase.NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{})
	if err := inbox.Load(); err != nil {
		t.Fatal(err)
	}
	m := New(inbox, Config{EditorCmd: "vi"})

	c, err := m.editorCommand()
	if err != nil {
		t.Fatalf("editorCommand() = %v", err)
	}
	if c == nil {
		t.Fatal("タスクを選んでいるのに *exec.Cmd が nil")
	}
	want := []string{"vi", "/tmp/tk-test/やること.md"} // fakeDetailStore.EditPath が返すパス
	if !slices.Equal(c.Args, want) {
		t.Errorf("Args = %v, want %v", c.Args, want)
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
