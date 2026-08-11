package gh

import "testing"

func TestParseDetailPassing(t *testing.T) {
	data := []byte(`{"additions":304,"changedFiles":1,"deletions":0,"reviewRequests":[],
		"statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS","name":"test"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "passing" {
		t.Errorf("CI = %q, want passing", got.CI)
	}
	if got.Additions != 304 || got.Deletions != 0 || got.ChangedFiles != 1 {
		t.Errorf("差分 = +%d -%d (%d files)", got.Additions, got.Deletions, got.ChangedFiles)
	}
	if got.Reviews != "" {
		t.Errorf("Reviews = %q, want 空", got.Reviews)
	}
}

func TestParseDetailFailing(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"SUCCESS","name":"a"},
		{"status":"COMPLETED","conclusion":"FAILURE","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "failing" {
		t.Errorf("CI = %q, want failing", got.CI)
	}
}

// 走っている途中のチェックが1つでもあれば pending。
func TestParseDetailPending(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"SUCCESS","name":"a"},
		{"status":"IN_PROGRESS","conclusion":"","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "pending" {
		t.Errorf("CI = %q, want pending", got.CI)
	}
}

// 失敗が確定していれば、走行中があっても failing を優先する。
func TestParseDetailFailingBeatsPending(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"FAILURE","name":"a"},
		{"status":"IN_PROGRESS","conclusion":"","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "failing" {
		t.Errorf("CI = %q, want failing", got.CI)
	}
}

// CI が設定されていない PR は空文字。"passing" と嘘をつかない。
func TestParseDetailNoChecks(t *testing.T) {
	got, err := parseDetail([]byte(`{"statusCheckRollup":[],"reviewRequests":[]}`))
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "" {
		t.Errorf("CI = %q, want 空", got.CI)
	}
}

// SKIPPED / NEUTRAL は失敗扱いにしない。
func TestParseDetailSkippedIsPassing(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"SKIPPED","name":"a"},
		{"status":"COMPLETED","conclusion":"NEUTRAL","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "passing" {
		t.Errorf("CI = %q, want passing", got.CI)
	}
}

func TestParseDetailReviewCount(t *testing.T) {
	data := []byte(`{"reviewRequests":[{"login":"alice"},{"login":"bob"}],"statusCheckRollup":[]}`)
	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.Reviews != "2人待ち" {
		t.Errorf("Reviews = %q, want 2人待ち", got.Reviews)
	}
}

func TestParseDetailBrokenJSON(t *testing.T) {
	if _, err := parseDetail([]byte(`{`)); err == nil {
		t.Error("壊れた JSON でエラーが返らなかった")
	}
}

// StatusContext は CheckRun と違い status/conclusion を持たず state を持つ。
func TestParseDetailStatusContextSuccess(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[{"state":"SUCCESS","name":"ci/build"}]}`)
	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "passing" {
		t.Errorf("CI = %q, want passing", got.CI)
	}
}

func TestParseDetailStatusContextFailure(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[{"state":"FAILURE","name":"ci/build"}]}`)
	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "failing" {
		t.Errorf("CI = %q, want failing", got.CI)
	}
}

func TestParseDetailStatusContextError(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[{"state":"ERROR","name":"ci/build"}]}`)
	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "failing" {
		t.Errorf("CI = %q, want failing", got.CI)
	}
}

func TestParseDetailStatusContextPending(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[{"state":"PENDING","name":"ci/build"}]}`)
	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "pending" {
		t.Errorf("CI = %q, want pending", got.CI)
	}
}

// CheckRun が成功していても StatusContext が失敗していれば failing を優先する。
func TestParseDetailMixedRollupFailingWins(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"SUCCESS","name":"a"},
		{"state":"FAILURE","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "failing" {
		t.Errorf("CI = %q, want failing", got.CI)
	}
}

// StatusContext の status は空文字なので、CheckRun 用の pending 判定に
// 引っかけてはいけない。CheckRun/StatusContext ともに成功なら passing。
func TestParseDetailMixedRollupPassing(t *testing.T) {
	data := []byte(`{"statusCheckRollup":[
		{"status":"COMPLETED","conclusion":"SUCCESS","name":"a"},
		{"state":"SUCCESS","name":"b"}]}`)

	got, err := parseDetail(data)
	if err != nil {
		t.Fatalf("parseDetail() error = %v", err)
	}
	if got.CI != "passing" {
		t.Errorf("CI = %q, want passing", got.CI)
	}
}
