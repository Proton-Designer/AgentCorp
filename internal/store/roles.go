package store

import "database/sql"

// Role is a reusable agent archetype: a named system-prompt template with a
// display glyph. The roles table has existed since Phase 1; these methods are
// what finally make it a feature — hire can pick a role instead of re-typing a
// system prompt every time.
type Role struct {
	Name   string
	Glyph  string
	Prompt string
}

// UpsertRole inserts or edits a role. role is the primary key, so re-running
// with the same name is an edit, not a duplicate — idempotent, matching the
// rest of the store's write style.
func (s *Store) UpsertRole(r Role) error {
	_, err := s.db.Exec(
		`INSERT INTO roles (role, glyph, prompt) VALUES (?,?,?)
		 ON CONFLICT(role) DO UPDATE SET glyph = excluded.glyph, prompt = excluded.prompt`,
		r.Name, r.Glyph, r.Prompt)
	return err
}

// GetRole returns a role by name. ok is false (with a nil error) when no such
// role exists — a missing template is a normal outcome, not a failure.
func (s *Store) GetRole(name string) (Role, bool, error) {
	var r Role
	err := s.db.QueryRow(`SELECT role, glyph, prompt FROM roles WHERE role = ?`, name).
		Scan(&r.Name, &r.Glyph, &r.Prompt)
	if err == sql.ErrNoRows {
		return Role{}, false, nil
	}
	if err != nil {
		return Role{}, false, err
	}
	return r, true, nil
}

// ListRoles returns all roles, alphabetically — the order the hire picker shows.
func (s *Store) ListRoles() ([]Role, error) {
	rows, err := s.db.Query(`SELECT role, glyph, prompt FROM roles ORDER BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.Name, &r.Glyph, &r.Prompt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRole removes a role template.
func (s *Store) DeleteRole(name string) error {
	_, err := s.db.Exec(`DELETE FROM roles WHERE role = ?`, name)
	return err
}

// DefaultRoles are the archetypes seeded into a fresh store. Prompts are
// name-free: the hire flow prepends the agent's own identity line, so these
// describe the ROLE's behavior, which composes cleanly with any name and brief.
var DefaultRoles = []Role{
	{Name: "generalist", Glyph: "●", Prompt: "Work autonomously and pragmatically on whatever your manager asks. Ask for clarification only when genuinely blocked."},
	{Name: "researcher", Glyph: "◆", Prompt: "You specialize in research: investigate thoroughly, verify claims against real sources, and report findings concisely with evidence."},
	{Name: "engineer", Glyph: "⚙", Prompt: "You specialize in software engineering: write clean, well-tested code, keep changes small and reviewable, and explain tradeoffs plainly."},
	{Name: "reviewer", Glyph: "✦", Prompt: "You specialize in review: find real bugs, security issues, and risks; be adversarial but precise; verify a claim before you assert it."},
}

// SeedDefaultRoles installs DefaultRoles only when the table is empty, so a
// fresh company gets useful archetypes out of the box without ever clobbering
// an operator's own edits to a default on a later launch.
func (s *Store) SeedDefaultRoles() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM roles`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, r := range DefaultRoles {
		if err := s.UpsertRole(r); err != nil {
			return err
		}
	}
	return nil
}
