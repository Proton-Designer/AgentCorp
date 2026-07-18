package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
	"github.com/Proton-Designer/AgentCorp/internal/company"
)

func TestScopeMessagesKeepsOnlyCompanyTraffic(t *testing.T) {
	root := t.TempDir()
	if _, err := company.Create(root, "Galaxy", "co-1"); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "svc")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	croot := canonDir(t, root)

	peers := []broker.Peer{
		{ID: "mine", CWD: inside},
		{ID: "theirs", CWD: t.TempDir()}, // a different, unrelated tree
	}
	msgs := []broker.Message{
		{FromID: "mine", ToID: "theirs", Text: "hi"},        // my agent spoke → keep
		{FromID: "theirs", ToID: "mine", Text: "reply"},     // to my agent → keep
		{FromID: "theirs", ToID: "otherx", Text: "chatter"}, // unrelated → drop
		{FromID: "agentcorp", ToID: "mine", Text: "op msg"}, // console → keep
	}

	got := scopeMessages(croot, peers, msgs)
	if len(got) != 3 {
		t.Fatalf("kept %d messages, want 3 (dropped only the unrelated one): %+v", len(got), got)
	}
	for _, m := range got {
		if m.FromID == "theirs" && m.ToID == "otherx" {
			t.Fatal("unrelated cross-company message leaked into the company view")
		}
	}
}

func TestScopeMessagesUnscopedKeepsAll(t *testing.T) {
	msgs := []broker.Message{{FromID: "a", ToID: "b"}, {FromID: "c", ToID: "d"}}
	if got := scopeMessages("", nil, msgs); len(got) != 2 {
		t.Fatalf("unscoped must keep all messages, got %d", len(got))
	}
}
