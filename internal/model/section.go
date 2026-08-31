// Package model contains the UI-independent state used to render sections.
package model

// SectionID identifies a semantic section. IDs should be stable across status
// refreshes; display text and Section pointers need not be.
type SectionID string

// Section is one node in a status tree.
type Section struct {
	id       SectionID
	title    string
	children []*Section
}

// NewSection constructs a section and adopts children in their supplied order.
func NewSection(id SectionID, title string, children ...*Section) *Section {
	return &Section{id: id, title: title, children: append([]*Section(nil), children...)}
}

// ID returns the section's stable identity.
func (s *Section) ID() SectionID {
	if s == nil {
		return ""
	}
	return s.id
}

// Title returns the section's display heading.
func (s *Section) Title() string {
	if s == nil {
		return ""
	}
	return s.title
}

// Children returns a shallow copy, so callers cannot reorder the model's tree.
func (s *Section) Children() []*Section {
	if s == nil {
		return nil
	}
	return append([]*Section(nil), s.children...)
}

// Model owns section visibility, folding, and cursor identity.
type Model struct {
	sections []*Section
	byID     map[SectionID]*Section
	parent   map[SectionID]SectionID
	folded   map[SectionID]bool
	cursor   SectionID
	maxDepth int
}

// New creates a section model. The cursor starts at the first visible section.
func New(sections []*Section) *Model {
	m := &Model{folded: make(map[SectionID]bool)}
	m.replaceTree(sections)
	m.ensureCursor(nil)
	return m
}

// Sections returns a shallow copy of the model's roots.
func (m *Model) Sections() []*Section {
	return append([]*Section(nil), m.sections...)
}

// Section looks up a section by stable identity.
func (m *Model) Section(id SectionID) *Section { return m.byID[id] }

// ReplaceSections installs a freshly rendered tree while retaining fold and
// cursor state for IDs that still exist.
func (m *Model) ReplaceSections(sections []*Section) {
	oldVisible := m.VisibleSectionIDs()
	oldCursor := m.cursor
	m.replaceTree(sections)

	// Fold state only has meaning while its identity is in the current tree.
	for id := range m.folded {
		if m.byID[id] == nil {
			delete(m.folded, id)
		}
	}
	if m.isVisible(oldCursor) {
		m.cursor = oldCursor
		return
	}
	if ancestor := m.visibleAncestor(oldCursor); ancestor != "" {
		m.cursor = ancestor
		return
	}
	m.ensureCursor(oldVisible)
}

// ToggleFold toggles a section with children. If folding hides the cursor, the
// cursor moves to the folded ancestor.
func (m *Model) ToggleFold(id SectionID) {
	s := m.byID[id]
	if s == nil || len(s.children) == 0 {
		return
	}
	oldVisible := m.VisibleSectionIDs()
	if m.folded[id] {
		delete(m.folded, id)
	} else {
		m.folded[id] = true
	}
	if !m.isVisible(m.cursor) {
		if ancestor := m.visibleAncestor(m.cursor); ancestor != "" {
			m.cursor = ancestor
		} else {
			m.ensureCursor(oldVisible)
		}
	}
}

// IsFolded reports the explicit fold state of a section.
func (m *Model) IsFolded(id SectionID) bool { return m.folded[id] }

// SetGlobalDepth limits the visible tree to depth levels. Roots are depth 1;
// a non-positive value removes the limit.
func (m *Model) SetGlobalDepth(depth int) {
	oldVisible := m.VisibleSectionIDs()
	m.maxDepth = depth
	if !m.isVisible(m.cursor) {
		if ancestor := m.visibleAncestor(m.cursor); ancestor != "" {
			m.cursor = ancestor
		} else {
			m.ensureCursor(oldVisible)
		}
	}
}

// SetCursor selects id when it is currently visible. It reports whether the
// cursor changed to the requested section.
func (m *Model) SetCursor(id SectionID) bool {
	if !m.isVisible(id) {
		return false
	}
	m.cursor = id
	return true
}

// Cursor returns the stable ID under the cursor, or the empty ID for no rows.
func (m *Model) Cursor() SectionID { return m.cursor }

// VisibleSectionIDs returns visible sections in pre-order rendering order.
func (m *Model) VisibleSectionIDs() []SectionID {
	visible := m.visibleSections()
	ids := make([]SectionID, len(visible))
	for i, section := range visible {
		ids[i] = section.id
	}
	return ids
}

// VisibleSections returns visible sections in pre-order rendering order.
func (m *Model) VisibleSections() []*Section {
	return append([]*Section(nil), m.visibleSections()...)
}

func (m *Model) replaceTree(sections []*Section) {
	m.sections = append([]*Section(nil), sections...)
	m.byID = make(map[SectionID]*Section)
	m.parent = make(map[SectionID]SectionID)
	var index func([]*Section, SectionID)
	index = func(nodes []*Section, parent SectionID) {
		for _, section := range nodes {
			if section == nil {
				continue
			}
			// Stable IDs are expected to be unique. Keeping the first occurrence
			// makes malformed input deterministic without changing New's compact API.
			if _, exists := m.byID[section.id]; exists {
				continue
			}
			m.byID[section.id] = section
			m.parent[section.id] = parent
			index(section.children, section.id)
		}
	}
	index(m.sections, "")
}

func (m *Model) visibleSections() []*Section {
	var result []*Section
	var walk func([]*Section, int)
	walk = func(nodes []*Section, depth int) {
		for _, section := range nodes {
			if section == nil || m.byID[section.id] != section {
				continue
			}
			result = append(result, section)
			if m.folded[section.id] || (m.maxDepth > 0 && depth >= m.maxDepth) {
				continue
			}
			walk(section.children, depth+1)
		}
	}
	walk(m.sections, 1)
	return result
}

func (m *Model) isVisible(id SectionID) bool {
	if id == "" {
		return false
	}
	for _, visible := range m.visibleSections() {
		if visible.id == id {
			return true
		}
	}
	return false
}

func (m *Model) visibleAncestor(id SectionID) SectionID {
	for id != "" && m.byID[id] != nil {
		id = m.parent[id]
		if m.isVisible(id) {
			return id
		}
	}
	return ""
}

func (m *Model) ensureCursor(oldOrder []SectionID) {
	visible := m.VisibleSectionIDs()
	m.cursor = nearestVisibleCursor(oldOrder, visible, m.cursor)
}

func nearestVisibleCursor(oldOrder, visible []SectionID, cursor SectionID) SectionID {
	if len(visible) == 0 {
		return ""
	}
	visibleSet := make(map[SectionID]bool, len(visible))
	for _, id := range visible {
		visibleSet[id] = true
	}
	oldIndex := -1
	for i, id := range oldOrder {
		if id == cursor {
			oldIndex = i
			break
		}
	}
	// Prefer the next surviving semantic row, then the preceding row.
	if oldIndex >= 0 {
		for distance := 1; distance < len(oldOrder); distance++ {
			if next := oldIndex + distance; next < len(oldOrder) && visibleSet[oldOrder[next]] {
				return oldOrder[next]
			}
			if previous := oldIndex - distance; previous >= 0 && visibleSet[oldOrder[previous]] {
				return oldOrder[previous]
			}
		}
	}
	return visible[0]
}
