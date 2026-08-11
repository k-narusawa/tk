package gh

import (
	"bytes"
	"fmt"
	"os/exec"
)

// DiffCommand はページャで表示するので tea.ExecProcess で包む前提。
func DiffCommand(repo string, number int) *exec.Cmd {
	return exec.Command("gh", "pr", "diff", fmt.Sprint(number), "--repo", repo)
}

// RunWeb はブラウザを開いて即座に終了するので、ターミナルを明け渡す必要がない。
// stderr を wrapRunError に渡すので、未ログイン等の理由が bare な
// "exit status 1" ではなくユーザーに読める形で返る。
func RunWeb(repo string, number int) error {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "pr", "view", fmt.Sprint(number), "--repo", repo, "--web")
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return wrapRunError(err, stderr.Bytes(), nil)
	}
	return nil
}
