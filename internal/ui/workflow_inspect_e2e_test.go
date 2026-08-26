package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newInspectE2EModel(t *testing.T) *Model {
	t.Helper()
	r := newUIE2ERepo(t)
	r.write("story.txt", "one\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "inspection first")
	r.write("story.txt", "one\ntwo\n")
	r.git("add", "--", "story.txt")
	r.git("commit", "-m", "inspection second")
	r.write("story.txt", "one\ntwo\nworking\n")
	m := newE2EModel(t, r)
	sendE2EKey(t, m, keyMsg("f2"))
	return m
}

func sendInspectSequence(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		sendE2EKey(t, m, keyMsg(key))
	}
}

func assertInspectDetail(t *testing.T, m *Model, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(m.detail, value) {
			t.Fatalf("inspection detail omitted %q:\n%s", value, m.detail)
		}
	}
	if strings.ContainsAny(m.detail, "\x1b\x00") {
		t.Fatalf("inspection detail retained terminal controls: %q", m.detail)
	}
}

func TestInspectTopLevelFamiliesThroughModelUpdate(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{"d", []string{"d", "d"}, []string{"Unstaged diff", "+working"}},
		{"D", []string{"D", "g"}, []string{"Unstaged diff", "+working"}},
		{"l", []string{"l", "l"}, []string{"Log", "inspection second", "inspection first"}},
		{"y", []string{"y", "y"}, []string{"References", "Local branches", "main"}},
		{"Y", []string{"Y"}, []string{"Cherries", "inspection second"}},
		{"H", []string{"H"}, []string{"Section information", "Section:", "Repository operation: none"}},
		{"e", []string{"e"}, []string{"Terminal comparison (unified)", "+working"}},
		{"E", []string{"E", "u"}, []string{"Terminal comparison (unified): unstaged", "+working"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newInspectE2EModel(t)
			sendInspectSequence(t, m, test.keys...)
			assertInspectDetail(t, m, test.want...)
		})
	}
}

func TestInspectLogRefreshPropagatesLimitOptionThroughModelUpdate(t *testing.T) {
	m := newInspectE2EModel(t)
	sendInspectSequence(t, m, "L", "-", "n", "1")
	sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	sendInspectSequence(t, m, "g")
	assertInspectDetail(t, m, "Log", "inspection second")
	if strings.Contains(m.detail, "inspection first") {
		t.Fatalf("-n 1 was not propagated:\n%s", m.detail)
	}
}

func TestInspectionResultViewPagesWithoutLaunchingWork(t *testing.T) {
	m := newInspectE2EModel(t)
	sendInspectSequence(t, m, "l", "l")
	// Preserve the real query result and extend it beyond the viewport to exercise
	// the generic detail pane's paging boundary deterministically.
	m.detail += strings.Repeat("\ncontinued result", 100)
	before := m.detailOffset
	sendE2EKey(t, m, keyMsg("pgdown"))
	if m.busy || m.workflowLoading || m.detailOffset <= before {
		t.Fatalf("page down launched work or moved backwards: busy=%v loading=%v offset=%d", m.busy, m.workflowLoading, m.detailOffset)
	}
}
