package twitch

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gallowaysoftware/fence/internal/show"
)

// PollOption is one answer: the word viewers type after !vote, its label on the panel, and the
// words the strategist is told ("hold the stock for better prices").
type PollOption struct {
	Word, Label, Fact string
}

// PollSpec is a vote over a fixed list. Locks are the directive keys it is about: a format that
// locks one of them (internal/format) never gets the poll, since the plan could not follow it.
type PollSpec struct {
	ID       string
	Question string
	Options  []PollOption
	Locks    []string
	// Offer limits the poll to a shop with the mortgage on offer.
	Offer bool
}

// Polls is the morning-plan catalogue, one per day in turn, built from the directives levers
// (docs/PROTOCOL.md, "Directives"): what to do with stock, haggling, cash against stock, stolen
// goods, and the mortgage when it is on offer.
var Polls = []PollSpec{
	{ID: "stock", Question: "Tomorrow: hold the stock or dump it?", Locks: []string{"sell"}, Options: []PollOption{
		{"hold", "hold for better prices", "hold the stock for better prices"},
		{"dump", "dump it for cash", "dump stock for cash"},
	}},
	{ID: "haggle", Question: "Tomorrow at the counter: how hard to haggle?", Locks: []string{"bargain", "bargain_buy"}, Options: []PollOption{
		{"hard", "haggle every deal", "haggle hard on every deal"},
		{"smart", "only when it pays", "haggle only when it pays"},
		{"fair", "never haggle", "never haggle"},
	}},
	{ID: "cash", Question: "Tomorrow: sit on cash or buy stock?", Locks: []string{"cash_floor"}, Options: []PollOption{
		{"save", "keep a cash cushion", "keep a bigger cash cushion"},
		{"buy", "spend it on stock", "spend cash on stock"},
	}},
	{ID: "stolen", Question: "Tomorrow: touch stolen goods?", Locks: []string{"buy.hot_stock_cap"}, Options: []PollOption{
		{"clean", "nothing stolen", "buy nothing stolen"},
		{"risky", "stolen if cheap", "buy stolen goods when cheap"},
	}},
	{ID: "mortgage", Question: "The loan shark's offer: sign it?", Locks: []string{"mortgage", "loan"}, Offer: true, Options: []PollOption{
		{"sign", "sign the mortgage", "sign the mortgage"},
		{"wait", "not yet", "not sign the mortgage yet"},
	}},
}

// Poll statuses.
const (
	PollOpen   = "open"
	PollClosed = "closed"
)

// Poll is one vote: the first !vote of each viewer counts, a viewer's later ones do not, and a
// viewer sending commands faster than Cooldown is ignored until they slow down.
type Poll struct {
	Spec PollSpec
	// For is what the tally feeds ("day 34's plan"); ForDay the plan's day.
	For      string
	ForDay   int
	Opened   time.Time
	ClosesAt time.Time
	Status   string
	Cooldown time.Duration
	// Fork is the strategist's own choice at a fork poll, for the fact.
	Fork string

	votes map[string]int
	last  map[string]time.Time
	count []int
	Stats PollStats
	Fact  string
	// Answer is the strategist's "Chat:" line from the plan that read the tally.
	Answer string
	Note   string
}

// PollStats counts what chat sent: votes counted, repeats from a viewer who already voted,
// commands from a viewer inside the cooldown, words not on the list, and chat lines that were not
// a vote at all (which go nowhere).
type PollStats struct {
	Counted, Repeat, Throttled, Invalid, Ignored int
}

// NewPoll opens spec now until closes.
func NewPoll(spec PollSpec, forWhat string, forDay int, now, closes time.Time) *Poll {
	return &Poll{Spec: spec, For: forWhat, ForDay: forDay, Opened: now, ClosesAt: closes, Status: PollOpen,
		Cooldown: 2 * time.Second, votes: map[string]int{}, last: map[string]time.Time{}, count: make([]int, len(spec.Options))}
}

// Vote results.
const (
	VoteCounted   = "counted"
	VoteRepeat    = "repeat"
	VoteThrottled = "throttled"
	VoteInvalid   = "invalid"
	VoteIgnored   = "ignored"
	VoteClosed    = "closed"
)

// Message is one chat line. UserID is Twitch's stable id (a login can change).
type Message struct {
	UserID string
	Login  string
	Text   string
}

// Vote applies a chat line; only `!vote WORD` or `!vote N` (1-based) is a vote.
func (p *Poll) Vote(m Message, now time.Time) string {
	word, ok := voteWord(m.Text)
	if !ok {
		p.Stats.Ignored++
		return VoteIgnored
	}
	if p.Status != PollOpen {
		return VoteClosed
	}
	if t, seen := p.last[m.UserID]; seen && now.Sub(t) < p.Cooldown {
		p.last[m.UserID] = now
		p.Stats.Throttled++
		return VoteThrottled
	}
	p.last[m.UserID] = now
	i := p.option(word)
	if i < 0 {
		p.Stats.Invalid++
		return VoteInvalid
	}
	if _, voted := p.votes[m.UserID]; voted {
		p.Stats.Repeat++
		return VoteRepeat
	}
	p.votes[m.UserID] = i
	p.count[i]++
	p.Stats.Counted++
	return VoteCounted
}

func voteWord(text string) (string, bool) {
	f := strings.Fields(strings.ToLower(text))
	if len(f) < 2 || f[0] != "!vote" {
		return "", false
	}
	return strings.TrimFunc(f[1], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), true
}

func (p *Poll) option(word string) int {
	if n, err := strconv.Atoi(word); err == nil && n >= 1 && n <= len(p.Spec.Options) {
		return n - 1
	}
	return slices.IndexFunc(p.Spec.Options, func(o PollOption) bool { return o.Word == word })
}

// Counts are the votes per option, in order.
func (p *Poll) Counts() []int { return slices.Clone(p.count) }

// Total is the number of viewers who voted.
func (p *Poll) Total() int { return len(p.votes) }

// Close ends the vote and writes its fact when at least minVotes viewers voted.
func (p *Poll) Close(minVotes int) {
	if p.Status == PollClosed {
		return
	}
	p.Status = PollClosed
	if p.Total() < max(minVotes, 1) {
		p.Note = fmt.Sprintf("%d vote(s): too few to send", p.Total())
		return
	}
	p.Fact = p.fact()
}

// fact is the one line the strategist reads: only the fixed labels and the counts, never a word
// a viewer typed.
func (p *Poll) fact() string {
	order := make([]int, len(p.count))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return p.count[b] - p.count[a] })
	top := order[0]
	n := p.Total()
	pct := func(i int) int { return (200*p.count[i] + n) / (2 * n) }
	var b strings.Builder
	if len(order) > 1 && p.count[order[1]] == p.count[top] {
		fmt.Fprintf(&b, "Twitch chat split evenly on %q", p.Spec.Question)
	} else {
		fmt.Fprintf(&b, "Twitch chat voted %d %% to %s", pct(top), p.Spec.Options[top].Fact)
	}
	var rest []string
	for _, i := range order {
		rest = append(rest, fmt.Sprintf("%s %d", p.Spec.Options[i].Word, p.count[i]))
	}
	fmt.Fprintf(&b, " (%s; %d viewers, %s)", strings.Join(rest, ", "), n, p.For)
	if p.Fork != "" {
		fmt.Fprintf(&b, "; you chose %q", p.Fork)
	}
	return b.String() + "."
}

// View is the poll for the panel.
func (p *Poll) View() *show.AudiencePoll {
	a := &show.AudiencePoll{Question: p.Spec.Question, For: p.For, Status: p.Status, Total: p.Total(), Fact: p.Fact, Answer: p.Answer, Note: p.Note}
	if p.Status == PollOpen {
		a.ClosesAt = p.ClosesAt.UnixMilli()
	}
	for i, o := range p.Spec.Options {
		a.Options = append(a.Options, show.AudienceOption{Word: o.Word, Label: o.Label, Votes: p.count[i]})
	}
	return a
}

// ForkSpec turns a fork the strategist was asked (a `candidates` event with metric "choice",
// docs/STREAM.md "Model agency") into a poll over its options: the words are the options'
// numbers and first words, so `!vote 2` or `!vote sign` both work. It reports the option chosen.
func ForkSpec(question string, labels []string, chosen int) (PollSpec, string) {
	s := PollSpec{ID: "fork", Question: question}
	used := map[string]bool{}
	for i, l := range labels {
		w := strconv.Itoa(i + 1)
		if f := strings.FieldsFunc(strings.ToLower(l), func(r rune) bool { return !unicode.IsLetter(r) }); len(f) > 0 && !used[f[0]] {
			w = f[0]
		}
		used[w] = true
		s.Options = append(s.Options, PollOption{Word: w, Label: l, Fact: "choose " + strconv.Quote(l)})
	}
	if chosen >= 0 && chosen < len(labels) {
		return s, labels[chosen]
	}
	return s, ""
}
