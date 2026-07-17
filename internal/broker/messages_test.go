package broker

import (
	"path/filepath"
	"testing"
)

// Against a real broker: this machine has hundreds of real messages from
// unrelated live sessions. Assert shape only — never print, log, or include
// Text in a failure message. Their content isn't ours to surface, and that
// discipline holds in tests exactly as much as in the shipped product.
func TestListMessagesAgainstRealBroker(t *testing.T) {
	path := realBrokerPath(t)

	msgs, err := ListMessages(path)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("want at least one message in this broker's history, got zero")
	}
	for _, m := range msgs {
		if m.ID == 0 {
			t.Fatal("message has zero ID") // no %v: never print the row, it carries Text
		}
		if m.FromID == "" || m.ToID == "" {
			t.Fatal("message has empty FromID or ToID")
		}
		if m.SentAt == "" {
			t.Fatal("message has empty SentAt")
		}
		if m.Text == "" {
			t.Fatal("message has empty Text") // check presence only, never print the content
		}
	}
}

// Messages must be ordered oldest-first: Throughput's bucketing and Ticker's
// "most recent" both depend on this ordering rather than re-sorting it
// themselves.
func TestListMessagesOrderedOldestFirst(t *testing.T) {
	path := realBrokerPath(t)

	msgs, err := ListMessages(path)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for i := 1; i < len(msgs); i++ {
		if msgs[i].SentAt < msgs[i-1].SentAt {
			t.Fatalf("messages out of order at index %d: %q < %q", i, msgs[i].SentAt, msgs[i-1].SentAt)
		}
	}
}

func TestListMessagesOnMissingDBReturnsErrorNotEmptySlice(t *testing.T) {
	msgs, err := ListMessages(filepath.Join(t.TempDir(), "does-not-exist.db"))
	if err == nil {
		t.Fatalf("want an error for a nonexistent broker db, got msgs=%v, err=nil", msgs)
	}
	if msgs != nil {
		t.Fatalf("want a nil slice alongside the error, got %v", msgs)
	}
}
