package markdown

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k-narusawa/tk/internal/domain"
)

// Store は Load 時点のファイルの mtime/size を憶えておき、Save 時に外部で
// 変更されていないか確認する。この状態があるため、同じ Store の Load/Save を
// 複数 goroutine から並行に呼ぶのは安全ではない
// （usecase.Inbox がロックを掛けて呼ぶ前提）。
type Store struct {
	path string

	mtime time.Time
	size  int64
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (domain.TaskList, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		// 空ファイル相当。最初の Save で作る。
		s.mtime, s.size = time.Time{}, 0
		return domain.Parse([]string{""}), nil
	}
	if err != nil {
		return domain.TaskList{}, err
	}
	if info, err := os.Stat(s.path); err == nil {
		s.mtime, s.size = info.ModTime(), info.Size()
	}
	return domain.Parse(strings.Split(string(data), "\n")), nil
}

// Save は一時ファイルに全文を書いてから os.Rename で被せる。
// 途中でプロセスが死んでもファイルが壊れない。
func (s *Store) Save(t domain.TaskList) error {
	targetPath := s.path
	tmpDir := filepath.Dir(s.path)
	// シンボリックリンク先を解決してから書き換える。解決せずに os.Rename すると
	// リンク自体が通常ファイルに置き換わり、リンク先（Obsidian vault 等）が
	// 取り残される。存在しないファイルは解決できないので初回 Save はそのまま進む。
	if resolved, err := filepath.EvalSymlinks(s.path); err == nil {
		targetPath = resolved
		tmpDir = filepath.Dir(resolved)
	}

	f, err := os.CreateTemp(tmpDir, ".tk-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // Rename 成功後は存在しないので無害

	if _, err := f.WriteString(strings.Join(t.Render(), "\n")); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if info, err := os.Stat(targetPath); err == nil {
		// 既存ファイルのパーミッションを引き継ぐ。CreateTemp は 0600 で作るため、
		// 何もしないと初回 Save 後にユーザーの tasks.md が 0600 に化けてしまう。
		if err := os.Chmod(tmp, info.Mode()); err != nil {
			return err
		}
		// Load 時点から外部で変更されていないか確認する。見逃すと、他のエディタで
		// 加えられた変更をここで丸ごと上書きしてしまう。
		if !info.ModTime().Equal(s.mtime) || info.Size() != s.size {
			return errors.New("tasks.md が外部で変更されている。R で再読み込みしてください")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Stat が not-exist なら外部で削除されただけなので、新規作成として進める。

	if err := os.Rename(tmp, targetPath); err != nil {
		return err
	}

	// 次の Save が今回の書き込みを「外部変更」と誤検知しないよう更新する。
	if info, err := os.Stat(targetPath); err == nil {
		s.mtime, s.size = info.ModTime(), info.Size()
	}
	return nil
}
