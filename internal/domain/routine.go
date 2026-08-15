package domain

import (
	"regexp"
	"strings"
)

var routineRe = regexp.MustCompile(`^\s*-\s+(.*)$`)

// ParseRoutines は routines.md から監視項目を1行1件で拾う。TaskList と違って
// 全行を保持しないのは、tk が routines.md に書き戻さないため。追加・削除は
// エディタでやる。
//
// 名前は結果ファイルの名前と実行状態のキーを兼ねるので、重複は捨てる。
func ParseRoutines(lines []string) []Item {
	var out []Item
	seen := map[string]bool{}
	for _, line := range lines {
		m := routineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, Item{ID: RoutineID(name), Kind: KindRoutine, Title: name})
	}
	return out
}
