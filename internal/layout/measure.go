package layout

// Measure computes the horizontal cells a node's whole subtree needs, and
// memoizes it on each node for Position to reuse. Post-order: children first,
// then the parent, which is why one pass suffices.
//
// A subtree is as wide as the greater of (a) the node's own card and (b) the
// sum of its children's subtrees plus the gaps between them. Taking the max is
// what prevents sibling overlap.
func Measure(n *Node, gap int) int {
	kids := n.visibleChildren()
	if len(kids) == 0 {
		n.subtreeW = n.W
		return n.subtreeW
	}

	total := 0
	for i, c := range kids {
		if i > 0 {
			total += gap
		}
		total += Measure(c, gap)
	}
	if n.W > total {
		total = n.W
	}
	n.subtreeW = total
	return total
}
