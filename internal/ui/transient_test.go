package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestTransientCatalogImplementedSuffixManifest(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	m.snapshot.summary.Branch, m.snapshot.summary.Upstream = "main", "origin/main"
	for prefix := range prefixCatalogs {
		catalog, _ := m.transientCatalog(prefix)
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				if entry.Kind == keymap.KindSuffix && !entry.Prefix {
					_, installed := m.workflowHandlers[entry.Command]
					if entry.Available && !installed || entry.Active && entry.Available != installed {
						t.Errorf("%s %s availability=%v, installed=%v (%s)", prefix, entry.Key, entry.Available, installed, entry.Reason)
					}
				}
				if prefix == "f" && entry.Key == "f" {
					t.Fatal("f f must be absent from the catalog")
				}
			}
		}
	}
}

func TestTransientMenusHaveNoRegistryDrift(t *testing.T) {
	for prefix, catalog := range prefixCatalogs {
		var name string
		for _, top := range keymap.BindingsFor(keymap.SchemeVim, keymap.ContextStatus) {
			if strings.Join(top.Sequence, " ") == prefix {
				name = top.UpstreamCommand
				break
			}
		}
		registered := keymap.BindingsForTransient(keymap.SchemeVim, name)
		identities := map[string]int{}
		for _, binding := range registered {
			identities[strings.Join(binding.Sequence[1:], " ")+"\x00"+binding.UpstreamCommand+"\x00"+string(binding.Kind)+"\x00"+binding.Group+"\x00"+strings.Join(binding.Conditions, "\x01")]++
		}
		seen := 0
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				seen++
				identity := entry.Key + "\x00" + entry.UpstreamCommand + "\x00" + string(entry.Kind) + "\x00" + group.Title + "\x00" + strings.Join(entry.Conditions, "\x01")
				identities[identity]--
				if !entry.Available && (entry.Category == "" || entry.Reason == "") {
					t.Errorf("%s %s is implicitly grey: %+v", prefix, entry.Key, entry)
				}
			}
		}
		if seen != len(registered) {
			t.Errorf("%s menu has %d entries, registry has %d", prefix, seen, len(registered))
		}
		for identity, count := range identities {
			if count != 0 {
				t.Errorf("%s occurrence multiset drift for %q: %d", prefix, identity, count)
			}
		}
	}
}

func TestAllManifestTransientsProduceExactRuntimeCatalogs(t *testing.T) {
	m := New(&gitbackend.Repository{})
	occurrences := 0
	for _, tr := range keymap.Transients() {
		catalog, ok := m.transientCatalog(tr.Name)
		if !ok || catalog.Title == "" {
			t.Fatalf("%s has no titled runtime catalog", tr.Name)
		}
		got := 0
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				got++
				if !entry.Available && (entry.Category == "" || entry.Reason == "") {
					t.Errorf("%s has implicit grey row: %+v", tr.Name, entry)
				}
			}
		}
		want := len(keymap.BindingsForTransient(keymap.SchemeVim, tr.Name))
		if got != want {
			t.Errorf("%s catalog=%d manifest=%d", tr.Name, got, want)
		}
		occurrences += got
	}
	if occurrences != 554 {
		t.Fatalf("runtime catalog occurrences=%d, want 554", occurrences)
	}
}

func TestDispatcherRendererMatchesMagitStructureAndDimensions(t *testing.T) {
	catalog := dispatcherCatalog(schemeVim)
	for _, section := range catalog {
		for _, column := range section.Columns {
			for _, entry := range column {
				if !entry.Available && (entry.Category == "" || entry.Reason == "") {
					t.Errorf("implicitly grey dispatcher entry: %+v", entry)
				}
				if entry.Category != menuEntryRegistry && entry.Category != menuEntryMissing && entry.Category != menuEntryContext && entry.Category != menuEntryPresentation {
					t.Errorf("untyped dispatcher source: %+v", entry)
				}
			}
		}
	}
	for _, width := range []int{20, 40, 60, 95, 120} {
		for _, height := range []int{1, 2, 3, 5, 8, 14, 24} {
			got := renderDispatcher(catalog, width, height, 0)
			if w, h := lipgloss.Width(got), lipgloss.Height(got); w != width || h != height {
				t.Errorf("dispatcher %dx%d rendered %dx%d", width, height, w, h)
			}
		}
	}

	wide := ansi.Strip(renderDispatcher(catalog, 120, 30, 0))
	if strings.ContainsAny(wide, "╔╗╚╝║═") {
		t.Fatalf("dispatcher must be borderless: %q", wide)
	}
	for _, heading := range []string{"Transient and dwim commands", "Applying changes", "Essential commands"} {
		if !strings.Contains(wide, heading) {
			t.Fatalf("dispatcher omitted heading %q", heading)
		}
	}
	firstEntries := strings.Split(wide, "\n")[1]
	for _, entry := range []string{"A Apply", "i Ignore", "r Rebase"} {
		if !strings.Contains(firstEntries, entry) {
			t.Fatalf("source columns were not aligned side-by-side in %q", firstEntries)
		}
	}
	if strings.Contains(firstEntries, "b × Branch") || !strings.Contains(wide, "g Refresh current buffer") || !strings.Contains(wide, "× unavailable") {
		t.Fatalf("implemented/unavailable markers are inaccurate: %q", wide)
	}
}

func TestTransientCompactFallbackPrioritizesAvailableSuffixes(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4} {
		got := ansi.Strip(renderTransient(prefixCatalogs["f"], 60, height, 0))
		if lipgloss.Width(got) != 60 || lipgloss.Height(got) != height {
			t.Fatalf("compact fallback %dx%d has wrong dimensions", 60, height)
		}
		if !strings.Contains(got, "p push remote") {
			t.Fatalf("compact fallback omitted implemented suffixes: %q", got)
		}
		if strings.ContainsAny(got, "╔╗╚╝") {
			t.Fatalf("compact fallback should be borderless: %q", got)
		}
	}
}

func TestTransientCompactFallbackPagesEveryAvailableSuffix(t *testing.T) {
	catalog := prefixCatalogs["f"]
	maximum := transientMaximumOffset(catalog, 20, 1)
	var pages string
	for offset := 0; offset <= maximum; offset++ {
		pages += "\n" + ansi.Strip(renderTransient(catalog, 20, 1, offset))
	}
	for _, suffix := range []string{"p push", "u upstream", "e elsewhere", "a all remot"} {
		if !strings.Contains(pages, suffix) {
			t.Fatalf("compact pages omitted %q: %q", suffix, pages)
		}
	}
	available := 0
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if entry.Available {
				available++
			}
		}
	}
	if top := ansi.Strip(renderTransient(catalog, 20, 1, 0)); !strings.Contains(top, "↓1-1/"+strconv.Itoa(available)) {
		t.Fatalf("compact first page has no continuation marker: %q", top)
	}
	last := strconv.Itoa(available)
	if bottom := ansi.Strip(renderTransient(catalog, 20, 1, maximum)); !strings.Contains(bottom, "↑"+last+"-"+last+"/"+last) {
		t.Fatalf("compact last page has no continuation marker: %q", bottom)
	}
}

func TestDispatcherHintsAndPagingReachBottom(t *testing.T) {
	catalog := dispatcherCatalog(schemeVim)
	top := ansi.Strip(renderDispatcher(catalog, 60, 8, 0))
	if !strings.Contains(top, "q/Esc close") || !strings.Contains(top, "PageUp/PageDown") || !strings.Contains(top, "1-") {
		t.Fatalf("top hint is incomplete: %q", top)
	}
	bottom := ansi.Strip(renderDispatcher(catalog, 60, 8, 10000))
	if !strings.Contains(bottom, "C-x i × Show Info manual") {
		t.Fatalf("bottom page did not retain the final essential entry: %q", bottom)
	}
	all := ""
	maximum := dispatcherMaximumOffset(catalog, 20, 5)
	for offset := 0; offset <= maximum; offset++ {
		all += ansi.Strip(renderDispatcher(catalog, 20, 5, offset))
	}
	if !strings.Contains(all, "! Run") || !strings.Contains(all, "C-x i × Show Info") {
		t.Fatalf("narrow paging made commands unreachable: %q", all)
	}
}

func TestDispatcherUsesSchemeApplicableDiscardBinding(t *testing.T) {
	ctx := keymap.Context{View: keymap.ViewStatus, Section: keymap.SectionUnstaged}
	vim := ansi.Strip(renderDispatcher(dispatcherCatalog(schemeVim, ctx), 120, 30, 0))
	magit := ansi.Strip(renderDispatcher(dispatcherCatalog(schemeMagit, ctx), 120, 30, 0))
	if !strings.Contains(vim, "x Discard") || strings.Contains(vim, "k Discard") || !strings.Contains(magit, "k Discard") || strings.Contains(magit, "x Discard") {
		t.Fatalf("discard bindings do not follow scheme\nvim: %q\nmagit: %q", vim, magit)
	}
}

func TestPrefixTransientInteraction(t *testing.T) {
	for _, cancel := range []tea.KeyPressMsg{keyMsg("q"), tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})} {
		m := New(&gitbackend.Repository{})
		m.width, m.height = 60, 16
		_, _ = m.Update(keyMsg("f"))
		if m.resolver.PendingPrefix() != "f" || !strings.Contains(ansi.Strip(m.render()), "Fetch") {
			t.Fatal("f did not immediately open its transient")
		}
		_, cmd := m.Update(cancel)
		if cmd != nil || m.resolver.PendingPrefix() != "" {
			t.Fatalf("%s did not cancel transient", cancel.String())
		}
	}

	m := New(&gitbackend.Repository{})
	m.width, m.height, m.loading = 40, 10, false
	before := m.tree.Cursor()
	_, _ = m.Update(keyMsg("b"))
	_, cmd := m.Update(keyMsg("h"))
	if cmd != nil || m.resolver.PendingPrefix() != "b" || m.tree.Cursor() != before || !strings.Contains(m.message, "not implemented") {
		t.Fatal("unavailable suffix did not remain open without navigation")
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.transientOffset == 0 || m.transientOffset > transientMaximumOffset(prefixCatalogs["b"], m.width, m.height-4) {
		t.Fatal("PageDown did not scroll a transient")
	}
	_, _ = m.Update(keyMsg("f2"))
	if m.resolver.PendingPrefix() != "" || m.scheme != schemeMagit {
		t.Fatal("F2 did not cancel the transient and change scheme")
	}
}

func TestAvailableSuffixDispatchesAndHelpTransitionsToPrefix(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.loading = 80, 20, false
	_, _ = m.Update(keyMsg("c"))
	_, cmd := m.Update(keyMsg("c"))
	if cmd == nil || m.resolver.PendingPrefix() != "" {
		t.Fatal("c c did not dispatch the reviewed commit workflow")
	}
	_, _ = m.Update(cmd())
	if m.mode != modeWorkflow || m.mode == modeCommit {
		t.Fatalf("c c used obsolete commit mode: mode=%d", m.mode)
	}
	m.setMode(modeStatus)
	_, _ = m.Update(keyMsg("?"))
	_, _ = m.Update(keyMsg("P"))
	if m.mode != modeStatus || m.resolver.PendingPrefix() != "P" || !strings.Contains(ansi.Strip(m.render()), "Push") {
		t.Fatal("help did not transition directly into the push transient")
	}
}

func TestDispatcherDispatchesRefreshAndApplicableChange(t *testing.T) {
	refresh := New(nil)
	refresh.width, refresh.height, refresh.loading = 80, 20, false
	_, _ = refresh.Update(keyMsg("?"))
	_, cmd := refresh.Update(keyMsg("g"))
	if cmd == nil || refresh.mode != modeStatus || !refresh.busy {
		t.Fatal("g did not close the dispatcher and refresh")
	}

	stage := New(&gitbackend.Repository{})
	stage.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "work.txt", Unstaged: gitbackend.ChangeModified}}}})
	stage.loading = false
	for id, row := range stage.rows {
		if row.path == "work.txt" {
			stage.tree.SetCursor(id)
		}
	}
	_, _ = stage.Update(keyMsg("?"))
	_, cmd = stage.Update(keyMsg("s"))
	if cmd == nil || stage.mode != modeStatus || !stage.busy {
		t.Fatal("s did not close the dispatcher and stage the selected change")
	}
}

func TestDispatcherCanonicalTabAndContextualChangeReasons(t *testing.T) {
	entry, ok := dispatcherEntry(dispatcherCatalog(schemeVim), "tab")
	if !ok || entry.Display != "Tab" || entry.Command != keymap.CommandToggleSection || !entry.Available {
		t.Fatalf("Tab entry = %+v, found=%v", entry, ok)
	}
	for _, key := range []string{"s", "u", "x"} {
		entry, ok = dispatcherEntry(dispatcherCatalog(schemeVim), key)
		if !ok || entry.Available || entry.Reason == "" {
			t.Errorf("context-free %s = %+v", key, entry)
		}
	}
}

func TestTopLevelTransientOpensFromManifestIdentity(t *testing.T) {
	m := New(nil)
	m.loading = false
	_, cmd := m.Update(keyMsg("F"))
	if cmd != nil || m.resolver.ActiveTransient() != "magit-pull" {
		t.Fatalf("F did not open pull transient: active=%q message=%q", m.resolver.ActiveTransient(), m.message)
	}
}

func TestDispatcherDispatchesAggregateChangesAndProcesses(t *testing.T) {
	for key, command := range map[string]keymap.CommandID{"S": keymap.CommandStageAll, "U": keymap.CommandUnstageAll, "$": keymap.CommandShowProcesses} {
		entry, ok := dispatcherEntry(dispatcherCatalog(schemeVim), key)
		if !ok || !entry.Available || entry.Command != command {
			t.Fatalf("dispatcher %q entry = %+v, found=%v", key, entry, ok)
		}
	}
	m := New(nil)
	m.width, m.height, m.loading = 100, 24, false
	m.stageAll = func(context.Context) error { return nil }
	m.unstageAll = func(context.Context) error { return nil }
	m.snapshotLoader = func(context.Context) (snapshot, error) { return snapshot{}, nil }
	for _, key := range []string{"S", "U"} {
		m.busy = false
		_, _ = m.Update(keyMsg("?"))
		_, cmd := m.Update(keyMsg(key))
		if cmd == nil || m.mode != modeStatus || !m.busy {
			t.Fatalf("dispatcher %q did not execute aggregate action", key)
		}
	}
	m.busy = false
	_, _ = m.Update(keyMsg("?"))
	_, cmd := m.Update(keyMsg("$"))
	if cmd != nil || m.mode != modeProcess {
		t.Fatal("dispatcher $ did not open processes")
	}
}

func TestUnavailableSuffixStaysInActiveTransientAndQuestionOpensDispatcher(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.width, m.height, m.loading = 80, 20, false
	for _, sequence := range [][2]string{{"b", "h"}, {"c", "d"}, {"M", "P"}} {
		m.cancelPrefix()
		_, _ = m.Update(keyMsg(sequence[0]))
		_, _ = m.Update(keyMsg(sequence[1]))
		if m.resolver.PendingPrefix() != sequence[0] || !strings.Contains(m.message, "unavailable") {
			t.Fatalf("%s %s left active transient: prefix=%q message=%q", sequence[0], sequence[1], m.resolver.PendingPrefix(), m.message)
		}
	}
	m.transientOffset = 7
	_, _ = m.Update(keyMsg("?"))
	if m.mode != modeHelp || m.resolver.PendingPrefix() != "" || m.transientOffset != 0 {
		t.Fatal("question mark did not replace prefix with dispatcher")
	}
}

func TestAvailableSuffixUsesCatalogAction(t *testing.T) {
	original := prefixCatalogs["b"]
	changed := original
	changed.Groups = append([]menuGroup(nil), original.Groups...)
	for i := range changed.Groups {
		changed.Groups[i].Entries = append([]menuEntry(nil), original.Groups[i].Entries...)
		for j := range changed.Groups[i].Entries {
			if changed.Groups[i].Entries[j].Key == "b" && changed.Groups[i].Entries[j].Available {
				changed.Groups[i].Entries[j].Command = keymap.CommandShowProcesses
			}
		}
	}
	prefixCatalogs["b"] = changed
	defer func() { prefixCatalogs["b"] = original }()

	m := New(&gitbackend.Repository{})
	m.loading = false
	_, _ = m.Update(keyMsg("b"))
	_, cmd := m.Update(keyMsg("b"))
	if cmd != nil || m.mode != modeProcess || m.resolver.PendingPrefix() != "" || m.transientOffset != 0 {
		t.Fatal("available suffix was not dispatched from its catalog action")
	}
}

func TestNestedSuffixTraversalAndInfixDoNotCollide(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.loading = false
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	_, _ = m.Update(keyMsg("M"))
	_, cmd := m.Update(keyMsg("d"))
	if cmd != nil || m.resolver.PendingPrefix() != "M d" {
		t.Fatalf("M d did not enter nested suffix path: prefix=%q", m.resolver.PendingPrefix())
	}
	_, cmd = m.Update(keyMsg("u"))
	if cmd == nil || m.resolver.PendingPrefix() != "" {
		t.Fatalf("M d u did not dispatch registered workflow: cmd=%v prefix=%q", cmd != nil, m.resolver.PendingPrefix())
	}

	m.cancelPrefix()
	_, _ = m.Update(keyMsg("M"))
	_, cmd = m.Update(keyMsg("u"))
	if cmd != nil || m.resolver.PendingPrefix() != "M" || !strings.Contains(m.message, "Configure dialog") {
		t.Fatalf("M u collided with nested suffix: cmd=%v prefix=%q message=%q", cmd != nil, m.resolver.PendingPrefix(), m.message)
	}
}

func TestRuntimeOptionCapabilitiesNeverExposeIgnoredInfixes(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.snapshot.summary.Branch, m.snapshot.summary.Upstream = "main", "origin/main"
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	for prefix := range prefixCatalogs {
		catalog, _ := m.transientCatalog(prefix)
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				if entry.Kind != keymap.KindInfix {
					continue
				}
				if !entry.Available {
					if entry.Reason == "" {
						t.Errorf("%s %s has no option capability reason", prefix, entry.Key)
					}
				}
			}
		}
	}
	commit, _ := m.transientCatalog("c")
	for upstream, available := range map[string]bool{
		"transient:magit-commit:--verbose": false,
		"magit-commit:--reedit-message":    false,
		"magit:--gpg-sign":                 true,
	} {
		var found bool
		for _, group := range commit.Groups {
			for _, entry := range group.Entries {
				if entry.UpstreamCommand == upstream {
					found = true
					if entry.Available != available {
						t.Errorf("commit option %s availability=%v want %v (%s)", upstream, entry.Available, available, entry.Reason)
					}
				}
			}
		}
		if !found {
			t.Errorf("commit option %s missing", upstream)
		}
	}
}

func TestRuntimeTransientCapabilitiesAreExplicit(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.snapshot.summary.Branch, m.snapshot.summary.Upstream = "main", "origin/main"
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	for prefix := range prefixCatalogs {
		catalog, _ := m.transientCatalog(prefix)
		for _, group := range catalog.Groups {
			for _, entry := range group.Entries {
				if !entry.Available && (entry.Category == "" || entry.Reason == "") {
					t.Errorf("%s %s has implicit availability: %+v", prefix, entry.Key, entry)
				}
			}
		}
	}
}
