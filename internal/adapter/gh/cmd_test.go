package gh

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDiffCommandArgs(t *testing.T) {
	cmd := DiffCommand("app/payment", 412)
	want := []string{"gh", "pr", "diff", "412", "--repo", "app/payment"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}

// RunWeb は実プロセスを起動するので、gh の失敗を模した stderr を
// wrapRunError（RunWeb が使うのと同じ関数）に直接通して検証する。
func TestRunWebErrorIncludesStderr(t *testing.T) {
	err := wrapRunError(errors.New("exit status 1"), []byte("gh: not logged in\n"), nil)
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %q, stderr が含まれていない", err)
	}
}
