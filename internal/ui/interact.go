package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/layout"
)

// mode is the input mode. The tree view is normal; everything else captures
// keystrokes until dismissed. Keeping this an explicit enum rather than a pile
// of booleans means exactly one modal can be open at a time, by construction.
type mode int

const (
	modeNormal  mode = iota
	modeHire         // entering a name for a new agent
	modeMessage      // composing a message to the selected agent
	modeSearch       // filtering the tree
	modeConfirm      // a fire/disband confirmation is up
	modeAdopt        // picking an unmanaged peer to adopt into the chart
	modeInspect      // a detail panel for the selected agent is up
)

// input is a minimal single-line text field. Bubble Tea has a textinput
// bubble, but for one line a few keys is less than importing it — and it keeps
// the modal state trivially testable.
type input struct {
	prompt string
	value  string
}

func (in *input) handle(key string) (done, cancel bool) {
	switch key {
	case "enter":
		return true, false
	case "esc":
		return false, true
	case "backspace":
		if len(in.value) > 0 {
			r := []rune(in.value)
			in.value = string(r[:len(r)-1])
		}
	default:
		// Only accept printable single characters; ignore control keys.
		if len([]rune(key)) == 1 {
			in.value += key
		}
	}
	return false, false
}

// handleModalKey routes a keypress while a modal is open. Returns the possibly
// updated model and whether the key was consumed by the modal.
func (m Model) handleModalKey(key string) (Model, tea.Cmd, bool) {
	switch m.mode {
	case modeHire:
		done, cancel := m.hireInput.handle(key)
		if cancel {
			m.mode = modeNormal
			return m, nil, true
		}
		if done {
			cmd := m.submitHire(m.hireInput.value)
			m.mode = modeNormal
			return m, cmd, true
		}
		return m, nil, true

	case modeMessage:
		done, cancel := m.msgInput.handle(key)
		if cancel {
			m.mode = modeNormal
			return m, nil, true
		}
		if done {
			cmd := m.submitMessage(m.msgInput.value)
			m.mode = modeNormal
			return m, cmd, true
		}
		return m, nil, true

	case modeSearch:
		done, cancel := m.searchInput.handle(key)
		if cancel {
			m.searchInput.value = ""
			m.mode = modeNormal
			m.applyFilter()
			return m, nil, true
		}
		if done {
			m.mode = modeNormal // keep the filter applied
			return m, nil, true
		}
		m.applyFilter()
		return m, nil, true

	case modeAdopt:
		switch key {
		case "esc":
			m.mode = modeNormal
		case "up", "k":
			if m.adoptCursor > 0 {
				m.adoptCursor--
			}
		case "down", "j":
			if m.live != nil && m.adoptCursor < len(m.live.unmanaged)-1 {
				m.adoptCursor++
			}
		case "enter":
			cmd := m.doAdopt()
			m.mode = modeNormal
			return m, cmd, true
		}
		return m, nil, true

	case modeInspect:
		// The panel stays open while you arrow through the org — inspect-as-you-
		// move. esc / i / enter close it.
		switch key {
		case "esc", "i", "enter", "q":
			m.mode = modeNormal
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
			}
		}
		return m, nil, true

	case modeConfirm:
		switch key {
		case "esc":
			m.confirm = nil
			m.mode = modeNormal
		case "f":
			if m.confirm != nil && m.confirm.kind == confirmFire {
				cmd := m.doFire()
				m.confirm = nil
				m.mode = modeNormal
				return m, cmd, true
			}
		case "D":
			if m.confirm != nil && m.confirm.kind == confirmDisband {
				cmd := m.doDisband()
				m.confirm = nil
				m.mode = modeNormal
				return m, cmd, true
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// openHire, openMessage, etc. are the entry points from normal-mode keys.
func (m *Model) openHire() {
	m.hireInput = input{prompt: "hire — name:"}
	m.mode = modeHire
}

func (m *Model) openMessage() {
	if m.selected() == nil {
		return
	}
	m.msgInput = input{prompt: "message " + m.selected().ID + ":"}
	m.mode = modeMessage
}

func (m *Model) openSearch() {
	m.searchInput = input{prompt: "/"}
	m.mode = modeSearch
}

// openAdopt opens the picker for unmanaged peers — live sessions on the machine
// (in this company) that AgentCorp didn't spawn and doesn't yet track.
func (m *Model) openAdopt() {
	if m.live == nil || len(m.live.unmanaged) == 0 {
		m.flash = "no unmanaged agents to adopt"
		return
	}
	m.adoptCursor = 0
	m.mode = modeAdopt
}

// openInspect opens the detail panel for the selected agent.
func (m *Model) openInspect() {
	if m.selected() == nil {
		m.flash = "nothing selected to inspect"
		return
	}
	m.mode = modeInspect
}

// applyFilter dims nodes that don't match the search. A non-matching node is
// not removed — hiding a parent would orphan its matching children on screen —
// it's marked so the renderer can gray it.
func (m *Model) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.searchInput.value))
	m.filter = q
}

// matchesFilter reports whether a node matches the active search.
func (m Model) matchesFilter(n *layout.Node) bool {
	if m.filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(n.ID), m.filter)
}
