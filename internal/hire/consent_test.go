package hire

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/spawn"
)

// THE ONE THAT MATTERS. An unset consent path must FAIL CLOSED. A wiring bug
// that forgets to set it must refuse to spawn — never silently spawn without
// the operator's grant.
func TestFlowRefusesToSpawnWithoutConsent(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, st := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "p", TTY: "ttys1"}}, nil
	})
	f.ConsentPath = "" // unset — the wiring-bug case

	_, err := f.Run(context.Background(), req(t))
	if err == nil {
		t.Fatal("spawned with no consent path set — must fail closed")
	}
	if !strings.Contains(err.Error(), "consent") {
		t.Fatalf("error does not name consent: %v", err)
	}
	if len(a.specs) != 0 {
		t.Fatal("a session was launched without consent")
	}
	nodes, _ := st.ListNodes()
	if len(nodes) != 0 {
		t.Fatal("a node row was written despite refusing to spawn")
	}
}

// A consent file that doesn't exist is not consent.
func TestFlowRefusesWhenConsentNeverGranted(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, _ := testFlow(t, a, func() ([]broker.Peer, error) { return nil, nil })
	f.ConsentPath = filepath.Join(t.TempDir(), "never-written")

	if _, err := f.Run(context.Background(), req(t)); err == nil {
		t.Fatal("spawned with no consent recorded")
	}
	if len(a.specs) != 0 {
		t.Fatal("a session was launched without consent")
	}
}

// Granted consent lets the hire proceed.
func TestFlowProceedsWithConsent(t *testing.T) {
	a := &fakeAdapter{handle: spawn.Handle{SpawnRef: "%1", TTY: "/dev/ttys1"}}
	f, _ := testFlow(t, a, func() ([]broker.Peer, error) {
		return []broker.Peer{{ID: "p", TTY: "ttys1"}}, nil
	})
	f.ConsentPath = filepath.Join(t.TempDir(), "consent")
	if err := RecordConsent(f.ConsentPath); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Run(context.Background(), req(t)); err != nil {
		t.Fatalf("Run with consent granted: %v", err)
	}
}

// Consent to an OLD version is not consent to the current terms. Silently
// carrying a stale grant past a material change to what we're asking for is
// the same dishonesty in slow motion.
func TestStaleConsentVersionDoesNotCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "consent")
	if err := writeFile(path, "version=0\ngranted_at=2020-01-01T00:00:00Z\n"); err != nil {
		t.Fatal(err)
	}
	if HasConsent(path) {
		t.Fatal("a grant for version 0 counted as consent to version " + ConsentVersion)
	}
}

func TestRecordThenHasConsentRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "consent")
	if HasConsent(path) {
		t.Fatal("consent before it was granted")
	}
	if err := RecordConsent(path); err != nil {
		t.Fatal(err)
	}
	if !HasConsent(path) {
		t.Fatal("consent not recognised after being recorded")
	}
}

// The text must actually disclose what we're doing. A consent screen that
// omits the consequence isn't consent — it's a speed bump with a checkbox.
func TestConsentTextDisclosesTheRealConsequences(t *testing.T) {
	for _, must := range []string{
		"ON YOUR BEHALF",  // that we auto-accept
		"EVERY session",   // that it's not one-time
		"forge",           // that forged messages are possible
		"cannot block",    // and that we can't stop them
		"There isn't one", // that no proper suppression flag exists
	} {
		if !strings.Contains(ConsentText, must) {
			t.Fatalf("consent text omits %q — an operator agreeing to this would "+
				"not know what they agreed to", must)
		}
	}
}

func writeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o600)
}
