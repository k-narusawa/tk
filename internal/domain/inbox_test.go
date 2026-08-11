package domain

import "testing"

func pr(repo string, number int, role Role) Item {
	return Item{
		ID:     PRID(repo, number),
		Kind:   KindPR,
		Title:  "some pr",
		Repo:   repo,
		Number: number,
		Role:   role,
	}
}

// 自分の PR に自分がレビュー依頼されている場合、review 側を優先する。
func TestMergePRsPrefersReview(t *testing.T) {
	review := []Item{pr("app/payment", 412, RoleReview)}
	mine := []Item{pr("app/payment", 412, RoleMine), pr("app/stock", 409, RoleMine)}

	got := MergePRs(review, mine)
	if len(got) != 2 {
		t.Fatalf("重複排除後の件数 = %d, want 2", len(got))
	}
	if got[0].Role != RoleReview {
		t.Errorf("got[0].Role = %q, want %q", got[0].Role, RoleReview)
	}
	if got[1].ID != PRID("app/stock", 409) {
		t.Errorf("got[1].ID = %q, want %q", got[1].ID, PRID("app/stock", 409))
	}
}

func TestMergePRsKeepsOrderWithinGroup(t *testing.T) {
	review := []Item{pr("a/x", 1, RoleReview), pr("a/y", 2, RoleReview)}
	got := MergePRs(review, nil)
	if got[0].ID != PRID("a/x", 1) || got[1].ID != PRID("a/y", 2) {
		t.Errorf("グループ内の順序が変わった: %+v", got)
	}
}

func TestMergePRsEmpty(t *testing.T) {
	if got := MergePRs(nil, nil); len(got) != 0 {
		t.Errorf("空入力で %d 件返った", len(got))
	}
}

// 並び順: 未完了タスク → review PR → mine PR → 完了済みタスク
func TestSortInbox(t *testing.T) {
	tasks := []Item{
		{ID: TaskID(2), Kind: KindTask, Title: "未完了1"},
		{ID: TaskID(3), Kind: KindTask, Title: "完了", Done: true},
		{ID: TaskID(4), Kind: KindTask, Title: "未完了2"},
	}
	prs := []Item{
		pr("a/mine", 1, RoleMine),
		pr("a/review", 2, RoleReview),
	}

	got := SortInbox(tasks, prs)

	want := []ID{
		TaskID(2),           // 未完了タスク（書かれた順）
		TaskID(4),           //
		PRID("a/review", 2), // レビュー依頼された PR
		PRID("a/mine", 1),   // 自分の PR
		TaskID(3),           // 完了済みタスク
	}
	if len(got) != len(want) {
		t.Fatalf("件数 = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID != w {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, w)
		}
	}
}

// Role の値は RoleReview/RoleMine の2つに限らない（ゼロ値など）。
// 2本の PR ループが RoleReview / RoleMine で分岐していると、どちらにも
// 一致しない Role のアイテムが一覧から消える。
func TestSortInboxKeepsUnknownRole(t *testing.T) {
	prs := []Item{pr("a/x", 1, Role(""))}
	got := SortInbox(nil, prs)
	if len(got) != 1 || got[0].ID != PRID("a/x", 1) {
		t.Errorf("Role がゼロ値の PR が失われた: %+v", got)
	}
}

func TestSortInboxTasksOnly(t *testing.T) {
	tasks := []Item{{ID: TaskID(0), Kind: KindTask, Title: "だけ"}}
	got := SortInbox(tasks, nil)
	if len(got) != 1 || got[0].ID != TaskID(0) {
		t.Errorf("タスクのみの並び順が壊れた: %+v", got)
	}
}
