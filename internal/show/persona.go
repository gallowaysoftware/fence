package show

import (
	"path/filepath"
	"strings"

	"github.com/gallowaysoftware/fence/internal/route"
)

// DefaultPersona narrates a run whose player has none, so every run has a character.
const DefaultPersona = "magnate"

// Persona is how a docs/personas player sounds on stream: the file says how it plays; the
// show adds a Kokoro voice and a speaking style, which the decider never sees.
type Persona struct {
	ID   string
	Name string
	// Tagline is the file's Path in a few words ("nothing but nerve").
	Tagline string
	Voice   string
	Style   string
	// Text is the persona file, for the narrator's sense of what the player is after.
	Text string
}

type personaVoice struct{ voice, style string }

var personaVoices = map[string]personaVoice{
	"shark":         {"am_fenrir", "a cocky street hustler: short, punchy, predatory; every customer is an angle to work"},
	"magnate":       {"bm_george", "an unflappable old-money operator: measured and dryly amused; the shop is a portfolio and every setback a line item"},
	"honest-broker": {"af_bella", "a warm, exacting dealer proud of a clean name: a jeweller's patience, quietly disapproving of shortcuts"},
	"fence":         {"am_michael", "a low, conspiratorial black-market insider who winks at the audience and never calls anything stolen"},
	"vintner":       {"bf_emma", "a romantic about the craft, impatient with everything else: wine, water and time are all that matter"},
	"cook":          {"af_nicole", "a tense, clinical chemist under a cartel's thumb, counting purity and days"},
	"smelter":       {"am_puck", "a gruff scrapyard philosopher: everything is ore waiting to happen"},
	"rancher":       {"am_adam", "a folksy, unbothered farmer among rats and fish, with gallows humour about the butchering"},
	"armourer":      {"bm_lewis", "a clipped, by-the-book arms dealer who respects Security and the paperwork"},
	"water-baron":   {"af_sarah", "a smug utility monopolist: water is power and the whole station is thirsty"},
}

// PersonaVoice is the Kokoro voice for a persona id, so each persona sounds like itself
// across runs. Unknown personas get the default persona's.
func PersonaVoice(id string) string {
	if v, ok := personaVoices[id]; ok {
		return v.voice
	}
	return personaVoices[DefaultPersona].voice
}

// LoadPersona reads a persona; an unreadable file still has its id as a name and, for a
// known id, its voice and style.
func LoadPersona(dir, id string) Persona {
	p := Persona{ID: id, Name: id, Voice: PersonaVoice(id), Style: personaVoices[id].style}
	if p.Style == "" {
		p.Style = "dry, quick and a little rueful: a pawnbroker who has seen everything walk through the door"
	}
	text, err := LoadPersonaText(dir, id)
	if err != nil {
		return p
	}
	p.Text = text
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if t, ok := strings.CutPrefix(l, "# "); ok {
			p.Name = strings.TrimSpace(t)
		}
		if t, ok := strings.CutPrefix(l, "Path:"); ok && p.Tagline == "" {
			if i := strings.IndexAny(t, ".;,("); i >= 0 {
				t = t[:i]
			}
			p.Tagline = capitalize(strings.TrimSpace(t))
		}
	}
	return p
}

// SetRoute puts a route (a name or a file) on the name card and in the narrator's
// background; an unreadable route keeps only its name.
func (p *Player) SetRoute(ref string) {
	if ref == "" {
		return
	}
	r, err := route.Load("", ref)
	if err != nil {
		p.Route = strings.TrimSuffix(filepath.Base(ref), ".json")
		return
	}
	p.Route, p.RouteBrief = r.Name, r.Brief
}

// SetPersona puts a persona on the name card; id "" sets def instead.
func (p *Player) SetPersona(dir, id, def string) {
	if id == "" {
		id = def
	}
	if id == "" {
		return
	}
	per := LoadPersona(dir, id)
	p.Persona, p.PersonaName, p.PersonaTagline = id, per.Name, per.Tagline
}

// Mood is the shop's position as the narrator should feel it: Name for the log, Cue for the
// prompt. It carries no numbers, so it can never put a wrong one in a line.
type Mood struct{ Name, Cue string }

// MoodOf reads the clock and the last day's net-worth change: swagger when the shop is flush,
// sweat when the next payment is not covered.
func MoodOf(c *Clock, lastNet *int64) Mood {
	if c == nil || c.Cash < 0 {
		return Mood{}
	}
	if c.Owned {
		return Mood{"triumphant", "The shop is yours. Gloat a little; you earned it."}
	}
	owed := c.RentDue + c.Arrears
	if c.MortgageWeekly > 0 {
		owed = c.MortgageWeekly + c.Arrears
	}
	var m Mood
	switch {
	case c.Arrears > 0 || (owed > 0 && c.Cash < owed):
		m = Mood{"broke", "You are broke: the next payment is not covered. Sweat. Clipped, nervous, gallows humour; no bravado."}
	case owed > 0 && c.Cash < 2*owed:
		m = Mood{"tight", "Money is tight: covered, but barely. Tense and watchful, counting every coin."}
	// Cash against a small hard-game rent alone made the shop "flush" from day 30 and every line a
	// boast (the full replay demo); the buyout is the real race, so flush needs progress on it.
	case c.BuyoutPrice > 0 && c.NetWorth*10 >= c.BuyoutPrice*6,
		owed > 0 && c.Cash >= 6*owed && (c.BuyoutPrice == 0 || c.NetWorth*10 >= c.BuyoutPrice*3):
		m = Mood{"flush", "You are flush. Let the swagger show: confident, a touch smug, enjoying it."}
	default:
		m = Mood{"steady", "Business is steady. Even-keeled; save the drama for real news."}
	}
	if lastNet != nil {
		switch big := max(600, c.NetWorth/25); {
		case *lastNet <= -big:
			m.Cue += " Yesterday hurt, and it still stings."
		case *lastNet >= big:
			m.Cue += " Yesterday was a good one."
		}
	}
	return m
}

// paceCues put the run's standing against its lab book (ghost.go) in the mood, without numbers.
var paceCues = map[string]string{
	PaceAhead:  " You are ahead of the book: richer this morning than the lab's typical run of this route by the same day.",
	PaceOnPace: " You are on pace with the book: about where the lab's runs of this route were by now.",
	PaceBehind: " You are behind the book: poorer this morning than the lab's typical run of this route by the same day.",
}

// withPace adds the race against the book to a mood; a bought shop has nothing left to race.
func withPace(m Mood, pace string) Mood {
	if m.Name == "" || m.Name == "triumphant" || paceCues[pace] == "" {
		return m
	}
	m.Cue += paceCues[pace]
	return m
}
