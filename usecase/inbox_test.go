package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/k-narusawa/tk/domain"
)

type fakeStore struct {
	list     domain.TaskList
	saved    []domain.TaskList
	loadErr  error
	saveErr  error
	loadCall int
}

func (f *fakeStore) Load() (domain.TaskList, error) {
	f.loadCall++
	return f.list, f.loadErr
}

func (f *fakeStore) Save(t domain.TaskList) error {
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
	in := NewInbox(store, &fakePRs{}, &fakeDetails{})

	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	items := in.Items()
	if len(items) != 1 || items[0].Title != "やること" {
		t.Errorf("Items() = %+v", items)
	}
}

func TestLoadPropagatesError(t *testing.T) {
	want := errors.New("permission denied")
	in := NewInbox(&fakeStore{loadErr: want}, &fakePRs{}, &fakeDetails{})
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
	in := NewInbox(&fakeStore{list: taskList("")}, prs, details)

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
	if len(in.Items()) != 2 {
		t.Errorf("Items() = %d 件, want 2", len(in.Items()))
	}
}

func TestDetailIsCached(t *testing.T) {
	details := &fakeDetails{detail: domain.PRDetail{CI: "passing"}}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview)},
	}}
	in := NewInbox(&fakeStore{list: taskList("")}, prs, details)
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
	in := NewInbox(&fakeStore{list: taskList("")}, &fakePRs{}, &fakeDetails{})
	if _, err := in.Detail(context.Background(), domain.PRID("no/such", 1)); err == nil {
		t.Error("存在しない ID でエラーが返らなかった")
	}
}

func TestToggleSavesAndUpdates(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{})
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
	items := in.Items()
	if len(items) != 1 || !items[0].Done {
		t.Errorf("Items() = %+v, want Done=true", items)
	}
}

// 保存に失敗したら内部状態を変えない。画面とファイルが食い違わないため。
func TestToggleKeepsStateOnSaveError(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n"), saveErr: errors.New("disk full")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := in.Toggle(domain.TaskID(0)); err == nil {
		t.Fatal("Toggle() がエラーを返さなかった")
	}
	if items := in.Items(); items[0].Done {
		t.Error("保存失敗後に Done が true になっている")
	}
}

func TestAddSavesAndUpdates(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] 既存\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := in.Add("新規"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := strings.Join(store.saved[0].Render(), "\n"); got != "- [ ] 既存\n- [ ] 新規\n" {
		t.Errorf("保存内容 = %q", got)
	}
	if len(in.Items()) != 2 {
		t.Errorf("Items() = %d 件, want 2", len(in.Items()))
	}
}

func TestRefreshPropagatesError(t *testing.T) {
	want := errors.New("gh: not logged in")
	in := NewInbox(&fakeStore{list: taskList("")}, &fakePRs{err: want}, &fakeDetails{})
	if err := in.Refresh(context.Background()); !errors.Is(err, want) {
		t.Errorf("Refresh() error = %v, want %v", err, want)
	}
}

func TestFind(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	in := NewInbox(store, &fakePRs{}, &fakeDetails{})
	if err := in.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if _, ok := in.Find(domain.TaskID(0)); !ok {
		t.Error("Find が既存の ID を見つけられなかった")
	}
	if _, ok := in.Find(domain.TaskID(99)); ok {
		t.Error("Find が存在しない ID を見つけた")
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	prs := &fakePRs{byRole: map[domain.Role][]domain.Item{
		domain.RoleReview: {pr("a/x", 1, domain.RoleReview), pr("b/y", 2, domain.RoleReview)},
		domain.RoleMine:   {pr("c/z", 3, domain.RoleMine), pr("d/w", 4, domain.RoleMine)},
	}}
	details := &fakeDetails{detail: domain.PRDetail{CI: "passing"}}

	in := NewInbox(store, prs, details)
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

	// Concurrent Items and Find calls
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = in.Items()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = in.Find(domain.TaskID(0))
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
