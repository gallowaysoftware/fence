package strategy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func barterReq(decision string, day int, theirs ...string) *protocol.DecideRequest {
	return &protocol.DecideRequest{SessionID: "s1", DecisionID: decision, Kind: "barter", GameDay: day,
		Context: json.RawMessage(fmt.Sprintf(`{"store":{"cash":900},"counter":{"theirs":[%s],"yours_staged":[]}}`, strings.Join(theirs, ",")))}
}

func sellerItem(uid int, id, name string, price int) string {
	return fmt.Sprintf(`{"uid":%d,"id":%q,"name":%q,"price":%d,"estimated_value":%d}`, uid, id, name, price, price)
}

func TestWindfallReplan(t *testing.T) {
	blast := sellerItem(70, "furnace_module_blast", "Blast Module", 150)
	bay := sellerItem(71, "smuggler_bay", "Smuggler Bay", 250)
	rack := sellerItem(72, "wine_rack", "Wine Rack", 350)
	watch := sellerItem(73, "watch", "Watch", 300)
	bought := func(decision string, uid int) protocol.Outcome {
		return protocol.Outcome{SessionID: "s1", DecisionID: decision, GameDay: 10,
			Result: json.RawMessage(fmt.Sprintf(`{"kind":"trade","side":"bought","items":[%d],"cash_delta":-150}`, uid))}
	}
	tests := []struct {
		name     string
		noPlan   bool
		reqs     []*protocol.DecideRequest
		outcomes []protocol.Outcome
		want     int
		wantIn   []string
	}{
		{name: "seller offers an engine item", reqs: []*protocol.DecideRequest{barterReq("d1", 10, blast)}, want: 1,
			wantIn: []string{"Blast Module #70 is on a seller's counter at 150", "use ≈ 32 a night (448 over 14 nights) vs sale 150", "shop cash 900", "Windfall: grab furnaces"}},
		{name: "ordinary item is no windfall", reqs: []*protocol.DecideRequest{barterReq("d1", 10, watch)}, want: 0},
		{name: "same offer twice re-plans once", reqs: []*protocol.DecideRequest{barterReq("d1", 10, blast), barterReq("d2", 10, blast)}, want: 1},
		{name: "offered then bought re-plans for each", reqs: []*protocol.DecideRequest{barterReq("d1", 10, blast)},
			outcomes: []protocol.Outcome{bought("d1", 70)}, want: 2, wantIn: []string{"Blast Module #70 has just been acquired by the shop"}},
		{name: "a purchase of an ordinary item is no windfall", reqs: []*protocol.DecideRequest{barterReq("d1", 10, watch)},
			outcomes: []protocol.Outcome{bought("d1", 73)}, want: 0},
		{name: "at most two a day", reqs: []*protocol.DecideRequest{barterReq("d1", 10, blast, bay, rack)}, want: 2},
		{name: "no plan to amend", noPlan: true, reqs: []*protocol.DecideRequest{barterReq("d1", 10, blast)}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLLM{}
			s := New(Options{LLM: f, Persona: "Path: metal.\nWindfall: grab furnaces and blast modules. Stay on path unless a windfall clearly beats it."})
			defer s.Close()
			if !tt.noPlan {
				s.Event("s1", snapshotEvent("morning", 10, 900))
				s.wg.Wait()
			}
			for _, r := range tt.reqs {
				s.Decision("s1", r, protocol.DecideResponse{})
			}
			for _, o := range tt.outcomes {
				s.Outcome("s1", o)
			}
			s.wg.Wait()
			var prompts []string
			for _, r := range f.requests() {
				if r.System == windfallSystem {
					prompts = append(prompts, r.Messages[0].Content)
				}
			}
			if len(prompts) != tt.want {
				t.Fatalf("%d windfall re-plans, want %d", len(prompts), tt.want)
			}
			all := strings.Join(prompts, "\n")
			for _, w := range tt.wantIn {
				if !strings.Contains(all, w) {
					t.Errorf("windfall prompts missing %q:\n%s", w, all)
				}
			}
			plan, _, _ := s.CurrentPlan("s1")
			if got := strings.Count(plan, "\nWINDFALL: blast module -> pivot? yes"); got != tt.want {
				t.Fatalf("plan carries %d WINDFALL deltas, want %d:\n%s", got, tt.want, plan)
			}
			if tt.want > 0 {
				if !strings.Contains(prompts[0], "TODAY'S PLAN:\nCALENDAR") {
					t.Errorf("windfall prompt lacks the plan:\n%s", prompts[0])
				}
				if d, _ := s.Day("s1", 10); !strings.Contains(d.Plan, "WINDFALL:") {
					t.Errorf("the day's record lacks the delta, so tonight's review cannot score it")
				}
			}
		})
	}
}

func TestTidyWindfall(t *testing.T) {
	tests := []struct{ in, want string }{
		{"**WINDFALL:** x -> pivot? no, path first\n* keep cash\nno other change\nextra", "WINDFALL: x -> pivot? no, path first\n- keep cash\n- no other change"},
		{"Sure.\nWINDFALL: y -> pivot? yes, z", "WINDFALL: y -> pivot? yes, z"},
		{"no windfall line at all", ""},
		{"WINDFALL: " + strings.Repeat("long ", 100), ""},
	}
	for _, tt := range tests {
		got := TidyWindfall(tt.in)
		if tt.want == "" && strings.HasPrefix(tt.in, "WINDFALL") {
			if len(got) > maxWindfallBytes || !strings.HasPrefix(got, "WINDFALL:") {
				t.Errorf("TidyWindfall(long) = %d bytes %q", len(got), got)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("TidyWindfall(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
