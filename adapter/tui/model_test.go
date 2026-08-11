package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/k-narusawa/tk/domain"
	"github.com/k-narusawa/tk/usecase"
)

type fakeStore struct {
	list    domain.TaskList
	saved   []domain.TaskList
	saveErr error
}

func (f *fakeStore) Load() (domain.TaskList, error) { return f.list, nil }

func (f *fakeStore) Save(t domain.TaskList) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, t)
	f.list = t
	return nil
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
// SortInbox の並びでは review が先に来るので items[0] が review 側になる。
func prPair() *fakePRs {
	return &fakePRs{items: map[domain.Role]domain.Item{
		domain.RoleReview: {ID: domain.PRID("a/x", 1), Kind: domain.KindPR, Repo: "a/x", Number: 1, Title: "fix", Role: domain.RoleReview},
		domain.RoleMine:   {ID: domain.PRID("a/y", 2), Kind: domain.KindPR, Repo: "a/y", Number: 2, Title: "feat", Role: domain.RoleMine},
	}}
}

func newTestModel(t *testing.T, store *fakeStore) Model {
	t.Helper()
	inbox := usecase.NewInbox(store, nil, nil)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return New(inbox)
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
// このとき width/height は 0 で、right = width-left-4 は負になる。
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
}

// Init() が返す cmd を実行して PR が一覧に反映されることを確認する。
// TTY が無い環境では実際に起動して確認できないので、これがその代わり。
func TestInitFetchesPRsAsync(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	prs := &fakePRs{items: map[domain.Role]domain.Item{
		domain.RoleReview: {ID: domain.PRID("a/x", 1), Kind: domain.KindPR, Repo: "a/x", Number: 1, Role: domain.RoleReview},
		domain.RoleMine:   {ID: domain.PRID("a/y", 2), Kind: domain.KindPR, Repo: "a/y", Number: 2, Role: domain.RoleMine},
	}}
	inbox := usecase.NewInbox(store, prs, nil)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox)

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
	inbox := usecase.NewInbox(store, &fakePRs{err: wantErr}, nil)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox)

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

func TestRKeyReturnsRefreshCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if cmd == nil {
		t.Fatal("r で cmd が nil")
	}
}

// 保存失敗のエラーは、その後の PR 取得成功で消えてはいけない。
// 別々の原因のエラーを取得成功が一括で握り潰す、が今回の実バグ。
func TestSuccessfulPRRefreshKeepsSaveError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	inbox := usecase.NewInbox(store, &fakePRs{}, nil)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	m := New(inbox)

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
	inbox := usecase.NewInbox(store, prPair(), details)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox)

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
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox)

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
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{err: wantErr})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox)
	wantItemCount := len(m.items)
	for i, it := range m.items {
		if it.Kind == domain.KindPR {
			m.cursor = i
			break
		}
	}

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
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox)
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
	inbox := usecase.NewInbox(store, prPair(), &fakeDetails{})
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := inbox.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	m := New(inbox)
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

// 左右のペインの枠が同じ行で閉じること。ズレると端末で一目で分かる。
// viewport が box の外寸（枠を含む）に合わせられていると、枠の内側に
// 収まらず箱が膨らみ、左右で高さが変わる。
func TestPanesHaveEqualHeight(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] 一つ目\n- [ ] 二つ目\n")})

	for _, size := range [][2]int{{100, 30}, {80, 24}, {120, 40}, {60, 15}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		lines := strings.Split(mm.View().Content, "\n")

		leftClose, rightClose := -1, -1
		for i, ln := range lines {
			if leftClose < 0 && strings.HasPrefix(ln, "╰") {
				leftClose = i
			}
			if strings.HasSuffix(strings.TrimRight(ln, " "), "╯") {
				rightClose = i
			}
			if w := lipgloss.Width(ln); w > size[0] {
				t.Errorf("size %v: %d行目が端末幅を超えている（幅=%d, want <=%d）: %q", size, i, w, size[0], ln)
			}
		}
		if leftClose != rightClose {
			t.Errorf("size %v: 左右の枠の高さが違う（左=%d行目, 右=%d行目, 差=%d行）",
				size, leftClose, rightClose, rightClose-leftClose)
		}
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
