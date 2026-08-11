package domain

// MergePRs は2本のクエリ結果を repo と number で重複排除する。
// review を先に走査するので、両方に現れた PR は review 扱いになる。
func MergePRs(review, mine []Item) []Item {
	seen := make(map[ID]bool, len(review)+len(mine))
	out := make([]Item, 0, len(review)+len(mine))
	for _, group := range [][]Item{review, mine} {
		for _, it := range group {
			if seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, it)
		}
	}
	return out
}

// SortInbox の並び順は固定。ユーザーが変更する手段は用意しない。
// タスクは自分で書いた順序に意味があるので並べ替えない。
// PR は「他人を待たせているもの」を先に出す。
func SortInbox(tasks, prs []Item) []Item {
	out := make([]Item, 0, len(tasks)+len(prs))
	for _, it := range tasks {
		if !it.Done {
			out = append(out, it)
		}
	}
	for _, it := range prs {
		if it.Role == RoleReview {
			out = append(out, it)
		}
	}
	for _, it := range prs {
		if it.Role != RoleReview {
			out = append(out, it)
		}
	}
	for _, it := range tasks {
		if it.Done {
			out = append(out, it)
		}
	}
	return out
}
