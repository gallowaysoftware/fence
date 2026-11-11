package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/format"
)

func TestCommands(t *testing.T) {
	tmp := t.TempDir()
	formats := filepath.Join(tmp, "formats")
	os.MkdirAll(formats, 0o755)
	os.WriteFile(filepath.Join(formats, "sprint.json"), []byte(`{"title":"Sprint","difficulty":"hard","deadline_day":91,"handicaps":["no_mortgage"],
		"card":"c","pitch":"p","lab":{"runs":0,"wins":0}}`), 0o644)
	sums := filepath.Join(tmp, "sums")
	os.MkdirAll(sums, 0o755)
	for seed, s := range map[int]string{8300001: `"game_over":"buyout","last_day":84`, 8300002: `"game_over":"buyout","last_day":98`, 8300003: `"game_over":"rent","last_day":40`} {
		os.WriteFile(filepath.Join(sums, strconv.Itoa(seed)+".json"), []byte(`{"seed":`+strconv.Itoa(seed)+`,`+s+`}`), 0o644)
	}
	os.WriteFile(filepath.Join(sums, "sprinter.json"), []byte(`{"_route":{}}`), 0o644)
	record := filepath.Join(tmp, "record.jsonl")
	t.Chdir(filepath.Join("..", ".."))
	runCmd := func(args ...string) string {
		var out, errb bytes.Buffer
		if err := run(context.Background(), append(args[:1:1], append([]string{"-formats", formats, "-record", record}, args[1:]...)...), &out, &errb); err != nil {
			t.Fatalf("%v: %v %s", args, err, errb.String())
		}
		return out.String()
	}

	if out := runCmd("tally", "-update", "sprint", sums); !strings.Contains(out, "sprint: 1/3 won") {
		t.Errorf("tally: %s", out)
	}
	f, err := format.Load(formats, "sprint")
	if err != nil || f.Lab.Runs != 3 || f.Lab.Wins != 1 || f.Lab.Endings["late"] != 1 || f.Lab.Seeds != "8300001–003" || f.Lab.Route != "sprinter" {
		t.Fatalf("updated lab %+v %v", f.Lab, err)
	}

	var d map[string]any
	if err := json.Unmarshal([]byte(runCmd("directives", "sprint")), &d); err != nil || d["mortgage"] != "decline" || d["_format"] != "sprint" {
		t.Errorf("directives %v %v", d, err)
	}

	runCmd("record", "-run", "qwen27:r1", "-ending", "buyout", "-day", "98", "sprint")
	if out := runCmd("record", "-run", "qwen27:r1", "-ending", "buyout", "-day", "98", "sprint"); !strings.Contains(out, "already") {
		t.Errorf("record twice: %q", out)
	}
	if out := runCmd("card", "sprint"); !strings.Contains(out, "0 wins in 1 attempt (last: bought too late, day 98)") || !strings.Contains(out, "once in 3") {
		t.Errorf("card: %s", out)
	}
	if out := runCmd("list"); !strings.Contains(out, "sprint") || !strings.Contains(out, "1/3") {
		t.Errorf("list: %s", out)
	}
}
