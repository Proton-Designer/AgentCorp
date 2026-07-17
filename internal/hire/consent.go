package hire

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConsentText is shown once, on AgentCorp's first run, before it can spawn anything.
//
// This exists because of a deliberate ethical decision (spec §6.2 / SE-5), and
// the reasoning belongs next to the code rather than only in a doc:
//
// Claude Code guards development channels behind a warning the human must
// accept — on EVERY session, not once per machine. AgentCorp clears that gate
// automatically on every hire, which DEFEATS THE GATE BY DESIGN. The gate
// exists so a person consciously opts into a risky capability; automating it
// removes exactly the consciousness it was built to require.
//
// We do it anyway, because asking per-hire would make the tool unusable and
// because running AgentCorp *is* the opt-in. But that only holds if the consent is
// obtained honestly, once, where a human can actually answer — showing the
// same warning, and saying plainly that we will accept on their behalf from
// then on. Silently clicking through a security prompt the user never saw
// would be exactly the overselling this project refuses everywhere else.
const ConsentText = `
  AgentCorp spawns agents with development channels enabled.

  Claude Code guards this behind a warning, and it asks EVERY session —
  not once per machine:

      "WARNING: Loading development channels...
       1. I am using this for local development
       2. Exit"

  That warning exists so a human consciously opts in. AgentCorp will accept it
  ON YOUR BEHALF for every agent it spawns, because asking you once per
  hire would make the tool unusable.

  This means: any agent AgentCorp spawns can push messages into your other
  Claude Code sessions, and any local process can forge messages that
  AgentCorp will surface but cannot block.

  We looked for a flag to suppress the prompt properly. There isn't one.

  If you'd rather not grant that, do not run AgentCorp — the tool is the consent.
`

// ConsentRecord is what gets written once the operator agrees.
type ConsentRecord struct {
	GrantedAt string
	Version   string // which text they agreed to
}

// ConsentVersion changes whenever ConsentText changes materially.
//
// Bumping it re-asks. That is the point: consent to one set of terms is not
// consent to a different set, and silently carrying an old grant forward past
// a material change would be the same dishonesty in slow motion.
const ConsentVersion = "1"

// ConsentPath is where the grant is recorded.
func ConsentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agentcorp", "consent"), nil
}

// HasConsent reports whether the operator has agreed to the CURRENT text.
//
// A grant for an older version does not count — see ConsentVersion.
func HasConsent(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "version=") &&
			strings.TrimPrefix(line, "version=") == ConsentVersion {
			return true
		}
	}
	return false
}

// RecordConsent persists the grant.
func RecordConsent(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("version=%s\ngranted_at=%s\n",
		ConsentVersion, time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(path, []byte(body), 0o600)
}

// RequireConsent is the gate every spawn path must pass.
//
// Returns an error rather than prompting: the caller owns the UI. What matters
// here is that there is exactly ONE place that decides, and it fails closed.
func RequireConsent(path string) error {
	if HasConsent(path) {
		return nil
	}
	return fmt.Errorf("AgentCorp has not been granted consent to auto-accept the " +
		"development-channels warning on your behalf; run `agentcorp consent` to review it")
}
