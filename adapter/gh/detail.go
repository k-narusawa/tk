package gh

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/k-narusawa/tk/domain"
)

type DetailSource struct{}

func NewDetailSource() *DetailSource { return &DetailSource{} }

func (d *DetailSource) Detail(ctx context.Context, repo string, number int) (domain.PRDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := run(ctx,
		"pr", "view", fmt.Sprint(number),
		"--repo", repo,
		"--json", "statusCheckRollup,reviewRequests,additions,deletions,changedFiles",
	)
	if err != nil {
		return domain.PRDetail{}, err
	}
	return parseDetail(out)
}

type detailResult struct {
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`

	ReviewRequests []struct {
		Login string `json:"login"`
	} `json:"reviewRequests"`

	StatusCheckRollup []struct {
		Status     string `json:"status"`     // CheckRun
		Conclusion string `json:"conclusion"` // CheckRun
		State      string `json:"state"`      // StatusContext
	} `json:"statusCheckRollup"`
}

func parseDetail(data []byte) (domain.PRDetail, error) {
	var r detailResult
	if err := json.Unmarshal(data, &r); err != nil {
		return domain.PRDetail{}, fmt.Errorf("gh の JSON を読めない: %w", err)
	}

	d := domain.PRDetail{
		CI:           rollupState(r),
		Additions:    r.Additions,
		Deletions:    r.Deletions,
		ChangedFiles: r.ChangedFiles,
	}
	if n := len(r.ReviewRequests); n > 0 {
		d.Reviews = fmt.Sprintf("%d人待ち", n)
	}
	return d, nil
}

// rollupState は failing > pending > passing の優先順で判定する。
// CheckRun は status/conclusion、StatusContext は state を持つ。
// チェックが1つも無い PR は空文字を返す（passing と嘘をつかない）。
func rollupState(r detailResult) string {
	if len(r.StatusCheckRollup) == 0 {
		return ""
	}

	pending := false
	for _, c := range r.StatusCheckRollup {
		switch c.Conclusion {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "failing"
		}
		switch c.State {
		case "FAILURE", "ERROR":
			return "failing"
		}
		// CheckRun: status が COMPLETED 以外なら実行中。
		// StatusContext は status が空なので、ここで pending 扱いしてはいけない。
		if c.Status != "" && c.Status != "COMPLETED" {
			pending = true
		}
		switch c.State {
		case "PENDING", "EXPECTED":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "passing"
}
