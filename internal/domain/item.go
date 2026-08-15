package domain

import "fmt"

type Kind int

const (
	KindTask Kind = iota
	KindPR
	KindRoutine
)

type Role string

const (
	RoleReview Role = "review" // レビューを依頼された
	RoleMine   Role = "mine"   // 自分が author
)

// ID は "task:3" / "pr:app/payment#412" / "routine:golang のリリース" の形。
type ID string

func TaskID(lineNo int) ID { return ID(fmt.Sprintf("task:%d", lineNo)) }

func PRID(repo string, number int) ID { return ID(fmt.Sprintf("pr:%s#%d", repo, number)) }

// RoutineID は名前をそのまま使う。行番号を使うタスクと違い、routines.md の
// 行が入れ替わっても実行状態と結果ファイルが迷子にならないため。
func RoutineID(name string) ID { return ID("routine:" + name) }

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

// PRDetail は一覧に載せない重い情報。右ペイン表示時にだけ取得する。
type PRDetail struct {
	CI           string // "passing" / "failing" / "pending" / ""
	Reviews      string // "2人待ち" など
	Additions    int
	Deletions    int
	ChangedFiles int
}
