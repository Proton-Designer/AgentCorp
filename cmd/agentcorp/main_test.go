package main

import "testing"

func TestSanitizeNameStripsEscapesAndControls(t *testing.T) {
	cases := map[string]string{
		"t\x1b[Bi\x1bl\n":  "til",       // arrows/escape dropped, letters kept
		"  Acme Corp  \n":  "Acme Corp", // normal name, trimmed
		"\x1b[B\x1b[A\r\n": "",          // pure navigation -> empty (unscoped)
		"Galaxy":           "Galaxy",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
