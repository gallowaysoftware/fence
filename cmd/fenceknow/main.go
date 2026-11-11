// Command fenceknow builds the strategy-knowledge base: it discovers community sources
// about Probably Stolen, fetches them, has a model extract and merge claims, checks the
// mechanical ones against the decompiled game, and writes the material the primer is
// curated from. Every stage caches per source or per cluster under -dir, so `all` can be
// rerun after an interruption.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gallowaysoftware/fence/internal/knowledge"
	"github.com/gallowaysoftware/fence/internal/llm"
)

type config struct {
	dir, seeds, docs     string
	baseURL, model       string
	decomp, gamedata     string
	workers, maxTokens   int
	only                 string
	topicBatch, sysLimit int
	verifyMax            int
}

func main() {
	var c config
	fs := flag.NewFlagSet("fenceknow", flag.ExitOnError)
	fs.StringVar(&c.dir, "dir", "var/knowledge", "cache and data directory")
	fs.StringVar(&c.seeds, "seeds", "docs/knowledge/seeds.json", "curated seed sources")
	fs.StringVar(&c.docs, "docs", "docs/knowledge", "where SOURCES.md and SYSTEMS.md are written")
	fs.StringVar(&c.baseURL, "base-url", envOr("KNOWLEDGE_BASE_URL", ""), "OpenAI-compatible endpoint for bulk reading")
	fs.StringVar(&c.model, "model", envOr("KNOWLEDGE_MODEL", "glm53flash"), "model name")
	fs.StringVar(&c.decomp, "decomp", envOr("FENCE_DECOMP", "var/decomp"), "directory of the decompiled Assembly-CSharp (.cs files)")
	fs.StringVar(&c.gamedata, "gamedata", "var/gamedata", "extracted data tables")
	fs.IntVar(&c.workers, "workers", 1, "parallel model calls (the heavy cell has few slots; games need them)")
	fs.IntVar(&c.maxTokens, "max-tokens", 12000, "completion budget per call")
	fs.StringVar(&c.only, "only", "", "restrict per-source stages to these comma-separated source ids")
	fs.IntVar(&c.topicBatch, "batch", 120, "claims per clustering call")
	fs.IntVar(&c.sysLimit, "systems", 90, "catalogue at most this many systems")
	fs.IntVar(&c.verifyMax, "verify-max", 0, "verify at most this many mechanic clusters, highest priority first (0: all)")
	if len(os.Args) < 2 {
		usage(fs)
	}
	stage := os.Args[1]
	_ = fs.Parse(os.Args[2:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	st := knowledge.Store{Dir: c.dir}
	var err error
	switch stage {
	case "discover":
		err = discover(ctx, c, st)
	case "fetch":
		err = fetch(ctx, c, st)
	case "extract":
		err = extract(ctx, c, st)
	case "cluster":
		err = cluster(ctx, c, st)
	case "contradict":
		err = contradict(ctx, c, st)
	case "verify":
		err = verify(ctx, c, st)
	case "systems":
		err = systems(ctx, c, st)
	case "distil":
		err = distil(c, st)
	case "status":
		err = status(c, st)
	case "search":
		err = search(c, fs.Args())
	case "all":
		for _, f := range []func() error{
			func() error {
				if exists(st.SourcesPath()) {
					return nil
				}
				return discover(ctx, c, st)
			},
			func() error { return fetch(ctx, c, st) },
			func() error { return extract(ctx, c, st) },
			func() error { return cluster(ctx, c, st) },
			func() error { return contradict(ctx, c, st) },
			func() error { return verify(ctx, c, st) },
			func() error { return systems(ctx, c, st) },
			func() error { return distil(c, st) },
			func() error { return status(c, st) },
		} {
			if err = f(); err != nil {
				break
			}
		}
	default:
		usage(fs)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage(fs *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, "usage: fenceknow discover|fetch|extract|cluster|contradict|verify|systems|distil|status|all [flags]\n       fenceknow search [flags] term...")
	fs.PrintDefaults()
	os.Exit(2)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func (c config) client(thinking bool) llm.Client {
	temp := 0.2
	return &llm.OpenAI{BaseURL: c.baseURL, Model: c.model, OpenAIOptions: llm.OpenAIOptions{
		NoSchema: true, Thinking: &thinking, Temperature: &temp,
	}, HTTP: longHTTP}
}

// Bulk calls run for minutes; the bridge's client is tuned for sub-second decisions.
var longHTTP = &http.Client{Timeout: 15 * time.Minute}

func loadSources(st knowledge.Store, only string) ([]knowledge.Source, map[string]knowledge.Source, error) {
	srcs, err := knowledge.LoadSources(st.SourcesPath())
	if err != nil {
		return nil, nil, fmt.Errorf("%w (run `fenceknow discover` first)", err)
	}
	byID := map[string]knowledge.Source{}
	for _, s := range srcs {
		byID[s.ID] = s
	}
	if only != "" {
		var sel []knowledge.Source
		for _, id := range strings.Split(only, ",") {
			if s, ok := byID[strings.TrimSpace(id)]; ok {
				sel = append(sel, s)
			}
		}
		srcs = sel
	}
	return srcs, byID, nil
}

func discover(ctx context.Context, c config, st knowledge.Store) error {
	seeds, err := knowledge.LoadSeeds(c.seeds)
	if err != nil {
		return err
	}
	prev, _ := knowledge.LoadSources(st.SourcesPath())
	srcs := knowledge.Discover(ctx, knowledge.NewFetcher(), seeds, prev, log.Printf)
	log.Printf("discover: %d sources", len(srcs))
	return knowledge.SaveSources(st.SourcesPath(), srcs, time.Now().UTC().Format(time.RFC3339))
}

func fetch(ctx context.Context, c config, st knowledge.Store) error {
	srcs, _, err := loadSources(st, c.only)
	if err != nil {
		return err
	}
	f := knowledge.NewFetcher()
	results := map[string]knowledge.FetchResult{}
	var prev []knowledge.FetchResult
	_ = readJSONFile(st.FetchLogPath(), &prev)
	for _, r := range prev {
		results[r.ID] = r
	}
	// Videos go one at a time with a pause: YouTube throttles transcript requests hard.
	var mu sync.Mutex
	var wg sync.WaitGroup
	var pages, videos []knowledge.Source
	for _, s := range srcs {
		if s.Type == knowledge.TypeVideo {
			videos = append(videos, s)
		} else {
			pages = append(pages, s)
		}
	}
	record := func(r knowledge.FetchResult) {
		mu.Lock()
		defer mu.Unlock()
		results[r.ID] = r
		if r.OK {
			if r.Via != "cache" {
				log.Printf("fetch %s: %d chars via %s", r.ID, r.Chars, r.Via)
			}
		} else {
			log.Printf("fetch %s: %s", r.ID, r.Error)
		}
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, s := range pages {
			if ctx.Err() != nil {
				return
			}
			record(f.Fetch(ctx, st, s))
		}
	}()
	go func() {
		defer wg.Done()
		for _, s := range videos {
			if ctx.Err() != nil {
				return
			}
			r := f.Fetch(ctx, st, s)
			record(r)
			if r.Via != "cache" {
				time.Sleep(4 * time.Second)
			}
		}
	}()
	wg.Wait()
	var all []knowledge.FetchResult
	ok := 0
	for _, r := range results {
		all = append(all, r)
		if r.OK {
			ok++
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	log.Printf("fetch: %d/%d sources have text", ok, len(all))
	return writeJSONFile(st.FetchLogPath(), all)
}

func extract(ctx context.Context, c config, st knowledge.Store) error {
	srcs, _, err := loadSources(st, c.only)
	if err != nil {
		return err
	}
	ex := &knowledge.Extractor{LLM: c.client(false), MaxTokens: c.maxTokens}
	var mu sync.Mutex
	kept, dropped, failed := 0, 0, 0
	parallel(ctx, srcs, c.workers, func(s knowledge.Source) {
		if !exists(st.TextPath(s.ID)) {
			return
		}
		cached := exists(st.ClaimsPath(s.ID))
		k, d, err := ex.ExtractSource(ctx, st, s)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			failed++
			log.Printf("extract %s: %v", s.ID, err)
			return
		}
		kept += k
		dropped += d
		if !cached {
			log.Printf("extract %s: %d claims (%d dropped)", s.ID, k, d)
		}
	})
	log.Printf("extract: %d claims kept, %d dropped this run, %d sources failed", kept, dropped, failed)
	return ctx.Err()
}

func allClaims(st knowledge.Store, srcs []knowledge.Source) ([]knowledge.Claim, error) {
	var all []knowledge.Claim
	for _, s := range srcs {
		if !exists(st.ClaimsPath(s.ID)) {
			continue
		}
		cs, err := knowledge.ReadClaims(st.ClaimsPath(s.ID))
		if err != nil {
			return nil, err
		}
		all = append(all, cs...)
	}
	return all, nil
}

func cluster(ctx context.Context, c config, st knowledge.Store) error {
	srcs, byID, err := loadSources(st, "")
	if err != nil {
		return err
	}
	all, err := allClaims(st, srcs)
	if err != nil {
		return err
	}
	// Opinions stay in claims.jsonl but are not worth a model call to merge.
	var claims []knowledge.Claim
	for _, cl := range all {
		if cl.Kind != "opinion" {
			claims = append(claims, cl)
		}
	}
	if exists(st.ClustersPath()) {
		var ks []knowledge.Cluster
		if err := readJSONFile(st.ClustersPath(), &ks); err == nil && clusteredAll(ks, claims) {
			log.Printf("cluster: cached (%d clusters)", len(ks))
			return nil
		}
	}
	cl := &knowledge.Clusterer{LLM: c.client(false), Batch: c.topicBatch, MaxTokens: c.maxTokens, Workers: c.workers}
	ks, err := cl.ClusterAll(ctx, claims, byID, log.Printf)
	if err != nil {
		return err
	}
	log.Printf("cluster: %d claims -> %d clusters", len(claims), len(ks))
	_ = os.Remove(filepath.Join(c.dir, "contradictions.done"))
	return writeJSONFile(st.ClustersPath(), ks)
}

// clusteredAll reports whether the cached clustering covers exactly these claims; new
// claims from new sources mean clustering again.
func clusteredAll(ks []knowledge.Cluster, claims []knowledge.Claim) bool {
	n := 0
	in := map[string]bool{}
	for _, k := range ks {
		for _, m := range k.Members {
			in[m] = true
			n++
		}
	}
	if n != len(claims) {
		return false
	}
	for _, c := range claims {
		if !in[c.ID] {
			return false
		}
	}
	return true
}

// contradict replaces the clustering pass's contradiction links with a dedicated pass;
// a marker file makes it run once per clustering.
func contradict(ctx context.Context, c config, st knowledge.Store) error {
	marker := filepath.Join(c.dir, "contradictions.done")
	if exists(marker) {
		return nil
	}
	var ks []knowledge.Cluster
	if err := readJSONFile(st.ClustersPath(), &ks); err != nil {
		return err
	}
	cl := &knowledge.Clusterer{LLM: c.client(false), Batch: c.topicBatch, MaxTokens: c.maxTokens, Workers: c.workers}
	if err := cl.Contradictions(ctx, ks, log.Printf); err != nil {
		return err
	}
	if err := writeJSONFile(st.ClustersPath(), ks); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func loadCorpus(c config) (*knowledge.Corpus, error) {
	if c.decomp == "" {
		return nil, fmt.Errorf("no decompile: pass -decomp or set FENCE_DECOMP")
	}
	return knowledge.LoadCorpus(c.decomp, c.gamedata)
}

func verify(ctx context.Context, c config, st knowledge.Store) error {
	var ks []knowledge.Cluster
	if err := readJSONFile(st.ClustersPath(), &ks); err != nil {
		return err
	}
	srcs, _, err := loadSources(st, "")
	if err != nil {
		return err
	}
	claims, err := allClaims(st, srcs)
	if err != nil {
		return err
	}
	byClaim := map[string]knowledge.Claim{}
	for _, cl := range claims {
		byClaim[cl.ID] = cl
	}
	corpus, err := loadCorpus(c)
	if err != nil {
		return err
	}
	v := &knowledge.Verifier{LLM: c.client(true), Corpus: corpus, MaxTokens: c.maxTokens, Budget: 24000}
	order := knowledge.VerifyOrder(ks)
	if c.verifyMax > 0 && len(order) > c.verifyMax {
		order = order[:c.verifyMax]
	}
	var todo []int
	for _, i := range order {
		if exists(st.VerifyPath(ks[i].ID)) {
			var vd knowledge.Verdict
			if readJSONFile(st.VerifyPath(ks[i].ID), &vd) == nil {
				ks[i].Verdict = &vd
				ks[i].Confidence = knowledge.Confidence(ks[i])
			}
			continue
		}
		todo = append(todo, i)
	}
	log.Printf("verify: %d to check, %d cached", len(todo), len(order)-len(todo))
	counts := map[string]int{}
	const termBatch = 15
	for start := 0; start < len(todo) && ctx.Err() == nil; start += termBatch {
		batch := todo[start:min(start+termBatch, len(todo))]
		sel := make([]knowledge.Cluster, len(batch))
		for j, i := range batch {
			sel[j] = ks[i]
		}
		terms, err := v.Terms(ctx, sel)
		if err != nil {
			log.Printf("verify terms: %v", err)
			continue
		}
		var mu sync.Mutex
		parallel(ctx, batch, c.workers, func(i int) {
			k := ks[i]
			var ex []string
			for _, m := range k.Members {
				if cl, ok := byClaim[m]; ok && len(ex) < 3 {
					ex = append(ex, cl.Claim)
				}
			}
			var t []string
			for j, b := range batch {
				if b == i {
					t = terms[j]
				}
			}
			vd, err := v.Verify(ctx, st, k, ex, t)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Printf("verify %s: %v", k.ID, err)
				counts["error"]++
				return
			}
			ks[i].Verdict = &vd
			ks[i].Confidence = knowledge.Confidence(ks[i])
			counts[vd.Verdict]++
		})
		log.Printf("verify: %d/%d checked: %v", min(start+termBatch, len(todo)), len(todo), counts)
		if err := writeJSONFile(st.ClustersPath(), ks); err != nil {
			return err
		}
	}
	log.Printf("verify: %d mechanic clusters checked or cached, this run: %v", len(order), counts)
	return writeJSONFile(st.ClustersPath(), ks)
}

func systems(ctx context.Context, c config, st knowledge.Store) error {
	srcs, byID, err := loadSources(st, "")
	if err != nil {
		return err
	}
	claims, err := allClaims(st, srcs)
	if err != nil {
		return err
	}
	corpus, err := loadCorpus(c)
	if err != nil {
		return err
	}
	sc := &knowledge.SystemCataloguer{LLM: c.client(false), Corpus: corpus, MaxTokens: c.maxTokens, Max: c.sysLimit}
	sys, err := sc.Catalogue(ctx, claims, byID, filepath.Join(c.dir, "systems.json"), log.Printf)
	if err != nil {
		return err
	}
	if err := knowledge.AssessCoverage(".", sys); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(c.dir, "systems.final.json"), sys); err != nil {
		return err
	}
	out := filepath.Join(c.docs, "SYSTEMS.md")
	log.Printf("systems: %d catalogued -> %s", len(sys), out)
	return os.WriteFile(out, []byte(knowledge.RenderSystems(sys, time.Now().Format("2006-01-02"))), 0o644)
}

func distil(c config, st knowledge.Store) error {
	var ks []knowledge.Cluster
	if err := readJSONFile(st.ClustersPath(), &ks); err != nil {
		return err
	}
	srcs, byID, err := loadSources(st, "")
	if err != nil {
		return err
	}
	claims, err := allClaims(st, srcs)
	if err != nil {
		return err
	}
	for i := range ks {
		ks[i].Confidence = knowledge.Confidence(ks[i])
	}
	if err := os.WriteFile(st.AllClaimsPath(), knowledge.EncodeJSONL(knowledge.JoinClaims(claims, ks)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(st.DigestPath(), []byte(knowledge.Digest(ks, byID, 40)), 0o644); err != nil {
		return err
	}
	out := filepath.Join(c.docs, "SOURCES.md")
	if err := os.WriteFile(out, []byte(knowledge.RenderSources(srcs, claims, ks, readFetchLog(st))), 0o644); err != nil {
		return err
	}
	log.Printf("distil: %s, %s, %s", st.AllClaimsPath(), st.DigestPath(), out)
	return nil
}

func readFetchLog(st knowledge.Store) map[string]knowledge.FetchResult {
	var rs []knowledge.FetchResult
	_ = readJSONFile(st.FetchLogPath(), &rs)
	m := map[string]knowledge.FetchResult{}
	for _, r := range rs {
		m[r.ID] = r
	}
	return m
}

func status(c config, st knowledge.Store) error {
	srcs, _, err := loadSources(st, "")
	if err != nil {
		return err
	}
	type row struct{ total, text, claims, n int }
	byType := map[string]*row{}
	var types []string
	for _, s := range srcs {
		r := byType[s.Type]
		if r == nil {
			r = &row{}
			byType[s.Type] = r
			types = append(types, s.Type)
		}
		r.total++
		if exists(st.TextPath(s.ID)) {
			r.text++
		}
		if exists(st.ClaimsPath(s.ID)) {
			r.claims++
			cs, _ := knowledge.ReadClaims(st.ClaimsPath(s.ID))
			r.n += len(cs)
		}
	}
	sort.Strings(types)
	fmt.Printf("%-12s %7s %7s %9s %7s\n", "type", "sources", "fetched", "extracted", "claims")
	var t row
	for _, ty := range types {
		r := byType[ty]
		fmt.Printf("%-12s %7d %7d %9d %7d\n", ty, r.total, r.text, r.claims, r.n)
		t.total += r.total
		t.text += r.text
		t.claims += r.claims
		t.n += r.n
	}
	fmt.Printf("%-12s %7d %7d %9d %7d\n", "total", t.total, t.text, t.claims, t.n)
	var ks []knowledge.Cluster
	if readJSONFile(st.ClustersPath(), &ks) == nil {
		conf, verd := map[string]int{}, map[string]int{}
		mech := 0
		for _, k := range ks {
			conf[knowledge.Confidence(k)]++
			if k.Kind == "mechanic" {
				mech++
				if k.Verdict != nil {
					verd[k.Verdict.Verdict]++
				} else {
					verd["pending"]++
				}
			}
		}
		fmt.Printf("\nclusters: %d (mechanic %d)\nconfidence: %v\nverdicts: %v\n", len(ks), mech, conf, verd)
	}
	return nil
}

// search prints what the verifier would show the model for these terms; for spot checks.
func search(c config, terms []string) error {
	corpus, err := loadCorpus(c)
	if err != nil {
		return err
	}
	for _, s := range corpus.Search(terms, 16000) {
		fmt.Printf("--- %s:%d\n%s\n", s.File, s.Start, strings.Join(s.Lines, "\n"))
	}
	return nil
}

func parallel[T any](ctx context.Context, items []T, workers int, fn func(T)) {
	ch := make(chan T)
	var wg sync.WaitGroup
	for w := 0; w < max(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				fn(it)
			}
		}()
	}
	for _, it := range items {
		if ctx.Err() != nil {
			break
		}
		ch <- it
	}
	close(ch)
	wg.Wait()
}
