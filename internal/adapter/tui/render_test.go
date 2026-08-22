package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 記号（**, ##, -）がそのまま見えているのが「読みにくい」の正体だったので、
// 描画後に残っていないことを確かめる。色そのものは端末依存なので見ない。
func TestRenderMarkdownRemovesMarkers(t *testing.T) {
	var r mdRenderer
	out := ansi.Strip(r.render("**太字**\n\n- 一つ目\n", 58))

	if strings.Contains(out, "**") {
		t.Errorf("強調の記号が残っている:\n%s", out)
	}
	if !strings.Contains(out, "太字") || !strings.Contains(out, "一つ目") {
		t.Errorf("本文が消えている:\n%s", out)
	}
}

// 描画結果がペインの幅を超えると、枠が膨らんで下のペインを押し出す。
// glamour は折り返し幅の外側に左右の余白を足すので、その分を引けているか。
func TestRenderMarkdownFitsWidth(t *testing.T) {
	const width = 40
	var r mdRenderer
	long := strings.Repeat("長い行になるように詰めた文章。", 20)

	for _, ln := range strings.Split(r.render(long, width), "\n") {
		if w := ansi.StringWidth(ln); w > width {
			t.Errorf("幅 %d を超えた行がある（%d 桁）: %q", width, w, ansi.Strip(ln))
		}
	}
}

// 狭すぎるときと空のときは素通しする。1文字ずつ折り返された結果より、
// 素の Markdown のほうが読める。
func TestRenderMarkdownPassesThroughWhenTooNarrow(t *testing.T) {
	var r mdRenderer
	if got := r.render("**太字**", 10); got != "**太字**" {
		t.Errorf("狭いときに描画してしまった: %q", got)
	}
	if got := r.render("", 58); got != "" {
		t.Errorf("空を描画してしまった: %q", got)
	}
}
