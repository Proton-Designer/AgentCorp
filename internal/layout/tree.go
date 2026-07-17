// Package layout implements Reingold-Tilford tree positioning for the AgentCorp
// hero screen. Lipgloss's tree package renders indented lists and provides no
// topology (verified: zero source hits for center/midpoint/subtree-width), so
// this is ours to own. See spec §13.2.
//
// Everything here is a pure function. This package must never import store/,
// ui/, Bubble Tea, or Lipgloss.
package layout

type Node struct {
	ID        string
	W, H      int // rendered card size in terminal cells
	Collapsed bool
	Children  []*Node

	subtreeW int // memo, filled by Measure
	X, Y     int // filled by Position

	// lo/hi are the memoized *rendered extent* of this node's whole subtree —
	// its leftmost and rightmost occupied cell. Filled by Position.
	//
	// Not cosmetic: centering a parent over its immediate children's card
	// edges is wrong when a child is itself narrower than its own children's
	// span (its card sits inside its sub-band, not flush at the edge), which
	// understates the true span one level up. Recursive form of the same bug.
	// Memoizing the extent as each node finalizes makes the correct reference
	// O(n) instead of the O(n²) of re-walking descendants at every ancestor.
	lo, hi int
}

func (n *Node) SubtreeW() int { return n.subtreeW }

// Extent returns the subtree's rendered [leftmost, rightmost) cells.
func (n *Node) Extent() (int, int) { return n.lo, n.hi }

// visibleChildren returns children unless collapsed. A collapsed node measures
// and positions as a leaf — this is the single place collapse is interpreted,
// so §7's fold behavior falls out of the layout for free.
func (n *Node) visibleChildren() []*Node {
	if n.Collapsed {
		return nil
	}
	return n.Children
}
