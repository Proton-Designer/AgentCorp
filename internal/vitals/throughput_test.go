package vitals

import (
	"testing"
	"time"

	"github.com/Proton-Designer/AgentCorp/internal/broker"
)

func tmsg(sentAt string) broker.Message {
	return broker.Message{ID: 1, FromID: "a", ToID: "b", Text: "x", SentAt: sentAt}
}

func TestThroughputBucketsByMinute(t *testing.T) {
	// window is 3 minutes, now = 12:03:00, so buckets cover
	// [12:00,12:01) [12:01,12:02) [12:02,12:03)
	now := mustParse("2026-07-16T12:03:00Z")
	msgs := []broker.Message{
		tmsg("2026-07-16T12:00:10Z"), // bucket 0
		tmsg("2026-07-16T12:00:40Z"), // bucket 0
		tmsg("2026-07-16T12:01:05Z"), // bucket 1
		tmsg("2026-07-16T12:02:59Z"), // bucket 2
	}

	got := Throughput(msgs, 3*time.Minute, now)
	want := []int{2, 1, 1}
	if len(got) != len(want) {
		t.Fatalf("Throughput = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Throughput = %v, want %v", got, want)
		}
	}
}

func TestThroughputExcludesMessagesOutsideWindow(t *testing.T) {
	now := mustParse("2026-07-16T12:03:00Z")
	msgs := []broker.Message{
		tmsg("2026-07-16T11:59:00Z"), // before window
		tmsg("2026-07-16T12:03:30Z"), // after "now"
		tmsg("2026-07-16T12:01:00Z"), // inside window
	}

	got := Throughput(msgs, 3*time.Minute, now)
	total := 0
	for _, c := range got {
		total += c
	}
	if total != 1 {
		t.Fatalf("total counted = %d, want 1 (only the in-window message)", total)
	}
}

func TestThroughputIgnoresUnparseableTimestamps(t *testing.T) {
	now := mustParse("2026-07-16T12:03:00Z")
	msgs := []broker.Message{
		{ID: 1, FromID: "a", ToID: "b", Text: "x", SentAt: "not-a-timestamp"},
		tmsg("2026-07-16T12:01:00Z"),
	}
	got := Throughput(msgs, 3*time.Minute, now)
	total := 0
	for _, c := range got {
		total += c
	}
	if total != 1 {
		t.Fatalf("total counted = %d, want 1 (malformed timestamp skipped, not crashed on)", total)
	}
}

func TestThroughputEmptyMessagesAllZero(t *testing.T) {
	now := mustParse("2026-07-16T12:03:00Z")
	got := Throughput(nil, 3*time.Minute, now)
	for i, c := range got {
		if c != 0 {
			t.Fatalf("bucket %d = %d, want 0", i, c)
		}
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 buckets for a 3-minute window", len(got))
	}
}

func TestThroughputZeroOrNegativeWindowReturnsNil(t *testing.T) {
	now := mustParse("2026-07-16T12:03:00Z")
	if got := Throughput(nil, 0, now); got != nil {
		t.Fatalf("Throughput with 0 window = %v, want nil", got)
	}
	if got := Throughput(nil, -time.Minute, now); got != nil {
		t.Fatalf("Throughput with negative window = %v, want nil", got)
	}
}

func TestThroughputSubMinuteWindowGetsOneBucket(t *testing.T) {
	now := mustParse("2026-07-16T12:03:00Z")
	got := Throughput([]broker.Message{tmsg("2026-07-16T12:02:50Z")}, 30*time.Second, now)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != 1 {
		t.Fatalf("got[0] = %d, want 1", got[0])
	}
}
