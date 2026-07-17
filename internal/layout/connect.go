package layout

// Seg is one box-drawing glyph at an absolute cell.
type Seg struct {
	X, Y int
	Rune rune
}

// Connectors walks the positioned tree and emits the box-drawing glyphs that
// join each parent to its children.
//
// Geometry, for vgap=2:
//
//	   ╭─ parent ─╮
//	        │        <- stem, at parent center
//	   ┌────┴────┐   <- bus row: corners at child centers, tee under parent
//	   │         │   <- drops, at each child center
//	child     child
func Connectors(root *Node, vgap int) []Seg {
	var out []Seg
	var walk func(*Node)
	walk = func(n *Node) {
		kids := n.visibleChildren()
		if len(kids) == 0 {
			return
		}

		pcx := n.X + n.W/2
		stemY := n.Y + n.H
		busY := stemY + 1
		childY := n.Y + n.H + vgap

		out = append(out, Seg{X: pcx, Y: stemY, Rune: '│'})

		if len(kids) == 1 {
			// Straight drop; no bus needed.
			for y := busY; y < childY; y++ {
				out = append(out, Seg{X: pcx, Y: y, Rune: '│'})
			}
			walk(kids[0])
			return
		}

		lc := kids[0].X + kids[0].W/2
		rc := kids[len(kids)-1].X + kids[len(kids)-1].W/2

		// Horizontal bus.
		for x := lc; x <= rc; x++ {
			out = append(out, Seg{X: x, Y: busY, Rune: '─'})
		}
		// Corners and tees, applied over the bus.
		out = append(out, Seg{X: lc, Y: busY, Rune: '┌'})
		out = append(out, Seg{X: rc, Y: busY, Rune: '┐'})
		for _, c := range kids[1 : len(kids)-1] {
			out = append(out, Seg{X: c.X + c.W/2, Y: busY, Rune: '┬'})
		}
		if pcx > lc && pcx < rc {
			out = append(out, Seg{X: pcx, Y: busY, Rune: '┴'})
		}
		// Drops from bus to each child.
		for _, c := range kids {
			for y := busY + 1; y < childY; y++ {
				out = append(out, Seg{X: c.X + c.W/2, Y: y, Rune: '│'})
			}
			walk(c)
		}
	}
	walk(root)
	return out
}
