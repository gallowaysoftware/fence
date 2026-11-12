package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReport(t *testing.T) {
	block := t.TempDir()
	run := func(arm string, seed int, end string, days int, fees, sales, bought float64) {
		write(t, filepath.Join(block, arm, "s"+itoa(seed)+".json"), `{"seed":`+itoa(seed)+`,"game_over":"`+end+`","days_played":`+itoa(days)+
			`,"player":{"appraisal":{"fees_total":`+ftoa(fees)+`}},"player_days":[{"cash_flow":{"trade_sales":`+ftoa(sales)+`,"trade_purchases":`+ftoa(bought)+`}}]}`)
	}
	for s, d := range map[int]int{1: 98, 2: 91, 3: 105} {
		run("appraiser", s, "buyout", d, 6000, 3000, 1000)
	}
	run("hoard", 1, "buyout", 105, 6000, 2000, 1000)
	run("hoard", 2, "buyout", 101, 6000, 2000, 1000)
	run("hoard", 3, "rent", 40, 1000, 500, 100)
	trace := `{"k":"seller","day":3,"wholesale":true,"bought":[{"id":"bandage","t":["MEDICAL"],"paid":80}]}
{"k":"buyer","day":4,"faction":"UPPER_LEVELS","items":[{"id":"bandage","t":["MEDICAL"],"got":120},{"id":"wine_bottle","t":["FOOD"],"got":50}]}
{"k":"day","day":4,"units":10,"events":["MEDICAL:50"]}
{"k":"day","day":7,"units":20,"events":[]}
`
	write(t, filepath.Join(block, "hoard", "trace", "s1.jsonl"), trace)
	var out bytes.Buffer
	arms := []arm{{name: "appraiser"}, {name: "hoard"}}
	if err := report(block, arms, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out.String(), "\n")
	if !strings.Contains(lines[2], "hoard") || !strings.Contains(lines[2], "2/3") || !strings.Contains(lines[2], "+8.5 ±") {
		t.Errorf("report:\n%s", out.String())
	}
	f, n := readFingerprint(filepath.Join(block, "hoard"))
	if n != 1 || f.SalesWeek != 120 || f.BoughtWeek != 80 || f.WholesalePct != 100 || f.EventPct != 100 || f.ULPct != 100 || f.Stock != 15 || f.Mix["MEDICAL"] != 1 {
		t.Errorf("fingerprint %+v", f)
	}
	if d := tradeDistance(f, f); d != 0 {
		t.Errorf("distance to itself %v", d)
	}
	shares, _ := income(loadRuns(filepath.Join(block, "appraiser")))
	if math.Abs(shares[0]-75) > 0.01 || math.Abs(shares[1]-25) > 0.01 {
		t.Errorf("income %v", shares)
	}
}

func itoa(i int) string     { return strconv.Itoa(i) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
