package usecase

import (
	"context"
	"fmt"
	"sync"

	"github.com/k-narusawa/tk/domain"
)

type Inbox struct {
	store   TaskStore
	prs     PRSource
	details PRDetailSource

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
	i.tasks = t
	return nil
}

func (i *Inbox) Items() []domain.Item {
	return domain.SortInbox(i.tasks.Items(), i.prItems)
}

func (i *Inbox) Find(id domain.ID) (domain.Item, bool) {
	for _, it := range i.Items() {
		if it.ID == id {
			return it, true
		}
	}
	return domain.Item{}, false
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
	i.prItems = domain.MergePRs(review, mine)
	return nil
}

func (i *Inbox) Toggle(id domain.ID) error { return i.save(i.tasks.Toggle(id)) }

func (i *Inbox) Add(title string) error { return i.save(i.tasks.Add(title)) }

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
	if d, ok := i.cache[id]; ok {
		return d, nil
	}
	for _, it := range i.prItems {
		if it.ID != id {
			continue
		}
		d, err := i.details.Detail(ctx, it.Repo, it.Number)
		if err != nil {
			return domain.PRDetail{}, err
		}
		i.cache[id] = d
		return d, nil
	}
	return domain.PRDetail{}, fmt.Errorf("PR が見つからない: %s", id)
}
