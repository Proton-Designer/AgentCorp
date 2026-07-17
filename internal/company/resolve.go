package company

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolution is the outcome of resolving a directory to a company.
//
// Found distinguishes "walked up to the filesystem root and no company owns
// this directory" (Found == false, nil error) from "a company file exists but
// could not be read or parsed" (a non-nil error) — the same not-empty-vs-broken
// discipline the broker read side keeps. When Found is true, Root is the
// absolute directory that holds the .agentcorp/ definition.
type Resolution struct {
	Company Company
	Root    string
	Found   bool
}

// Resolve walks up from dir to the nearest ancestor that contains a
// .agentcorp/company.toml, mirroring how git locates the repository that owns a
// path. The nearest definition wins, so a subtree with its own company.toml is
// its own company and does not fall through to a parent's.
//
// A missing definition all the way to the filesystem root is not an error — it
// is simply an unscoped directory (Found == false). A definition that exists
// but is unreadable or malformed IS an error: a company whose identity we
// cannot read is a problem to surface, not to silently treat as absent.
func Resolve(dir string) (Resolution, error) {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve company: %w", err)
	}
	for {
		candidate := filepath.Join(cur, ConfigDir, ConfigFile)
		data, err := os.ReadFile(candidate)
		switch {
		case err == nil:
			c, perr := ParseConfig(data)
			if perr != nil {
				return Resolution{}, fmt.Errorf("resolve company at %s: %w", candidate, perr)
			}
			// Canonicalize the root so membership compares the same physical
			// directory regardless of how a path reached us. On macOS in
			// particular, /var is a symlink to /private/var, and a peer's cwd
			// and the launch dir can legitimately arrive in different forms;
			// without this, a real in-company session would be hidden. cur
			// definitely exists (we just read a file under it), so EvalSymlinks
			// should succeed — but fall back rather than fail the resolve.
			root := cur
			if resolved, rerr := filepath.EvalSymlinks(cur); rerr == nil {
				root = resolved
			}
			return Resolution{Company: c, Root: root, Found: true}, nil
		case os.IsNotExist(err):
			// No definition at this level; keep walking up.
		default:
			// Exists but unreadable (permissions, a directory in its place, …).
			return Resolution{}, fmt.Errorf("resolve company at %s: %w", candidate, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return Resolution{Found: false}, nil // reached the filesystem root
		}
		cur = parent
	}
}

// Member reports whether the working directory cwd belongs to the company
// rooted at companyRoot, using the same nearest-wins rule as Resolve: cwd is a
// member iff resolving it lands on exactly companyRoot. A cwd inside a nested
// sub-company therefore does NOT count as a member of the outer company.
//
// A cwd that resolves to no company at all is not a member (false, nil). A cwd
// that cannot be resolved because of a genuine I/O fault returns that error;
// callers deciding what to display may choose to treat that as "exclude".
func Member(companyRoot, cwd string) (bool, error) {
	res, err := Resolve(cwd)
	if err != nil {
		return false, err
	}
	if !res.Found {
		return false, nil
	}
	return res.Root == companyRoot, nil
}

// Create writes a new company definition into dir/.agentcorp/company.toml and
// returns the stored Company. It refuses to overwrite an existing definition —
// callers must Resolve first and only create when nothing owns the directory —
// so a second launch in the same folder can never silently replace a company's
// identity.
//
// name is validated non-empty after trimming; id is caller-supplied (generated
// once, like node ids) and must be non-empty. The stored name is the trimmed
// value so stray leading/trailing whitespace never becomes part of a company's
// identity.
func Create(dir, name, id string) (Company, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Company{}, fmt.Errorf("create company: name is empty")
	}
	if strings.TrimSpace(id) == "" {
		return Company{}, fmt.Errorf("create company: id is empty")
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return Company{}, fmt.Errorf("create company: %w", err)
	}
	cfgDir := filepath.Join(base, ConfigDir)
	cfgPath := filepath.Join(cfgDir, ConfigFile)
	if _, err := os.Stat(cfgPath); err == nil {
		return Company{}, fmt.Errorf("create company: %s already exists", cfgPath)
	} else if !os.IsNotExist(err) {
		return Company{}, fmt.Errorf("create company: stat %s: %w", cfgPath, err)
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return Company{}, fmt.Errorf("create company: %w", err)
	}
	c := Company{ID: id, Name: name}
	if err := os.WriteFile(cfgPath, FormatConfig(c), 0o644); err != nil {
		return Company{}, fmt.Errorf("create company: write %s: %w", cfgPath, err)
	}
	return c, nil
}
