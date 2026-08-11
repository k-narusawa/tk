package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/k-narusawa/tk/domain"
)

// timeout は gh の打ち切り時間。遅いネットワークで TUI を待たせない。
const timeout = 10 * time.Second

type PRSource struct{}

func NewPRSource() *PRSource { return &PRSource{} }

func searchArgs(role domain.Role) []string {
	filter := "--author=@me"
	if role == domain.RoleReview {
		filter = "--review-requested=@me"
	}
	return []string{
		"search", "prs",
		"--state=open",
		filter,
		"--json", "number,title,repository,url",
	}
}

func (s *PRSource) Fetch(ctx context.Context, role domain.Role) ([]domain.Item, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := run(ctx, searchArgs(role)...)
	if err != nil {
		return nil, err
	}
	return parsePRs(out, role)
}

type searchResult struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

func parsePRs(data []byte, role domain.Role) ([]domain.Item, error) {
	var results []searchResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("gh の JSON を読めない: %w", err)
	}

	items := make([]domain.Item, 0, len(results))
	for _, r := range results {
		repo := r.Repository.NameWithOwner
		items = append(items, domain.Item{
			ID:     domain.PRID(repo, r.Number),
			Kind:   domain.KindPR,
			Title:  r.Title,
			Repo:   repo,
			Number: r.Number,
			URL:    r.URL,
			Role:   role,
		})
	}
	return items, nil
}

// run は gh の stderr をそのままエラーに含める。未ログインや PATH 不在の
// 理由がユーザーに見えるようにするため。
func run(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh: %s", msg)
		}
		return nil, fmt.Errorf("gh: %w", err)
	}
	return stdout.Bytes(), nil
}
