package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// The night decision (docs/STREAM.md section 4): after the shutter closes the player takes
// one outing — Dr. Jackson's, the dumping ground, the phone, or no outing. This bridge picks
// the outing, not what happens on it: the dump's searches and Jackson's counter play the
// role's script (the mod asks its own barter/stage decisions there).
const (
	nightKind   = "night"
	optGoJacks  = "go_jackson"
	optScav     = "scavenge"
	optStayIn   = "stay_in"
	optCallWild = "call_wild"
	locJackson  = "dr_jackson"
	locDump     = "dumping_ground"
)

// woundSerious is the shop's wound level at which the door shuts: no outing at all
// (PlayerStore.healthData.woundState > 5; the prompt's door gates).
const woundSerious = 5

// nightCtx is the night context (docs/PROTOCOL.md). Every field is a pointer or a slice so
// a value the mod could not read stays distinguishable from zero.
type nightCtx struct {
	Day          *int64           `json:"day"`
	Cash         *int64           `json:"cash"`
	RentDue      *int64           `json:"rent_due"`
	RentDaysLeft *int64           `json:"rent_days_left"`
	Wound        *int64           `json:"wound"`
	Locations    []nightLoc       `json:"locations"`
	Services     []nightSvc       `json:"services"`
	JacksonWants map[string]int64 `json:"jackson_wants"`
	BagItems     *int64           `json:"bag_items"`
	PlanNotes    string           `json:"plan_notes"`
}

type nightLoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type nightSvc struct {
	ID    string `json:"id"`
	Cost  *int64 `json:"cost"`
	Using *bool  `json:"using"`
}

func parseNight(raw json.RawMessage) (*nightCtx, error) {
	var c nightCtx
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Cash == nil {
		return nil, errors.New("night context has no cash")
	}
	return &c, nil
}

func (c *nightCtx) hasLoc(id string) bool {
	for _, l := range c.Locations {
		if l.ID == id {
			return true
		}
	}
	return false
}

// wound is the shop's wound state (0 = sound), 0 when the mod could not read it.
func (c *nightCtx) wound() int64 {
	if c.Wound == nil {
		return 0
	}
	return *c.Wound
}

// woundName states the wound level a player can read: 0 none, 1-2 minor, 3-4 fairly
// serious, 5 serious near the door-shut, above 5 the door is shut.
func woundName(w int64) string {
	switch {
	case w == 0:
		return "none"
	case w <= 2:
		return fmt.Sprintf("minor (%d)", w)
	case w <= 4:
		return fmt.Sprintf("fairly serious (%d)", w)
	case w <= woundSerious:
		return fmt.Sprintf("serious, near the door-shut (%d)", w)
	default:
		return fmt.Sprintf("serious, the door is shut (%d)", w)
	}
}

// nightWants lists what Dr. Jackson's counter would buy tonight, sorted and dropped at 0.
func (c *nightCtx) nightWants() []string {
	if len(c.JacksonWants) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.JacksonWants))
	for k := range c.JacksonWants {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		n := c.JacksonWants[k]
		if n <= 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s x%d", k, n))
	}
	return out
}

func renderNight(b *strings.Builder, c *nightCtx) {
	day := valueOr(c.Day, int64(-1))
	if day >= 0 {
		fmt.Fprintf(b, "Night (day %d)\n", day)
	} else {
		b.WriteString("Night\n")
	}
	parts := []string{fmt.Sprintf("cash %d", *c.Cash)}
	if c.RentDue != nil {
		parts = append(parts, fmt.Sprintf("rent %d due", *c.RentDue))
	}
	if c.RentDaysLeft != nil {
		parts = append(parts, fmt.Sprintf("%d night(s) left", *c.RentDaysLeft))
	}
	fmt.Fprintf(b, "%s, wound: %s\n", strings.Join(parts, ", "), woundName(c.wound()))
	for _, l := range c.Locations {
		name := l.Name
		if name == "" {
			name = l.ID
		}
		fmt.Fprintf(b, "Location: %s\n", name)
	}
	var svc []string
	for _, s := range c.Services {
		if !isTrue(s.Using) {
			continue
		}
		if s.Cost != nil {
			svc = append(svc, fmt.Sprintf("%s (%d/night)", strings.ToLower(s.ID), *s.Cost))
		} else {
			svc = append(svc, strings.ToLower(s.ID))
		}
	}
	if len(svc) > 0 {
		fmt.Fprintf(b, "Services on tonight: %s\n", strings.Join(svc, ", "))
	}
	if w := c.nightWants(); len(w) > 0 {
		fmt.Fprintf(b, "Dr. Jackson can stock: %s\n", strings.Join(w, ", "))
	}
	if c.BagItems != nil {
		fmt.Fprintf(b, "Bag: %d item(s)\n", *c.BagItems)
	}
	if c.PlanNotes != "" {
		fmt.Fprintf(b, "The plan: %s\n", c.PlanNotes)
	}
}

// valueOr is v or fallback when v is nil; the render never crashes on a missing field.
func valueOr(v *int64, fallback int64) int64 {
	if v == nil {
		return fallback
	}
	return *v
}

func nightNotes(c *nightCtx) func(id, label string) string {
	if len(c.JacksonWants) > 0 {
		return func(id, _ string) string {
			switch id {
			case optGoJacks:
				return "his counter buys materials and sells equipment — worth it when jackson_wants is non-empty or cash covers a kit"
			case optScav:
				return "free loot, but wound odds (night log shows the state) and the junk split"
			case optStayIn:
				return "no cost, no gain"
			case optCallWild:
				return "browse Wild's network for a phone order"
			}
			return ""
		}
	}
	return func(id, _ string) string {
		switch id {
		case optGoJacks:
			return "his counter buys materials and sells equipment — worth it when cash covers a kit"
		case optScav:
			return "free loot, but wound odds (night log shows the state) and the junk split"
		case optStayIn:
			return "no cost, no gain"
		case optCallWild:
			return "browse Wild's network for a phone order"
		}
		return ""
	}
}

// rentImminent says whether the rent falls due within rentDays nights (heuristic.go): the
// nights to keep cash in hand and the wound rested, not to spend on a kit.
func (c *nightCtx) rentImminent() bool {
	return c.RentDaysLeft != nil && *c.RentDaysLeft <= rentDays
}

// nightHeuristic is the night's zero-inference baseline: a non-empty jackson_wants, or
// rent not imminent with cash to spare, buys the trip to Dr. Jackson's; otherwise an open
// dumping ground and a wound at most minor scavenges; otherwise the night is at home.
func nightHeuristic(req *protocol.DecideRequest) Decision {
	c, err := parseNight(req.Context)
	if err != nil {
		return heuristicDefer("the night context does not have the expected shape")
	}
	opts := inspectOptions(req)
	has := func(id string) bool { _, ok := opts[id]; return ok }
	switch {
	case has(optGoJacks) && !c.rentImminent():
		if len(c.JacksonWants) > 0 {
			return choose(optGoJacks, fmt.Sprintf("Dr. Jackson will buy %s: that is the trip's point.", strings.Join(c.nightWants(), ", ")))
		}
		if c.RentDaysLeft != nil && *c.RentDaysLeft > 2 && c.RentDue != nil && *c.Cash > *c.RentDue+800 {
			return choose(optGoJacks, fmt.Sprintf("Rent is not imminent (%d left) and cash %d is well clear of the %d due: the kit is worth a night.", *c.RentDaysLeft, *c.Cash, *c.RentDue))
		}
	case has(optScav) && !c.rentImminent() && c.hasLoc(locDump) && c.wound() <= 1:
		return choose(optScav, fmt.Sprintf("Nothing to move at Jackson's and the shop is %s: the dumping ground pays for itself free.", woundName(c.wound())))
	case has(optStayIn):
		why := "No wants at Jackson's"
		if c.rentImminent() {
			why += fmt.Sprintf(", rent %d is due in %d", *c.RentDue, *c.RentDaysLeft)
		}
		if c.wound() > 1 {
			why += fmt.Sprintf(", and the shop is %s", woundName(c.wound()))
		}
		return choose(optStayIn, why+", so the night is at home.")
	}
	return heuristicDefer("no night option fits the listed context")
}

// checkNightChoice refuses an outing the visible state cannot be at: go_jackson without
// Dr. Jackson's in the locations, scavenge without the dumping ground.
func checkNightChoice(req *protocol.DecideRequest, choice protocol.Choice) error {
	switch choice.Option {
	case optGoJacks, optScav:
		c, err := parseNight(req.Context)
		if err != nil {
			return errors.New("the night context does not parse, so no outing can be checked")
		}
		id := locJackson
		if choice.Option == optScav {
			id = locDump
		}
		if !c.hasLoc(id) {
			return fmt.Errorf("option %q is not allowed: the locations list has no %q", choice.Option, id)
		}
	}
	return nil
}

// nightSystem states the night's rules in our own words.
var nightSystem = strings.TrimSpace(`
You advise the owner of a pawnshop in the cyberpunk game "Probably Stolen". The shutter is closed; the player takes ONE outing tonight, or none. You pick the outing; what happens on it (the dump's searches, Dr. Jackson's counter) is played on the role's script or your own counter advice.

NIGHT
- After closing: one outing a night, or stay in. The door shuts (no outing at all) above a wound of 5, during a crackdown, and before after-hours is introduced; the wound you carry is in the prompt.
- Dr. Jackson's: he buys scrap, ore and flux at a markup and sells furnace modules, hydroponics gear, purifiers, yeast and converters — his stock is the production engines' kit source. A night there is worth it when he would buy something you hold, or when cash covers a kit worth a night.
- The dumping ground: free finds (junk, split by the bag and pockets) but a wound chance per search; a major wound costs days of rest and the door shuts above 5.
- Wild's network: a phone line that browses favors, services and referrals for the night.
- Services you keep on bill nightly; the prompt lists them.
- The plan's goals for the day decide the rest: when the prompt opens with the day's plan, serve it.

YOU SEE ONLY WHAT THE PLAYER SEES. A field that is missing is unknown, not zero; the wound level is the state the night log shows, named for you in the prompt.

JUDGMENT
- Serve the plan first: it says which engines and which night's spend matter today.
- Dr. Jackson's buys: take the trip when he wants what you hold, or when rent is not imminent and the cash covers a kit; otherwise the dump pays for itself free while the shop is sound.
- A wound above 2 makes the dump's odds dear; above 5 there is no outing at all.
- Rent due within 2 nights: do not spend the bill on a kit.
- call_wild is the lightest move: it costs a night's sleep and nothing else; stay_in when nothing is worth even that.
` + answerRules)
