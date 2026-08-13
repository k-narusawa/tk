package usecase

import (
	"context"
	"fmt"
	"sync"

	"github.com/k-narusawa/tk/internal/domain"
)

type Inbox struct {
	store   TaskStore
	prs     PRSource
	details PRDetailSource

	mu      sync.Mutex
	tasks   domain.TaskList
	prItems []domain.Item
	cache   map[domain.ID]domain.PRDetail
}

func NewInbox(store TaskStore, prs PRSource, details PRDetailSource) *Inbox {
	return &Inbox{
		store:   store,
		prs:     prs,
		details: details,
		cache:   make(map[domain.ID]domain.PRDetail),
	}
}

func (i *Inbox) Load() error {
	t, err := i.store.Load()
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.tasks = t
	i.mu.Unlock()
	return nil
}

func (i *Inbox) Tasks() []domain.Item {
	i.mu.Lock()
	defer i.mu.Unlock()
	return domain.SortTasks(i.tasks.Items())
}

func (i *Inbox) PRs() []domain.Item {
	i.mu.Lock()
	defer i.mu.Unlock()
	return domain.SortPRs(i.prItems)
}

// Refresh は2つの role を並行に取って重複排除する。
func (i *Inbox) Refresh(ctx context.Context) error {
	var review, mine []domain.Item
	var errReview, errMine error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		review, errReview = i.prs.Fetch(ctx, domain.RoleReview)
	}()
	go func() {
		defer wg.Done()
		mine, errMine = i.prs.Fetch(ctx, domain.RoleMine)
	}()
	wg.Wait()

	if errReview != nil {
		return errReview
	}
	if errMine != nil {
		return errMine
	}

	merged := domain.MergePRs(review, mine)
	i.mu.Lock()
	i.prItems = merged
	i.mu.Unlock()
	return nil
}

func (i *Inbox) Toggle(id domain.ID) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.save(i.tasks.Toggle(id))
}

func (i *Inbox) Add(title string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.save(i.tasks.Add(title))
}

// save assumes i.mu is held.
// save は書き込みが成功してから内部状態を差し替える。
// 逆にすると、失敗したときに画面とファイルが食い違う。
func (i *Inbox) save(next domain.TaskList) error {
	if err := i.store.Save(next); err != nil {
		return err
	}
	i.tasks = next
	return nil
}

// Detail は右ペインに表示するときに初めて呼ばれる。結果はキャッシュする。
func (i *Inbox) Detail(ctx context.Context, id domain.ID) (domain.PRDetail, error) {
	i.mu.Lock()
	cached, hit := i.cache[id]
	var target domain.Item
	found := false
	for _, it := range i.prItems {
		if it.ID == id {
			target, found = it, true
			break
		}
	}
	i.mu.Unlock()

	if hit {
		return cached, nil
	}
	if !found {
		return domain.PRDetail{}, fmt.Errorf("PR が見つからない: %s", id)
	}

	d, err := i.details.Detail(ctx, target.Repo, target.Number)
	if err != nil {
		return domain.PRDetail{}, err
	}

	i.mu.Lock()
	i.cache[id] = d
	i.mu.Unlock()
	return d, nil
}
