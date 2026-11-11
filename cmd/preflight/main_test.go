package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wav() []byte {
	n := 4000
	b := make([]byte, 44+2*n)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*n))
	copy(b[8:], "WAVEfmt ")
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*n))
	return b
}

// rig is a machine where every check passes: the installed build verified, the mod built and
// installed, a fresh backup, a bridge, a strategist and narrator, Kokoro, the panel and an OBS scene.
func rig(t *testing.T) config {
	t.Helper()
	root := t.TempDir()
	c := config{repo: filepath.Join(root, "repo"), gameDir: filepath.Join(root, "game"), gameData: filepath.Join(root, "data"),
		backupDir: filepath.Join(root, "backups"), steamApps: filepath.Join(root, "steamapps"), compat: true, narrTest: true,
		persona: "shark", minFreeMB: 3000, minDiskMB: 100, timeout: 5 * time.Second,
		obsDirs: []string{filepath.Join(root, "obs")}}
	c.personas = filepath.Join(c.repo, "docs/personas")
	c.wavOut = filepath.Join(c.repo, "var/show/preflight.wav")
	write(t, filepath.Join(c.gameDir, "Probably Stolen_Data/Managed/Assembly-CSharp.dll"), "game")
	write(t, filepath.Join(c.steamApps, "appmanifest_4348910.acf"), "\"AppState\"\n{\n\t\"buildid\"\t\t\"26525552\"\n}\n")
	write(t, filepath.Join(c.repo, "compat/versions/26525552.json"), "{}")
	write(t, filepath.Join(c.repo, "docs/personas/shark.md"), "# Shark\n")
	write(t, filepath.Join(c.repo, "mod/src/Fence.Mod/bin/Release/netstandard2.1/Fence.Mod.dll"), "mod-v1")
	write(t, filepath.Join(c.gameDir, "Mods/fence/Fence.Mod.dll"), "mod-v1")
	write(t, filepath.Join(c.gameData, "save_1.es3"), "save")
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(filepath.Join(c.gameData, "save_1.es3"), old, old)
	write(t, filepath.Join(c.backupDir, "20261110-120000/save_1.es3"), "save")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"ok":true,"protocol":1}`)) })
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[{"id":"qwen27/qwen27","status":{"value":"ready"}},{"id":"narrator"},{"id":"big","status":{"value":"unloaded"}}]}`))
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Kwargs map[string]any `json:"chat_template_kwargs"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Kwargs["enable_thinking"] != false {
			// a thinking model's 40 tokens all go to its reasoning
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": ""}}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "Evening, all."}}}})
	})
	mux.HandleFunc("POST /v1/audio/speech", func(w http.ResponseWriter, _ *http.Request) { w.Write(wav()) })
	var said atomic.Int64
	mux.HandleFunc("POST /v1/ingest", func(w http.ResponseWriter, _ *http.Request) {
		said.Store(time.Now().UnixMilli() + 300)
		w.Write([]byte(`{"published":1}`))
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, _ *http.Request) {
		say := ""
		if at := said.Load(); at > 0 && time.Now().UnixMilli() >= at {
			say = fmt.Sprintf(`,"say":{"at":%d,"kind":"say","say":{"line":"On the air.","audio":"/audio/line-000009.wav","voice":"am_fenrir","source":"llm"}}`, at)
		}
		w.Write([]byte(`{"player":{"id":"live","model":"qwen27"},"day":12,"setup":["Strategist: qwen27"],"ticker":[],"moments":[],"board":[],"net_worth":[]` + say + `}`))
	})
	for _, p := range []string{"/{$}", "/column", "/lower", "/about"} {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c.bridge, c.show, c.ttsURL = srv.URL, srv.URL, srv.URL
	c.stratProvider, c.stratURL, c.stratModel = "openai", srv.URL+"/v1", "qwen27"
	c.narrURL, c.narrModel = srv.URL+"/v1", "narrator"
	write(t, filepath.Join(root, "obs/basic/scenes/Fence.json"), `{"name":"Fence","sources":[
		{"name":"Game","id":"pipewire-window-capture-source","settings":{}},
		{"name":"Panel","id":"browser_source","settings":{"url":"`+srv.URL+`/","width":1920,"height":1080,"reroute_audio":true}}]}`)

	c.runCompat = func(context.Context, config) (int, string, error) { return 0, "ok", nil }
	c.gpuFree = func(context.Context) (int64, int64, error) { return 4000, 32607, nil }
	c.gameUp = func() bool { return false }
	c.diskFree = func(string) (int64, error) { return 50000, nil }
	return c
}

func byName(rs []Result) map[string]Result {
	out := map[string]Result{}
	for _, r := range rs {
		out[r.Name] = r
	}
	return out
}

func TestAllGo(t *testing.T) {
	c := rig(t)
	c.soundcheck = true
	rs := Checks(context.Background(), c)
	for _, r := range rs {
		if r.Status != Go {
			t.Errorf("%s: %s %s", r.Name, r.Status, r.Detail)
		}
	}
	var out bytes.Buffer
	Print(&out, rs)
	if !strings.Contains(out.String(), "VERDICT: GO.") || strings.Contains(out.String(), "127.0.0.1") {
		t.Errorf("table (no addresses on a table that may be shown on stream):\n%s", out.String())
	}
}

func TestNoGoAndWarnings(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*config)
		check  string
		want   Status
		detail string
	}{
		{"the game patched", func(c *config) {
			write(t, filepath.Join(c.steamApps, "appmanifest_4348910.acf"), "\"buildid\"\t\t\"27000000\"\n")
		}, "game build", NoGo, "make compat"},
		{"a hook went missing", func(c *config) {
			c.runCompat = func(context.Context, config) (int, string, error) { return 1, "MISSING StoreBell::OnClick", nil }
		}, "compat hooks", NoGo, "StoreBell::OnClick"},
		{"no dotnet", func(c *config) {
			c.runCompat = func(context.Context, config) (int, string, error) { return 0, "", os.ErrNotExist }
		}, "compat hooks", Warn, "make compat"},
		{"stale mod", func(c *config) { write(t, filepath.Join(c.gameDir, "Mods/fence/Fence.Mod.dll"), "mod-v0") }, "mod installed", Warn, "make mod-install"},
		{"saves newer than the backup", func(c *config) { write(t, filepath.Join(c.gameData, "save_1.es3"), "played") }, "save backup", NoGo, "make backup-saves"},
		{"bridge down", func(c *config) { c.bridge = "http://127.0.0.1:1" }, "bridge", NoGo, "start it"},
		{"strategist model missing", func(c *config) { c.stratModel = "glm" }, "strategist", NoGo, "does not serve glm"},
		{"anthropic without a key", func(c *config) { c.stratProvider = "anthropic" }, "strategist", NoGo, "ANTHROPIC_API_KEY"},
		{"narrator unloaded locally", func(c *config) { c.narrModel = "big" }, "narrator LLM", Warn, "not loaded"},
		{"no narrator", func(c *config) { c.narrURL = "" }, "narrator LLM", Warn, "captions"},
		{"tts down", func(c *config) { c.ttsURL = "http://127.0.0.1:1" }, "TTS test line", Warn, "captions only"},
		{"unknown persona", func(c *config) { c.persona = "nobody" }, "TTS test line", NoGo, "no file"},
		{"panel down", func(c *config) { c.show = "http://127.0.0.1:1" }, "fenceshow panel", NoGo, "start fenceshow"},
		{"panel audio not routed", func(c *config) {
			write(t, filepath.Join(c.obsDirs[0], "basic/scenes/Fence.json"), `{"name":"Fence","sources":[
				{"id":"xcomposite_input","settings":{}},{"id":"browser_source","settings":{"url":"`+c.show+`/column"}}]}`)
		}, "OBS scene", Warn, "does not route audio"},
		{"no OBS here", func(c *config) { c.obsDirs = nil }, "OBS scene", Warn, "by hand"},
		{"5090 full", func(c *config) {
			c.gpuFree = func(context.Context) (int64, int64, error) { return 1200, 32607, nil }
		}, "GPU memory", Warn, "CPU Kokoro"},
		{"game up with a local narrator", func(c *config) { c.gameUp = func() bool { return true } }, "game process", Warn, "captions only"},
		{"disk full", func(c *config) { c.diskFree = func(string) (int64, error) { return 10, nil } }, "disk", NoGo, "need room"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := rig(t)
			tc.break_(&c)
			r := byName(Checks(context.Background(), c))[tc.check]
			if r.Status != tc.want || !strings.Contains(r.Detail, tc.detail) {
				t.Errorf("%s = %s %q, want %s with %q", tc.check, r.Status, r.Detail, tc.want, tc.detail)
			}
		})
	}
}

func TestBackupNow(t *testing.T) {
	c := rig(t)
	write(t, filepath.Join(c.gameData, "save_1.es3"), "played")
	c.backup = true
	r := byName(Checks(context.Background(), c))["save backup"]
	if r.Status != Go || !strings.Contains(r.Detail, "backed up 1 save") {
		t.Fatalf("backup = %+v", r)
	}
	if r := byName(Checks(context.Background(), c))["save backup"]; r.Status != Go || strings.Contains(r.Detail, "backed up") {
		t.Errorf("the second run finds the backup current: %+v", r)
	}
}

func TestRunExitCodes(t *testing.T) {
	var out bytes.Buffer
	env := func(k string) string {
		if k == "HOME" {
			return t.TempDir()
		}
		return ""
	}
	code, err := run(context.Background(), []string{"-compat=false", "-game-dir", t.TempDir(), "-bridge", "http://127.0.0.1:1", "-show", "http://127.0.0.1:1",
		"-tts-url", "http://127.0.0.1:1", "-repo", t.TempDir(), "-timeout", "1s"}, env, &out, &out)
	if err != nil || code != 1 || !strings.Contains(out.String(), "VERDICT: NO-GO") {
		t.Errorf("code %d err %v\n%s", code, err, out.String())
	}
}
