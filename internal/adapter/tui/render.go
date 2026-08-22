package tui

import (
	"strings"

	"charm.land/glamour/v2"
)

// mdRenderer は Markdown を端末向けに描画する。glamour のレンダラは幅ごとに
// 使い回す。スタイル定義の読み込みが毎回走るのに対し、詳細ペインの描画は
// カーソルを動かすたびに走るため。
type mdRenderer struct {
	r     *glamour.TermRenderer
	width int
}

// mdMinWidth は描画をあきらめる幅。glamour は見出しや引用に接頭辞と余白を
// 足すので、狭すぎると1文字ずつ折り返して読めなくなる。素の Markdown を
// そのまま出したほうがまし。
const mdMinWidth = 20

// render は失敗しても元の文字列を返す。色が付かないだけで内容は読めるので、
// 詳細ペインを空にするより良い。
func (m *mdRenderer) render(md string, width int) string {
	if strings.TrimSpace(md) == "" || width < mdMinWidth {
		return md
	}
	if m.r == nil || m.width != width {
		// 左右の余白（既定 2 桁ずつ）は折り返し幅の外側に付くので、その分を
		// 引いておかないとペインの幅を超えて枠が崩れる。
		r, err := glamour.NewTermRenderer(
			glamour.WithEnvironmentConfig(), // GLAMOUR_STYLE、無ければ dark
			glamour.WithWordWrap(width-4),
		)
		if err != nil {
			return md
		}
		m.r, m.width = r, width
	}
	out, err := m.r.Render(md)
	if err != nil {
		return md
	}
	// glamour は前後に空行を付ける。詳細ペインはヘッダ行の直後から始まるので、
	// そのぶんの隙間は呼び出し側で決める。
	return strings.Trim(out, "\n")
}
