package gh

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/k-narusawa/tk/internal/domain"
)

// gh search prs --state=open --author=@me --json number,title,repository,url の実出力
const searchJSON = `[
  {"number":412,"repository":{"name":"payment","nameWithOwner":"app/payment"},"title":"fix: 決済のnull落ち","url":"https://github.com/app/payment/pull/412"},
  {"number":409,"repository":{"name":"stock","nameWithOwner":"app/stock"},"title":"feat: 在庫API","url":"https://github.com/app/stock/pull/409"}
]`

func TestParsePRs(t *testing.T) {
	got, err := parsePRs([]byte(searchJSON), domain.RoleReview)
	if err != nil {
		t.Fatalf("parsePRs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2", len(got))
	}

	want := domain.Item{
		ID:     domain.PRID("app/payment", 412),
		Kind:   domain.KindPR,
		Title:  "fix: 決済のnull落ち",
		Repo:   "app/payment",
		Number: 412,
		URL:    "https://github.com/app/payment/pull/412",
		Role:   domain.RoleReview,
	}
	if got[0] != want {
		t.Errorf("got[0] = %+v\nwant  = %+v", got[0], want)
	}
	if got[1].Role != domain.RoleReview {
		t.Errorf("got[1].Role = %q, want %q", got[1].Role, domain.RoleReview)
	}
}

func TestParsePRsEmpty(t *testing.T) {
	got, err := parsePRs([]byte(`[]`), domain.RoleMine)
	if err != nil {
		t.Fatalf("parsePRs() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("件数 = %d, want 0", len(got))
	}
}

// repository が空の要素はエラーにする。PRID("", n) は他のリポジトリの
// PR とID衝突して MergePRs に一方を消され、しかも空 Repo は
// gh pr view --repo "" のようにその後の操作でも使い物にならない。
func TestParsePRsEmptyRepoErrors(t *testing.T) {
	data := []byte(`[{"number":1,"title":"t","repository":{"nameWithOwner":""},"url":"https://x"}]`)
	if _, err := parsePRs(data, domain.RoleMine); err == nil {
		t.Error("repository が空でエラーが返らなかった")
	}
}

func TestParsePRsBrokenJSON(t *testing.T) {
	if _, err := parsePRs([]byte(`not json`), domain.RoleMine); err == nil {
		t.Error("壊れた JSON でエラーが返らなかった")
	}
}

func TestSearchArgsByRole(t *testing.T) {
	tests := []struct {
		role domain.Role
		want string
	}{
		{domain.RoleReview, "--review-requested=@me"},
		{domain.RoleMine, "--author=@me"},
	}
	for _, tt := range tests {
		args := searchArgs(tt.role)
		if !contains(args, tt.want) {
			t.Errorf("searchArgs(%q) = %v, %q が含まれない", tt.role, args, tt.want)
		}
	}
}

func TestWrapRunError(t *testing.T) {
	_, missingBinErr := exec.LookPath("tk-nonexistent-binary")

	tests := []struct {
		name      string
		err       error
		stderr    string
		ctxErr    error
		wantIn    string
		wantNotIn string
	}{
		{
			name:   "stderrあり タイムアウトでない",
			err:    errors.New("exit status 1"),
			stderr: "gh: not logged in\n",
			ctxErr: nil,
			wantIn: "not logged in",
		},
		{
			name:      "stderrあり タイムアウトでも stderr が優先される",
			err:       errors.New("signal: killed"),
			stderr:    "gh: rate limited\n",
			ctxErr:    context.DeadlineExceeded,
			wantIn:    "rate limited",
			wantNotIn: "タイムアウト",
		},
		{
			name:   "stderr無し タイムアウト",
			err:    errors.New("signal: killed"),
			stderr: "",
			ctxErr: context.DeadlineExceeded,
			wantIn: "タイムアウト",
		},
		{
			name:   "stderr無し タイムアウトでもない",
			err:    missingBinErr,
			stderr: "",
			ctxErr: nil,
			wantIn: missingBinErr.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapRunError(tt.err, []byte(tt.stderr), tt.ctxErr)
			if !strings.Contains(got.Error(), tt.wantIn) {
				t.Errorf("wrapRunError() = %q, %q を含んでいない", got, tt.wantIn)
			}
			if tt.wantNotIn != "" && strings.Contains(got.Error(), tt.wantNotIn) {
				t.Errorf("wrapRunError() = %q, %q を含んではいけない", got, tt.wantNotIn)
			}
		})
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
