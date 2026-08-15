package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/k-narusawa/tk/internal/domain"
)

type fakeStore struct {
	list     domain.TaskList
	saved    []domain.TaskList
	loadErr  error
	saveErr  error
	loadCall int
	saveCall int
}

func (f *fakeStore) Load() (domain.TaskList, error) {
	f.loadCall++
	return f.list, f.loadErr
}

func (f *fakeStore) Save(t domain.TaskList) error {
	f.saveCall++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, t)
	f.list = t
	return nil
}

type fakePRs struct {
	byRole map[domain.Role][]domain.Item
	err    error
	calls  int
	mu     sync.Mutex
}

func (f *fakePRs) Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.byRole[role], f.err
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

type fakeDetails struct {
	detail domain.PRDetail
	err    error
	calls  int
	mu     sync.Mutex
}

func (f *fakeDetails) Detail(ctx context.Context, repo string, number int) (domain.PRDetail, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.detail, f.err
}

func taskList(s string) domain.TaskList { return domain.Parse(strings.Split(s, "\n")) }

func pr(repo string, number int, role domain.Role) domain.Item {
	return domain.Item{
		ID:     domain.PRID(repo, number),
		Kind:   domain.KindPR,
		Repo:   repo,
		Number: number,
		Role:   role,
	}
}

func TestLoadReadsTasks(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})

	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tasks := in.Tasks()
	if len(tasks) != 1 || tasks[0].Title != "やること" {
		t.Errorf("Tasks() = %+v", tasks)
	}
}

func TestLoadPropagatesError(t *testing.T) {
	want := errors.New("permission denied")
	in := NewInbox(&fakeStore{loadErr: want}, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); !errors.Is(err, want) {
		t.Errorf("Load() error = %v, want %v", err, want)
	}
}

// 起動時に PR 詳細を1回も叩かないこと。ここが性能上の約束。
func TestLoadAndRefreshDoNotFetchDetails(t *testing.T) {
	details := &fakeDetails{}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview)},
		domain.RoleMine:   {pr("a/y", 2, domain.RoleMine)},
	}}
	in := NewInbox(&fakeStore{list: taskList("")}, prs, details, &fakeDetailStore{}, &fakeRoutines{})

	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if details.calls != 0 {
		t.Errorf("PRDetailSource が %d 回呼ばれた, want 0", details.calls)
	}
	if prs.calls != 2 {
		t.Errorf("PRSource が %d 回呼ばれた, want 2", prs.calls)
	}
	if len(in.PRs()) != 2 {
		t.Errorf("PRs() = %d 件, want 2", len(in.PRs()))
	}
}

func TestDetailIsCached(t *testing.T) {
	details := &fakeDetails{detail: domain.PRDetail{CI: "passing"}}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview)},
	}}
	in := NewInbox(&fakeStore{list: taskList("")}, prs, details, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	id := domain.PRID("a/x", 1)
	for range 3 {
		got, err := in.Detail(context.Background(), id)
		if err != nil {
			t.Fatalf("Detail() error = %v", err)
		}
		if got.CI != "passing" {
			t.Errorf("Detail().CI = %q, want passing", got.CI)
		}
	}
	if details.calls != 1 {
		t.Errorf("PRDetailSource が %d 回呼ばれた, want 1（キャッシュが効いていない）", details.calls)
	}
}

func TestDetailUnknownID(t *testing.T) {
	in := NewInbox(&fakeStore{list: taskList("")}, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if _, err := in.Detail(context.Background(), domain.PRID("no/such", 1)); err == nil {
		t.Error("存在しない ID でエラーが返らなかった")
	}
}

// タスク種の ID は prItems の中を探しても絶対に見つからないので、
// 存在しない PR ID と同じ not-found に落ちる。
func TestDetailTaskKindIDNotFound(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tasks := in.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("前提が崩れている: Tasks() = %+v", tasks)
	}

	if _, err := in.Detail(context.Background(), tasks[0].ID); err == nil {
		t.Error("タスク種の ID でエラーが返らなかった")
	}
}

// droppingPRs は1回目の Fetch だけ review 側に PR を返し、2回目以降は
// 空を返す。「一覧から消えた PR」を Refresh 経由で再現するためのもの。
type droppingPRs struct {
	mu    sync.Mutex
	calls map[domain.Role]int
}

func (f *droppingPRs) Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error) {
	f.mu.Lock()
	f.calls[role]++
	n := f.calls[role]
	f.mu.Unlock()

	if role == domain.RoleReview && n == 1 {
		return []domain.Item{pr("a/x", 1, domain.RoleReview)}, nil
	}
	return nil, nil
}

// 一覧に一度は載った PR でも、次の Refresh で消えたなら未キャッシュの
// Detail は not-found に落ちる（キャッシュに残っていた古い詳細を返さない）。
func TestDetailRemovedPRIDNotFound(t *testing.T) {
	prs := &droppingPRs{calls: make(map[domain.Role]int)}
	in := NewInbox(&fakeStore{list: taskList("")}, prs, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})

	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() 1回目 error = %v", err)
	}
	id := domain.PRID("a/x", 1)
	if items := in.PRs(); !containsPR(items, "a/x", 1) {
		t.Fatalf("前提が崩れている: 1回目後の PRs() = %+v", items)
	}

	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() 2回目 error = %v", err)
	}
	if items := in.PRs(); containsPR(items, "a/x", 1) {
		t.Fatalf("前提が崩れている: 2回目後も一覧に残っている: %+v", items)
	}

	if _, err := in.Detail(context.Background(), id); err == nil {
		t.Error("一覧から消えた PR ID でエラーが返らなかった")
	}
}

func TestToggleSavesAndUpdates(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := in.Toggle(domain.TaskID(0)); err != nil {
		t.Fatalf("Toggle() error = %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("Save が %d 回呼ばれた, want 1", len(store.saved))
	}
	if got := strings.Join(store.saved[0].Render(), "\n"); got != "- [x] やること\n" {
		t.Errorf("保存内容 = %q", got)
	}
	// 完了済みタスクは末尾に回る
	tasks := in.Tasks()
	if len(tasks) != 1 || !tasks[0].Done {
		t.Errorf("Tasks() = %+v, want Done=true", tasks)
	}
}

// 保存に失敗したら内部状態を変えない。画面とファイルが食い違わないため。
// Save が実際に呼ばれたことと状態が変わらないことの両方を主張しないと、
// 「先に状態を更新して失敗したらロールバックする」別実装でも通ってしまう。
func TestToggleKeepsStateOnSaveError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := in.Toggle(domain.TaskID(0)); err == nil {
		t.Fatal("Toggle() がエラーを返さなかった")
	}
	if store.saveCall != 1 {
		t.Errorf("Save が %d 回呼ばれた, want 1", store.saveCall)
	}
	if tasks := in.Tasks(); tasks[0].Done {
		t.Error("保存失敗後に Done が true になっている")
	}
}

func TestAddSavesAndUpdates(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 既存\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := in.Add("新規"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := strings.Join(store.saved[0].Render(), "\n"); got != "- [ ] 既存\n- [ ] 新規\n" {
		t.Errorf("保存内容 = %q", got)
	}
	if len(in.Tasks()) != 2 {
		t.Errorf("Tasks() = %d 件, want 2", len(in.Tasks()))
	}
}

func TestRefreshPropagatesError(t *testing.T) {
	want := errors.New("gh: not logged in")
	in := NewInbox(&fakeStore{list: taskList("")}, &fakePRs{err: want}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Refresh(context.Background()); !errors.Is(err, want) {
		t.Errorf("Refresh() error = %v, want %v", err, want)
	}
}

// partialFailurePRs は role ごとに呼び出し回数を数え、2回目の Fetch で
// review 側は別の PR を返し、mine 側はエラーを返す。「片方失敗」を再現する。
type partialFailurePRs struct {
	mu    sync.Mutex
	calls map[domain.Role]int
}

func (f *partialFailurePRs) Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error) {
	f.mu.Lock()
	f.calls[role]++
	n := f.calls[role]
	f.mu.Unlock()

	if role == domain.RoleMine && n == 2 {
		return nil, errors.New("rate limited")
	}
	if role == domain.RoleReview && n == 2 {
		return []domain.Item{pr("a/z", 3, domain.RoleReview)}, nil
	}
	if role == domain.RoleReview {
		return []domain.Item{pr("a/x", 1, domain.RoleReview)}, nil
	}
	return []domain.Item{pr("a/y", 2, domain.RoleMine)}, nil
}

func containsPR(items []domain.Item, repo string, number int) bool {
	for _, it := range items {
		if it.Repo == repo && it.Number == number {
			return true
		}
	}
	return false
}

// 2回目の Refresh で review は成功、mine は失敗する状況を再現する。
// このとき、片方の失敗で良い状態の一覧を上書きしてはいけない。
func TestRefreshKeepsPreviousPRsOnPartialFailure(t *testing.T) {
	prs := &partialFailurePRs{calls: make(map[domain.Role]int)}
	in := NewInbox(&fakeStore{list: taskList("")}, prs, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})

	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() 1回目 error = %v", err)
	}
	items := in.PRs()
	if !containsPR(items, "a/x", 1) || !containsPR(items, "a/y", 2) {
		t.Fatalf("1回目後の PRs() = %+v", items)
	}

	if err := in.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh() 2回目でエラーが返らなかった")
	}

	items = in.PRs()
	if !containsPR(items, "a/x", 1) || !containsPR(items, "a/y", 2) {
		t.Errorf("部分失敗後に以前の一覧が失われた: %+v", items)
	}
	if containsPR(items, "a/z", 3) {
		t.Errorf("部分失敗なのに新しい review PR が反映されている: %+v", items)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview), pr("b/y", 2, domain.RoleReview)},
		domain.RoleMine:   {pr("c/z", 3, domain.RoleMine), pr("d/w", 4, domain.RoleMine)},
	}}
	details := &fakeDetails{detail: domain.PRDetail{CI: "passing"}}

	in := NewInbox(store, prs, details, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	// Detail from multiple goroutines with distinct PR IDs
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := domain.PRID([]string{"a/x", "b/y", "c/z", "d/w"}[idx-1], idx)
			_, err := in.Detail(context.Background(), id)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	// Concurrent Tasks and PRs calls
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = in.Tasks()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = in.PRs()
		}()
	}

	// Concurrent Toggle calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := in.Toggle(domain.TaskID(0)); err != nil {
			errCh <- err
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent access error: %v", err)
	}
}

// タブごとに独立した一覧が取れること。Tasks() に PR が、PRs() に
// タスクが混ざってはいけない。
func TestTasksAndPRsAreSeparate(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n- [x] 済んだこと\n")}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview)},
		domain.RoleMine:   {pr("a/y", 2, domain.RoleMine)},
	}}
	in := NewInbox(store, prs, nil, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := in.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	tasks := in.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("Tasks() = %d 件, want 2", len(tasks))
	}
	for _, it := range tasks {
		if it.Kind != domain.KindTask {
			t.Errorf("Tasks() に PR が混ざっている: %+v", it)
		}
	}
	if tasks[0].Done {
		t.Errorf("Tasks() の先頭が完了済み: %+v", tasks[0])
	}

	prItems := in.PRs()
	if len(prItems) != 2 {
		t.Fatalf("PRs() = %d 件, want 2", len(prItems))
	}
	for _, it := range prItems {
		if it.Kind != domain.KindPR {
			t.Errorf("PRs() にタスクが混ざっている: %+v", it)
		}
	}
	if prItems[0].Role != domain.RoleReview {
		t.Errorf("PRs() の先頭が review でない: %+v", prItems[0])
	}
}

// Refresh 前は PRs() が空でも Tasks() は取れること。
func TestTasksBeforeRefresh(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, nil, nil, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(in.Tasks()) != 1 {
		t.Errorf("Tasks() = %d 件, want 1", len(in.Tasks()))
	}
	if len(in.PRs()) != 0 {
		t.Errorf("Refresh 前の PRs() = %d 件, want 0", len(in.PRs()))
	}
}

func TestBodyReadsDetailOfSelectedTask(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 認証まわりのリファクタ @today\n- [ ] 別のタスク\n")}
	details := &fakeDetailStore{bodies: map[string]string{"認証まわりのリファクタ": "Cookie の SameSite を Lax に"}}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, details, &fakeRoutines{})
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
	in := NewInbox(store, &fakePRs{}, &fakeDetails{}, details, &fakeRoutines{})
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
	in := NewInbox(&fakeStore{list: taskList("- [ ] やること\n")}, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, &fakeRoutines{})
	if err := in.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := in.DetailPath(domain.TaskID(99)); err == nil {
		t.Error("存在しない ID なのにエラーにならなかった")
	}
}

// fakeRoutines は routines.md と routines/ 配下のインメモリ版。
type fakeRoutines struct {
	names     []string
	prompts   map[string]string
	results   map[string]string
	listErr   error
	appendErr error
}

func (f *fakeRoutines) List() ([]domain.Item, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	lines := make([]string, 0, len(f.names))
	for _, n := range f.names {
		lines = append(lines, "- "+n)
	}
	return domain.ParseRoutines(lines), nil
}

func (f *fakeRoutines) Body(name string) (string, error)     { return f.prompts[name], nil }
func (f *fakeRoutines) EditPath(name string) (string, error) { return "/routines/" + name + ".md", nil }
func (f *fakeRoutines) Result(name string) (string, error)   { return f.results[name], nil }

func (f *fakeRoutines) AppendResult(name, body string) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	if f.results == nil {
		f.results = map[string]string{}
	}
	f.results[name] += body
	return nil
}

func routineInbox(t *testing.T, r *fakeRoutines) *Inbox {
	t.Helper()
	in := NewInbox(&fakeStore{list: taskList("")}, &fakePRs{}, &fakeDetails{}, &fakeDetailStore{}, r)
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return in
}

func TestRoutines(t *testing.T) {
	in := routineInbox(t, &fakeRoutines{names: []string{"golang", "rust"}})
	got := in.Routines()
	if len(got) != 2 || got[0].Title != "golang" {
		t.Fatalf("Routines() = %v", got)
	}
	// routines.md に書いた順のまま出すこと。タスクと違って完了状態が無く、
	// 並べ替える理由がない。
	if got[1].Title != "rust" {
		t.Errorf("順序が変わっている: %v", got)
	}
}

// 返した一覧を呼び出し側が書き換えても内部状態が壊れないこと。
func TestRoutinesReturnsCopy(t *testing.T) {
	in := routineInbox(t, &fakeRoutines{names: []string{"golang"}})
	in.Routines()[0].Title = "改竄"
	if got := in.Routines(); got[0].Title != "golang" {
		t.Errorf("内部状態が書き換わった: %v", got)
	}
}

func TestRoutinePromptAndResult(t *testing.T) {
	r := &fakeRoutines{
		names:   []string{"golang"},
		prompts: map[string]string{"golang": "調べて"},
		results: map[string]string{"golang": "1.24"},
	}
	in := routineInbox(t, r)
	id := domain.RoutineID("golang")

	if got, err := in.RoutinePrompt(id); err != nil || got != "調べて" {
		t.Errorf("RoutinePrompt() = %q, %v", got, err)
	}
	if got, err := in.RoutineResult(id); err != nil || got != "1.24" {
		t.Errorf("RoutineResult() = %q, %v", got, err)
	}
	if err := in.SaveRoutineResult(id, "\n1.25"); err != nil {
		t.Fatal(err)
	}
	if r.results["golang"] != "1.24\n1.25" {
		t.Errorf("追記されていない: %q", r.results["golang"])
	}
}

// 知らない ID で実行や編集に進ませない。空文字を返すと、エディタが妙な
// 場所を開いたり、結果が無関係なファイルに書かれたりする。
func TestRoutineUnknownID(t *testing.T) {
	in := routineInbox(t, &fakeRoutines{names: []string{"golang"}})
	id := domain.RoutineID("知らない")

	if _, err := in.RoutinePrompt(id); err == nil {
		t.Error("RoutinePrompt() が未知の ID でエラーにならない")
	}
	if _, err := in.RoutinePath(id); err == nil {
		t.Error("RoutinePath() が未知の ID でエラーにならない")
	}
	if err := in.SaveRoutineResult(id, "本文"); err == nil {
		t.Error("SaveRoutineResult() が未知の ID でエラーにならない")
	}
}

// Load は tasks.md と routines.md をまとめて読み直す。routines.md だけ
// 古いままだと、R を押しても増えた監視項目が出てこない。
func TestLoadReloadsRoutines(t *testing.T) {
	r := &fakeRoutines{names: []string{"golang"}}
	in := routineInbox(t, r)

	r.names = append(r.names, "rust")
	if err := in.Load(); err != nil {
		t.Fatal(err)
	}
	if len(in.Routines()) != 2 {
		t.Errorf("Load() 後の件数 = %d, want 2", len(in.Routines()))
	}
}
