package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/hire"
	"github.com/Proton-Designer/AgentCorp/internal/lifecycle"
	"github.com/Proton-Designer/AgentCorp/internal/msg"
	"github.com/Proton-Designer/AgentCorp/internal/store"
	"github.com/Proton-Designer/AgentCorp/internal/vitals"
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
		// The hire's overall deadline MUST exceed the bind wait, or this context
		// cancels a hire that was about to succeed and the bind timeout becomes a
		// lie (observed: a session registered fine but the hire was marked failed
		// with "context deadline exceeded"). Budget = the bind wait plus margin
		// for spawn and gate-clearing, which have their own shorter timeouts.
		hireDeadline := flow.BindTimeout + 60*time.Second
		if hireDeadline < 90*time.Second {
			hireDeadline = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), hireDeadline)
		defer cancel()
		res, err := flow.Run(ctx, hire.Request{
			Name:     name,
			Role:     "agent",
			Workdir:  workdir,
			ParentID: parentID,
			Prompt:   "You are a AgentCorp agent named " + name + ".",
		})
		if err != nil {
			return actionResultMsg{text: fmt.Sprintf("hire %q failed: %v", name, err)}
		}
		if res.Pending {
			return actionResultMsg{text: fmt.Sprintf("%q spawning — will bind when its session registers", name)}
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
		// Operator identity is "agentcorp" — surfaced honestly, never spoofing a peer.
		if err := msg.Send(db, "agentcorp", to, text); err != nil {
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
				_ = msg.Send(db, "agentcorp", child.PeerID, notice.Message)
			}
		}
		killProcess(victim)
		// Remove the node outright rather than tombstoning it. Its children were
		// just reparented, so nothing is orphaned — and a node the operator
		// deliberately fired should leave the chart, not linger as a dead marker
		// (which reads as "nothing happened"). Crash-detected deaths still
		// tombstone-and-render via the sync layer; this is only the explicit path.
		if err := st.DeleteNode(victim.NodeID); err != nil {
			return actionResultMsg{text: "fire: remove failed: " + err.Error()}
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
			// The whole subtree is going, deepest first — no survivor to orphan,
			// so remove each row rather than leaving a thicket of dead markers.
			_ = st.DeleteNode(c.NodeID)
		}
		return actionResultMsg{text: fmt.Sprintf("disbanded %q: %d session(s) terminated", root.Name, len(casualties))}
	}
}

// doAdopt turns the selected unmanaged peer into a node: a live, bound row we
// didn't spawn. spawn_ref/bind_tty stay empty (we own no pane for it), so the
// sync layer's broker-signal-only contract governs its life — Decide already
// handles spawn_ref=="" correctly. Adoption IS the binding, by operator
// assertion rather than a tty match.
func (m Model) doAdopt() tea.Cmd {
	if m.live == nil || m.adoptCursor < 0 || m.adoptCursor >= len(m.live.unmanaged) {
		return flash("adopt cancelled")
	}
	peer := m.live.unmanaged[m.adoptCursor]

	// Re-check the peer is still live at confirm time, not at selection time —
	// never adopt a corpse. The list can be a tick stale.
	live := false
	for _, p := range m.live.peers {
		if p.ID == peer.ID {
			live = true
			break
		}
	}
	if !live {
		return flash("cannot adopt %s: that agent is gone", shortPeerID(peer.ID))
	}

	// Adopt under the selected node if it's a valid living parent; else at root.
	parentID := ""
	if sel := m.selected(); sel != nil {
		if row, ok := m.nodeRowByName(sel.ID); ok && row.State != "dead" {
			parentID = row.NodeID
		}
	}

	name := adoptName(peer)
	node := store.Node{
		NodeID:    "a-" + time.Now().UTC().Format("20060102T150405.000000"),
		PeerID:    peer.ID,
		Name:      name,
		Role:      "adopted",
		ParentID:  parentID,
		Workdir:   peer.CWD,
		SpawnMode: "adopted",
		State:     "alive",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	st := m.live.st
	return func() tea.Msg {
		if err := st.InsertNode(node); err != nil {
			// peer_id is UNIQUE: a double-adopt fails loudly here rather than
			// silently duplicating the peer into two nodes.
			return actionResultMsg{text: fmt.Sprintf("adopt failed: %v", err)}
		}
		return actionResultMsg{text: fmt.Sprintf("adopted %q", name)}
	}
}

// adoptName derives a readable, collision-resistant name for an adopted peer:
// the working-directory basename (what the session is about) plus a short slice
// of the peer id (unique), clipped to the card's label width.
func adoptName(p broker.Peer) string {
	base := filepath.Base(p.CWD)
	if base == "." || base == "/" || base == "" {
		base = "peer"
	}
	id := p.ID
	if len(id) > 4 {
		id = id[:4]
	}
	name := base + "-" + id
	if r := []rune(name); len(r) > cardW-2 {
		name = string(r[:cardW-2])
	}
	return name
}

// shortPeerID clips a peer id for a status message.
func shortPeerID(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

func nodeByNodeID(nodes []store.Node, id string) (store.Node, bool) {
	for _, n := range nodes {
		if n.NodeID == id {
			return n, true
		}
	}
	return store.Node{}, false
}

// killProcess terminates the tmux pane a node was spawned into. Best-effort:
// if the pane is already gone (the agent exited on its own), that's success,
// not an error.
func killProcess(n store.Node) {
	if n.SpawnRef == "" {
		return // adopted / non-tmux: nothing of ours to kill
	}
	_ = broker.KillPane(n.SpawnRef)
}
