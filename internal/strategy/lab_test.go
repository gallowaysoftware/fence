package strategy

import (
	"strings"
	"testing"
)

func TestLabRender(t *testing.T) {
	raw := `{"phase":"morning","store":{"day":20},"lab":{"step":"Mixed","product":"dream_dust","batches":2,"pills":33,"pill_value":120,"recovering":false,"alcohol_ml":2,"scanner":true,"expect":{"dream_dust":{"purity":99.2,"grade":"Very High","units_per_input":5.5,"value_per_input":140}},"scanned":{"ml":500,"liquids":{"dream_cap_extract":{"ul":480000,"pct":96.0},"water":{"ul":20000,"pct":4.0}},"toxicity":"toxic","euphoria":"strong"},"bottles":[{"id":"mini_bottle","scan":{"ml":250,"liquids":{"alcohol":{"ul":175000,"pct":70.0},"water":{"ul":75000,"pct":30.0}}}}]}}`
	s, err := parseSnapshot(jsonRaw(raw))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	s.production().renderLab(&b)
	out := b.String()
	for _, want := range []string{"CHEMISTRY", "dream_dust in progress", "analyser", "dream_cap_extract: 480000 µL (96.00%)", "toxic", "euphoria: strong", "mini_bottle", "alcohol: 175000 µL (70.00%)", "purity 99.2% (Very High)"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderLab output missing %q:\n%s", want, out)
		}
	}
	t.Log("\n" + out)
}

func jsonRaw(s string) []byte { return []byte(s) }
