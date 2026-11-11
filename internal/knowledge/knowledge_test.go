package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
)

func TestChunkText(t *testing.T) {
	line := func(i int) string { return strings.Repeat(string(rune('a'+i%26)), 39) } // 40 chars with \n
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, line(i))
	}
	long := strings.Join(lines, "\n")
	tests := []struct {
		name          string
		text          string
		max, overlap  int // tokens
		wantChunks    int
		wantMaxChars  int
		wantOverlapOK bool
	}{
		{"empty", "", 100, 10, 0, 0, false},
		{"fits", "one\ntwo", 100, 10, 1, 400, false},
		{"splits with overlap", long, 250, 20, 5, 1000, true},
		{"no overlap", long, 250, 0, 4, 1000, false},
		{"single huge line", strings.Repeat("word ", 1000), 100, 10, 12, 400, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChunkText(tt.text, tt.max, tt.overlap)
			if len(got) != tt.wantChunks {
				t.Fatalf("chunks = %d, want %d", len(got), tt.wantChunks)
			}
			for i, c := range got {
				if c.Index != i {
					t.Errorf("chunk %d has index %d", i, c.Index)
				}
				if len(c.Text) > tt.wantMaxChars {
					t.Errorf("chunk %d is %d chars, max %d", i, len(c.Text), tt.wantMaxChars)
				}
			}
			if tt.wantOverlapOK {
				for i := 1; i < len(got); i++ {
					prevLast := got[i-1].Text[strings.LastIndexByte(got[i-1].Text, '\n')+1:]
					if !strings.HasPrefix(got[i].Text, strings.SplitN(got[i].Text, "\n", 2)[0]) || !strings.Contains(got[i].Text, prevLast) {
						t.Errorf("chunk %d does not repeat the tail of chunk %d", i, i-1)
					}
				}
			}
			// Every input line survives somewhere.
			all := ""
			for _, c := range got {
				all += c.Text + "\n"
			}
			for _, l := range strings.Split(tt.text, "\n") {
				if strings.TrimSpace(l) != "" && !strings.Contains(all, strings.TrimSpace(l)) && len(l) < tt.max*charsPerToken {
					t.Errorf("line %q lost", l)
				}
			}
		})
	}
}

func TestCleanVTT(t *testing.T) {
	autoSubs := `WEBVTT
Kind: captions
Language: en

00:00:00.160 --> 00:00:02.390 align:start position:0%

hello<00:00:00.400><c> and</c><00:00:00.800><c> welcome</c>

00:00:02.390 --> 00:00:02.400 align:start position:0%
hello and welcome


00:00:02.400 --> 00:00:05.000 align:start position:0%
hello and welcome
to<00:00:03.000><c> the</c><c> shop</c>

00:00:05.000 --> 00:00:05.010
to the shop


00:00:40.000 --> 00:00:42.000
[Music]

00:00:42.000 --> 00:00:44.000
buy low &amp; sell high
`
	autoSubs = strings.ReplaceAll(autoSubs, "position:0%\n\n", "position:0%\n \n")
	autoSubs = strings.ReplaceAll(autoSubs, "welcome\n\n", "welcome\n \n")
	plain := `WEBVTT

00:00:00.160 --> 00:00:02.399
You can have a shop full of valuable

00:00:02.399 --> 00:00:04.560
merchandise and still lose

01:02:03.000 --> 01:02:05.000
your run.
`
	tests := []struct {
		name, in string
		para     int
		want     string
	}{
		{"youtube rolling captions", autoSubs, 30, "[0:00] hello and welcome to the shop\n[0:42] buy low & sell high\n"},
		{"plain cues and hour stamps", plain, 30, "[0:00] You can have a shop full of valuable merchandise and still lose\n[1:02:03] your run.\n"},
		{"empty", "WEBVTT\n", 30, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanVTT(tt.in, tt.para); got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestParseClaims(t *testing.T) {
	good := `{"claim":"Rent rises 5% each week on Mark Up difficulty.","topic":"rent & cash","system":"Rent","kind":"mechanic","conditions":{"difficulty":"Mark Up"},"locator":"[0:46]","quote":"rent increases by 5%"}`
	tests := []struct {
		name        string
		out         string
		wantN       int
		wantDropped int
		wantErr     bool
	}{
		{"wrapped object", `{"claims":[` + good + `]}`, 1, 0, false},
		{"fenced with thinking", "<think>hmm {not json}</think>```json\n{\"claims\":[" + good + "]}\n```", 1, 0, false},
		{"bare array", `[` + good + `]`, 1, 0, false},
		{"json lines", good + "\n" + good, 2, 0, false},
		{"bad topic and kind dropped", `{"claims":[` + good + `,{"claim":"Something concrete enough here.","topic":"weather","kind":"mechanic"},{"claim":"Another concrete claim here.","topic":"rent & cash","kind":"fact"}]}`, 1, 2, false},
		{"too short dropped", `{"claims":[{"claim":"buy","topic":"stock & buying","kind":"strategy"}]}`, 0, 1, false},
		{"topic spelled with and", `{"claims":[{"claim":"Hold food before a shortage event.","topic":"Market events and speculation","kind":"strategy"}]}`, 1, 0, false},
		{"conditions as string kept", `{"claims":[{"claim":"Hold food before a shortage event.","topic":"market events & speculation","kind":"strategy","conditions":"days 1-7"}]}`, 1, 0, false},
		{"empty list", `{"claims":[]}`, 0, 0, false},
		{"garbage", `I could not find anything.`, 0, 0, true},
		{"truncated by token limit", `{"claims":[` + good + `,` + good + `,{"claim":"Cut off mid`, 2, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, d, err := ParseClaims(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(cs) != tt.wantN || d != tt.wantDropped {
				t.Fatalf("got %d claims, %d dropped; want %d, %d", len(cs), d, tt.wantN, tt.wantDropped)
			}
		})
	}
	cs, _, _ := ParseClaims(`{"claims":[` + good + `]}`)
	if cs[0].System != "rent" || cs[0].Conditions.Difficulty != "Mark Up" {
		t.Errorf("normalisation: %+v", cs[0])
	}
	long := strings.Repeat("x", 300)
	cs, _, _ = ParseClaims(`{"claims":[{"claim":"A claim with a very long quote.","topic":"early game","kind":"strategy","quote":"` + long + `"}]}`)
	if n := len([]rune(cs[0].Quote)); n != maxQuoteLen {
		t.Errorf("quote truncated to %d runes, want %d", n, maxQuoteLen)
	}
}

func TestMergeGroups(t *testing.T) {
	texts := []string{"a", "b", "c", "d", "e"}
	tests := []struct {
		name       string
		groups     []Group
		wantSets   [][]int
		wantStmts  []string
		wantContra []contraPair
	}{
		{"no groups: singletons", nil, [][]int{{0}, {1}, {2}, {3}, {4}}, []string{"a", "b", "c", "d", "e"}, nil},
		{
			"merge, leftovers, duplicates and out of range",
			[]Group{{Members: []int{3, 1}, Statement: "A+C"}, {Members: []int{1, 2, 9, 0}, Statement: "B"}, {Members: []int{}}},
			[][]int{{0, 2}, {1}, {3}, {4}},
			[]string{"A+C", "B", "d", "e"},
			nil,
		},
		{
			"contradictions dedup and drop self/dropped",
			[]Group{
				{Members: []int{1}, Statement: "x", Contradicts: []int{2, 1, 3}, Note: "n"},
				{Members: []int{2}, Statement: "y", Contradicts: []int{1}},
				{Members: []int{1}}, // fully duplicate, dropped
			},
			[][]int{{0}, {1}, {2}, {3}, {4}},
			[]string{"x", "y", "c", "d", "e"},
			[]contraPair{{A: 0, B: 1, Note: "n"}},
		},
		{"empty statement falls back to first member", []Group{{Members: []int{5, 4}}}, [][]int{{3, 4}, {0}, {1}, {2}}, []string{"d", "a", "b", "c"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := MergeGroups(texts, tt.groups)
			if !reflect.DeepEqual(p.Sets, tt.wantSets) || !reflect.DeepEqual(p.Statements, tt.wantStmts) || !reflect.DeepEqual(p.Contra, tt.wantContra) {
				t.Errorf("got %v %v %v\nwant %v %v %v", p.Sets, p.Statements, p.Contra, tt.wantSets, tt.wantStmts, tt.wantContra)
			}
		})
	}
}

// fakeLLM answers clustering prompts by grouping lines whose text is equal ignoring
// case, and flags "never X" against "always X".
type fakeLLM struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.fail {
		return llm.Response{}, errors.New("down")
	}
	var items []string
	for _, l := range strings.Split(req.Messages[0].Content, "\n") {
		if i := strings.Index(l, ". "); i > 0 && l[0] >= '0' && l[0] <= '9' {
			items = append(items, l[i+2:])
		}
	}
	key := func(s string) string {
		s = strings.ToLower(s)
		s = strings.TrimPrefix(s, "always ")
		return strings.TrimPrefix(s, "never ")
	}
	groupOf := map[string]int{}
	var groups []Group
	for i, it := range items {
		k := strings.ToLower(it)
		if g, ok := groupOf[k]; ok {
			groups[g].Members = append(groups[g].Members, i+1)
			continue
		}
		groupOf[k] = len(groups)
		groups = append(groups, Group{Members: []int{i + 1}, Statement: strings.ToLower(it)})
	}
	for gi := range groups {
		for gj := range groups {
			a, b := groups[gi].Statement, groups[gj].Statement
			if gi != gj && key(a) == key(b) && strings.HasPrefix(a, "always") && strings.HasPrefix(b, "never") {
				groups[gi].Contradicts = append(groups[gi].Contradicts, gj+1)
				groups[gi].Note = "opposite advice"
			}
		}
	}
	out, _ := json.Marshal(map[string]any{"groups": groups})
	return llm.Response{Text: string(out)}, nil
}

func TestClusterAll(t *testing.T) {
	srcs := map[string]Source{
		"v1": {ID: "v1", Type: TypeVideo, Channel: "HumbleSeal", Quality: QualityHigh, URL: "https://youtube.com/watch?v=aaaaaaaaaaa"},
		"v2": {ID: "v2", Type: TypeVideo, Channel: "HumbleSeal", Quality: QualityHigh, URL: "https://youtube.com/watch?v=bbbbbbbbbbb"},
		"w1": {ID: "w1", Type: TypeWiki, Quality: QualityHigh, URL: "https://wikily.gg/probably-stolen/rent"},
		"o1": {ID: "o1", Type: TypePlayer, Quality: QualityOwner, URL: "docs/knowledge/PLAYER-NOTES.md"},
	}
	mk := func(src, text, topic, kind, system string) Claim {
		return Claim{ID: ClaimID(src, text), Claim: text, Topic: topic, Kind: kind, SourceID: src, System: system}
	}
	// Batch 2 and the system sort split both the duplicate and the contradiction across
	// first-level batches; only the second pass can join them.
	claims := []Claim{
		mk("v1", "Pay rent first", "rent & cash", "strategy", "a"),
		mk("v2", "Always buy insurance", "rent & cash", "strategy", "a"),
		mk("w1", "pay rent first", "rent & cash", "strategy", "b"),
		mk("v1", "Never buy insurance", "rent & cash", "strategy", "b"),
		mk("w1", "Rent is due on Sundays", "endgame/buyout", "mechanic", ""),
		mk("o1", "Scavenge every night", "scavenging", "strategy", ""),
	}
	c := &Clusterer{LLM: &fakeLLM{}, Batch: 2, Workers: 2}
	ks, err := c.ClusterAll(context.Background(), claims, srcs, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	byStmt := map[string]Cluster{}
	for _, k := range ks {
		byStmt[k.Statement] = k
	}
	if len(ks) != 5 {
		t.Fatalf("got %d clusters: %+v", len(ks), ks)
	}
	rent := byStmt["pay rent first"]
	if len(rent.Members) != 2 || rent.Origins != 2 || rent.Confidence != ConfMany {
		t.Errorf("cross-batch merge: %+v", rent)
	}
	always, never := byStmt["always buy insurance"], byStmt["never buy insurance"]
	if len(always.Contradicts) != 1 || always.Contradicts[0].ID != never.ID || len(never.Contradicts) != 1 {
		t.Errorf("contradiction not linked both ways: %+v / %+v", always, never)
	}
	if byStmt["Scavenge every night"].Confidence != ConfOwner {
		t.Errorf("owner notes not top trust: %+v", byStmt["Scavenge every night"])
	}
	if k := byStmt["Rent is due on Sundays"]; k.Kind != "mechanic" || k.Confidence != ConfSingle {
		t.Errorf("single: %+v", k)
	}

	// Same channel twice is one voice.
	one := []Claim{mk("v1", "Hold food for shortages", "market events & speculation", "strategy", ""), mk("v2", "hold food for shortages", "market events & speculation", "strategy", "")}
	ks, _ = c.ClusterAll(context.Background(), one, srcs, t.Logf)
	if len(ks) != 1 || ks[0].Origins != 1 || len(ks[0].Sources) != 2 {
		t.Errorf("origins: %+v", ks)
	}

	if _, err := (&Clusterer{LLM: &fakeLLM{fail: true}, Batch: 3}).ClusterAll(context.Background(), claims, srcs, t.Logf); err == nil {
		t.Error("model failure not reported")
	}
}

func TestConfidenceVerdictWins(t *testing.T) {
	k := Cluster{Origins: 1, Contradicts: []ContraRef{{ID: "x"}}, Verdict: &Verdict{Verdict: VerdictConfirmed}}
	if got := Confidence(k); got != ConfVerified {
		t.Errorf("got %s", got)
	}
	k.Verdict.Verdict = VerdictContradicted
	if got := Confidence(k); got != ConfCodeWrong {
		t.Errorf("got %s", got)
	}
	k.Verdict.Verdict = VerdictUnverifiable
	if got := Confidence(k); got != ConfSingle {
		t.Errorf("got %s", got)
	}
}

func TestNormaliseSystems(t *testing.T) {
	labels := []string{"wine", "winemaking", "furnace", "dumping ground", "scavenging"}
	got := NormaliseSystems(labels, []GameSystem{
		{Name: "Wine", Aliases: []string{"winemaking", "bloomberry wine"}},
		{Name: "dumping ground scavenging", Aliases: []string{"dumping ground", "scavenging", "wine"}},
		{Name: "", Aliases: []string{"furnace"}},
	})
	want := []GameSystem{
		{Name: "wine", Aliases: []string{"wine", "winemaking"}},
		{Name: "dumping ground scavenging", Aliases: []string{"dumping ground", "scavenging"}},
		{Name: "furnace", Aliases: []string{"furnace"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestRobots(t *testing.T) {
	rules := ParseRobots("User-agent: Googlebot\nDisallow: /\n\nUser-Agent: *\nAllow: /\nDisallow: /api/\nDisallow: /*/edit\nDisallow: /*?s=\nDisallow: /x$\n")
	tests := []struct {
		path string
		want bool
	}{
		{"/probably-stolen/wine", true},
		{"/api/media", false},
		{"/probably-stolen/edit", false},
		{"/search?s=wine", false},
		{"/x", false},
		{"/xy", true},
	}
	for _, tt := range tests {
		if got := RobotsAllows(rules, tt.path); got != tt.want {
			t.Errorf("%s: got %v want %v (rules %v)", tt.path, got, tt.want, rules)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	page := `<html><head><title>x</title><script>var a=1;</script></head><body><nav>menu</nav><main>` +
		strings.Repeat("<p>filler text for length</p>", 30) +
		`<h2>Rent &amp; buyout</h2><ul><li>Rent is due Sunday</li></ul><table><tr><td>Mark Up</td><td>12,000</td></tr></table></main><footer>f</footer></body></html>`
	got := HTMLToText(page)
	for _, want := range []string{"## Rent & buyout", "- Rent is due Sunday", "Mark Up | 12,000"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, bad := range []string{"var a", "menu", "<p>"} {
		if strings.Contains(got, bad) {
			t.Errorf("kept %q", bad)
		}
	}
}

func TestSourceOrigin(t *testing.T) {
	a := Source{Type: TypeWiki, URL: "https://wikily.gg/probably-stolen/wine"}
	b := Source{Type: TypeWiki, URL: "https://www.wikily.gg/probably-stolen/haggling"}
	if a.Origin() != b.Origin() {
		t.Errorf("one wiki, two origins: %s %s", a.Origin(), b.Origin())
	}
	if id := SourceID(TypeVideo, "https://www.youtube.com/watch?v=za-C7jyypWU"); !strings.HasPrefix(id, "video-za-c7jyypwu-") {
		t.Errorf("video id %s", id)
	}
	if youtubeID("https://youtu.be/r-dHCBQer64") != "r-dHCBQer64" {
		t.Error("youtu.be id")
	}
}

func TestVerifyOrder(t *testing.T) {
	ks := []Cluster{
		{ID: "opinion", Kind: "opinion", Topic: "heat & stolen", Origins: 9},
		{ID: "lore", Kind: "mechanic", Topic: "difficulty", Origins: 5},
		{ID: "inspect", Kind: "mechanic", Topic: "contraband & inspections", Origins: 1},
		{ID: "water", Kind: "mechanic", Topic: "stock & buying", Statement: "Water sells for more in a crisis", Origins: 1},
		{ID: "strategy", Kind: "strategy", Topic: "rent & cash", Origins: 3},
	}
	var got []string
	for _, i := range VerifyOrder(ks) {
		got = append(got, ks[i].ID)
	}
	if want := []string{"inspect", "lore", "water"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

type fixedLLM string

func (f fixedLLM) Complete(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Text: string(f)}, nil
}

func TestContradictions(t *testing.T) {
	ks := []Cluster{
		{ID: "a", Topic: "rent & cash", Kind: "mechanic", Origins: 3, Contradicts: []ContraRef{{ID: "stale"}}},
		{ID: "b", Topic: "rent & cash", Kind: "mechanic", Origins: 1},
		{ID: "c", Topic: "rent & cash", Kind: "opinion", Origins: 1},
	}
	c := &Clusterer{LLM: fixedLLM(`{"pairs":[{"a":1,"b":2,"note":"5% vs 10%"},{"a":1,"b":1},{"a":1,"b":9}]}`), Batch: 10}
	if err := c.Contradictions(context.Background(), ks, t.Logf); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ks[0].Contradicts, []ContraRef{{ID: "b", Note: "5% vs 10%"}}) || len(ks[1].Contradicts) != 1 || ks[2].Contradicts != nil {
		t.Errorf("links: %+v", ks)
	}
	if ks[0].Confidence != ConfMany || ks[2].Confidence != ConfSingle {
		t.Errorf("confidence: %s %s", ks[0].Confidence, ks[2].Confidence)
	}
}
