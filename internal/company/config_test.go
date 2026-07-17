package company

import (
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Company
		wantErr bool
	}{
		{
			name: "well-formed quoted",
			in:   "id = \"co-1\"\nname = \"Acme Corp\"\n",
			want: Company{ID: "co-1", Name: "Acme Corp"},
		},
		{
			name: "comments and blank lines ignored",
			in:   "# a company\n\n  id = \"co-2\"  \n\n  name = \"Beta\"\n# trailing\n",
			want: Company{ID: "co-2", Name: "Beta"},
		},
		{
			name: "bare (unquoted) values",
			in:   "id = co-3\nname = Gamma\n",
			want: Company{ID: "co-3", Name: "Gamma"},
		},
		{
			name: "unknown key ignored for forward-compat",
			in:   "id = \"co-4\"\nname = \"Delta\"\ntier = \"enterprise\"\n",
			want: Company{ID: "co-4", Name: "Delta"},
		},
		{
			name: "escaped quote in name",
			in:   "id = \"co-5\"\nname = \"The \\\"Real\\\" Co\"\n",
			want: Company{ID: "co-5", Name: `The "Real" Co`},
		},
		{
			name: "escaped newline in name",
			in:   "id = \"co-6\"\nname = \"Line1\\nLine2\"\n",
			want: Company{ID: "co-6", Name: "Line1\nLine2"},
		},
		{name: "missing id", in: "name = \"NoID\"\n", wantErr: true},
		{name: "missing name", in: "id = \"co-7\"\n", wantErr: true},
		{name: "empty id value", in: "id = \"\"\nname = \"X\"\n", wantErr: true},
		{name: "line without equals", in: "id = \"co-8\"\nname\n", wantErr: true},
		{name: "unterminated quote", in: "id = \"co-9\nname = \"Y\"\n", wantErr: true},
		{name: "unknown escape", in: "id = \"co-\\x\"\nname = \"Z\"\n", wantErr: true},
		{name: "totally empty", in: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseConfig([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The property that makes the format trustworthy: anything we write, we can
// read back — even names with the characters most likely to break a naive
// single-line config format.
func TestFormatParseRoundTrip(t *testing.T) {
	companies := []Company{
		{ID: "co-1", Name: "Acme Corp"},
		{ID: "co-2", Name: `Quotes "inside" name`},
		{ID: "co-3", Name: "Tabs\tand\nnewlines"},
		{ID: "co-4", Name: `Back\slashes\here`},
		{ID: "co-5", Name: "Trailing spaces   "},
		{ID: "co-with-#hash", Name: "# not a comment"},
		{ID: "co-6", Name: "name = with equals"},
	}
	for _, c := range companies {
		t.Run(c.ID, func(t *testing.T) {
			data := FormatConfig(c)
			got, err := ParseConfig(data)
			if err != nil {
				t.Fatalf("round-trip parse failed for %+v: %v\nrendered:\n%s", c, err, data)
			}
			if got != c {
				t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v\nrendered:\n%s", got, c, data)
			}
		})
	}
}

// A hand-edited file that opens a value quote but omits the closing one must be
// reported, not silently truncated into a plausible-looking name.
func TestFormatIsCommentedForHumans(t *testing.T) {
	data := string(FormatConfig(Company{ID: "co-1", Name: "Acme"}))
	if !strings.Contains(data, "# AgentCorp company definition.") {
		t.Fatalf("expected a human-readable header comment, got:\n%s", data)
	}
	if !strings.Contains(data, "leave 'id' stable") {
		t.Fatalf("expected guidance to leave id stable, got:\n%s", data)
	}
}

func TestKeysOfStable(t *testing.T) {
	got := keysOf()
	want := []string{"id", "name"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
