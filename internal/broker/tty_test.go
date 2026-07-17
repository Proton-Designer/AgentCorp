package broker

import "testing"

func TestNormalizeTTY(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tmux format strips /dev/", "/dev/ttys024", "ttys024"},
		{"already bare passes through", "ttys000", "ttys000"},
		{"empty stays empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeTTY(c.in); got != c.want {
				t.Fatalf("NormalizeTTY(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// The whole point of normalization: the tmux format and the broker format
// for the same physical pane must compare equal after normalizing both
// sides. Exact string equality between them never holds (spec §6.1).
func TestNormalizeTTYMakesTmuxAndBrokerFormatsEqual(t *testing.T) {
	tmuxFormat := "/dev/ttys024"
	brokerFormat := "ttys024"

	if tmuxFormat == brokerFormat {
		t.Fatal("test fixture invalid: formats must differ before normalization")
	}
	if NormalizeTTY(tmuxFormat) != NormalizeTTY(brokerFormat) {
		t.Fatalf("NormalizeTTY(%q)=%q != NormalizeTTY(%q)=%q, want equal",
			tmuxFormat, NormalizeTTY(tmuxFormat), brokerFormat, NormalizeTTY(brokerFormat))
	}
}
