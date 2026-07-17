package company

import (
	"os"
	"path/filepath"
	"testing"
)

// seed writes a company.toml under dir/.agentcorp for a company named `name`.
func seed(t *testing.T, dir, id, name string) {
	t.Helper()
	if _, err := Create(dir, name, id); err != nil {
		t.Fatalf("seed %s: %v", dir, err)
	}
}

// canon canonicalizes a directory the same way Resolve does, so tests can
// compare a returned Root against an expected path even when the temp dir is
// reached through a symlink (e.g. macOS /var -> /private/var).
func canon(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return resolved
}

func TestResolveFindsCompanyAtStartDir(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-1", "Acme")

	res, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Found {
		t.Fatal("expected Found")
	}
	if res.Company.Name != "Acme" || res.Company.ID != "co-1" {
		t.Fatalf("got %+v", res.Company)
	}
	if res.Root != canon(t, root) {
		t.Fatalf("root: got %q want %q", res.Root, canon(t, root))
	}
}

func TestResolveWalksUp(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-1", "Acme")
	deep := filepath.Join(root, "team", "service", "pkg")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(deep)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Found || res.Root != canon(t, root) {
		t.Fatalf("expected to resolve up to %q, got %+v", canon(t, root), res)
	}
}

func TestResolveNearestWins(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-outer", "Outer")
	inner := filepath.Join(root, "sub")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	seed(t, inner, "co-inner", "Inner")
	deep := filepath.Join(inner, "x")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(deep)
	if err != nil {
		t.Fatal(err)
	}
	if res.Company.ID != "co-inner" || res.Root != canon(t, inner) {
		t.Fatalf("nearest company should win: got %+v", res)
	}
}

func TestResolveUnscopedIsNotAnError(t *testing.T) {
	// A fresh temp dir with no .agentcorp anywhere up to the fs root.
	dir := t.TempDir()
	res, err := Resolve(dir)
	if err != nil {
		t.Fatalf("unscoped dir should not error: %v", err)
	}
	if res.Found {
		t.Fatalf("expected Found=false, got %+v", res)
	}
}

func TestResolveMalformedConfigIsError(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ConfigDir)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Present but missing the required id — a broken definition, not absence.
	if err := os.WriteFile(filepath.Join(cfgDir, ConfigFile), []byte("name = \"X\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root); err == nil {
		t.Fatal("expected an error for a malformed company config")
	}
}

func TestMemberNearestWins(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-outer", "Outer")
	inner := filepath.Join(root, "sub")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	seed(t, inner, "co-inner", "Inner")

	// A dir directly under root (but not under inner) is a member of the outer.
	sibling := filepath.Join(root, "other")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}

	// companyRoot always originates from a Resolution.Root in real use, so it
	// arrives canonicalized — mirror that here.
	outerRoot := canon(t, root)
	innerRoot := canon(t, inner)

	if ok, err := Member(outerRoot, sibling); err != nil || !ok {
		t.Fatalf("sibling should belong to outer: ok=%v err=%v", ok, err)
	}
	// A dir under the inner company must NOT count as a member of the outer.
	if ok, err := Member(outerRoot, inner); err != nil || ok {
		t.Fatalf("inner subtree must not belong to outer: ok=%v err=%v", ok, err)
	}
	// It IS a member of the inner company.
	if ok, err := Member(innerRoot, inner); err != nil || !ok {
		t.Fatalf("inner dir should belong to inner: ok=%v err=%v", ok, err)
	}
}

func TestMemberUnscopedIsFalse(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-1", "Acme")
	elsewhere := t.TempDir() // a different tree, no company

	if ok, err := Member(root, elsewhere); err != nil || ok {
		t.Fatalf("unscoped cwd should not be a member: ok=%v err=%v", ok, err)
	}
}

func TestCreateRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	seed(t, root, "co-1", "First")
	if _, err := Create(root, "Second", "co-2"); err == nil {
		t.Fatal("Create must refuse to overwrite an existing company definition")
	}
	// The original must be intact.
	res, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if res.Company.ID != "co-1" || res.Company.Name != "First" {
		t.Fatalf("original clobbered: %+v", res.Company)
	}
}

func TestCreateValidatesInput(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, "   ", "co-1"); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, err := Create(root, "Name", "  "); err == nil {
		t.Fatal("empty id must be rejected")
	}
}

func TestCreateTrimsName(t *testing.T) {
	root := t.TempDir()
	c, err := Create(root, "  Padded Co  ", "co-1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Padded Co" {
		t.Fatalf("name should be trimmed, got %q", c.Name)
	}
}
