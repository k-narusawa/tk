package domain

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind int

const (
	KindTask Kind = iota
	KindPR
)

type Role string

const (
	RoleReview Role = "review" // レビューを依頼された
	RoleMine   Role = "mine"   // 自分が author
)

// ID は "task:3" / "pr:app/payment#412" の形。
type ID string

func TaskID(lineNo int) ID { return ID(fmt.Sprintf("task:%d", lineNo)) }

// TaskLine は "task:3" から行番号（0 始まり）を取り出す。タスク以外なら false。
func (id ID) TaskLine() (int, bool) {
	s, ok := strings.CutPrefix(string(id), "task:")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func PRID(repo string, number int) ID { return ID(fmt.Sprintf("pr:%s#%d", repo, number)) }

type Item struct {
	ID    ID
	Kind  Kind
	Title string

	// KindTask のみ
	Done bool
	Tag  string // "@today" など
	Body string // チェックボックス行に続くインデント行。段落は "\n\n" で繋がる

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
