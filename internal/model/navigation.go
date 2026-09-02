package model

// MoveToParent moves the cursor to its parent section. It reports whether the
// cursor moved.
func (m *Model) MoveToParent() bool {
	parent := m.parent[m.cursor]
	if parent == "" || !m.isVisible(parent) {
		return false
	}
	m.cursor = parent
	return true
}

// MoveToNextSibling moves to the next sibling. As in Magit, when there is no
// next sibling it falls back to the next visible section.
func (m *Model) MoveToNextSibling() bool {
	if next := m.sibling(m.cursor, 1); next != "" {
		m.cursor = next
		return true
	}
	return m.moveVisible(1)
}

// MoveToPreviousSibling moves to the previous sibling. As in Magit, when
// there is no previous sibling it falls back to the previous visible section,
// which is normally the parent for a first child.
func (m *Model) MoveToPreviousSibling() bool {
	if previous := m.sibling(m.cursor, -1); previous != "" {
		m.cursor = previous
		return true
	}
	return m.moveVisible(-1)
}

// RevealSelectedDepth reveals the selected section's tree to depth levels.
// The selected section is level 1. Depth must be between 1 and 4. Sections
// outside the selected tree and the cursor are left unchanged.
func (m *Model) RevealSelectedDepth(depth int) bool {
	section := m.byID[m.cursor]
	if section == nil || depth < 1 || depth > 4 {
		return false
	}
	m.revealDepth(section, depth)
	return true
}

// RevealGlobalDepth reveals every top-level tree to depth levels. Roots are
// level 1. Depth must be between 1 and 4.
func (m *Model) RevealGlobalDepth(depth int) bool {
	if depth < 1 || depth > 4 {
		return false
	}
	oldVisible := m.VisibleSectionIDs()
	// This operation supersedes the older global visibility limit while leaving
	// SetGlobalDepth's behavior unchanged for existing callers.
	m.maxDepth = 0
	for _, section := range m.sections {
		if section != nil && m.byID[section.id] == section {
			m.revealDepth(section, depth)
		}
	}
	m.retainVisibleCursor(oldVisible)
	return true
}

// CycleLocal cycles the selected tree through collapsed, children-only, and
// fully expanded states. It reports false for a missing or leaf selection.
func (m *Model) CycleLocal() bool {
	section := m.byID[m.cursor]
	if section == nil || len(section.children) == 0 {
		return false
	}

	switch {
	case m.folded[section.id]:
		delete(m.folded, section.id)
		m.foldChildren(section)
	case m.hasFoldedDescendant(section):
		m.unfoldTree(section)
	default:
		m.folded[section.id] = true
	}
	return true
}

// CycleGlobal cycles all top-level trees through collapsed, children-only,
// and fully expanded states.
func (m *Model) CycleGlobal() bool {
	if len(m.byID) == 0 {
		return false
	}
	oldVisible := m.VisibleSectionIDs()
	m.maxDepth = 0
	roots := m.currentRoots()
	state := m.globalCycleState(roots)
	for _, root := range roots {
		m.applyGlobalCycle(root, state)
	}
	m.retainVisibleCursor(oldVisible)
	return true
}

type cycleState int

const (
	cycleCollapse cycleState = iota
	cycleShowChildren
	cycleExpand
)

func (m *Model) currentRoots() []*Section {
	roots := make([]*Section, 0, len(m.sections))
	for _, root := range m.sections {
		if root != nil && m.byID[root.id] == root {
			roots = append(roots, root)
		}
	}
	return roots
}

func (m *Model) globalCycleState(roots []*Section) cycleState {
	state := cycleCollapse
	for _, root := range roots {
		if m.folded[root.id] {
			return cycleShowChildren
		}
		if m.hasFoldedDescendant(root) {
			state = cycleExpand
		}
	}
	return state
}

func (m *Model) applyGlobalCycle(root *Section, state cycleState) {
	switch state {
	case cycleShowChildren:
		delete(m.folded, root.id)
		m.foldChildren(root)
	case cycleExpand:
		m.unfoldTree(root)
	case cycleCollapse:
		if len(root.children) > 0 {
			m.folded[root.id] = true
		}
	}
}

func (m *Model) sibling(id SectionID, direction int) SectionID {
	if m.byID[id] == nil {
		return ""
	}
	nodes := m.siblingNodes(id)
	for i, section := range nodes {
		if section != m.byID[id] {
			continue
		}
		return m.visibleSiblingFrom(nodes, i+direction, direction)
	}
	return ""
}

func (m *Model) siblingNodes(id SectionID) []*Section {
	parent := m.parent[id]
	if parent == "" {
		return m.sections
	}
	return m.byID[parent].children
}

func (m *Model) visibleSiblingFrom(nodes []*Section, start, direction int) SectionID {
	for i := start; i >= 0 && i < len(nodes); i += direction {
		candidate := nodes[i]
		if candidate != nil && m.byID[candidate.id] == candidate && m.isVisible(candidate.id) {
			return candidate.id
		}
	}
	return ""
}

func (m *Model) moveVisible(direction int) bool {
	visible := m.VisibleSectionIDs()
	for i, id := range visible {
		if id == m.cursor {
			i += direction
			if i >= 0 && i < len(visible) {
				m.cursor = visible[i]
				return true
			}
			return false
		}
	}
	return false
}

func (m *Model) revealDepth(section *Section, depth int) {
	if len(section.children) == 0 {
		delete(m.folded, section.id)
		return
	}
	if depth == 1 {
		m.folded[section.id] = true
		return
	}
	delete(m.folded, section.id)
	for _, child := range section.children {
		if child != nil && m.byID[child.id] == child {
			m.revealDepth(child, depth-1)
		}
	}
}

func (m *Model) foldChildren(section *Section) {
	for _, child := range section.children {
		if child == nil || m.byID[child.id] != child {
			continue
		}
		if len(child.children) > 0 {
			m.folded[child.id] = true
		} else {
			delete(m.folded, child.id)
		}
	}
}

func (m *Model) unfoldTree(section *Section) {
	delete(m.folded, section.id)
	for _, child := range section.children {
		if child != nil && m.byID[child.id] == child {
			m.unfoldTree(child)
		}
	}
}

func (m *Model) hasFoldedDescendant(section *Section) bool {
	for _, child := range section.children {
		if child == nil || m.byID[child.id] != child {
			continue
		}
		if m.folded[child.id] || m.hasFoldedDescendant(child) {
			return true
		}
	}
	return false
}

func (m *Model) retainVisibleCursor(oldVisible []SectionID) {
	if m.isVisible(m.cursor) {
		return
	}
	if ancestor := m.visibleAncestor(m.cursor); ancestor != "" {
		m.cursor = ancestor
		return
	}
	m.ensureCursor(oldVisible)
}
