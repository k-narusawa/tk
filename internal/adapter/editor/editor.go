// Package editor はタスクの詳細を書くためのエディタ起動コマンドを組み立てる。
package editor

import (
	"errors"
	"os/exec"
	"strings"
)

// Command は詳細ファイルを開く *exec.Cmd を返す。1タスク1ファイルなので
// 行ジャンプは要らない。"+N" を渡さないぶん、vi 系以外のエディタでも
// 引数がそのまま通る。tea.ExecProcess で包むのは adapter/tui の仕事。
func Command(editorCmd, path string) (*exec.Cmd, error) {
	fields := strings.Fields(editorCmd)
	if len(fields) == 0 {
		return nil, errors.New("エディタが指定されていない")
	}
	return exec.Command(fields[0], append(fields[1:], path)...), nil
}
