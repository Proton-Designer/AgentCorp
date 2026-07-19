package ui

// motionLevel is the global motion budget — the single lever that decides how
// much of the living-company animation actually moves. It exists because the
// features compose technically but stack visually: with every ambient effect on
// at once, an always-max screen reads as noise, not command. The budget lets the
// operator (and the honest defaults) keep motion tasteful.
//
//	off    — a still chart. Every overlay is a no-op; the render path falls back
//	         to the plain styled renderer. For screenshots, SSH on a slow link,
//	         or anyone who just wants the data.
//	calm   — ambient life only: breathing status LEDs, the occasional message
//	         particle. The default — alive, never busy.
//	lively — everything the current view offers, at full tempo.
type motionLevel uint8

const (
	motionOff motionLevel = iota
	motionCalm
	motionLively
)

func (l motionLevel) String() string {
	switch l {
	case motionCalm:
		return "calm"
	case motionLively:
		return "lively"
	default:
		return "off"
	}
}

// next cycles off → calm → lively → off, the order the 'v' key steps through.
func (l motionLevel) next() motionLevel {
	return (l + 1) % 3
}

// animates reports whether this level draws any motion at all. off is the only
// level that doesn't, so the render and frame-scheduling paths gate on this
// rather than comparing against a literal.
func (l motionLevel) animates() bool { return l != motionOff }

// viewMode selects how the org is drawn. The tree is the default org-chart; the
// office is the floor-plan reframe (office.go). They're mutually exclusive renders
// of the same data — a toggle, not a layer — because they position the same nodes
// differently and can't share the viewport.
type viewMode uint8

const (
	viewTree viewMode = iota
	viewOffice
	viewMission
)

func (v viewMode) String() string {
	switch v {
	case viewOffice:
		return "office"
	case viewMission:
		return "mission control"
	default:
		return "tree"
	}
}
