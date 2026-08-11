package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/k-narusawa/tk/domain"
	"github.com/k-narusawa/tk/usecase"
)

type fakeStore struct {
	list  domain.TaskList
	saved []domain.TaskList
}

func (f *fakeStore) Load() (domain.TaskList, error) { return f.list, nil }

func (f *fakeStore) Save(t domain.TaskList) error {
	f.saved = append(f.saved, t)
	f.list = t
	return nil
}

func taskList(s string) domain.TaskList { return domain.Parse(strings.Split(s, "\n")) }

func newTestModel(t *testing.T, store *fakeStore) Model {
	t.Helper()
	inbox := usecase.NewInbox(store, nil, nil)
	if err := inbox.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return New(inbox)
}

func TestWindowSizeRendersNonEmptyView(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	got, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = got.(Model)

	v := m.View()
	if v.Content == "" {
		t.Error("View().Content が空")
	}
}

func TestJKMovesCursor(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] 一\n- [ ] 二\n- [ ] 三\n")})

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Fatalf("j 後の cursor = %d, want 1", m.cursor)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = got.(Model)
	if m.cursor != 2 {
		t.Fatalf("j 後の cursor = %d, want 2", m.cursor)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	m = got.(Model)
	if m.cursor != 1 {
		t.Fatalf("k 後の cursor = %d, want 1", m.cursor)
	}
}

func TestSpaceTogglesTask(t *testing.T) {
	store := &fakeStore{list: taskList("- [ ] やること\n")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m = got.(Model)

	if len(store.saved) == 0 {
		t.Fatal("Save が呼ばれていない")
	}
	last := store.saved[len(store.saved)-1]
	rendered := strings.Join(last.Render(), "\n")
	if !strings.Contains(rendered, "- [x]") {
		t.Errorf("保存内容に - [x] が無い: %q", rendered)
	}
}

func TestNAddsTaskViaTextinput(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)
	if !m.adding {
		t.Fatal("n 後に adding が true になっていない")
	}

	for _, r := range "新規タスク" {
		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = got.(Model)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = got.(Model)

	if m.adding {
		t.Error("enter 後も adding が true のまま")
	}
	if len(store.saved) == 0 {
		t.Fatal("Save が呼ばれていない")
	}
	rendered := strings.Join(store.saved[len(store.saved)-1].Render(), "\n")
	if !strings.Contains(rendered, "新規タスク") {
		t.Errorf("保存内容に新規タスクが無い: %q", rendered)
	}
}

// 空白だけのタイトルは、余計な "- [ ] " 行を作るので保存しない。
func TestNRejectsWhitespaceOnlyTitle(t *testing.T) {
	store := &fakeStore{list: taskList("")}
	m := newTestModel(t, store)

	got, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = got.(Model)

	for _, r := range "   " {
		got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		m = got.(Model)
	}

	got, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = got.(Model)

	if m.adding {
		t.Error("enter 後も adding が true のまま")
	}
	if len(store.saved) != 0 {
		t.Errorf("空白のみのタイトルで Save が呼ばれた: %+v", store.saved)
	}
}

// 起動直後、最初の WindowSizeMsg が来る前に bubbletea は View() を呼ぶ。
// このとき width/height は 0 で、right = width-left-4 は負になる。
func TestViewBeforeWindowSize(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	if m.width != 0 || m.height != 0 {
		t.Fatalf("前提が崩れている: width=%d height=%d", m.width, m.height)
	}
	v := m.View() // panic しないこと
	if v.Content == "" {
		t.Error("View().Content が空")
	}
}

// 極端に小さい端末でも落ちないこと
func TestViewTinyTerminal(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	for _, size := range [][2]int{{1, 1}, {2, 2}, {5, 3}, {0, 30}, {100, 0}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mm := updated.(Model)
		if v := mm.View(); v.Content == "" {
			t.Errorf("size %v で View().Content が空", size)
		}
	}
}

func TestQReturnsQuitCmd(t *testing.T) {
	m := newTestModel(t, &fakeStore{list: taskList("- [ ] やること\n")})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if cmd == nil {
		t.Fatal("q で cmd が nil")
	}
}
