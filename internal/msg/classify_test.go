package msg

import (
	"testing"

	"github.com/aymanmohammed/crew/internal/broker"
	"github.com/aymanmohammed/crew/internal/store"
)

func TestClassifyKnownWhenFromIDBoundToOurNode(t *testing.T) {
	m := broker.Message{FromID: "peer-a", ToID: "peer-b", Text: "hi"}
	nodes := []store.Node{{NodeID: "n1", PeerID: "peer-a"}}
	live := []broker.Peer{{ID: "peer-a"}, {ID: "peer-b"}}

	if got := Classify(m, nodes, live); got != Known {
		t.Fatalf("Classify = %v, want Known", got)
	}
}

func TestClassifyUnknownWhenLivePeerNotOurs(t *testing.T) {
	m := broker.Message{FromID: "peer-x", ToID: "peer-b", Text: "hi"}
	nodes := []store.Node{{NodeID: "n1", PeerID: "peer-a"}}
	live := []broker.Peer{{ID: "peer-a"}, {ID: "peer-b"}, {ID: "peer-x"}}

	if got := Classify(m, nodes, live); got != Unknown {
		t.Fatalf("Classify = %v, want Unknown (a real live peer, just not ours)", got)
	}
}

// The actual threat model this project has documented since the very start:
// the substrate accepts a from_id that matches no live peer at all -- the
// bundled CLI literally hardcodes from_id:"cli", never a registered peer.
func TestClassifyForgedWhenFromIDMatchesNoLivePeer(t *testing.T) {
	m := broker.Message{FromID: "cli", ToID: "peer-b", Text: "hi"}
	nodes := []store.Node{{NodeID: "n1", PeerID: "peer-a"}}
	live := []broker.Peer{{ID: "peer-a"}, {ID: "peer-b"}}

	if got := Classify(m, nodes, live); got != Forged {
		t.Fatalf("Classify = %v, want Forged", got)
	}
}

// A peer that WAS ours but has since gone -- our node still records the old
// peer_id (state=dead, tombstoned, row retained) but the peer is no longer
// live. A message claiming to be from that id now is exactly as suspect as
// any other unmatched-live-peer case: forged, not known, since the identity
// it claims isn't currently anyone.
func TestClassifyForgedWhenClaimingADeadNodesOldPeerID(t *testing.T) {
	m := broker.Message{FromID: "peer-a-old", ToID: "peer-b", Text: "hi"}
	nodes := []store.Node{{NodeID: "n1", PeerID: "peer-a-old", State: "dead"}}
	live := []broker.Peer{{ID: "peer-b"}} // peer-a-old is not in the live set

	if got := Classify(m, nodes, live); got != Forged {
		t.Fatalf("Classify = %v, want Forged (claimed identity is not currently live)", got)
	}
}

func TestOriginString(t *testing.T) {
	for o, want := range map[Origin]string{Known: "known", Unknown: "unknown", Forged: "forged"} {
		if got := o.String(); got != want {
			t.Fatalf("Origin(%d).String() = %q, want %q", o, got, want)
		}
	}
}
