package markdown

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// DetailStore はタスク1件ぶんの詳細ファイルを読む。Store と違って
// mtime/size の状態を持たないので、並行に呼んでも安全。
type DetailStore struct{ dir string }

// NewDetailStore は tasks.md のパスから詳細ディレクトリを決める。
// 専用の環境変数を増やさないための導出。
//
//	~/.config/tk/tasks.md → ~/.config/tk/tasks/
//	/tmp/t.md             → /tmp/t/
//
// 拡張子が無いと落とした結果がファイル自身と衝突するので、その場合だけ
// ".d" を足す。衝突したままだと MkdirAll がファイルにぶつかって失敗する。
func NewDetailStore(tasksFile string) *DetailStore {
	// filepath.Ext は ".tasks" のような dotfile 名を丸ごと拡張子とみなす。
	// そのまま落とすと dir が親ディレクトリまで縮み、詳細ファイルが
	// $HOME に散らばる。
	base := filepath.Base(tasksFile)
	ext := filepath.Ext(base)
	if ext == base {
		ext = ""
	}
	dir := strings.TrimSuffix(tasksFile, ext)
	if dir == tasksFile {
		dir += ".d"
	}
	return &DetailStore{dir: dir}
}

func (d *DetailStore) Dir() string { return d.dir }

// maxNameBytes はファイル名の上限。macOS / Linux の 255 バイト。
const maxNameBytes = 255

// detailName はタスクのタイトルを詳細ファイルの名前に変換する。
// タイトルがそのまま読めることを優先し、使えない文字だけを潰す。
func detailName(title string) string {
	s := strings.TrimSpace(title)
	s = strings.NewReplacer("/", "-", "\x00", "-").Replace(s)
	s = strings.TrimSpace(s)
	// 先頭のドットは隠しファイルになるので潰す。ls で見えないと存在を忘れる。
	if strings.HasPrefix(s, ".") {
		s = "-" + s[1:]
	}
	s = truncateBytes(s, maxNameBytes-len(".md"))
	if s == "" {
		s = "_"
	}
	return s + ".md"
}

// truncateBytes は UTF-8 のルーン境界を割らずに n バイト以内へ切り詰める。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Body は詳細の本文。ファイルが無ければ空文字を返す。詳細を持たない
// タスクのほうが普通なので、未作成をエラーにしない。
//
// カーソル移動のたびに bubbletea の Update から同期的に呼ぶので、遅い
// ファイルシステム（ネットワークマウント、クラウド同期フォルダ）や
// 巨大な詳細ファイルは UI を止める。直すなら tea.Cmd に読み込みを逃がす。
func (d *DetailStore) Body(title string) (string, error) {
	data, err := os.ReadFile(filepath.Join(d.dir, detailName(title)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EditPath は e が開くパス。tk は詳細を書かないのでファイルは作らないが、
// 親ディレクトリだけは作る。エディタは親が無いと保存に失敗する。
func (d *DetailStore) EditPath(title string) (string, error) {
	// ディレクトリ内のファイル名がタスクのタイトルそのものなので、0o755 だと
	// ls ~someone/.config/tk/tasks だけで一覧が漏れる。個々のファイルの
	// モードとは無関係にディレクトリ自体を締める。
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(d.dir, detailName(title)), nil
}
