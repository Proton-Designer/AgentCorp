package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

// 'l' opens the activity feed, resolving peer ids to node names, newest first.
func TestActivityFeed(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "boss", CreatedAt: "1"},
		store.Node{NodeID: "2", Name: "worker", Role: "dev", ParentID: "1", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", PeerID: "w1", CreatedAt: "2"},
	)
	m.live.msgs = []broker.Message{
		{FromID: "boss", ToID: "w1", Text: "start the build", SentAt: "2026-07-18T06:00:00Z"},
		{FromID: "w1", ToID: "boss", Text: "build green", SentAt: "2026-07-18T06:05:00Z"},
	}
	m = send(m, "l")
	if m.mode != modeFeed {
		t.Fatalf("l did not open the feed (mode=%v)", m.mode)
	}
	v := m.View()
	// Peer ids resolved to names, both messages shown.
	for _, want := range []string{"ceo", "worker", "start the build", "build green", "2 messages"} {
		if !strings.Contains(v, want) {
			t.Fatalf("feed missing %q:\n%s", want, v)
		}
	}
	// Newest ("build green") should appear before oldest ("start the build").
	if strings.Index(v, "build green") > strings.Index(v, "start the build") {
		t.Fatal("feed is not newest-first")
	}
	nm, _ := m.Update(key("esc"))
	if nm.(Model).mode != modeNormal {
		t.Fatal("esc did not close the feed")
	}
}

// The feed handles an empty message set without panicking.
func TestActivityFeedEmpty(t *testing.T) {
	m, _ := liveModelWith(t,
		store.Node{NodeID: "1", Name: "ceo", Role: "lead", Workdir: "/t", SpawnMode: "tmux-window", State: "alive", CreatedAt: "1"},
	)
	m.live.msgs = nil
	m = send(m, "l")
	if !strings.Contains(m.View(), "no messages yet") {
		t.Fatalf("empty feed should say so:\n%s", m.View())
	}
}
