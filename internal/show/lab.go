package show

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const cartelHandler = "Cartel Handler"

var (
	switchedRe  = regexp.MustCompile(`switched (\d+) time`)
	dustCountRe = regexp.MustCompile(`Do you have the (\d+) Dream Dust`)
	cookRe      = regexp.MustCompile(`cook at least (\d+) Dream Dust`)
	pureRe      = regexp.MustCompile(`at least (\d+)% pure`)
)

// labRun is one run of machine presses between the autopilot's other actions: the live chem cam
// shows each machine as it is pressed, so the show tells the run as one line beside it.
type labRun struct {
	machines []string
	presses  map[string][]string
	switches int
}

// labStep takes a press or a thermostat correction into the current run.
func (b *Builder) labStep(d autopilotAction) bool {
	switch {
	case d.Action == "chore" && strings.HasPrefix(d.Why, "pressed ") && d.Target != nil && d.Target.Name != "":
		r := b.labOpen()
		m := d.Target.Name
		if _, ok := r.presses[m]; !ok {
			r.machines = append(r.machines, m)
		}
		r.presses[m] = append(r.presses[m], strings.TrimPrefix(d.Why, "pressed "))
		return true
	case d.Action == "thermostat":
		n := 1
		if m := switchedRe.FindStringSubmatch(d.Why); m != nil {
			n, _ = strconv.Atoi(m[1])
		}
		b.labOpen().switches += n
		return true
	}
	return false
}

func (b *Builder) labOpen() *labRun {
	if b.lab == nil {
		b.lab = &labRun{presses: map[string][]string{}}
	}
	return b.lab
}

func (b *Builder) flushLab() []Event {
	r := b.lab
	b.lab = nil
	if r == nil {
		return nil
	}
	var parts []string
	for _, m := range r.machines {
		counts := map[string]int{}
		var order []string
		for _, p := range r.presses[m] {
			if counts[p] == 0 {
				order = append(order, p)
			}
			counts[p]++
		}
		for _, p := range order {
			part := p + " on the " + m
			if counts[p] > 1 {
				part += fmt.Sprintf(" (×%d)", counts[p])
			}
			parts = append(parts, part)
		}
	}
	held := "the heating plate switched " + plural(r.switches, "time") + " to hold the batch's window"
	// Corrections alone run all day between customers: ticker news, not a lab beat.
	if len(parts) == 0 {
		return []Event{{Kind: KindThought, Level: LevelTicker, Clock: b.Clock(), Text: "In the lab: " + held + "."}}
	}
	text := "In the lab: pressed " + joinAnd(parts)
	if r.switches > 0 {
		text += "; " + held
	}
	return []Event{{Kind: KindThought, Level: LevelNotable, Beat: BeatLab, Clock: b.Clock(), Text: text + "."}}
}

// cartelChoice turns an answer to the chemist start's handler into its beat: the weekly count is a
// moment (an empty counter there ends the run), the mid-week check-in a notable line.
func (b *Builder) cartelChoice(choice string, history []string) []Event {
	all := strings.Join(history, "\n")
	grade := ""
	if m := pureRe.FindStringSubmatch(all); m != nil {
		grade = " at " + m[1] + "%+ purity"
	}
	if m := dustCountRe.FindStringSubmatch(all); m != nil {
		return []Event{{Kind: KindThought, Level: LevelMoment, Beat: BeatCartelQuota, Who: cartelHandler, Clock: b.Clock(),
			Text: fmt.Sprintf("The cartel's count: the handler takes the Dream Dust off the counter (quota %s%s).", m[1], grade)}}
	}
	if m := cookRe.FindStringSubmatch(all); m != nil {
		answer := "told the handler all is well"
		if strings.Contains(strings.ToLower(choice), "ingredients") {
			answer = "asked the handler for more ingredients"
		}
		return []Event{{Kind: KindThought, Level: LevelNotable, Who: cartelHandler, Clock: b.Clock(),
			Text: fmt.Sprintf("The cartel checks in: %s; the next count wants %s Dream Dust%s.", answer, m[1], grade)}}
	}
	return nil
}
