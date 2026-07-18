package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/hire"
	"github.com/Proton-Designer/AgentCorp/internal/lifecycle"
	"github.com/Proton-Designer/AgentCorp/internal/msg"
	"github.com/Proton-Designer/AgentCorp/internal/resume"
	"github.com/Proton-Designer/AgentCorp/internal/snapshot"
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
func (m Model) submitHire(name, roleTemplate string) tea.Cmd {
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
			Name:         name,
			Role:         "agent",
			Workdir:      workdir,
			ParentID:     parentID,
			Prompt:       "You are a AgentCorp agent named " + name + ".",
			RoleTemplate: roleTemplate, // "" = default; else resolves the stored role
		})
		if err != nil {
			return actionResultMsg{text: fmt.Sprintf("hire %q failed: %v", name, err)}
		}
		if res.RoleMissing != "" {
			return actionResultMsg{text: fmt.Sprintf("hired %q, but role %q wasn't found — used the default prompt", name, res.RoleMissing)}
		}
		if res.Pending {
			return actionResultMsg{text: fmt.Sprintf("%q spawning — will bind when its session registers", name)}
		}
		return actionResultMsg{text: fmt.Sprintf("hired %q (peer %s)", name, res.PeerID)}
	}
}

// broadcastTargets returns the live, bound descendants of rootName (the subtree
// minus the root itself) — the agents a broadcast would actually reach. Pending,
// dead, and unbound nodes are excluded because there's no peer to deliver to.
//
// Returns a non-nil error only when something BROKE while computing the set (the
// store read failed, the node vanished, the subtree walk hit a cycle). A clean
// (nil, nil) means "this manager genuinely has no reachable team" — the caller
// must not conflate the two, the same not-broken-vs-empty discipline the broker
// and tick keep everywhere else.
func (m Model) broadcastTargets(rootName string) ([]store.Node, error) {
	if m.live == nil {
		return nil, fmt.Errorf("no live session")
	}
	root, ok := m.nodeRowByName(rootName)
	if !ok {
		return nil, fmt.Errorf("%q is no longer in the chart", rootName)
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("reading the org: %w", err)
	}
	sub, err := lifecycle.Subtree(nodes, root.NodeID)
	if err != nil {
		return nil, err
	}
	var targets []store.Node
	for _, n := range sub {
		if n.NodeID == root.NodeID {
			continue // your team, not yourself
		}
		if n.State == "alive" && n.PeerID != "" {
			targets = append(targets, n)
		}
	}
	return targets, nil
}

// submitBroadcast sends one message to every reachable agent in the selected
// node's subtree, reporting per-target results — never a single pass/fail. A
// broadcast that silently dropped some recipients while reporting "sent" would
// be a worse lie than failing them all visibly.
func (m Model) submitBroadcast(text string) tea.Cmd {
	if m.live == nil {
		return flash("broadcast unavailable: no live session")
	}
	sel := m.selected()
	if sel == nil || text == "" {
		return flash("broadcast cancelled")
	}
	targets, err := m.broadcastTargets(sel.ID)
	if err != nil {
		return flash("broadcast failed: %v", err)
	}
	if len(targets) == 0 {
		return flash("no reachable team under %s", sel.ID)
	}
	db := m.live.brokerDB
	root := sel.ID
	return func() tea.Msg {
		sent := 0
		var failedNames []string
		for _, t := range targets {
			if err := msg.Send(db, "agentcorp", t.PeerID, text); err != nil {
				failedNames = append(failedNames, t.Name)
			} else {
				sent++
			}
		}
		if len(failedNames) == 0 {
			return actionResultMsg{text: fmt.Sprintf("broadcast queued → %d in %s's team", sent, root)}
		}
		// Name who to follow up with, not just a count.
		who := strings.Join(failedNames, ", ")
		if len(who) > 40 {
			who = who[:40] + "…"
		}
		return actionResultMsg{text: fmt.Sprintf("broadcast to %s's team: %d queued, failed for %s", root, sent, who)}
	}
}

// doMove reparents the selected agent under the picked target (or root),
// re-validating the cycle/dead-parent rules at confirm time since the picker
// list can be a tick stale, and notifying the moved agent of its new manager.
func (m Model) doMove() tea.Cmd {
	if m.live == nil {
		return flash("move unavailable: no live session")
	}
	sel := m.selected()
	if sel == nil {
		return flash("move cancelled")
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return flash("move cancelled")
	}
	newParentID, newParentName := "", "(root)"
	if m.moveCursor > 0 && m.moveCursor-1 < len(m.moveTargets) {
		t := m.moveTargets[m.moveCursor-1]
		newParentID, newParentName = t.NodeID, t.Name
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return flash("move failed: %v", err)
	}
	if err := lifecycle.CheckMove(nodes, row.NodeID, newParentID); err != nil {
		return flash("%v", err)
	}
	st := m.live.st
	db := m.live.brokerDB
	moverID, moverName, moverPeer := row.NodeID, row.Name, row.PeerID
	return func() tea.Msg {
		if err := st.SetParent(moverID, newParentID); err != nil {
			return actionResultMsg{text: fmt.Sprintf("move failed: %v", err)}
		}
		// Reconcile the fiction to the mesh: tell the moved agent who it now
		// reports to (best-effort — a failed notice must not fail the move).
		if moverPeer != "" {
			_ = msg.Send(db, "agentcorp", moverPeer, "You now report to "+newParentName+".")
		}
		return actionResultMsg{text: fmt.Sprintf("moved %q under %s", moverName, newParentName)}
	}
}

// submitRename renames the selected agent, rejecting a blank name or one
// already used by another live node — names are how the UI identifies nodes,
// so a duplicate would make actions ambiguous.
func (m Model) submitRename(newName string) tea.Cmd {
	if m.live == nil {
		return flash("rename unavailable: no live session")
	}
	sel := m.selected()
	if sel == nil {
		return flash("rename cancelled")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return flash("rename cancelled: empty name")
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return flash("rename cancelled")
	}
	if newName == row.Name {
		return flash("rename cancelled: unchanged")
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return flash("rename failed: %v", err)
	}
	for _, n := range nodes {
		// Uniqueness must hold across live AND dead rows: dead nodes are still
		// rendered (dim) with their name as the layout id, and nodeRowByName is a
		// first-match-by-created_at lookup with no live/dead disambiguation — so a
		// live node sharing a dead node's name would make every name-based action
		// silently target whichever row came first. (Found by 23qm3cgf.)
		if n.NodeID != row.NodeID && n.Name == newName {
			return flash("name %q is already taken", newName)
		}
	}
	st := m.live.st
	id, old := row.NodeID, row.Name
	return func() tea.Msg {
		if err := st.SetName(id, newName); err != nil {
			return actionResultMsg{text: fmt.Sprintf("rename failed: %v", err)}
		}
		return actionResultMsg{text: fmt.Sprintf("renamed %q → %q", old, newName)}
	}
}

// submitRevive brings a dead agent back with its memory (claude --resume). If
// the agent's session transcript is gone — or it never had one (adopted /
// pre-session-id) — it can't be revived, so it points the operator at the two
// real options instead: delete it (x) or adopt a replacement (a).
func (m Model) submitRevive() tea.Cmd {
	if m.live == nil {
		return flash("revive unavailable: no live session")
	}
	sel := m.selected()
	if sel == nil {
		return flash("nothing selected")
	}
	row, ok := m.nodeRowByName(sel.ID)
	if !ok {
		return flash("revive cancelled")
	}
	if row.State != "dead" {
		return flash("%s isn't dead — nothing to revive", sel.ID)
	}
	home, _ := os.UserHomeDir()
	if row.SessionID == "" || !resume.Exists(home, row.Workdir, row.SessionID) {
		return flash("no resumable session for %q — its memory is gone. Press x to delete it, or a to adopt a replacement.", row.Name)
	}

	if m.live.hireFlow == nil {
		return flash("revive unavailable: hire flow not wired")
	}
	flow := m.live.hireFlow
	return func() tea.Msg {
		deadline := flow.BindTimeout + 60*time.Second
		if deadline < 90*time.Second {
			deadline = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		res, err := flow.Revive(ctx, row)
		if err != nil {
			if errors.Is(err, hire.ErrSessionGone) {
				return actionResultMsg{text: fmt.Sprintf("%q's session is gone — press x to delete it or a to adopt a replacement", row.Name)}
			}
			return actionResultMsg{text: fmt.Sprintf("revive %q failed: %v", row.Name, err)}
		}
		if res.Pending {
			return actionResultMsg{text: fmt.Sprintf("reviving %q — reconnecting when its session registers", row.Name)}
		}
		return actionResultMsg{text: fmt.Sprintf("revived %q (peer %s)", row.Name, res.PeerID)}
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

// doExport writes a JSON + Markdown snapshot of the org into the launch
// directory and flashes the path. A durable, shareable record of the company
// at a moment — the JSON for tooling, the Markdown tree for humans.
func (m Model) doExport() tea.Cmd {
	if m.live == nil {
		return flash("export unavailable: no live session")
	}
	nodes, err := m.live.st.ListNodes()
	if err != nil {
		return flash("export failed: %v", err)
	}
	company := m.live.company.Name
	dir := m.live.hireWorkdir
	if dir == "" {
		dir = "."
	}
	now := time.Now()
	stamp := now.UTC().Format("20060102T150405")
	snap := snapshot.Build(company, now.UTC().Format(time.RFC3339), nodes)
	return func() tea.Msg {
		jsonData, err := snap.JSON()
		if err != nil {
			return actionResultMsg{text: fmt.Sprintf("export failed: %v", err)}
		}
		base := filepath.Join(dir, "agentcorp-snapshot-"+stamp)
		if err := os.WriteFile(base+".json", jsonData, 0o644); err != nil {
			return actionResultMsg{text: fmt.Sprintf("export failed: %v", err)}
		}
		if err := os.WriteFile(base+".md", []byte(snap.Markdown()), 0o644); err != nil {
			return actionResultMsg{text: fmt.Sprintf("export wrote JSON but not Markdown: %v", err)}
		}
		return actionResultMsg{text: "exported → " + base + ".json / .md"}
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
	// A dead selection falls back to root — correct (never target a dead
	// parent), but say so, since a silent substitution reads like a wrong claim.
	parentID := ""
	parentNote := ""
	if sel := m.selected(); sel != nil {
		if row, ok := m.nodeRowByName(sel.ID); ok {
			if row.State == "dead" {
				parentNote = " at root (selection was dead)"
			} else {
				parentID = row.NodeID
			}
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
			// silently duplicating the peer into two nodes. Translate the raw
			// driver constraint text into something an operator can read.
			if strings.Contains(err.Error(), "UNIQUE") {
				return actionResultMsg{text: fmt.Sprintf("%q is already tracked in the chart", name)}
			}
			return actionResultMsg{text: fmt.Sprintf("adopt failed: %v", err)}
		}
		return actionResultMsg{text: fmt.Sprintf("adopted %q%s", name, parentNote)}
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
