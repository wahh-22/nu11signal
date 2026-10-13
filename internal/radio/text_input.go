package radio

import tea "charm.land/bubbletea/v2"

// remoteSearchInput opens or resumes SEARCH at its input, unlike desktop /
// which restores a parked detail page exactly as it was left.
func (m Model) remoteSearchInput() (Model, tea.Cmd) {
	if !m.canSearch() {
		m.setStatus(noSearch)
		return m, nil
	}
	focus := m.focusList()
	next, open := m.searchAgain()
	// Opening a parked branch can restore a detail page or a selected row.
	// A second searchAgain drops those pages and focuses the kept term.
	next, edit := next.searchAgain()
	return next, tea.Batch(focus, open, edit)
}

// textInputSelected is narrower than typing: desktop SEARCH keeps accepting
// text on result rows, but those rows must receive OK instead of opening an IME.
func (m Model) textInputSelected() bool {
	return m.editor.mode == editName || (m.top().kind == viewSearch && m.input.Focused() && m.focus == areaList && m.cursor() < 0)
}
