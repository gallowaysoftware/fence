package knowledge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// UserAgent names the project and nobody's personal address.
const UserAgent = "fence-knowledge/0.1 (personal game-strategy research; +https://github.com/gallowaysoftware/fence)"

const maxPage = 8 << 20

// Fetcher downloads sources politely: robots.txt is honoured, and requests to one host
// are spaced by Delay.
type Fetcher struct {
	HTTP  *http.Client
	Delay time.Duration
	// Exec runs an external tool (yt-dlp via uvx); tests replace it.
	Exec func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

	mu     sync.Mutex
	last   map[string]time.Time
	robots map[string][]string // host -> disallowed prefixes for *
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		HTTP:  &http.Client{Timeout: 60 * time.Second},
		Delay: 3 * time.Second,
		Exec:  runTool,
	}
}

func runTool(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "ERROR:"); i >= 0 {
			msg = msg[i:]
		}
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return out, fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return out, nil
}

var errRobots = errors.New("disallowed by robots.txt")

// Get fetches u with the project's User-Agent after checking robots.txt.
func (f *Fetcher) Get(ctx context.Context, u string) ([]byte, string, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return nil, "", err
	}
	if !f.allowed(ctx, pu) {
		return nil, "", errRobots
	}
	return f.get(ctx, pu)
}

func (f *Fetcher) get(ctx context.Context, pu *url.URL) ([]byte, string, error) {
	f.wait(pu.Host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pu.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Language", "en")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: HTTP %d", pu, resp.StatusCode)
	}
	return b, resp.Header.Get("Content-Type"), nil
}

func (f *Fetcher) wait(host string) {
	f.mu.Lock()
	if f.last == nil {
		f.last = map[string]time.Time{}
	}
	next := f.last[host].Add(f.Delay)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	f.last[host] = next
	f.mu.Unlock()
	time.Sleep(time.Until(next))
}

func (f *Fetcher) allowed(ctx context.Context, u *url.URL) bool {
	f.mu.Lock()
	if f.robots == nil {
		f.robots = map[string][]string{}
	}
	rules, ok := f.robots[u.Host]
	f.mu.Unlock()
	if !ok {
		ru := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/robots.txt"}
		b, _, err := f.get(ctx, ru)
		if err == nil {
			rules = ParseRobots(string(b))
		}
		f.mu.Lock()
		f.robots[u.Host] = rules
		f.mu.Unlock()
	}
	return RobotsAllows(rules, u.RequestURI())
}

// ParseRobots returns the Disallow rules of the "User-agent: *" group.
func ParseRobots(txt string) []string {
	var rules []string
	inStar, sawRule := false, false
	sc := bufio.NewScanner(strings.NewReader(txt))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if sawRule {
				inStar, sawRule = false, false
			}
			if v == "*" {
				inStar = true
			}
		case "disallow", "allow":
			sawRule = true
			if inStar && k == "disallow" && v != "" {
				rules = append(rules, v)
			}
		}
	}
	return rules
}

// RobotsAllows matches path against Disallow prefixes, honouring "*" and a trailing "$".
func RobotsAllows(rules []string, path string) bool {
	for _, r := range rules {
		pat := "^" + strings.ReplaceAll(regexp.QuoteMeta(strings.TrimSuffix(r, "$")), `\*`, ".*")
		if strings.HasSuffix(r, "$") {
			pat += "$"
		}
		if ok, _ := regexp.MatchString(pat, path); ok {
			return false
		}
	}
	return true
}

// FetchResult is one line of the fetch log.
type FetchResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Chars int    `json:"chars,omitempty"`
	Via   string `json:"via,omitempty"`
	Error string `json:"error,omitempty"`
	At    string `json:"at"`
}

// Fetch writes the raw download and the cleaned text for src. A source whose text is
// already cached is skipped.
func (f *Fetcher) Fetch(ctx context.Context, st Store, src Source) FetchResult {
	res := FetchResult{ID: src.ID, At: time.Now().UTC().Format(time.RFC3339)}
	if b, err := os.ReadFile(st.TextPath(src.ID)); err == nil {
		res.OK, res.Chars, res.Via = true, len(b), "cache"
		return res
	}
	text, via, err := f.fetchText(ctx, st, src)
	if err == nil && len(strings.TrimSpace(text)) < 200 {
		err = fmt.Errorf("only %d chars of text", len(strings.TrimSpace(text)))
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	header := fmt.Sprintf("SOURCE: %s\nTITLE: %s\nTYPE: %s\n", src.URL, src.Title, src.Type)
	if src.Channel != "" {
		header += "BY: " + src.Channel + "\n"
	}
	if src.Date != "" {
		header += "DATE: " + src.Date + "\n"
	}
	if err := writeFile(st.TextPath(src.ID), []byte(header+"\n"+text)); err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK, res.Chars, res.Via = true, len(text), via
	return res
}

func (f *Fetcher) fetchText(ctx context.Context, st Store, src Source) (string, string, error) {
	switch src.Type {
	case TypeVideo:
		return f.fetchTranscript(ctx, st, src)
	case TypePlayer:
		b, err := os.ReadFile(src.URL)
		return string(b), "file", err
	case TypeReddit:
		u := strings.TrimRight(strings.SplitN(src.URL, "?", 2)[0], "/") + "/.rss"
		b, _, err := f.Get(ctx, u)
		if err != nil {
			return "", "", err
		}
		_ = writeFile(st.RawPath(src.ID, ".xml"), b)
		t, err := RedditAtomToText(b)
		return t, "rss", err
	case TypePatchNotes:
		if strings.Contains(src.URL, "ISteamNews") {
			b, _, err := f.Get(ctx, src.URL)
			if err != nil {
				return "", "", err
			}
			_ = writeFile(st.RawPath(src.ID, ".json"), b)
			t, err := SteamNewsToText(b)
			return t, "steam-news", err
		}
	}
	b, ctype, err := f.Get(ctx, src.URL)
	if err != nil {
		return "", "", err
	}
	if strings.Contains(ctype, "html") || bytes.Contains(b[:min(len(b), 512)], []byte("<html")) {
		_ = writeFile(st.RawPath(src.ID, ".html"), b)
		return HTMLToText(string(b)), "html", nil
	}
	_ = writeFile(st.RawPath(src.ID, ".txt"), b)
	return string(b), "plain", nil
}

// fetchTranscript tries yt-dlp's subtitles first and youtube-transcript-api second:
// YouTube rate-limits the subtitle endpoint yt-dlp uses well before the other.
func (f *Fetcher) fetchTranscript(ctx context.Context, st Store, src Source) (string, string, error) {
	vid := youtubeID(src.URL)
	if vid == "" {
		return "", "", fmt.Errorf("no video id in %s", src.URL)
	}
	dir := filepath.Dir(st.RawPath(src.ID, ""))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	vttPath := st.RawPath(src.ID, ".vtt")
	if !exists(vttPath) {
		var errs []string
		_, err := f.Exec(ctx, dir, "uvx", "yt-dlp", "--skip-download", "--write-auto-subs", "--write-subs",
			"--sub-langs", "en", "--sub-format", "vtt", "--sleep-requests", "1",
			"-o", src.ID+".%(ext)s", "https://www.youtube.com/watch?v="+vid)
		if got := filepath.Join(dir, src.ID+".en.vtt"); exists(got) {
			_ = os.Rename(got, vttPath)
		} else {
			errs = append(errs, fmt.Sprint(err))
			out, err := f.Exec(ctx, dir, "uvx", "--from", "youtube-transcript-api", "youtube_transcript_api",
				vid, "--languages", "en", "--format", "webvtt")
			if err != nil || !bytes.HasPrefix(bytes.TrimSpace(out), []byte("WEBVTT")) {
				errs = append(errs, fmt.Sprint(err))
				return "", "", fmt.Errorf("transcript: %s", strings.Join(errs, "; "))
			}
			if err := writeFile(vttPath, out); err != nil {
				return "", "", err
			}
		}
	}
	b, err := os.ReadFile(vttPath)
	if err != nil {
		return "", "", err
	}
	return CleanVTT(string(b), 30), "vtt", nil
}

var reYouTubeID = regexp.MustCompile(`(?:v=|youtu\.be/|/shorts/|/live/)([A-Za-z0-9_-]{11})`)

func youtubeID(u string) string {
	if m := reYouTubeID.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

var reBBCode = regexp.MustCompile(`\[/?[a-z0-9*]+(=[^\]]*)?\]`)

// SteamNewsToText renders ISteamNews/GetNewsForApp output, newest first.
func SteamNewsToText(b []byte) (string, error) {
	var r struct {
		AppNews struct {
			NewsItems []struct {
				Title     string `json:"title"`
				URL       string `json:"url"`
				Date      int64  `json:"date"`
				Contents  string `json:"contents"`
				FeedLabel string `json:"feedlabel"`
			} `json:"newsitems"`
		} `json:"appnews"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, it := range r.AppNews.NewsItems {
		if it.FeedLabel != "" && it.FeedLabel != "Community Announcements" && it.FeedLabel != "Product Update" {
			continue // press coverage syndicated into the feed
		}
		day := time.Unix(it.Date, 0).UTC().Format("2006-01-02")
		body := reBBCode.ReplaceAllString(it.Contents, "\n")
		if strings.Contains(body, "<") {
			body = HTMLToText(body)
		}
		fmt.Fprintf(&sb, "## %s (%s)\n%s\n\n", it.Title, day, strings.TrimSpace(body))
	}
	return sb.String(), nil
}
