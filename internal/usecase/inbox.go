package usecase

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/k-narusawa/tk/internal/domain"
)

type Inbox struct {
	store       TaskStore
	prs         PRSource
	details     PRDetailSource
	taskDetails TaskDetailStore
	routines    RoutineSource

	mu           sync.Mutex
	tasks        domain.TaskList
	prItems      []domain.Item
	routineItems []domain.Item
	cache        map[domain.ID]domain.PRDetail
}

func NewInbox(store TaskStore, prs PRSource, details PRDetailSource, taskDetails TaskDetailStore, routines RoutineSource) *Inbox {
	return &Inbox{
		store:       store,
		prs:         prs,
		details:     details,
		taskDetails: taskDetails,
		routines:    routines,
		cache:       make(map[domain.ID]domain.PRDetail),
	}
}

// Load はローカルのファイル（tasks.md と routines.md）をまとめて読み直す。
// どちらも同じ r / R で更新されるので、別々の入口にしない。
func (i *Inbox) Load() error {
	t, err := i.store.Load()
	if err != nil {
		return err
	}
	r, err := i.routines.List()
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.tasks, i.routineItems = t, r
	i.mu.Unlock()
	return nil
}

func (i *Inbox) Routines() []domain.Item {
	i.mu.Lock()
	defer i.mu.Unlock()
	return slices.Clone(i.routineItems)
}

// routineName は id に対応する監視項目の名前。指示・結果ファイルの名前の元になる。
func (i *Inbox) routineName(id domain.ID) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, it := range i.routineItems {
		if it.ID == id {
			return it.Title, true
		}
	}
	return "", false
}

// RoutinePrompt は AI に渡す指示。実行の直前に読むので、書き換えれば
// 次の実行から効く（review.md と同じ考え方）。
func (i *Inbox) RoutinePrompt(id domain.ID) (string, error) {
	name, ok := i.routineName(id)
	if !ok {
		return "", fmt.Errorf("routine が見つからない: %s", id)
	}
	return i.routines.Body(name)
}

// RoutineResult は右ペインに出す過去の実行結果。タスクの詳細と同じく
// キャッシュしないので、走り終えた結果が次に選んだ瞬間に出る。
func (i *Inbox) RoutineResult(id domain.ID) (string, error) {
	name, ok := i.routineName(id)
	if !ok {
		return "", nil
	}
	return i.routines.Result(name)
}

// SaveRoutineResult は実行結果を追記する。
func (i *Inbox) SaveRoutineResult(id domain.ID, body string) error {
	name, ok := i.routineName(id)
	if !ok {
		return fmt.Errorf("routine が見つからない: %s", id)
	}
	return i.routines.AppendResult(name, body)
}

// RoutinePath は e が開く指示ファイルのパス。存在しない ID はエラーにする
// （DetailPath と同じ理由で、空パスを返すとエディタが妙な場所を開く）。
func (i *Inbox) RoutinePath(id domain.ID) (string, error) {
	name, ok := i.routineName(id)
	if !ok {
		return "", fmt.Errorf("routine が見つからない: %s", id)
	}
	return i.routines.EditPath(name)
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
