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

// ListPath は n が開く routines.md そのもの。tk はこのファイルに書き戻さない
// ので、監視項目の追加・削除はエディタで直接やってもらう。
func (r *RoutineStore) ListPath() (string, error) {
	// 既定の ~/.config/tk/ は新規ユーザーのマシンにまだ無い。無いままでは
	// エディタが保存に失敗するので、先に作る。tasks.md と同じ場所なので、
	// Store.Save と同じく 0o700 で締める。
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return "", err
	}
	return r.path, nil
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

// PrependResult は実行結果を日時見出し付きで先頭に積む。上書きにすると
// 「前回から何が変わったか」が消えてしまい、監視の記録にならない。
// 溜まりすぎたら手で消す（tk は消さない）。
//
// 末尾ではなく先頭に置くのは、右ペインを開いた時点で最新が見えるようにする
// ため。追記で済まないので、書き換え中に落ちても過去の結果が消えないよう
// 一時ファイルに書いてから rename する。
func (r *RoutineStore) PrependResult(name, body string) error {
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		return err
	}
	path := filepath.Join(r.Dir(), resultName(name))
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	f, err := os.CreateTemp(r.Dir(), ".tk-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // Rename 成功後は存在しないので無害

	entry := fmt.Sprintf("## %s\n\n%s\n\n", time.Now().Format("2006-01-02 15:04"), strings.TrimRight(body, "\n"))
	if _, err := f.WriteString(entry + string(old)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
