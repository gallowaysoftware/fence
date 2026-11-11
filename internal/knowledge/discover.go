package knowledge

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Seeds is the hand-curated starting list (docs/knowledge/seeds.json): sources found by
// web search, plus listing pages and sitemaps to expand mechanically.
type Seeds struct {
	Sources []Source `json:"sources"`
	Expand  []Expand `json:"expand"`
}

type Expand struct {
	Kind    string `json:"kind"` // steam_guides | sitemap
	URL     string `json:"url"`
	Include string `json:"include,omitempty"` // regexp on the URL (sitemap)
	Exclude string `json:"exclude,omitempty"`
	Max     int    `json:"max"`
	Quality int    `json:"quality"`
	Type    string `json:"type,omitempty"` // source type for sitemap entries
}

func LoadSeeds(path string) (Seeds, error) {
	var s Seeds
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// Discover expands the seeds into the source list. Sources already in prev keep their
// metadata, so rediscovery never forgets a fetched video's details.
func Discover(ctx context.Context, f *Fetcher, seeds Seeds, prev []Source, logf func(string, ...any)) []Source {
	byURL := map[string]Source{}
	var order []string
	add := func(s Source) {
		s.URL = strings.TrimSpace(s.URL)
		if _, dup := byURL[s.URL]; dup {
			return
		}
		if s.ID == "" {
			s.ID = SourceID(s.Type, s.URL)
		}
		if s.Quality == 0 {
			s.Quality = QualityMid
		}
		byURL[s.URL] = s
		order = append(order, s.URL)
	}
	known := map[string]Source{}
	for _, p := range prev {
		known[p.URL] = p
	}
	for _, s := range seeds.Sources {
		if p, ok := known[s.URL]; ok {
			s = mergeMeta(s, p)
		}
		add(s)
	}
	for _, e := range seeds.Expand {
		found, err := expand(ctx, f, e)
		if err != nil {
			logf("expand %s %s: %v", e.Kind, e.URL, err)
			continue
		}
		logf("expand %s %s: %d sources", e.Kind, e.URL, len(found))
		for _, s := range found {
			if p, ok := known[s.URL]; ok {
				s = mergeMeta(s, p)
			}
			add(s)
		}
	}
	out := make([]Source, 0, len(order))
	for _, u := range order {
		s := byURL[u]
		if s.Type == TypeVideo && (s.Channel == "" || s.Duration == 0 || s.Date == "") {
			if err := videoMeta(ctx, f, &s); err != nil {
				logf("video meta %s: %v", s.URL, err)
			}
		}
		out = append(out, s)
	}
	return out
}

func mergeMeta(s, p Source) Source {
	if s.Title == "" {
		s.Title = p.Title
	}
	if s.Channel == "" {
		s.Channel = p.Channel
	}
	if s.Date == "" {
		s.Date = p.Date
	}
	if s.Duration == 0 {
		s.Duration = p.Duration
	}
	return s
}

func expand(ctx context.Context, f *Fetcher, e Expand) ([]Source, error) {
	switch e.Kind {
	case "steam_guides":
		b, _, err := f.Get(ctx, e.URL)
		if err != nil {
			return nil, err
		}
		return ParseSteamGuideList(string(b), e.Max, e.Quality), nil
	case "sitemap":
		locs, err := sitemapLocs(ctx, f, e.URL, 0)
		if err != nil {
			return nil, err
		}
		inc, err := regexp.Compile(e.Include)
		if err != nil {
			return nil, err
		}
		var exc *regexp.Regexp
		if e.Exclude != "" {
			if exc, err = regexp.Compile(e.Exclude); err != nil {
				return nil, err
			}
		}
		typ := e.Type
		if typ == "" {
			typ = TypeWiki
		}
		var out []Source
		for _, l := range locs {
			if !inc.MatchString(l) || exc != nil && exc.MatchString(l) {
				continue
			}
			out = append(out, Source{Type: typ, URL: l, Title: titleFromPath(l), Quality: e.Quality})
			if e.Max > 0 && len(out) >= e.Max {
				break
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown expand kind %q", e.Kind)
}

func sitemapLocs(ctx context.Context, f *Fetcher, u string, depth int) ([]string, error) {
	b, _, err := f.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var doc struct {
		XMLName  xml.Name
		URLs     []string `xml:"url>loc"`
		Sitemaps []string `xml:"sitemap>loc"`
	}
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	locs := doc.URLs
	if depth < 2 {
		for _, sm := range doc.Sitemaps {
			sub, err := sitemapLocs(ctx, f, strings.TrimSpace(sm), depth+1)
			if err != nil {
				continue
			}
			locs = append(locs, sub...)
		}
	}
	for i := range locs {
		locs[i] = strings.TrimSpace(locs[i])
	}
	return locs, nil
}

var (
	reGuideLink  = regexp.MustCompile(`href="(https://steamcommunity\.com/sharedfiles/filedetails/\?id=(\d+))[^"]*"`)
	reGuideTitle = regexp.MustCompile(`(?s)<div class="workshopItemTitle">(.*?)</div>`)
	reGuideBy    = regexp.MustCompile(`(?s)<div class="workshopItemAuthorName">\s*by&nbsp;(.*?)</div>`)
)

// ParseSteamGuideList reads a Steam Community guide browse page.
func ParseSteamGuideList(page string, max, quality int) []Source {
	var out []Source
	seen := map[string]bool{}
	// Each guide card holds one link followed by its title and author.
	cards := strings.Split(page, `class="workshopItemCollection`)
	if len(cards) < 2 {
		cards = strings.Split(page, `<a  href="https://steamcommunity.com/sharedfiles/filedetails/`)
	}
	for _, card := range cards {
		m := reGuideLink.FindStringSubmatch(card)
		if m == nil {
			if id := regexp.MustCompile(`^\?id=(\d+)`).FindStringSubmatch(card); id != nil {
				m = []string{"", "https://steamcommunity.com/sharedfiles/filedetails/?id=" + id[1], id[1]}
			} else {
				continue
			}
		}
		u := "https://steamcommunity.com/sharedfiles/filedetails/?id=" + m[2]
		if seen[u] {
			continue
		}
		seen[u] = true
		s := Source{Type: TypeSteamGuide, URL: u, Quality: quality}
		if t := reGuideTitle.FindStringSubmatch(card); t != nil {
			s.Title = collapseSpace(html.UnescapeString(vttTag.ReplaceAllString(t[1], "")))
		}
		if a := reGuideBy.FindStringSubmatch(card); a != nil {
			s.Channel = collapseSpace(html.UnescapeString(vttTag.ReplaceAllString(a[1], "")))
		}
		out = append(out, s)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func titleFromPath(u string) string {
	u = strings.TrimRight(strings.SplitN(u, "?", 2)[0], "/")
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	return strings.ReplaceAll(u, "-", " ")
}

// videoMeta asks yt-dlp for the metadata only; that endpoint is not the rate-limited one.
func videoMeta(ctx context.Context, f *Fetcher, s *Source) error {
	out, err := f.Exec(ctx, os.TempDir(), "uvx", "yt-dlp", "--skip-download", "--no-warnings",
		"--print", "%(title)s\t%(channel)s\t%(upload_date)s\t%(duration)s", s.URL)
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) != 4 {
		return fmt.Errorf("unexpected yt-dlp output %q", out)
	}
	if s.Title == "" {
		s.Title = parts[0]
	}
	s.Channel = parts[1]
	if d := parts[2]; len(d) == 8 {
		s.Date = d[:4] + "-" + d[4:6] + "-" + d[6:]
	}
	s.Duration, _ = strconv.Atoi(parts[3])
	return nil
}
