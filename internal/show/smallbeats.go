package show

import (
	"fmt"
	"strings"
)

// Small beats: the speakable moments between the big ones. A live day runs 3–6 minutes and
// had one or two moments in the dress rehearsal (0.8 lines a minute); these give the voice
// the counter, the night out, the morning plan, the road to the deed and the appraisal
// board's run, each rate-limited so the voice is denser without narrating every customer.

const (
	// counterBeatsPerDay caps the counter stories a day; counterGap is how many customers
	// pass between two.
	counterBeatsPerDay = 2
	counterGap         = 3
)

// milestones are the shares of the buyout price (in %) that net worth is marked crossing.
var milestones = []struct {
	pct   int64
	words string
}{{25, "a quarter of the way"}, {50, "halfway"}, {75, "three quarters of the way"}}

type night struct {
	location  string
	scavenges int
}

type haggle struct{ who, option, verdict string }

// haggled records an answered bargain; the customer's trade that follows tells it.
func (b *Builder) haggled(who, option, verdict string) {
	b.lastHaggle = &haggle{who, option, verdict}
}

// counterBeat promotes a trade below a moment to a counter story when it is worth one: a
// haggle that ended in a deal, or a notable trade, at most counterBeatsPerDay a day and
// counterGap customers apart.
func (b *Builder) counterBeat(ev *Event) {
	t := ev.Trade
	h := b.lastHaggle
	b.lastHaggle = nil
	if h != nil && h.who != t.Customer {
		h = nil
	}
	if ev.Level >= LevelMoment || b.counterBeats >= counterBeatsPerDay || (b.counterBeats > 0 && b.today.Customers-b.counterAt < counterGap) {
		return
	}
	if ev.Level < LevelNotable && (h == nil || t.Cash < 50) {
		return
	}
	b.counterBeats++
	b.counterAt = b.today.Customers
	ev.Level, ev.Beat = LevelMoment, BeatCounter
	if h != nil {
		ev.Text = strings.TrimSuffix(ev.Text, ".") + fmt.Sprintf(", after a haggle (%s): %s.", h.option, h.verdict)
	}
}

// goOut starts the night's outing; scavenge counts its searches.
func (b *Builder) goOut(location string) { b.night = &night{location: location} }

func (b *Builder) scavenge() {
	if b.night != nil {
		b.night.scavenges++
	}
}

// nightBeat tells what the night out brought home: the items in the sleep snapshot that the
// close snapshot did not have.
func (b *Builder) nightBeat(inv []itemView) []Event {
	n := b.night
	b.night = nil
	if n == nil || b.closeItems == nil {
		return nil
	}
	var names []string
	var est int64
	for _, it := range inv {
		if it.UID == 0 || b.closeItems[it.UID] || strings.HasPrefix(it.Where, "inside:") {
			continue
		}
		name := it.Name
		if name == "" {
			name = "item"
		}
		names = append(names, name)
		est += it.Estimate
	}
	place := strings.ReplaceAll(n.location, "_", " ")
	if n.location == "dr_jackson" {
		place = "Dr. Jackson's"
	}
	text := "Night at " + place
	if n.location == "dumping_ground" {
		text = "Night at the dumping ground"
	}
	if n.scavenges > 0 {
		text += ", " + plural(n.scavenges, "scavenge")
	}
	switch {
	case len(names) == 0:
		text += ": back with nothing."
	case est > 0:
		text += fmt.Sprintf(": back with %s (estimate %d).", summarizeItems(groupNames(names), 3), est)
	default:
		text += fmt.Sprintf(": back with %s.", summarizeItems(groupNames(names), 3))
	}
	return []Event{{Kind: KindThought, Level: LevelMoment, Beat: BeatNight, Clock: b.Clock(), Text: text}}
}

// milestoneBeat marks net worth crossing a quarter, half and three quarters of the buyout
// price, each once a run.
func (b *Builder) milestoneBeat() []Event {
	c := b.clock
	if c.Owned || c.BuyoutPrice <= 0 || c.NetWorth <= 0 {
		return nil
	}
	var out []Event
	for _, m := range milestones {
		if b.milestone >= m.pct || c.NetWorth*100 < c.BuyoutPrice*m.pct {
			continue
		}
		b.milestone = m.pct
		out = []Event{{Kind: KindThought, Level: LevelMoment, Beat: BeatMilestone, Clock: b.Clock(),
			Text: fmt.Sprintf("Net worth %s: %s to the %s buyout (%d%%).", num(c.NetWorth), m.words, num(c.BuyoutPrice), m.pct)}}
	}
	return out
}

// appraisalBeat counts accepted appraisals in a row and marks the third and every fifth.
func (b *Builder) appraisalBeat(fee *int64) []Event {
	if fee == nil {
		b.streak, b.streakFees = 0, 0
		return nil
	}
	b.streak++
	b.streakFees += *fee
	if b.streak != 3 && b.streak%5 != 0 {
		return nil
	}
	return []Event{{Kind: KindThought, Level: LevelMoment, Beat: BeatAppraisalStreak, Clock: b.Clock(),
		Text: fmt.Sprintf("Appraisal streak: %d in a row accepted, %s in fees over the streak.", b.streak, num(b.streakFees))}}
}
