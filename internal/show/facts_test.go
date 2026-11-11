package show

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// The moments behind the lines of the first demo (var/show/demo, day 41–43 of
// t5-hard-drift/drift-delta__apprentice__s970001), cut to what the check reads.
var (
	demoAug = Event{Day: 41, Kind: KindAug, Amount: 129, Text: "Shot an aug posing as an Upper Level Hospital Supplier. Bounty 129.",
		Clock: &Clock{Cash: 1528, NetWorth: 4226, RentDue: 321, RentDays: 1, BuyoutPrice: 24000}}
	demoSale = Event{Day: 41, Kind: KindTrade, Text: "Sold 7× BloomBerry and Pack of Hemingway Cigarettes (Counterfeit) to Tourist for 169. A counterfeit went out the door.",
		Trade: &Trade{Side: "sell", Count: 8, Cash: 169, Fakes: 1}, Clock: &Clock{Cash: 1697, NetWorth: 4226, RentDue: 321, RentDays: 1, BuyoutPrice: 24000}}
	demoPlan = Event{Day: 42, Kind: KindPlan, Text: "Day 42 plan: Stock up on wine and water, don't buy weapon and ammunition, no haggling, buy the Buyer Referral Service, bribe instead of a permit and shoot augs on sight.",
		Plan: &Plan{Day: 42, Source: "directives"}, Clock: &Clock{Cash: 1664, NetWorth: 4685, RentDue: 321, RentDays: 0, BuyoutPrice: 24000}}
	demoDay42 = Event{Day: 42, Kind: KindDay, Text: "Day 42 closes: cash 1408 (-256), net worth 4371 (-314). Rent 353 due in 7 days.",
		Summary: &DaySummary{NetWorthDelta: -314, CashDelta: -256, Sold: 416, SoldItems: 9, Bought: 351, BoughtItems: 14, Customers: 8},
		Clock:   &Clock{Cash: 1408, NetWorth: 4371, RentDue: 353, RentDays: 7, BuyoutPrice: 24000}}
	demoLoan = Event{Day: 43, Kind: KindMoney, Amount: -730, Text: "Lent 730 to Georgio.",
		Clock: &Clock{Cash: 2190, NetWorth: 4373, RentDue: 353, RentDays: 6, BuyoutPrice: 24000}}
	demoDay43 = Event{Day: 43, Kind: KindDay, Text: "Day 43 closes: cash 581 (-794), net worth 3726 (-671). Rent 353 due in 6 days.",
		Summary: &DaySummary{NetWorthDelta: -671, CashDelta: -794, Sold: 85, SoldItems: 1, Bought: 152, BoughtItems: 7, Customers: 6},
		Clock:   &Clock{Cash: 581, NetWorth: 3726, RentDue: 353, RentDays: 6, BuyoutPrice: 24000}}
)

func TestCheckLineDemo(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
		line string
		// want are substrings of the faults, in order; none: the line passes.
		want []string
	}{
		{"short with 1528 against 321", demoAug, "Shot a fake supplier for a hundred and thirty bounty. Rent is due tomorrow and I am short on cash.",
			[]string{`says "i am short" but cash 1,528 covers the rent 321`}},
		{"short two hundred with 1408 against 353", demoDay42, "Down three hundred net. Rent's due in a week, and I'm short two hundred.",
			[]string{`says "i'm short"`, "names two hundred"}},
		{"short for rent with 581 against 353", demoDay43, "Down nearly seven hundred today. I have five hundred eighty one, so I am short for rent.",
			[]string{`says "i am short" but cash 581 covers the rent 353 with 228 to spare`}},
		{"the sale, rounded", demoSale, "I sold those counterfeit smokes for a round 170. Hope the tourist's liver is tougher than his wallet.", nil},
		{"the loan, rounded and covering", demoLoan, "I lent Georgio seven hundred. Rent is still covered with a little left over.", nil},
		{"due today on the day", demoPlan, "Rent is due today, so I’m skipping the haggling. I’ll buy that referral service and hope the bribes cover the rest.", nil},
		{"colour only", demoPlan, "Cash is too thin for guns, so I’ll bribe instead. If I see an aug, I shoot first and check the paperwork later.", nil},
		{"margin is a fact", demoDay43, "Bad day, but cash still covers the rent with about 230 to spare.", nil},
		{"wrong sale price", demoSale, "Sold the smokes for 300 to a tourist.", []string{"says sold for 300 but the sale was 169"}},
		{"sold on a buy", Event{Day: 5, Trade: &Trade{Side: "buy", Cash: 80}, Text: "Bought a lamp for 80."}, "I sold the lamp for 80.", []string{"says sold, but this was a buy"}},
		{"today is the day, not the rent", demoDay43, "Down nearly seven hundred today, but the rent is still covered.", nil},
		{"up on a down day", demoDay43, "Up seven hundred today, a fine day.", []string{"says up seven hundred"}},
		{"due tomorrow when it is six days", demoDay43, "Rent is due tomorrow and I can smell it.", []string{"due tomorrow but it is due in 6 days"}},
		{"wrong days", demoDay43, "Rent in 3 days.", []string{"names 3"}},
		{"covered when short", Event{Day: 9, Clock: &Clock{Cash: 100, RentDue: 300, RentDays: 1}, Text: "Day 9."}, "The rent is covered, relax.", []string{"says the rent is covered but cash 100 is 200 short of 300"}},
		{"short when short", Event{Day: 9, Clock: &Clock{Cash: 100, RentDue: 300, RentDays: 1}, Text: "Day 9."}, "I'm short by two hundred and the landlord knows it.", nil},
		{"short of the buyout is not the rent", demoDay43, "I'm short of the buyout by a mile.", nil},
		{"owned shop", Event{Day: 90, Clock: &Clock{Cash: 100, Owned: true}, Text: "Day 90."}, "I am short on rent again.", []string{"the shop is owned"}},
		{"stale cash estimate", Event{Day: 69, Kind: KindMoney, Amount: 7004, Text: "Georgio paid back 7004.", Clock: &Clock{Cash: -840, MortgageWeekly: 1666, RentDays: 1}},
			"Seven thousand gone, and I am short for tomorrow.", nil},
		// Said in the full replay demo (var/show/demo-full, day 14): the weekday was invented.
		{"weekday", demoDay42, "The portfolio slips by three hundred. A quiet Tuesday.", []string{"names a weekday (tuesday)"}},
		{"mortgage payment covered", Event{Day: 77, Clock: &Clock{Cash: 5000, RentDue: 468, MortgageWeekly: 1666, RentDays: 3}, Text: "Day 77."},
			"The next payment is covered, with 3,334 to spare.", nil},
		// Said in the dress rehearsal (day 10) for "free to spend = 255", and passed the check.
		{"numbers run together", demoDay43, "Twenty-five five credits to play with.", []string{"runs two numbers together", "names twenty-five"}},
		{"one number in words", demoDay43, "Five hundred eighty one in the till.", nil},
		// Said in the dress rehearsal (day 29) and rejected twice: the words across the full stop
		// parsed as one number, and the price was not a purchase fact.
		{"price paid partly in favor", Event{Day: 29, Kind: KindPurchase, Amount: -633, Text: "Bought Store Renovation I: 750, 117 of it in Wild's favor."},
			"Store Renovation bought for seven fifty. One hundred seventeen in Wild's favor.", nil},
		{"no rent figure while mortgaged", Event{Day: 77, Clock: &Clock{Cash: 5000, RentDue: 468, MortgageWeekly: 1666, RentDays: 3}, Text: "Day 77."},
			"Rent of 468 is nothing to me now.", []string{"names 468"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckLine(tt.line, FactsOf(tt.ev))
			if len(got) != len(tt.want) {
				t.Fatalf("faults = %q, want %d like %q", got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(string(got[i]), w) {
					t.Errorf("fault %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestNumbers(t *testing.T) {
	cases := map[string][]int64{
		"a hundred and thirty bounty":               {130},
		"five hundred eighty one":                   {581},
		"three fifty due":                           {350},
		"two thousand three hundred and twelve":     {2312},
		"nineteen hundred, then 1,304 and 1.2k":     {1900, 1304, 1200},
		"one lamp, twenty one days, seven hundred":  {1, 21, 700},
		"a lamp and an aug":                         nil,
		"seven fifty. one hundred seventeen":        {750, 117},
		"three thousand, two hundred and forty-two": {3242},
		"seventy, then eighty":                      {70, 80},
	}
	for in, want := range cases {
		var got []int64
		for _, n := range Numbers(in) {
			got = append(got, n.Value)
		}
		if len(got) != len(want) {
			t.Errorf("Numbers(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("Numbers(%q) = %v, want %v", in, got, want)
			}
		}
	}
}

// scriptLLM answers each call with the next reply.
type scriptLLM struct {
	mu      sync.Mutex
	replies []string
	calls   [][]llm.Message
}

func (s *scriptLLM) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Messages)
	reply := s.replies[min(len(s.calls), len(s.replies))-1]
	return llm.Response{Text: reply}, nil
}

func TestNarratorChecksFacts(t *testing.T) {
	bad := "Down nearly seven hundred today. I have five hundred eighty one, so I am short for rent."
	good := "Down nearly seven hundred today, but the rent is still covered."

	s := &scriptLLM{replies: []string{bad, good}}
	sp := (&Narrator{LLM: s}).withDefaults().line(context.Background(), demoDay43)
	if sp.line != good || sp.source != "llm" || sp.check != "retry" || len(sp.faults) != 1 {
		t.Errorf("retry = %+v", sp)
	}
	if len(s.calls) != 2 || len(s.calls[1]) != 3 || !strings.Contains(s.calls[1][2].Content, "228 to spare") {
		t.Errorf("the retry did not name the fault: %+v", s.calls)
	}

	sp = (&Narrator{LLM: &scriptLLM{replies: []string{bad}}}).withDefaults().line(context.Background(), demoDay43)
	if sp.source != "caption" || sp.check != "fallback" || sp.line != demoDay43.Text {
		t.Errorf("fallback = %+v", sp)
	}

	sp = (&Narrator{LLM: &scriptLLM{replies: []string{good}}}).withDefaults().line(context.Background(), demoDay43)
	if sp.check != "pass" || sp.faults != nil {
		t.Errorf("pass = %+v", sp)
	}
}

func (n *Narrator) withDefaults() *Narrator {
	n.defaults()
	return n
}

func TestFactsOfCandidates(t *testing.T) {
	ev := Event{Day: 43, Kind: KindCandidates, Text: "Day 43 (georgio): considered keep the plan / lend 50; 4 simulated futures say lend 50.",
		Candidates: &Candidates{Day: 43, Options: []Candidate{
			{Label: "keep the plan", Score: 31.4, Rollouts: 4, Note: "net worth 7,410 after 28 days"},
			{Label: "lend 50", Score: 36.2, Rollouts: 4, Chosen: true, Note: "Georgio repays 4x; net worth 8,120 after 28 days"},
		}}}
	f := FactsOf(ev)
	for _, line := range []string{"Four futures agreed: lending half beats sitting tight.", "The sims put me at 8,120 a month out.", "Thirty-six points against thirty-one."} {
		if faults := CheckLine(line, f); len(faults) > 0 {
			t.Errorf("%q: %v", line, faults)
		}
	}
	if faults := CheckLine("The sims put me at 9,900.", f); len(faults) == 0 {
		t.Error("a number the slate never gave passed")
	}
}

// "Eleven niner bounty" for 119 reached the air in the variety replay: eleven is too small to check.
func TestCheckLineNiner(t *testing.T) {
	ev := Event{Kind: KindAug, Amount: 119, Text: "Shot an aug. Bounty 119."}
	if fs := CheckLine("Eleven niner bounty for a fake inventor.", FactsOf(ev)); len(fs) == 0 {
		t.Error("eleven niner passed for 119")
	}
	if fs := CheckLine("A 119 bounty for a fake inventor.", FactsOf(ev)); len(fs) != 0 {
		t.Errorf("119 failed: %v", fs)
	}
}

func TestCheckLineNightCount(t *testing.T) {
	ev := Event{Kind: KindThought, Beat: BeatNight, Text: "Night at the dumping ground, 5 scavenges: back with Junk (estimate 40)."}
	if fs := CheckLine("Three scavenges turned up nothing but rust.", FactsOf(ev)); len(fs) == 0 {
		t.Error("three scavenges passed for five")
	}
	if fs := CheckLine("Five trips into the trash, one bit of junk.", FactsOf(ev)); len(fs) != 0 {
		t.Errorf("five trips failed: %v", fs)
	}
}
