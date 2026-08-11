package gh

import (
	"reflect"
	"testing"
)

func TestDiffCommandArgs(t *testing.T) {
	cmd := DiffCommand("app/payment", 412)
	want := []string{"gh", "pr", "diff", "412", "--repo", "app/payment"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}

func TestWebCommandArgs(t *testing.T) {
	cmd := WebCommand("app/payment", 412)
	want := []string{"gh", "pr", "view", "412", "--repo", "app/payment", "--web"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}
