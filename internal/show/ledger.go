package show

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// The characters the show names on its own: the money events say who they are with.
const (
	georgio   = "Georgio"
	landlord  = "The Landlord"
	loanShark = "Loan Shark"
	jackson   = "Dr. Jackson"
)

// titles start a named character's name ("Dr. Jackson", "Officer Lun").
var titles = []string{"dr.", "dr", "doctor", "officer", "cadet", "captain", "sergeant", "mr.", "mrs.", "ms.", "miss", "uncle", "aunt"}

// isCharacter tells a named recurring character from a passer-by of a type ("Spacer", "Lower
// Level Citizen"), from the name alone: a title and a name, "The" and one word, someone's
// relative ("Eleanor's Uncle"), or one word that has held a dialogue with the shop. Two-word
// personal names with no dialogue ("John Smith") read like types and are missed.
func isCharacter(name string, speakers map[string]bool) bool {
	ws := strings.Fields(name)
	if len(ws) == 0 || !unicode.IsUpper([]rune(ws[0])[0]) {
		return false
	}
	switch {
	case len(ws) >= 2 && slices.Contains(titles, strings.ToLower(ws[0])):
		return true
	case len(ws) == 2 && ws[0] == "The":
		return true
	case strings.Contains(ws[0], "'s") && len(ws) == 2:
		return true
	case len(ws) == 1 && speakers[name]:
		return true
	}
	return false
}

func (b *Builder) character(name string) string {
	if isCharacter(name, b.speakers) {
		return name
	}
	return ""
}

func with(evs []Event, who string) []Event {
	for i := range evs {
		evs[i].Who = who
	}
	return evs
}

// moneyBets settles a payment bet on a rent or mortgage payment.
func (b *Builder) moneyBets(e *logEvent) []Event {
	var d struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return nil
	}
	return betEvents(b.bets.Money(d.Kind, max(e.GameDay, b.day)), b.Clock())
}

const (
	ledgerPeople = 12
	ledgerNotes  = 3
)

// Ledger remembers the named characters the run has dealt with, a few lines of history each,
// so the narrator can call back to them when they turn up again. Every line comes from a feed
// event and carries no number: the fact check only knows the moment's numbers.
type Ledger struct {
	people []*person
}

type person struct {
	name  string
	notes []ledgerNote
}

type ledgerNote struct {
	seq  int64
	text string
}

// Note records ev's dealing with its character, if it has one.
func (l *Ledger) Note(ev Event) {
	if ev.Who == "" {
		return
	}
	text := ledgerLine(ev)
	if text == "" {
		return
	}
	i := slices.IndexFunc(l.people, func(p *person) bool { return p.name == ev.Who })
	var p *person
	if i >= 0 {
		p = l.people[i]
		l.people = slices.Delete(l.people, i, i+1)
	} else {
		p = &person{name: ev.Who}
	}
	l.people = append([]*person{p}, l.people...)
	if len(l.people) > ledgerPeople {
		l.people = l.people[:ledgerPeople]
	}
	if k := len(p.notes); k > 0 && p.notes[k-1].text == text {
		return
	}
	p.notes = append(p.notes, ledgerNote{seq: ev.Seq, text: text})
	if len(p.notes) > ledgerNotes {
		p.notes = p.notes[1:]
	}
}

// Background is who's history before event seq, oldest first; "" when there is none.
func (l *Ledger) Background(who string, before int64) string {
	if who == "" {
		return ""
	}
	i := slices.IndexFunc(l.people, func(p *person) bool { return p.name == who })
	if i < 0 {
		return ""
	}
	var lines []string
	for _, n := range l.people[i].notes {
		if n.seq < before {
			lines = append(lines, n.text)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("%s, before today's moment: %s (most recent last)", who, strings.Join(lines, "; then "))
}

// ledgerLine is one dealing in words, from the character's side.
func ledgerLine(ev Event) string {
	switch {
	case ev.Kind == KindTrade && ev.Trade != nil:
		what := ledgerItems(ev.Trade.Items)
		switch {
		case ev.Trade.Side == "sold" && ev.Trade.Fakes > 0:
			return "bought a counterfeit from us"
		case ev.Trade.Side == "sold" && what != "":
			return "bought " + what + " from us"
		case ev.Trade.Side == "bought" && what != "":
			return "sold us " + what
		}
	case ev.Beat == BeatNight:
		if _, got, ok := strings.Cut(ev.Text, "back with "); ok {
			got = strings.TrimSuffix(strings.TrimSpace(strings.Split(got, " (")[0]), ".")
			if got == "nothing" || strings.ContainsAny(got, "0123456789") {
				return "a night at his place brought nothing home"
			}
			return "a night at his place brought home " + got
		}
	case strings.HasPrefix(ev.Text, "Haggled with"):
		switch {
		case strings.HasSuffix(ev.Text, "walked out."):
			return "walked out on our haggle"
		case strings.HasSuffix(ev.Text, "accepted."):
			return "gave way to our haggle"
		case strings.HasSuffix(ev.Text, "refused."):
			return "refused our haggle"
		}
	case ev.Who == georgio && ev.Amount < 0:
		return "borrowed money from us"
	case ev.Who == georgio && ev.Amount > 0:
		return "paid us back"
	case ev.Who == landlord && ev.Amount < 0:
		return "took our rent"
	case ev.Who == landlord:
		return "went without his rent"
	case ev.Who == loanShark:
		return "signed the mortgage with us"
	}
	return ""
}

// ledgerItems names up to two of a trade's item kinds, without counts or numbers.
func ledgerItems(items []string) string {
	var out []string
	for _, it := range items {
		if _, name, ok := strings.Cut(it, "× "); ok {
			it = name
		}
		if it == "" || it == "item" || strings.ContainsAny(it, "0123456789") {
			continue
		}
		out = append(out, it)
		if len(out) == 2 {
			break
		}
	}
	return joinAnd(out)
}
