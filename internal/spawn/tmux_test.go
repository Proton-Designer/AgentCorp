package spawn

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireTmux skips the test if tmux isn't on PATH, so the suite degrades
// gracefully in an environment without it rather than failing hard.
func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
}

// newTestAdapter returns an adapter on an isolated tmux socket (`-L`), so
// tests never touch the operator's real tmux server or the live broker four
// unrelated MyHomebase sessions depend on. claudeArgs defaults to a harmless
// command — Launch is exercised end to end, including every real tmux call,
// but nothing resembling a real `claude` process is ever started.
//
// Bootstraps one initial session on the socket before returning: `tmux
// new-window` attaches a window to an existing session, it does not create
// a server+session from scratch (found by running it against a bare socket
// and getting "error connecting ... No such file or directory"). This
// mirrors production: spec §8 has the CREW console itself occupy window 0
// of an already-running session before any hire ever calls Launch, so
// Launch legitimately assumes a session exists — bootstrapping one here is
// test setup, not a workaround for a real gap.
func newTestAdapter(t *testing.T) *TmuxWindowAdapter {
	t.Helper()
	requireTmux(t)

	socket := fmt.Sprintf("crew-spawn-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "console", "-c", "/tmp").CombinedOutput(); err != nil {
		t.Fatalf("bootstrap tmux session: %v: %s", err, out)
	}

	a := NewTmuxWindowAdapter(socket)
	a.claudeArgs = func(string) []string { return []string{"/bin/true"} }

	t.Cleanup(func() {
		exec.Command("tmux", "-L", socket, "kill-server").Run() // best-effort; errors if a server never started
	})
	return a
}

func writePromptFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	return path
}

// markerAppeared polls for marker's existence every 25ms up to timeout,
// returning true the instant it appears, false only once the full timeout
// has elapsed with no sighting.
//
// This exists because `tmux respawn-pane` (like any process launch) returns
// as soon as the new process is started, not once it has run — a plain
// os.Stat immediately afterward checks for the marker before an injected
// payload could possibly have had time to execute, so it passes whether the
// code is safe or not. Found the hard way: a mutation that reintroduced the
// exact vulnerability this package defends against still passed the
// unpolled version of this check. A bounded poll is fast when the code is
// vulnerable (fails on first sighting, well under the timeout) and
// necessarily takes the full timeout to confirm safety (absence can only be
// confirmed by waiting out the window) — that asymmetry is inherent to
// testing for the absence of an async event, not a flaw in this helper.
func markerAppeared(marker string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// --- Pure argv-construction checks: no execution at all -------------------

// This is the "build the command, assert its argv, and stop" half of the
// task: proves the real claude launch keeps hostile prompt content as one
// atomic argv element, without ever running a process to prove it.
func TestDefaultClaudeArgsKeepsPromptAsOneElement(t *testing.T) {
	hostile := "'; touch /tmp/pwned; ' `touch /tmp/pwned2` $(touch /tmp/pwned3)\nsecond line"
	args := defaultClaudeArgs(hostile)

	if len(args) != 5 {
		t.Fatalf("defaultClaudeArgs returned %d args, want 5: %q", len(args), args)
	}
	if args[0] != "claude" {
		t.Fatalf("args[0] = %q, want \"claude\"", args[0])
	}
	last := args[len(args)-1]
	if last != hostile {
		t.Fatalf("prompt content was altered: got %q, want %q", last, hostile)
	}
	// Every element besides the prompt must be a fixed, CREW-controlled
	// literal — never something built by concatenating operator text.
	for _, a := range args[:len(args)-1] {
		if strings.Contains(a, hostile) {
			t.Fatalf("hostile content leaked into a non-prompt argv element: %q", a)
		}
	}
}

func TestLaunchRejectsUnsupportedMode(t *testing.T) {
	a := NewTmuxWindowAdapter("") // no tmux call should ever happen
	_, err := a.Launch(context.Background(), Spec{Workdir: t.TempDir(), Mode: "tmux-split"})
	if err == nil {
		t.Fatal("want an error for an unimplemented mode, got nil")
	}
}

func TestLaunchRejectsMissingWorkdir(t *testing.T) {
	a := NewTmuxWindowAdapter("")
	_, err := a.Launch(context.Background(), Spec{Workdir: filepath.Join(t.TempDir(), "does-not-exist")})
	if err == nil {
		t.Fatal("want an error for a nonexistent workdir, got nil")
	}
}

func TestLaunchRejectsWorkdirThatIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewTmuxWindowAdapter("")
	_, err := a.Launch(context.Background(), Spec{Workdir: file})
	if err == nil {
		t.Fatal("want an error when workdir is a file, got nil")
	}
}

func TestLaunchRejectsMissingPromptFile(t *testing.T) {
	a := NewTmuxWindowAdapter("")
	_, err := a.Launch(context.Background(), Spec{
		Workdir:    t.TempDir(),
		PromptFile: filepath.Join(t.TempDir(), "does-not-exist.txt"),
	})
	if err == nil {
		t.Fatal("want an error for a missing prompt file, got nil")
	}
}

// --- Real tmux, isolated socket, harmless substitute command --------------

func TestLaunchAgainstRealTmux(t *testing.T) {
	a := newTestAdapter(t)
	promptFile := writePromptFile(t, "you are a helpful assistant")

	h, err := a.Launch(context.Background(), Spec{
		Name: "test-node", Role: "dev", Workdir: t.TempDir(), PromptFile: promptFile,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !strings.HasPrefix(h.SpawnRef, "%") {
		t.Fatalf("SpawnRef = %q, want a tmux pane id (starts with %%)", h.SpawnRef)
	}
	if h.TTY == "" {
		t.Fatal("TTY is empty")
	}
}

// The adversarial test named in the task, run for real against tmux: a hire
// with this exact payload as its Name must never execute anything. Only
// /bin/true is ever actually run in the pane — Name never reaches that
// command's argv at all, so this is purely about whether tmux's own
// argument handling (new-window -n) can be tricked into shell or
// command-chain execution.
//
// Uses markerAppeared (bounded poll), not an immediate os.Stat — the risk
// this test guards against is latent today (there is no code path from Name
// into anything executed), but a latent race in a security test is a
// security test that will lie the moment a future change gives Name such a
// path, exactly as happened with TestLaunchAdversarialPromptContentNeverExecutes.
func TestLaunchAdversarialNameNeverExecutes(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	payloads := []string{
		fmt.Sprintf("'; touch %s; '", marker),
		fmt.Sprintf("`touch %s`", marker),
		fmt.Sprintf("$(touch %s)", marker),
		"&& touch " + marker,
		"| touch " + marker,
	}

	for _, name := range payloads {
		t.Run(name, func(t *testing.T) {
			os.Remove(marker)
			a := newTestAdapter(t)
			_, err := a.Launch(context.Background(), Spec{
				Name: name, Workdir: t.TempDir(), PromptFile: writePromptFile(t, "x"),
			})
			// Launch must SUCCEED here — verified manually that tmux accepts
			// every one of these as a literal window name. An error would
			// make this test pass vacuously (no marker file because Launch
			// never got far enough to run anything), which would prove
			// nothing about tmux's argument handling.
			if err != nil {
				t.Fatalf("Launch failed on payload %q: %v (tmux is expected to accept this as a literal name)", name, err)
			}
			if markerAppeared(marker, time.Second) {
				t.Fatalf("marker file %s was created — payload %q executed", marker, name)
			}
		})
	}
}

// A literal newline in the name: tmux is expected to reject this outright
// ("invalid window name"), verified manually against the real binary before
// writing this test. Confirms Launch surfaces that as a clean error rather
// than panicking or silently doing something else, and that nothing executes.
//
// Exempt from the markerAppeared bounded-poll audit applied elsewhere in
// this file: when new-window itself fails, Launch returns immediately and
// respawn-pane is never called — there is no code path left running after
// this test's os.Stat that could still create the marker. That's a
// structural guarantee (respawn-pane is textually unreachable on this
// error path), not a timing assumption, which is the distinction that
// makes an immediate check safe here and nowhere else in this file.
func TestLaunchAdversarialNewlineInNameIsRejectedCleanly(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned-newline")
	os.Remove(marker)
	a := newTestAdapter(t)

	_, err := a.Launch(context.Background(), Spec{
		Name:       "line1\nline2 " + marker,
		Workdir:    t.TempDir(),
		PromptFile: writePromptFile(t, "x"),
	})
	if err == nil {
		t.Fatal("want an error for a newline-containing window name, got nil")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("marker file was created from a newline-containing name")
	}
}

// The other injection surface: prompt file CONTENT (not the name) flowing
// into --append-system-prompt via respawn-pane. Uses a claudeArgs override
// that echoes the prompt content back into the pane, so the captured output
// proves the content arrived literally rather than being interpreted.
func TestLaunchAdversarialPromptContentNeverExecutes(t *testing.T) {
	requireTmux(t)
	marker := filepath.Join(t.TempDir(), "pwned-prompt")
	os.Remove(marker)
	hostile := fmt.Sprintf("'; touch %s; ' `touch %s` $(touch %s)", marker, marker, marker)

	socket := fmt.Sprintf("crew-spawn-test-prompt-%d", os.Getpid())
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "console", "-c", "/tmp").CombinedOutput(); err != nil {
		t.Fatalf("bootstrap tmux session: %v: %s", err, out)
	}
	a := NewTmuxWindowAdapter(socket)
	a.claudeArgs = func(promptContent string) []string { return []string{"/bin/echo", promptContent} }
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })

	h, err := a.Launch(context.Background(), Spec{
		Name: "prompt-test", Workdir: t.TempDir(), PromptFile: writePromptFile(t, hostile),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// respawn-pane returns as soon as it starts the pane's process, not
	// once that process has run — an immediate os.Stat here checks for the
	// marker before an injected payload could possibly have executed, so it
	// would pass whether the code is safe or not. This is the exact bug a
	// mutation test (reintroducing the shell-string vulnerability this
	// package defends against) caught: the vulnerable build still passed
	// this check under the old immediate-stat form. markerAppeared polls
	// instead, so a real injection is actually observed rather than raced.
	if markerAppeared(marker, time.Second) {
		t.Fatal("marker file was created — prompt content executed as a shell command")
	}

	// Bonus verification: the echoed pane output should contain the literal
	// payload, proving it reached the process as inert text, not that it
	// silently vanished for an unrelated reason. Safe to check now — the
	// poll above already waited long enough for echo to have rendered.
	out, capErr := exec.Command("tmux", "-L", socket, "capture-pane", "-t", h.SpawnRef, "-p").Output()
	if capErr == nil && !strings.Contains(string(out), "touch") {
		t.Logf("captured pane output did not contain the literal payload (informational, not a hard failure): %q", out)
	}
}

// REGRESSION: the returned Handle.TTY must be the pane's tty AFTER respawn.
//
// respawn-pane -k allocates a fresh pty, so the tty changes across it while the
// pane id stays fixed. Capturing the tty from the pre-respawn shell records a
// value no live peer will ever register with, and every hire silently fails to
// bind. A real first hire surfaced exactly this: CREW stored ttys011, the pane
// was actually ttys016, bind never matched, the node went 'failed' while the
// agent ran on as unmanaged.
//
// This test runs a real (harmless) respawn and asserts the Handle's tty equals
// what tmux reports for that pane NOW — which is only true if capture happens
// after respawn.
func TestLaunchTTYIsPostRespawnTTY(t *testing.T) {
	requireTmux(t)
	socket := fmt.Sprintf("crew-tty-regression-%d", os.Getpid())
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "console", "-c", "/tmp").CombinedOutput(); err != nil {
		t.Fatalf("bootstrap tmux: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })

	a := NewTmuxWindowAdapter(socket)
	// A process that stays alive so the pane persists for the tty query.
	a.claudeArgs = func(string) []string { return []string{"/bin/sh", "-c", "sleep 5"} }

	h, err := a.Launch(context.Background(), Spec{
		Name: "tty-test", Workdir: t.TempDir(), PromptFile: writePromptFile(t, "x"),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// What tmux actually reports for the pane right now.
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", h.SpawnRef, "#{pane_tty}").Output()
	if err != nil {
		t.Fatalf("query pane tty: %v", err)
	}
	actual := strings.TrimSpace(string(out))

	if h.TTY != actual {
		t.Fatalf("Handle.TTY = %q but the pane's real tty is %q — a hire would look "+
			"for the wrong tty and never bind", h.TTY, actual)
	}
	if h.TTY == "" {
		t.Fatal("Handle.TTY is empty")
	}
}
