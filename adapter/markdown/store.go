package markdown

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/k-narusawa/tk/domain"
)

type Store struct{ path string }

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() (domain.TaskList, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		// 空ファイル相当。最初の Save で作る。
		return domain.Parse([]string{""}), nil
	}
	if err != nil {
		return domain.TaskList{}, err
	}
	return domain.Parse(strings.Split(string(data), "\n")), nil
}

// Save は一時ファイルに全文を書いてから os.Rename で被せる。
// 途中でプロセスが死んでもファイルが壊れない。
func (s *Store) Save(t domain.TaskList) error {
	f, err := os.CreateTemp(filepath.Dir(s.path), ".tk-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // Rename 成功後は存在しないので無害

	if _, err := f.WriteString(strings.Join(t.Render(), "\n")); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
