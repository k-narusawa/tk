package markdown

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k-narusawa/tk/internal/domain"
)

// RoutineStore は routines.md（監視項目の一覧）と routines/ 配下（項目ごとの
// 指示と実行結果）を扱う。指示ファイルの置き場所の決め方はタスクの詳細と
// まったく同じなので DetailStore に任せ、結果ファイルの分だけを足す。
//
// tk は routines.md に書き戻さない（追加・削除はエディタでやる）ので、
// Store のような外部変更の検知は要らない。
type RoutineStore struct {
	*DetailStore
	path string
}

func NewRoutineStore(path string) *RoutineStore {
	return &RoutineStore{DetailStore: NewDetailStore(path), path: path}
}

// List は routines.md を読む。ファイルが無ければ空を返す。まだ1件も
// 登録していない状態が普通なので、起動を止めない。
func (r *RoutineStore) List() ([]domain.Item, error) {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return domain.ParseRoutines(strings.Split(string(data), "\n")), nil
}

// resultName は指示ファイル名の ".md" を ".result.md" に差し替える。
// 同じ変換規則を通すので、名前に / が入っていても指示と結果が隣に並ぶ。
func resultName(name string) string {
	return strings.TrimSuffix(detailName(name), ".md") + ".result.md"
}

// Result は過去の実行結果すべて。一度も実行していなければ空文字を返す。
func (r *RoutineStore) Result(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.Dir(), resultName(name)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AppendResult は実行結果を日時見出し付きで追記する。上書きにすると
// 「前回から何が変わったか」が消えてしまい、監視の記録にならない。
// 溜まりすぎたら手で消す（tk は消さない）。
func (r *RoutineStore) AppendResult(name, body string) error {
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(r.Dir(), resultName(name)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "## %s\n\n%s\n\n", time.Now().Format("2006-01-02 15:04"), strings.TrimRight(body, "\n")); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
