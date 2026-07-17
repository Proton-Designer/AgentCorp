package msg

// DeliveryState is what CREW actually knows about a message, versus what it
// can only infer. Kept as one small enum with exactly two values so the two
// claims can never be silently upgraded into each other: notifications are
// never acknowledged (spec §S6/§10), so a successful Send proves the message
// reached the broker's transport and proves NOTHING about whether the
// recipient's session is even channel-active, let alone whether the model
// has read it.
type DeliveryState int

const (
	// Sent is the only state this package ever produces. It means Send
	// returned no error: the row exists in messages, delivered=0. Nothing
	// more can be asserted from here — no function in this package upgrades
	// a message to ActedOn.
	Sent DeliveryState = iota

	// ActedOn exists only so a CALLER has a distinct, named value to assign
	// after independently observing the recipient's own state change —
	// spec §13.1 (turn-boundary delivery, confirmed empirically): an inbound
	// message cannot preempt a busy agent, so "acted on" can only ever be
	// inferred later from an observed effect, never assumed at send time.
	// This package never returns it.
	ActedOn
)

// Render is the UI-facing label for a DeliveryState. Centralized here so
// every caller uses the same honest wording instead of inventing its own —
// a Sent message renders as "queued", never "delivered", per §13.1's design
// consequence that inbound messages must not be shown as instantly acted on.
func (s DeliveryState) Render() string {
	switch s {
	case Sent:
		return "queued"
	case ActedOn:
		return "acted on"
	default:
		return "?"
	}
}
