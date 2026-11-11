package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source types.
const (
	TypeWiki       = "wiki"
	TypeGuide      = "guide"       // editorial guide sites
	TypeSteamGuide = "steam_guide" // Steam Community guides
	TypeReddit     = "reddit"
	TypeForum      = "forum" // Steam discussions
	TypePatchNotes = "patch_notes"
	TypeModding    = "modding"
	TypeVideo      = "video"
	// TypePlayer is the owner's own notes, a repo file path rather than a URL.
	TypePlayer = "player_notes"
)

// Source quality tiers, used to rank strategy claims that cannot be checked in code.
const (
	// QualityOwner: the owner's first-hand play; top trust for strategy.
	QualityOwner = 4
	// QualityHigh: data-mined wikis, the developer, players with long hard-mode runs.
	QualityHigh = 3
	// QualityMid: player-written guides and threads.
	QualityMid = 2
	// QualityLow: listicles and search-bait guide sites.
	QualityLow = 1
)

type Source struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Channel  string `json:"channel,omitempty"` // video channel, guide author or site
	Date     string `json:"date,omitempty"`    // YYYY-MM-DD
	Duration int    `json:"duration_s,omitempty"`
	Quality  int    `json:"quality"`
	Notes    string `json:"notes,omitempty"`
}

// Origin identifies who stands behind a source, so two pages of one wiki or two videos
// of one channel count as one independent voice.
func (s Source) Origin() string {
	if s.Type == TypeVideo && s.Channel != "" {
		return "yt:" + strings.ToLower(s.Channel)
	}
	if s.Type == TypeSteamGuide && s.Channel != "" {
		return "steam:" + strings.ToLower(s.Channel)
	}
	if s.Type == TypeReddit || s.Type == TypeSteamGuide || s.Type == TypeForum {
		return s.URL // each thread or guide is its own voice when the author is unknown
	}
	u, err := url.Parse(s.URL)
	if err != nil {
		return s.URL
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// SourceID derives a short, stable, readable id from the URL.
func SourceID(typ, rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	slug := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		slug = strings.TrimPrefix(u.Host, "www.") + u.Path
		if v := u.Query().Get("v"); v != "" {
			slug = v
		} else if id := u.Query().Get("id"); id != "" {
			slug += "-" + id
		}
	}
	var b strings.Builder
	for _, r := range strings.ToLower(slug) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if s := b.String(); len(s) > 0 && s[len(s)-1] != '-' {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 48 {
		s = s[len(s)-48:]
	}
	return typ + "-" + strings.Trim(s, "-") + "-" + hex.EncodeToString(h[:2])
}

type SourceList struct {
	Generated string   `json:"generated,omitempty"`
	Sources   []Source `json:"sources"`
}

func LoadSources(path string) ([]Source, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l SourceList
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l.Sources, nil
}

func SaveSources(path string, srcs []Source, generated string) error {
	sort.SliceStable(srcs, func(i, j int) bool {
		if srcs[i].Type != srcs[j].Type {
			return srcs[i].Type < srcs[j].Type
		}
		return srcs[i].ID < srcs[j].ID
	})
	return writeJSON(path, SourceList{Generated: generated, Sources: srcs})
}

// Store lays out the cache under one directory (var/knowledge). Every stage writes its
// output per source or per cluster, so a rerun skips finished work.
type Store struct{ Dir string }

func (s Store) SourcesPath() string           { return filepath.Join(s.Dir, "sources.json") }
func (s Store) RawPath(id, ext string) string { return filepath.Join(s.Dir, "raw", id+ext) }
func (s Store) TextPath(id string) string     { return filepath.Join(s.Dir, "text", id+".txt") }
func (s Store) ClaimsPath(id string) string   { return filepath.Join(s.Dir, "claims", id+".jsonl") }
func (s Store) ClustersPath() string          { return filepath.Join(s.Dir, "clusters.json") }
func (s Store) VerifyPath(id string) string   { return filepath.Join(s.Dir, "verify", id+".json") }
func (s Store) AllClaimsPath() string         { return filepath.Join(s.Dir, "claims.jsonl") }
func (s Store) DigestPath() string            { return filepath.Join(s.Dir, "digest.md") }
func (s Store) FetchLogPath() string          { return filepath.Join(s.Dir, "fetch.json") }

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(b, '\n'))
}

// writeFile is atomic so an interrupted run never leaves a half-written cache entry
// that a resumed run would trust.
func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
