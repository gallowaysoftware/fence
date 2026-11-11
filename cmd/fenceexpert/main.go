// Command fenceexpert mines strong players' YouTube runs of Probably Stolen into decision
// tuples, per-creator strategy digests and cross-creator habits, and pilots reading game
// state off video frames. Everything it writes stays under -dir (var/expert, gitignored):
// transcripts and frames are third-party media and never enter the repo.
//
//	fenceexpert fetch | extract | merge | summarize | habits | frames -video ID [-n 20]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/expert"
	"github.com/gallowaysoftware/fence/internal/knowledge"
	"github.com/gallowaysoftware/fence/internal/llm"
)

type config struct {
	dir, baseURL, model    string
	visionURL, visionModel string
	unloadURL              string
	only, video, at        string
	chunkTokens, maxTokens int
	frames, height, every  int
}

func main() {
	var c config
	fs := flag.NewFlagSet("fenceexpert", flag.ExitOnError)
	fs.StringVar(&c.dir, "dir", "var/expert", "cache and data directory")
	fs.StringVar(&c.baseURL, "base-url", envOr("EXPERT_BASE_URL", ""), "OpenAI-compatible endpoint for transcript reading")
	fs.StringVar(&c.model, "model", envOr("EXPERT_MODEL", "glm53flash"), "model name")
	fs.StringVar(&c.visionURL, "vision-url", envOr("EXPERT_VISION_URL", ""), "OpenAI-compatible endpoint with image input")
	fs.StringVar(&c.visionModel, "vision-model", "localmodel/gemma-4-31b-mm", "vision model")
	fs.StringVar(&c.unloadURL, "unload-url", "http://127.0.0.1:9000/unload", "called after the frames stage so the vision model frees its VRAM")
	fs.StringVar(&c.only, "only", "", "restrict to these comma-separated YouTube ids")
	fs.StringVar(&c.video, "video", "", "frames: the YouTube id to pilot on")
	fs.IntVar(&c.chunkTokens, "chunk", 7000, "transcript tokens per model call")
	fs.IntVar(&c.maxTokens, "max-tokens", 10000, "completion budget per call")
	fs.IntVar(&c.frames, "n", 20, "frames: how many frames to read")
	fs.IntVar(&c.height, "height", 480, "frames: download resolution")
	fs.StringVar(&c.at, "at", "", "frames: comma-separated seconds to sample (overrides -every and decision timestamps)")
	fs.IntVar(&c.every, "every", 0, "frames: sample every N seconds instead of at decision timestamps (unnarrated videos)")
	if len(os.Args) < 2 {
		usage(fs)
	}
	stage := os.Args[1]
	_ = fs.Parse(os.Args[2:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st := expert.Store{Dir: c.dir}
	var err error
	switch stage {
	case "fetch":
		err = fetch(ctx, c, st)
	case "extract":
		err = extract(ctx, c, st)
	case "merge":
		err = merge(c, st)
	case "summarize":
		err = summarize(ctx, c, st)
	case "habits":
		err = habits(ctx, c, st)
	case "frames":
		err = frames(ctx, c, st)
	default:
		usage(fs)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage(fs *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, "usage: fenceexpert fetch|extract|merge|summarize|habits|frames [flags]")
	fs.PrintDefaults()
	os.Exit(2)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// The heavy cell is shared with the league and the knowledge pipeline: every call here is
// sequential, one in flight.
func (c config) client() llm.Client {
	off, temp := false, 0.2
	return &llm.OpenAI{BaseURL: c.baseURL, Model: c.model, OpenAIOptions: llm.OpenAIOptions{
		NoSchema: true, Thinking: &off, Temperature: &temp,
	}, HTTP: &http.Client{Timeout: 15 * time.Minute}}
}

func picked(c config, st expert.Store) ([]expert.Video, error) {
	sf, err := expert.LoadSources(st.SourcesPath())
	if err != nil {
		return nil, err
	}
	only := map[string]bool{}
	for _, id := range strings.Split(c.only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			only[id] = true
		}
	}
	var out []expert.Video
	for _, v := range sf.Videos {
		if len(only) > 0 && only[v.ID] || len(only) == 0 && v.Pick {
			out = append(out, v)
		}
	}
	return out, nil
}

func fetch(ctx context.Context, c config, st expert.Store) error {
	vs, err := picked(c, st)
	if err != nil {
		return err
	}
	f := knowledge.NewFetcher()
	for _, v := range vs {
		r := f.Fetch(ctx, st.Knowledge(), expert.KnowledgeSource(v))
		if r.OK {
			log.Printf("fetch %s: %d chars via %s", v.ID, r.Chars, r.Via)
		} else {
			log.Printf("fetch %s: %s", v.ID, r.Error)
		}
		if r.Via != "cache" {
			time.Sleep(5 * time.Second)
		}
	}
	return nil
}

func extract(ctx context.Context, c config, st expert.Store) error {
	vs, err := picked(c, st)
	if err != nil {
		return err
	}
	cl := c.client()
	for _, v := range vs {
		b, err := os.ReadFile(st.Knowledge().TextPath(expert.KnowledgeSource(v).ID))
		if err != nil {
			log.Printf("extract %s: no transcript (%v)", v.ID, err)
			continue
		}
		chunks := knowledge.ChunkText(string(b), c.chunkTokens, 200)
		for _, ch := range chunks {
			path := st.ChunkPath(v.ID, ch.Index)
			if _, err := os.Stat(path); err == nil {
				continue
			}
			t0 := time.Now()
			at := expert.FirstStamp(ch.Text)
			resp, err := cl.Complete(ctx, llm.Request{
				System:    expert.ExtractSystem,
				Messages:  []llm.Message{{Role: llm.RoleUser, Content: expert.ExtractPrompt(v, ch.Index+1, len(chunks), ch.Text)}},
				MaxTokens: c.maxTokens,
			})
			if err != nil {
				log.Printf("extract %s #%d: %v", v.ID, ch.Index, err)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			e, dropped, err := expert.ParseExtraction(resp.Text, v, at)
			if err != nil {
				log.Printf("extract %s #%d: parse: %v (stop %s)", v.ID, ch.Index, err, resp.StopReason)
				continue
			}
			if len(e.Decisions) == 0 {
				// An empty answer on a narrated chunk is a model failure, not a quiet passage;
				// leave it uncached so a rerun retries it.
				log.Printf("extract %s #%d: no decisions (stop %s, %d chars)", v.ID, ch.Index, resp.StopReason, len(resp.Text))
				continue
			}
			if err := writeJSON(path, e); err != nil {
				return err
			}
			log.Printf("extract %s #%d/%d: %d decisions, %d dropped, %s", v.ID, ch.Index+1, len(chunks), len(e.Decisions), dropped, time.Since(t0).Round(time.Second))
		}
	}
	return nil
}

// merge folds the per-chunk extractions into decisions.jsonl and runs.json.
func merge(c config, st expert.Store) error {
	sf, err := expert.LoadSources(st.SourcesPath())
	if err != nil {
		return err
	}
	out, err := os.Create(st.DecisionsPath() + ".tmp")
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	runs := map[string]expert.Facts{}
	n := 0
	for _, v := range sf.Videos {
		paths, _ := filepath.Glob(filepath.Join(c.dir, "extract", v.ID+".*.json"))
		sort.Strings(paths)
		if len(paths) == 0 {
			continue
		}
		var parts []expert.Facts
		for _, p := range paths {
			var e expert.Extraction
			if err := readJSON(p, &e); err != nil {
				return err
			}
			parts = append(parts, e.Facts)
			for _, d := range e.Decisions {
				b, _ := json.Marshal(d)
				w.Write(append(b, '\n'))
				n++
			}
		}
		runs[v.ID] = expert.RunFacts(parts)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	out.Close()
	if err := os.Rename(st.DecisionsPath()+".tmp", st.DecisionsPath()); err != nil {
		return err
	}
	log.Printf("merge: %d decisions from %d videos", n, len(runs))
	return writeJSON(st.RunsPath(), runs)
}

func loadDecisions(st expert.Store) ([]expert.Decision, error) {
	f, err := os.Open(st.DecisionsPath())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ds []expert.Decision
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var d expert.Decision
		if json.Unmarshal(sc.Bytes(), &d) == nil {
			ds = append(ds, d)
		}
	}
	return ds, sc.Err()
}

func summarize(ctx context.Context, c config, st expert.Store) error {
	ds, err := loadDecisions(st)
	if err != nil {
		return err
	}
	var runs map[string]expert.Facts
	if err := readJSON(st.RunsPath(), &runs); err != nil {
		return err
	}
	byCreator := map[string][]expert.Decision{}
	vidCreator := map[string]string{}
	for _, d := range ds {
		byCreator[d.Creator] = append(byCreator[d.Creator], d)
		vidCreator[d.VideoID] = d.Creator
	}
	names := make([]string, 0, len(byCreator))
	for k := range byCreator {
		names = append(names, k)
	}
	sort.Strings(names)
	cl := c.client()
	for _, name := range names {
		path := st.CreatorPath(expert.Slug(name))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		mine := map[string]expert.Facts{}
		for vid, f := range runs {
			if vidCreator[vid] == name {
				mine[vid] = f
			}
		}
		var sum expert.CreatorSummary
		if _, err := expert.Ask(ctx, cl, expert.SummarySystem, expert.SummaryPrompt(name, mine, byCreator[name]), c.maxTokens, &sum); err != nil {
			log.Printf("summarize %s: %v", name, err)
			continue
		}
		sum.Creator = name
		if err := writeJSON(path, sum); err != nil {
			return err
		}
		log.Printf("summarize %s: %d habits", name, len(sum.Habits))
	}
	return nil
}

func habits(ctx context.Context, c config, st expert.Store) error {
	paths, _ := filepath.Glob(filepath.Join(c.dir, "creators", "*.json"))
	sort.Strings(paths)
	var sums []expert.CreatorSummary
	for _, p := range paths {
		var s expert.CreatorSummary
		if err := readJSON(p, &s); err != nil {
			return err
		}
		sums = append(sums, s)
	}
	var out struct {
		Habits []expert.Habit `json:"habits"`
	}
	if _, err := expert.Ask(ctx, c.client(), expert.HabitsSystem, expert.HabitsPrompt(sums), c.maxTokens, &out); err != nil {
		return err
	}
	log.Printf("habits: %d", len(out.Habits))
	return writeJSON(st.HabitsPath(), out)
}

// frames pilots screen reading on one video: it samples frames at the transcript
// timestamps of extracted decisions and asks the vision model to read the state. The
// vision model shares the owner's GPU, so the stage refuses to start, and stops, while the
// game runs, and always unloads the model when done.
func frames(ctx context.Context, c config, st expert.Store) error {
	if c.video == "" {
		return fmt.Errorf("frames needs -video")
	}
	if run, err := expert.GameRunning(); err != nil || run {
		return fmt.Errorf("the game is running (or the check failed: %v); the 5090 is the owner's", err)
	}
	secs := map[int]bool{}
	for _, f := range strings.Split(c.at, ",") {
		var s int
		if _, err := fmt.Sscan(strings.TrimSpace(f), &s); err == nil {
			secs[s] = true
		}
	}
	var ds []expert.Decision
	if c.every == 0 && len(secs) == 0 {
		var err error
		if ds, err = loadDecisions(st); err != nil {
			return err
		}
	}
	for s := c.every; c.every > 0 && s < 6*3600; s += c.every {
		secs[s] = true
	}
	for _, d := range ds {
		if d.VideoID != c.video {
			continue
		}
		if s, ok := expert.Seconds(d.At); ok {
			secs[s+15] = true // the passage stamp precedes the screen it talks about
		}
	}
	vdir := st.VideoDir()
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		return err
	}
	vfile := filepath.Join(vdir, fmt.Sprintf("%s-%dp.mp4", c.video, c.height))
	if _, err := os.Stat(vfile); err != nil {
		cmd := exec.CommandContext(ctx, "uvx", "yt-dlp", "-f", fmt.Sprintf("bv*[height<=%d][ext=mp4]/bv*[height<=%d]", c.height, c.height),
			"--remux-video", "mp4", "-o", vfile, "https://www.youtube.com/watch?v="+c.video)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("download: %w", err)
		}
	}
	dur, err := duration(ctx, vfile)
	if err != nil {
		return err
	}
	var at []int
	for s := range secs {
		if s < dur {
			at = append(at, s)
		}
	}
	sort.Ints(at)
	at = spread(at, c.frames)
	if len(at) == 0 {
		return fmt.Errorf("no timestamps to sample for %s", c.video)
	}
	vis := expert.Vision{BaseURL: c.visionURL, Model: c.visionModel, HTTP: &http.Client{Timeout: 10 * time.Minute}}
	defer func() {
		resp, err := http.Get(c.unloadURL)
		if err != nil {
			log.Printf("unload: %v", err)
			return
		}
		resp.Body.Close()
		log.Printf("unload: HTTP %d", resp.StatusCode)
	}()
	var totalDur time.Duration
	var usage expert.Usage
	read := 0
	for _, s := range at {
		if run, _ := expert.GameRunning(); run {
			log.Printf("frames: the game started; stopping")
			break
		}
		img := st.FramePath(c.video, s)
		if err := os.MkdirAll(filepath.Dir(img), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(img); err != nil {
			cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-y", "-ss", fmt.Sprint(s), "-i", vfile, "-frames:v", "1", "-q:v", "3", img)
			if err := cmd.Run(); err != nil {
				log.Printf("frame %d: %v", s, err)
				continue
			}
		}
		jpeg, err := os.ReadFile(img)
		if err != nil {
			return err
		}
		fr, raw, u, dt, err := vis.Read(ctx, jpeg)
		rec := map[string]any{"video": c.video, "sec": s, "read": fr, "raw": raw, "usage": u, "ms": dt.Milliseconds()}
		if err != nil {
			rec["error"] = err.Error()
		}
		if err := writeJSON(st.ReadPath(c.video, s), rec); err != nil {
			return err
		}
		totalDur += dt
		usage.PromptTokens += u.PromptTokens
		usage.CompletionTokens += u.CompletionTokens
		read++
		log.Printf("frame %d: %s day=%s %s", s, dt.Round(100*time.Millisecond), fr.Day, fr.Screen)
	}
	if read > 0 {
		log.Printf("frames: %d read, %.1fs/frame, %d prompt + %d completion tokens/frame", read,
			totalDur.Seconds()/float64(read), usage.PromptTokens/read, usage.CompletionTokens/read)
	}
	return nil
}

func duration(ctx context.Context, file string) (int, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var f float64
	_, err = fmt.Sscan(strings.TrimSpace(string(out)), &f)
	return int(f), err
}

// spread keeps n values evenly spaced across xs.
func spread(xs []int, n int) []int {
	if len(xs) <= n {
		return xs
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, xs[i*len(xs)/n])
	}
	return out
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
