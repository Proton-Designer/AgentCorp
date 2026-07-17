package msg

import "testing"

func TestDeliveryStatesAreDistinct(t *testing.T) {
	if Sent == ActedOn {
		t.Fatal("Sent and ActedOn must be distinct values -- conflating them is the exact substrate-overselling this project refuses to do")
	}
}

// This is the literal requirement: a Sent message must render as queued,
// never as delivered or acted-on, per S11 (turn-boundary delivery) and S6
// (notifications are never acknowledged).
func TestSentRendersAsQueuedNeverDelivered(t *testing.T) {
	if got := Sent.Render(); got != "queued" {
		t.Fatalf("Sent.Render() = %q, want %q", got, "queued")
	}
}

func TestActedOnRendersDistinctlyFromSent(t *testing.T) {
	if got := ActedOn.Render(); got == Sent.Render() {
		t.Fatalf("ActedOn and Sent render identically (%q) -- defeats the point of keeping them distinct", got)
	}
}
