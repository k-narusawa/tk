package usecase

import (
	"context"

	"github.com/k-narusawa/tk/internal/domain"
)

// TaskStore は tasks.md の読み書き。adapter/markdown が実装する。
type TaskStore interface {
	Load() (domain.TaskList, error)
	Save(domain.TaskList) error
}

// TaskDetailStore はタスク1件ぶんの詳細ファイル。adapter/markdown が実装する。
// 詳細は tasks.md の中ではなく、タイトルを名前にした独立したファイルに置く。
type TaskDetailStore interface {
	// Body は詳細の本文。ファイルが無ければ空文字を返し、エラーにしない。
	Body(title string) (string, error)
	// EditPath は e が開くパス。親ディレクトリが無ければ作る。
	EditPath(title string) (string, error)
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
