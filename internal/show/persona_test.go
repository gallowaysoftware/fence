package show

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonaPrompt(t *testing.T) {
	file := "# Miser\nPath: hoard.\n\n## Notes (not in any prompt)\nSources: zz_notes\n## Samples\nzz_sample\n"
	if got := PersonaPrompt(file); got != "# Miser\nPath: hoard.\n\n" {
		t.Errorf("PersonaPrompt = %q", got)
	}
	if got := PersonaPrompt("# Miser\nPath: hoard.\n"); got != "# Miser\nPath: hoard.\n" {
		t.Errorf("without notes: %q", got)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "miser.md"), []byte(file), 0o644)
	p := LoadPersona(dir, "miser")
	if p.Name != "Miser" || p.Tagline != "Hoard" || strings.Contains(p.Text, "zz_") {
		t.Errorf("miser = %+v", p)
	}
}

// Every shipped persona sounds like itself: a voice of its own, a style, the co-host's angle and
// an image bank, and its notes stay out of what the narrator reads.
func TestPersonaFilesHaveAVoice(t *testing.T) {
	paths, err := filepath.Glob("../../docs/personas/*.md")
	if err != nil || len(paths) < 13 {
		t.Fatalf("%d personas (%v)", len(paths), err)
	}
	voices := map[string]string{}
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		v, ok := personaVoices[id]
		if !ok || v.style == "" || v.needle == "" || len(imageBanks[id]) != 3 {
			t.Errorf("%s: voice %+v, %d image groups", id, v, len(imageBanks[id]))
		}
		if other, dup := voices[v.voice]; dup {
			t.Errorf("%s and %s share the voice %s", id, other, v.voice)
		}
		voices[v.voice] = id
		if v.voice == "bf_alice" || v.voice == "bm_daniel" {
			t.Errorf("%s speaks in the co-host's voice %s", id, v.voice)
		}
		if strings.ContainsAny(v.needle, "0123456789") {
			t.Errorf("%s: a needle with a number fails the co-host's fact check: %q", id, v.needle)
		}
		if p := LoadPersona("../../docs/personas", id); strings.Contains(p.Text, PersonaNotes) || p.Tagline == "" {
			t.Errorf("%s: tagline %q, text with notes %v", id, p.Tagline, strings.Contains(p.Text, PersonaNotes))
		}
	}
}

func TestCohostAngle(t *testing.T) {
	f := &fakeLLM{reply: "A fish stall in a pawn shop."}
	n := &Narrator{PersonaD: "../../docs/personas", Persona: "fishmonger", Cohost: &Cohost{LLM: f}}
	n.defaults()
	n.Cohost.defaults()
	ev := Event{Day: 0, Beat: BeatRouteChoice, Text: "Day 0: the strategist picks the fish route."}
	if line, _ := n.cohostLine(context.Background(), ev, FactsOf(ev), "Fresh fillets every morning."); line == "" {
		t.Fatal("no co-host line")
	}
	if sys := f.lastSystem(); !strings.Contains(sys, "Your angle on this player: "+personaVoices["fishmonger"].needle) {
		t.Errorf("system lacks the angle: %q", sys)
	}
}
