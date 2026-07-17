package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/hire"
	"github.com/aymanmohammed/crew/internal/lifecycle"
	"github.com/aymanmohammed/crew/internal/msg"
	"github.com/aymanmohammed/crew/internal/store"
	"github.com/aymanmohammed/crew/internal/vitals"
)

// actionResultMsg carries the outcome of an action back to the UI as a flash.
type actionResultMsg struct{ text string }

func flash(format string, a ...any) tea.Cmd {
	text := fmt.Sprintf(format, a...)
	return func() tea.Msg { return actionResultMsg{text: text} }
}

// nodeRowByName finds the store row behind a tree node. The tree carries the
// display name; actions need the node_id and peer_id from the store.
func (m Model) nodeRowByName(name string) (store.Node, bool) {
	if m.live == nil {
		return store.Node{}, false
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return store.Node{}, false
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return store.Node{}, false
}

// submitHire runs the full hire flow in the background. It returns a command so
// the spawn + gate-clearing + bind (potentially seconds) never blocks the UI.
func (m Model) submitHire(name string) tea.Cmd {
	if m.live == nil || m.live.hireFlow == nil {
		return flash("hire unavailable: no live session")
	}
	if name == "" {
		return flash("hire cancelled: no name")
	}
	parentID := ""
	if sel := m.selected(); sel != nil {
		if row, ok := m.nodeRowByName(sel.ID); ok {
			parentID = row.NodeID
		}
	}
	flow := m.live.hireFlow
	workdir := m.live.hireWorkdir

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		res, err := flow.Run(ctx, hire.Request{
			Name:     name,
			Role:     "agent",
			Workdir:  workdir,
			ParentID: parentID,
			Prompt:   "You are a CREW agent named " + name + ".",
		})
		if err != nil {
			return actionResultMsg{text: fmt.Sprintf("hire %q failed: %v", name, err)}
		}
		return actionResultMsg{text: fmt.Sprintf("hired %q (peer %s)", name, res.PeerID)}
	}
}

// submitMessage sends an operator message to the selected agent.
func (m Model) submitMessage(text string) tea.Cmd {
	if m.live == nil {
		return flash("message unavailable: no live session")
	}
	sel := m.selected()
	if sel == nil || text == "" {
		return flash("message cancelled")
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok || row.PeerID == "" {
		return flash("cannot message %q: not a bound agent", sel.ID)
	}
	db := m.live.brokerDB
	to := row.PeerID
	return func() tea.Msg {
		// Operator identity is "crew" — surfaced honestly, never spoofing a peer.
		if err := msg.Send(db, "crew", to, text); err != nil {
			return actionResultMsg{text: fmt.Sprintf("send failed: %v", err)}
		}
		// "queued", never "delivered": the substrate never acks (S6).
		return actionResultMsg{text: fmt.Sprintf("queued → %s", sel.ID)}
	}
}

// openFireConfirm stages a fire. Reparent is computed now so the dialog can say
// how many reports survive and where they land.
func (m *Model) openFireConfirm() {
	sel := m.selected()
	if sel == nil || m.live == nil {
		return
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return
	}
	moves, err := lifecycle.Reparent(nodes, row.NodeID)
	if err != nil {
		m.flash = "cannot fire: " + err.Error()
		return
	}
	newParent := "(root)"
	for _, mv := range moves {
		if mv.NewParentID == "" {
			newParent = "(root)"
		} else if p, ok := nodeByNodeID(nodes, mv.NewParentID); ok {
			newParent = p.Name
		}
		break
	}
	m.confirm = &confirmState{
		kind: confirmFire, victim: row, moves: len(moves), newParent: newParent,
	}
	m.mode = modeConfirm
}

// openDisbandConfirm stages a cascade. Every casualty is enumerated so the
// dialog can list them and flag the active ones.
func (m *Model) openDisbandConfirm() {
	sel := m.selected()
	if sel == nil || m.live == nil {
		return
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return
	}
	casualties, err := lifecycle.Disband(nodes, row.NodeID)
	if err != nil {
		m.flash = "cannot disband: " + err.Error()
		return
	}
	statuses := map[string]vitals.Status{}
	for _, c := range casualties {
		statuses[c.NodeID] = vitals.NodeStatus(c, m.live.peers, m.live.msgs, time.Now(), ActivityWindow)
	}
	m.confirm = &confirmState{
		kind: confirmDisband, victim: row, casualties: casualties, statuses: statuses,
	}
	m.mode = modeConfirm
}

// doFire executes a confirmed fire: reparent children, notify them, tombstone
// the victim. The actual SIGTERM of the tmux pane is left to the sync layer's
// death detection — we tombstone the metadata and the process is reaped.
func (m Model) doFire() tea.Cmd {
	if m.confirm == nil || m.live == nil {
		return nil
	}
	victim := m.confirm.victim
	st := m.live.st
	db := m.live.brokerDB

	return func() tea.Msg {
		nodes, err := st.ListNodes()
		if err != nil {
			return actionResultMsg{text: "fire failed: " + err.Error()}
		}
		moves, err := lifecycle.Reparent(nodes, victim.NodeID)
		if err != nil {
			return actionResultMsg{text: "fire failed: " + err.Error()}
		}
		for _, mv := range moves {
			_ = st.SetParent(mv.NodeID, mv.NewParentID)
		}
		// Notify reparented children — the fiction only reconciles to the mesh
		// if we tell them (spec §6.3, LC-9). Best-effort: a failed notice must
		// not block the fire.
		for _, notice := range lifecycle.ReparentNotices(moves) {
			if child, ok := nodeByNodeID(nodes, notice.To); ok && child.PeerID != "" {
				_ = msg.Send(db, "crew", child.PeerID, notice.Message)
			}
		}
		killProcess(victim)
		if err := st.Tombstone(victim.NodeID, nowRFC3339()); err != nil {
			return actionResultMsg{text: "fire: tombstone failed: " + err.Error()}
		}
		return actionResultMsg{text: fmt.Sprintf("fired %q; %d report(s) reparented", victim.Name, len(moves))}
	}
}

// doDisband executes a confirmed cascade in kill order (deepest first).
func (m Model) doDisband() tea.Cmd {
	if m.confirm == nil || m.live == nil {
		return nil
	}
	root := m.confirm.victim
	st := m.live.st

	return func() tea.Msg {
		nodes, err := st.ListNodes()
		if err != nil {
			return actionResultMsg{text: "disband failed: " + err.Error()}
		}
		casualties, err := lifecycle.Disband(nodes, root.NodeID)
		if err != nil {
			return actionResultMsg{text: "disband failed: " + err.Error()}
		}
		for _, c := range casualties {
			killProcess(c)
			_ = st.Tombstone(c.NodeID, nowRFC3339())
		}
		return actionResultMsg{text: fmt.Sprintf("disbanded %q: %d session(s) terminated", root.Name, len(casualties))}
	}
}

func nodeByNodeID(nodes []store.Node, id string) (store.Node, bool) {
	for _, n := range nodes {
		if n.NodeID == id {
			return n, true
		}
	}
	return store.Node{}, false
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// killProcess terminates the tmux pane a node was spawned into. Best-effort:
// if the pane is already gone (the agent exited on its own), that's success,
// not an error.
func killProcess(n store.Node) {
	if n.SpawnRef == "" {
		return // adopted / non-tmux: nothing of ours to kill
	}
	_ = broker.KillPane(n.SpawnRef)
}
