package gh

import (
	"fmt"
	"os/exec"
)

// DiffCommand はページャで表示するので tea.ExecProcess で包む前提。
func DiffCommand(repo string, number int) *exec.Cmd {
	return exec.Command("gh", "pr", "diff", fmt.Sprint(number), "--repo", repo)
}

// WebCommand はブラウザを開いて即座に終了するので、
// ターミナルを明け渡す必要がない。
func WebCommand(repo string, number int) *exec.Cmd {
	return exec.Command("gh", "pr", "view", fmt.Sprint(number), "--repo", repo, "--web")
}
