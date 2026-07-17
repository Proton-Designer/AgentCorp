package hire

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakePane models a REAL terminal pane, which behaves two different ways
// depending on what is on it:
//
//   - A PROMPT holds. It sits there until you answer it.
//   - Anything else (splash, boot output) advances on its own.
//
// Getting this distinction right matters more than it looks. My first fake
// advanced on every capture — that models neither, and it made a correct
// implementation look broken. My second held on everything — that deadlocks on
// a boot screen nobody answers. A fake that doesn't model the real thing tests
// nothing, and worse, it sends you chasing bugs that aren't there.
//
// `holdAfter` polls of a non-prompt frame, the pane moves on.
type fakePane struct {
	frames    []string // screens in order
	prompts   []string // substrings that make a frame HOLD until answered
	holdAfter int      // captures a non-prompt frame lingers before advancing

	idx      int
	calls    int // capture calls, for asserting send ordering
	sinceAdv int
	sent     [][]string
	sendAt   []int // capture-count at the moment each send happened
}

func (f *fakePane) isPrompt(s string) bool {
	for _, p := range f.prompts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func (f *fakePane) cur() string {
	if len(f.frames) == 0 {
		return ""
	}
	if f.idx >= len(f.frames) {
		return f.frames[len(f.frames)-1]
	}
	return f.frames[f.idx]
}

func (f *fakePane) capture(ctx context.Context, pane string) (string, error) {
	f.calls++
	s := f.cur()
	// Non-prompt frames drift past on their own, as a booting session does.
	if !f.isPrompt(s) && f.idx < len(f.frames)-1 {
		f.sinceAdv++
		if f.holdAfter == 0 || f.sinceAdv >= f.holdAfter {
			f.idx++
			f.sinceAdv = 0
		}
	}
	return s, nil
}

func (f *fakePane) send(ctx context.Context, pane string, keys []string) error {
	f.sent = append(f.sent, keys)
	f.sendAt = append(f.sendAt, f.calls)
	f.idx++ // answering a prompt advances the screen, as it does for real
	f.sinceAdv = 0
	return nil
}

// gatePrompts are the substrings the real gates match on.
var gatePrompts = []string{
	"Do you trust the files in this folder?",
	"Loading development channels",
}

func clearer(f *fakePane) *GateClearer {
	return &GateClearer{
		Gates:    DefaultGates,
		Capture:  f.capture,
		SendKeys: f.send,
		Timeout:  2 * time.Second,
		Poll:     time.Millisecond,
	}
}

// Both gates fire (fresh workdir): both get answered, in order.
func TestClearAnswersBothGatesInOrder(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{
		"booting...",
		"Do you trust the files in this folder?\n 1. Yes",
		"Loading development channels\n 1. I am using this for local development",
		"ready >",
	}}
	if err := clearer(f).Clear(context.Background(), "%1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(f.sent) != 2 {
		t.Fatalf("sent %d key batches, want 2 (one per gate)", len(f.sent))
	}
}

// Trusted workdir: only the channel gate fires. The optional gate must be
// skipped without failing the hire — this is the common case for any workdir
// the operator has used before.
func TestClearSkipsAbsentOptionalGate(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{
		"booting...",
		"Loading development channels\n 1. I am using this for local development",
		"ready >",
	}}
	if err := clearer(f).Clear(context.Background(), "%1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("sent %d key batches, want 1 (channel gate only)", len(f.sent))
	}
}

// THE LOAD-BEARING TEST. Keys must NEVER be sent before the gate's text is
// actually on screen. A fixed-delay send would, on a fast machine, type Enter
// into the agent's prompt instead of the dialog — submitting an empty turn to
// a brand-new session. So: never blind.
//
// The pane sits on a splash screen for several polls before the prompt shows.
// Any send during that window is the bug.
func TestClearNeverSendsKeysBeforeSeeingTheGate(t *testing.T) {
	f := &fakePane{
		prompts:   gatePrompts,
		holdAfter: 4, // splash lingers for 4 polls before the prompt appears
		frames: []string{
			"booting...",
			"Loading development channels\n 1. I am using this for local development",
		},
	}
	c := &GateClearer{
		Gates:    []Gate{DefaultGates[1]}, // channel gate only
		Capture:  f.capture,
		SendKeys: f.send,
		Timeout:  2 * time.Second,
		Poll:     time.Millisecond,
	}
	if err := c.Clear(context.Background(), "%1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(f.sendAt) != 1 {
		t.Fatalf("sent %d times, want 1", len(f.sendAt))
	}
	if f.sendAt[0] < 4 {
		t.Fatalf("keys sent after only %d captures — before the gate was ever visible; "+
			"a real session would have received Enter at its prompt", f.sendAt[0])
	}
}

// Hiring into an ALREADY-TRUSTED directory is the common case, and it must not
// pay a grace window for a prompt that will never fire. Sequencing the gates
// would stall every such hire; polling for whichever gate is present costs
// nothing when one is absent.
func TestClearDoesNotStallWhenOptionalGateAbsent(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{
		"Loading development channels\n 1. I am using this for local development",
		"ready >",
	}}
	c := clearer(f)
	c.Timeout = 5 * time.Second // generous: the point is it returns fast anyway

	start := time.Now()
	if err := c.Clear(context.Background(), "%1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("took %s with the optional gate absent — every hire into a "+
			"trusted directory would pay this", elapsed)
	}
}

// A required gate that never appears must fail LOUDLY, not hang. The session
// is stuck somewhere we don't understand, and a silent hang would present as
// 'the hire never completed' with no explanation.
func TestClearFailsLoudlyWhenRequiredGateNeverAppears(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{"booting...", "some unrecognised screen"}}
	c := clearer(f)
	c.Timeout = 150 * time.Millisecond
	err := c.Clear(context.Background(), "%1")
	if err == nil {
		t.Fatal("required gate never appeared but Clear returned nil — the hire would hang silently")
	}
	if !strings.Contains(err.Error(), "channel-consent") {
		t.Fatalf("error does not name the stuck gate: %v", err)
	}
}

// Clear must not hang past its timeout even if the pane never changes.
func TestClearRespectsTimeout(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{"nothing ever happens here"}}
	c := clearer(f)
	c.Timeout = 100 * time.Millisecond

	done := make(chan struct{})
	go func() { _ = c.Clear(context.Background(), "%1"); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Clear hung past its timeout")
	}
}

// A cancelled context aborts promptly.
func TestClearHonoursContextCancellation(t *testing.T) {
	f := &fakePane{prompts: gatePrompts, frames: []string{"booting..."}}
	c := clearer(f)
	c.Timeout = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- c.Clear(ctx, "%1") }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Clear ignored context cancellation")
	}
}

// The channel gate must be non-optional. If someone marks it optional, hires
// silently proceed past an unanswered prompt and hang at bind instead —
// failing far from the cause. Measured: it fires on EVERY session.
func TestChannelGateIsNotOptional(t *testing.T) {
	for _, g := range DefaultGates {
		if g.Name == "channel-consent" && g.Optional {
			t.Fatal("channel-consent marked optional: it fires on every session " +
				"regardless of directory trust; making it optional would let hires " +
				"proceed past an unanswered prompt and fail later at bind")
		}
	}
}
