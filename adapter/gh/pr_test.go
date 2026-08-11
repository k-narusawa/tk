package gh

import (
	"testing"

	"github.com/k-narusawa/tk/domain"
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

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
