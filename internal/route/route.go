// Package route reads the stream's hard routes (docs/research/ROUTES.md): directives files
// whose "_route" key gives the start, perks and character of a tested way to win.
package route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultDir is where route names resolve, relative to the repository root.
const DefaultDir = "sim/lab/scripts/directives/routes"

// MaxBriefBytes bounds a brief: it rides in every morning prompt and narrator line.
const MaxBriefBytes = 600

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Route is one routes/<name>.json.
type Route struct {
	Name  string
	Path  string
	Start string
	Perks string
	// Persona is the docs/personas id that plays the route when a run names none.
	Persona string
	// Brief says what the shop does on this route, for the strategist and the narrator.
	Brief string
	// Base is the directives the executor plays, without "_route", as compact JSON.
	Base string
}

type meta struct {
	Start   string `json:"start"`
	Perks   string `json:"perks"`
	Persona string `json:"persona"`
	Brief   string `json:"brief"`
}

// IsPath tells a file path from a route name.
func IsPath(ref string) bool {
	return strings.ContainsAny(ref, `/\`) || strings.HasSuffix(ref, ".json")
}

// Resolve is the file a route ref names: dir/<name>.json for a name (DefaultDir when dir is
// empty), else the path itself.
func Resolve(dir, ref string) (string, error) {
	if IsPath(ref) {
		return ref, nil
	}
	if !nameRE.MatchString(ref) {
		return "", fmt.Errorf("route %q: not a name (lowercase letters, digits, - and _) or a path", ref)
	}
	if dir == "" {
		dir = DefaultDir
	}
	return filepath.Join(dir, ref+".json"), nil
}

// Load reads a route by name or path.
func Load(dir, ref string) (Route, error) {
	path, err := Resolve(dir, ref)
	if err != nil {
		return Route{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Route{}, fmt.Errorf("route %q: %w", ref, err)
	}
	return Parse(path, b)
}

// Parse reads a route file's bytes; path names it.
func Parse(path string, b []byte) (Route, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return Route{}, fmt.Errorf("route %s: %w", path, err)
	}
	r := Route{Name: strings.TrimSuffix(filepath.Base(path), ".json"), Path: path}
	m, ok := raw["_route"]
	if !ok {
		return Route{}, fmt.Errorf("route %s: no _route key", path)
	}
	var md meta
	if err := json.Unmarshal(m, &md); err != nil {
		return Route{}, fmt.Errorf("route %s: _route: %w", path, err)
	}
	if md.Start == "" || md.Perks == "" {
		return Route{}, fmt.Errorf("route %s: _route needs start and perks", path)
	}
	if len(md.Brief) > MaxBriefBytes {
		return Route{}, fmt.Errorf("route %s: brief is %d bytes, the limit is %d", path, len(md.Brief), MaxBriefBytes)
	}
	r.Start, r.Perks, r.Persona, r.Brief = md.Start, md.Perks, md.Persona, strings.TrimSpace(md.Brief)
	base, err := StripMeta(b)
	if err != nil {
		return Route{}, fmt.Errorf("route %s: %w", path, err)
	}
	r.Base = base
	return r, nil
}

// StripMeta is a directives file as compact JSON without its "_route" key, which only
// describes the route: the strategist is shown the policy, not the catalogue entry.
func StripMeta(b []byte) (string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return "", err
	}
	if _, ok := raw["_route"]; !ok {
		var buf bytes.Buffer
		if err := json.Compact(&buf, b); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	// Re-encoding sorts the keys; the file's own order matters to nobody.
	delete(raw, "_route")
	out, err := json.Marshal(raw)
	return string(out), err
}

// Text is the route as the strategist and the narrator are told it.
func (r Route) Text() string {
	if r.Brief == "" {
		return r.Name
	}
	return r.Name + ": " + r.Brief
}
