package radio

// textInputSelected is narrower than typing: desktop SEARCH keeps accepting
// text on result rows, but those rows must receive OK instead of opening an IME.
func (m Model) textInputSelected() bool {
	return m.editor.mode == editName || (m.top().kind == viewSearch && m.input.Focused() && m.focus == areaList && m.cursor() < 0)
}
