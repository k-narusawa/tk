// Package editor はタスクの詳細を書くためのエディタ起動コマンドを組み立てる。
package editor

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Command は tasks.md の指定行を開く *exec.Cmd を返す。line は 0 始まりの
// 行番号（domain.TaskID と同じ）で、エディタに渡す "+N" は 1 始まり。
// "+N" は vi 系の記法。それ以外のエディタでは行ジャンプが効かない。
// tea.ExecProcess で包むのは adapter/tui の仕事。
func Command(editorCmd, path string, line int) (*exec.Cmd, error) {
	fields := strings.Fields(editorCmd)
	if len(fields) == 0 {
		return nil, errors.New("エディタが指定されていない")
	}
	args := append(fields[1:], fmt.Sprintf("+%d", max(1, line+1)), path)
	return exec.Command(fields[0], args...), nil
}
