package usecase

import (
	"context"

	"github.com/k-narusawa/tk/domain"
)

// TaskStore は tasks.md の読み書き。adapter/markdown が実装する。
type TaskStore interface {
	Load() (domain.TaskList, error)
	Save(domain.TaskList) error
}

// PRSource は role ごとに1本のクエリを投げる。2本を並行に走らせて
// 重複排除するのは Inbox の仕事。
type PRSource interface {
	Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error)
}

// PRDetailSource は一覧に含まれない重い情報を取る。
type PRDetailSource interface {
	Detail(ctx context.Context, repo string, number int) (domain.PRDetail, error)
}
