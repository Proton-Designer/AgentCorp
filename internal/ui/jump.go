package ui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Click-to-open: clicking an agent's card (tree or mission view) — or pressing
// Enter on the selected one — switches the terminal straight to that agent's real
// tmux window. It resolves the node's spawn_ref pane to a session:window, so it
// works even when the agent lives in a DIFFERENT tmux session than this AgentCorp
// instance (the exact confusion that made agents feel unreachable).

// nodeRect is an agent card's clickable rectangle in absolute terminal cells,
// recorded during render so a mouse click maps back to the right agent.
type nodeRect struct {
	name       string
	x, y, w, h int
}

// recordNodeRects computes the on-screen rectangle of every visible agent card in
// the current view and stashes them for click hit-testing. baseRow is the terminal
// row the chart starts on (how many rows were already emitted above it). Only the
// tree and mission views draw clickable cards; office is skipped.
func (m Model) recordNodeRects(baseRow int) {
	if m.live == nil || m.root == nil {
		return
	}
	chartW, _ := extent(m.root)
	var offX, offY int
	switch m.view {
	case viewMission:
		// The chart sits inside the ORG panel: 2-cell outer indent + 1 panel wall,
		// then centered within the panel interior (leftW-2), one row below the top wall.
		inner := m.width - 2
		leftW := inner * 3 / 5
		offX = 3 + centerCols(leftW-2, chartW)
		offY = baseRow + 1
	case viewOffice:
		m.live.nodeRects = nil // office cards aren't wired for click yet
		return
	default: // tree
		offX = centerCols(m.width, chartW)
		offY = baseRow
	}
	rects := make([]nodeRect, 0, len(m.flat))
	for _, n := range m.flat {
		rects = append(rects, nodeRect{name: n.ID, x: offX + n.X, y: offY + n.Y, w: n.W, h: n.H})
	}
	m.live.nodeRects = rects
}

// centerCols is centerPad's width as an integer.
func centerCols(width, w int) int {
	if width > w {
		return (width - w) / 2
	}
	return 0
}

// nodeAt returns the agent whose card contains the point, if any.
func (m Model) nodeAt(x, y int) (string, bool) {
	if m.live == nil {
		return "", false
	}
	for _, r := range m.live.nodeRects {
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return r.name, true
		}
	}
	return "", false
}

// jumpToAgentCmd switches the terminal to the agent's real tmux window. It resolves
// the node's spawn_ref pane to session:window (so cross-session works) and issues a
// tmux switch-client. Honest failures: an adopted node with no spawn_ref, a pane
// that's gone, or AgentCorp not running inside tmux are each reported, never faked.
func (m Model) jumpToAgentCmd(name string) tea.Cmd {
	if m.live == nil {
		return nil
	}
	row, ok := m.nodeRowByName(name)
	if !ok {
		return flash("can't find %q", name)
	}
	if row.SpawnRef == "" {
		return flash("%s has no reachable session (adopted, or not spawned by AgentCorp)", name)
	}
	ref := row.SpawnRef
	return func() tea.Msg {
		target, err := exec.Command("tmux", "display-message", "-t", ref, "-p", "#{session_name}:#{window_index}").Output()
		if err != nil {
			return actionResultMsg{text: fmt.Sprintf("%s's session is gone (pane %s) — press z to revive or x to remove it", name, ref)}
		}
		t := strings.TrimSpace(string(target))
		if t == "" {
			return actionResultMsg{text: fmt.Sprintf("%s's window could not be resolved from pane %s", name, ref)}
		}
		if err := exec.Command("tmux", "switch-client", "-t", t).Run(); err != nil {
			return actionResultMsg{text: fmt.Sprintf("couldn't switch to %s (%s) — is AgentCorp running inside tmux? (%v)", name, t, err)}
		}
		return actionResultMsg{text: fmt.Sprintf("→ opened %s at %s   (Ctrl+B then 0 to come back)", name, t)}
	}
}
