package expert

import (
	"encoding/json"
	"testing"
)

func TestParseExtraction(t *testing.T) {
	v := Video{ID: "abc", Creator: "X"}
	out := `<think>x</think>{"decisions":[
 {"at":"12:30","day":"14","category":"Upgrade","situation":"cash 900","decision":"bought the winemaker upgrade","reason":"wine is immune to events"},
 {"at":"13:00","day":3,"category":"nonsense","situation":"","decision":"skipped the referral"},
 {"decision":"no"}],
 "facts":{"start":"Apprentice","bought":[{"item":"Winemaker","day":14}]}}`
	e, dropped, err := ParseExtraction(out, v, "10:00")
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 || len(e.Decisions) != 2 {
		t.Fatalf("dropped %d, kept %d", dropped, len(e.Decisions))
	}
	if d := e.Decisions[0]; d.Day != 14 || d.Category != "upgrade" || d.VideoID != "abc" {
		t.Errorf("first decision %+v", d)
	}
	if e.Decisions[1].Category != "other" {
		t.Errorf("unknown category kept: %q", e.Decisions[1].Category)
	}
	if e.Facts.Start != "Apprentice" || len(e.Facts.Bought) != 1 {
		t.Errorf("facts %+v", e.Facts)
	}
}

func TestRunFacts(t *testing.T) {
	f := RunFacts([]Facts{
		{Start: "Fence", Bought: []Bought{{Item: "Showcase I"}, {Item: "Winemaker", Day: 9}}, LastDay: 10, FirstDay: 1},
		{Difficulty: "hard", Bought: []Bought{{Item: "winemaker", Day: 12}, {Item: "Grace", Day: 7}}, LastDay: 20, FirstDay: 10, Ending: "buyout day 20"},
	})
	if f.Start != "Fence" || f.Difficulty != "hard" || f.FirstDay != 1 || f.LastDay != 20 || f.Ending != "buyout day 20" {
		t.Fatalf("scalars %+v", f)
	}
	want := []string{"Grace", "Winemaker", "Showcase I"}
	if len(f.Bought) != len(want) {
		t.Fatalf("bought %+v", f.Bought)
	}
	for i, w := range want {
		if f.Bought[i].Item != w {
			t.Errorf("bought[%d] = %q, want %q", i, f.Bought[i].Item, w)
		}
	}
}

func TestSeconds(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{{"1:05", 65, true}, {"[1:02:03]", 3723, true}, {"x", 0, false}} {
		got, ok := Seconds(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Seconds(%q) = %d,%v", tc.in, got, ok)
		}
	}
	if s := FirstStamp("header\n[2:10] hello\n[2:40] x"); s != "2:10" {
		t.Errorf("FirstStamp = %q", s)
	}
}

func TestNum(t *testing.T) {
	var fr FrameRead
	if err := json.Unmarshal([]byte(`{"day":3,"cash":"617$","rent_due":"1,200","offer":null,"reps":{"LL":"-12"}}`), &fr); err != nil {
		t.Fatal(err)
	}
	if *fr.Day.V != 3 || *fr.Cash.V != 617 || *fr.RentDue.V != 1200 || fr.Offer.V != nil || *fr.Reps["LL"].V != -12 {
		t.Errorf("%+v", fr)
	}
}
