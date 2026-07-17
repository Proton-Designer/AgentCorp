package ui

import (
	"strings"
	"testing"

	"github.com/Proton-Designer/AgentCorp/internal/company"
	"github.com/Proton-Designer/AgentCorp/internal/store"
)

func TestHeaderShowsCompanyWhenScoped(t *testing.T) {
	m, _ := liveModel(t)
	m = m.WithScope(company.Company{ID: "co-1", Name: "Acme Corp"}, "/co/root")
	if got := m.header(); !strings.Contains(got, "Acme Corp") {
		t.Fatalf("header should name the scoped company, got %q", got)
	}
}

func TestHeaderOmitsCompanyWhenUnscoped(t *testing.T) {
	m, _ := liveModel(t)
	// No WithScope call: unscoped.
	if got := m.header(); strings.Contains(got, "·") {
		t.Fatalf("unscoped header should not carry a company separator, got %q", got)
	}
}

func TestEmptyStateNamesCompany(t *testing.T) {
	// A live model with no nodes shows the splash; when scoped it must name the
	// company so an empty chart reads as "this company has no agents yet".
	s, err := store.Open(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	m := NewLive(s, nil).WithScope(company.Company{ID: "co-1", Name: "Beta LLC"}, "/co/root")
	if !strings.Contains(m.View(), "Beta LLC") {
		t.Fatalf("empty-state view should name the scoped company:\n%s", m.View())
	}
}

// WithScope with an empty root must leave the peer source and header unscoped —
// callers pass a resolution through unconditionally, and "no company" must be a
// clean no-op rather than a filter that hides everything.
func TestWithScopeEmptyRootIsUnscoped(t *testing.T) {
	m, _ := liveModel(t)
	m = m.WithScope(company.Company{}, "")
	if got := m.header(); strings.Contains(got, "·") {
		t.Fatalf("empty-root scope should stay unscoped, got header %q", got)
	}
}
