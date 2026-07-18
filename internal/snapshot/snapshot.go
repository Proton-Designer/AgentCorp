// Package snapshot renders the org's current state into a shareable, durable
// form — JSON for machines, a Markdown tree for humans. Pure formatting: nodes
// in, bytes out, no I/O, so it's exhaustively testable.
package snapshot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// Node is one agent in a snapshot. Parent is the parent's NAME (not its opaque
// node id) so the export is readable on its own.
type Node struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	State     string `json:"state"`
	Parent    string `json:"parent,omitempty"`
	PeerID    string `json:"peer_id,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
	SpawnMode string `json:"spawn_mode,omitempty"`
}

// Snapshot is the whole org at a moment.
type Snapshot struct {
	Company    string `json:"company,omitempty"`
	CapturedAt string `json:"captured_at"`
	Nodes      []Node `json:"nodes"`
}

// Build assembles a Snapshot from store rows. capturedAt is passed in (never
// read from the clock here) to keep this pure and deterministic.
func Build(company, capturedAt string, rows []store.Node) Snapshot {
	nameOf := make(map[string]string, len(rows))
	for _, r := range rows {
		nameOf[r.NodeID] = r.Name
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		nodes = append(nodes, Node{
			Name:      r.Name,
			Role:      r.Role,
			State:     r.State,
			Parent:    nameOf[r.ParentID],
			PeerID:    r.PeerID,
			Workdir:   r.Workdir,
			SpawnMode: r.SpawnMode,
		})
	}
	return Snapshot{Company: company, CapturedAt: capturedAt, Nodes: nodes}
}

// JSON renders the snapshot as indented JSON.
func (s Snapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// Markdown renders the snapshot as a human-readable indented org tree, with a
// count and the capture time. Roots are the nodes with no parent.
func (s Snapshot) Markdown() string {
	children := map[string][]Node{}
	for _, n := range s.Nodes {
		children[n.Parent] = append(children[n.Parent], n)
	}
	for k := range children {
		sort.Slice(children[k], func(i, j int) bool { return children[k][i].Name < children[k][j].Name })
	}

	var b strings.Builder
	title := "AgentCorp org"
	if s.Company != "" {
		title = "AgentCorp — " + s.Company
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString(fmt.Sprintf("_%d agents · captured %s_\n\n", len(s.Nodes), s.CapturedAt))

	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			indent := strings.Repeat("  ", depth)
			role := n.Role
			if role != "" {
				role = " · " + role
			}
			b.WriteString(fmt.Sprintf("%s- **%s** (%s%s)\n", indent, n.Name, n.State, role))
			walk(n.Name, depth+1)
		}
	}
	walk("", 0)
	return b.String()
}
