package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/llm"
)

// ChoiceFile is the name a choice is recorded under, one directory per run.
const ChoiceFile = "choice.json"

// DefaultRoute is what a run plays when the model's choice is unusable: the incumbent.
const DefaultRoute = "appraiser"

const chooseMaxTokens = 1500

// mortgageWeeks is the loan shark's term on hard (shopRules).
const mortgageWeeks = 16

const chooseSystem = `You are the strategist of a pawn shop in the game Probably Stolen, about to start a run on hard difficulty, live on a stream. The executor (a tuned script) will do the routine work every day: buying, selling, machines, nights. Your choice now decides what kind of shop this run is. Viewers will see your choice and your thesis and judge you by them, so choose for a reason you can state, not for safety alone. Answer with one JSON object and nothing else.`

const shopRules = `How a hard run is won: the rent grows 10 % a week; the run is won by buying the shop from the landlord (about 30 000), for cash or through the loan shark's mortgage (offered on rent mornings once the cash reaches twice its down payment; 16 weekly payments of ~1 700; a missed payment is repossession). Every route joins the Appraisal Board (its fees are 55–71 % of the money) and tells the loan shark "later" while the cash buyout projects by its buyout_by_day. Production (wine, ingots, water, rat meat) nets little cash before the buyout: routes differ mostly in what the shop shows and how fast it gets there.`

// Choice is the day-0 decision as recorded in the match (choice.json).
type Choice struct {
	// Route names the route played: a menu route, or the composition's name.
	Route string `json:"route"`
	// Composed is set when the model built its own route; Composition is it.
	Composed    bool         `json:"composed,omitempty"`
	Composition *Composition `json:"composition,omitempty"`
	Start       string       `json:"start"`
	Perks       string       `json:"perks"`
	// Persona plays the run: the card's archetype when it fixes one (PersonaFixed), else the
	// route's own. The thesis is written in the card's voice, or the route's when it fixes none.
	Persona      string `json:"persona,omitempty"`
	PersonaFixed bool   `json:"persona_fixed,omitempty"`
	Thesis       string `json:"thesis"`
	// Menu is the route names the model was offered; FixedStart the start the match fixed.
	Menu       []string `json:"menu"`
	FixedStart string   `json:"fixed_start,omitempty"`
	Recent     []string `json:"recent,omitempty"`
	// Excluded is the route taken off the menu as the one played last ("" when none was).
	Excluded string `json:"excluded,omitempty"`
	// Format is the run's stream format, whose deadline odds the menu showed.
	Format string `json:"format,omitempty"`
	// Fallback says why the model's answer was not played ("" when it was).
	Fallback string   `json:"fallback,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Raw      string   `json:"raw,omitempty"`
	Model    string   `json:"model,omitempty"`
	MS       int64    `json:"ms"`
}

type answer struct {
	Route   string       `json:"route"`
	Compose *Composition `json:"compose"`
	Thesis  string       `json:"thesis"`
}

var chooseSchema = json.RawMessage(`{"type":"object","properties":{"route":{"type":"string"},"compose":{"type":"object","properties":{"name":{"type":"string"},"base":{"type":"string"},"start":{"type":"string"},"perks":{"type":"string"},"levers":{"type":"object"}},"required":["name","base","levers"]},"thesis":{"type":"string"}},"required":["thesis"]}`)

// Ask is the day-0 question's setting beyond the menu.
type Ask struct {
	// FixedStart is the start the match fixes, "" for any.
	FixedStart string
	// Recent are the routes the stream's previous runs played, which the model is asked to vary from.
	Recent []string
	// Exclude is the route the stream played last: off the menu, so neither picked nor a
	// composition's base or name, unless it is the only route the menu has. A composition played
	// last is no menu route and excludes nothing. A composition whose levers rebuild the excluded
	// route is not caught: that takes deliberate work and still airs as a new route.
	Exclude string
	// Lean cuts the menu to one short line a route (its levers over the appraiser's and its wins)
	// and the levers to their names, for a small model on a tight context (drift).
	Lean bool
	// Format is the run's stream format, nil for none. With a deadline, each route shows its odds
	// of beating it (deadline_odds.json).
	Format *format.Format
	// Persona is the archetype the scenario card fixes (a docs/personas id), "" for none; the
	// model still picks the route. PersonaText is that persona's prompt part, as the strategist
	// loads it (strategy.LoadPersona): the thesis is written in its voice.
	Persona, PersonaText string
}

// ChoosePrompt is the day-0 ask: the start (fixed or free), the format, the menu with its lab
// record, the levers and the answer's shape.
func ChoosePrompt(m *Menu, a Ask) string {
	var b strings.Builder
	b.WriteString(shopRules + "\n\n")
	if len(a.Recent) > 0 {
		fmt.Fprintf(&b, "The stream's previous runs played, oldest first: %s. The audience has seen those; playing one again needs a reason of its own.\n\n", strings.Join(a.Recent, ", "))
	}
	if x := m.Excluded(); x != "" {
		fmt.Fprintf(&b, "The last run played %s, so it is off tonight's menu: you cannot pick it or compose from it.\n\n", x)
	}
	if a.FixedStart != "" {
		fmt.Fprintf(&b, "This match fixes your start: %s. Only routes that play it are on your menu.\n\n", a.FixedStart)
	} else {
		b.WriteString("You choose the start too: each route names the start and perks it plays.\n\n")
	}
	if f := a.Format; f != nil {
		b.WriteString(f.RulesText() + "\n")
		if f.DeadlineDay > 0 && f.DeadlineDay < mortgageWeeks*7 {
			b.WriteString("A mortgage takes 16 weekly payments, longer than the deadline allows: only a cash buyout beats it.\n")
		}
		if m.Deadline > 0 && m.Seeds != "" {
			fmt.Fprintf(&b, "So the record that matters is each route's odds of a buyout by day %d under these rules: the share of the lab's runs (executor alone, seeds %s, this format's rules applied) that bought the shop in time, with its 95 %% range. A route's usual record and median win day are without the deadline.\n", m.Deadline, m.Seeds)
		}
		b.WriteString("\n")
	}
	b.WriteString("THE MENU (every route was measured in our lab, the gym that runs the game's own rules):\n")
	if a.Lean {
		b.WriteString(m.LeanText())
	} else {
		b.WriteString(m.Text())
	}
	b.WriteString("\nOr COMPOSE your own route: take a menu route as the base and change levers. A composed route has never been played; the lab numbers above are your only evidence. Levers:\n")
	if a.Lean {
		b.WriteString(LeverNames() + "\n")
	} else {
		b.WriteString(LeverText())
	}
	if a.Persona != "" && strings.TrimSpace(a.PersonaText) != "" {
		fmt.Fprintf(&b, "\nYOUR CHARACTER: the run's card makes you this archetype, whatever route you choose. Write the thesis as this character: what it brags about, what it scorns, what it cares for. It may tip which route you prefer; it never changes the rules, the format or the menu above. The thesis speaks of the route you chose and what it does: never promise an engine or a purchase the route does not make:\n%s\n", strings.TrimSpace(a.PersonaText))
	} else {
		b.WriteString("\nYOUR CHARACTER: each route is played by its persona, named on its menu line; write the thesis in the voice of the route you choose.\n")
	}
	b.WriteString(`
Answer with one JSON object:
- to play a menu route: {"route": "<name>", "thesis": "<one paragraph>"}
- to compose: {"compose": {"name": "<new_name>", "base": "<menu route>", "levers": {"<lever>": <value>, ...}}, "thesis": "<one paragraph>"} (add "start" and "perks" only to change the base's).
The thesis is one paragraph, 60–120 words, in your character's voice, first person: what this shop will be, why it beats the alternatives you passed over (name one), and what you will watch for. It is shown to the viewers.`)
	return b.String()
}

// Choose asks c for the day-0 route. The result is always playable: an unusable answer
// falls back to a menu route (the model's named route if it is on the menu, else
// DefaultRoute) and says why.
func Choose(ctx context.Context, c llm.Client, menu *Menu, a Ask) (ch Choice) {
	fixedStart := a.FixedStart
	m := menu.For(fixedStart)
	ch = Choice{FixedStart: fixedStart, Recent: a.Recent}
	if a.Format != nil {
		ch.Format = a.Format.Name
		m = m.WithDeadline(*a.Format)
	}
	if a.Exclude != "" {
		m = m.Without(a.Exclude)
		ch.Excluded = m.Excluded()
	}
	for _, e := range m.Entries {
		ch.Menu = append(ch.Menu, e.Name)
	}
	start := time.Now()
	defer func() { ch.MS = time.Since(start).Milliseconds() }()
	msgs := []llm.Message{{Role: llm.RoleUser, Content: ChoosePrompt(m, a)}}
	req := llm.Request{System: chooseSystem, Messages: msgs, Schema: chooseSchema, SchemaName: "route_choice", MaxTokens: chooseMaxTokens}
	var ans answer
	var errs []string
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.Complete(ctx, req)
		if err != nil {
			errs = append(errs, err.Error())
			break
		}
		ch.Raw, ch.Model = resp.Text, resp.Model
		ans, errs = parseAnswer(resp.Text, m, fixedStart)
		if len(errs) == 0 {
			break
		}
		// One repair round: the model sees its answer and what was wrong with it.
		req.Messages = append(slicesClip(msgs), llm.Message{Role: llm.RoleAssistant, Content: resp.Text},
			llm.Message{Role: llm.RoleUser, Content: "That answer cannot be played: " + strings.Join(errs, "; ") + ". Answer again with one corrected JSON object."})
	}
	ch.Errors = errs
	ch.Thesis = strings.TrimSpace(ans.Thesis)
	if len(errs) == 0 && ans.Compose != nil {
		base, _ := m.Get(ans.Compose.Base)
		ch.Route, ch.Composed, ch.Composition = ans.Compose.Name, true, ans.Compose
		ch.Start, ch.Perks, ch.Persona = ans.Compose.start(base), ans.Compose.perks(base), base.Persona
		ch.fixPersona(a)
		return ch
	}
	name := ans.Route
	if len(errs) > 0 {
		ch.Fallback = "unusable answer: " + strings.Join(errs, "; ")
		if _, ok := m.Get(name); !ok {
			name = ""
		}
	}
	if name == "" {
		name = DefaultRoute
		if _, ok := m.Get(name); !ok && len(m.Entries) > 0 {
			name = m.Entries[0].Name
		}
	}
	e, _ := m.Get(name)
	ch.Route, ch.Start, ch.Perks, ch.Persona = e.Name, e.Start, e.Perks, e.Persona
	ch.fixPersona(a)
	return ch
}

// fixPersona puts the card's archetype over the route's: the card fixes who plays, the model
// only what.
func (ch *Choice) fixPersona(a Ask) {
	if a.Persona != "" {
		ch.Persona, ch.PersonaFixed = a.Persona, true
	}
}

// Recent are the routes chosen in dir's subdirectories (<dir>/*/choice.json), oldest first by
// the file's time, leaving out the one in except: the previous runs the next choice varies from.
func Recent(dir, except string) []string {
	paths, _ := filepath.Glob(filepath.Join(dir, "*", ChoiceFile))
	type c struct {
		at    time.Time
		route string
	}
	var cs []c
	for _, p := range paths {
		if except != "" && filepath.Clean(filepath.Dir(p)) == filepath.Clean(except) {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(p)
		var ch Choice
		if err != nil || json.Unmarshal(b, &ch) != nil || ch.Route == "" {
			continue
		}
		cs = append(cs, c{fi.ModTime(), ch.Route})
	}
	slices.SortStableFunc(cs, func(a, b c) int { return a.at.Compare(b.at) })
	out := make([]string, 0, len(cs))
	for _, x := range cs {
		out = append(out, x.route)
	}
	return out
}

func slicesClip(m []llm.Message) []llm.Message { return m[:len(m):len(m)] }

func parseAnswer(text string, m *Menu, fixedStart string) (answer, []string) {
	var a answer
	raw, err := llm.ExtractJSON(text)
	if err != nil {
		return a, []string{"no JSON object: " + err.Error()}
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, []string{"not the answer's shape: " + err.Error()}
	}
	if c := a.Compose; c != nil {
		liftMeta(c)
	}
	var errs []string
	if w := len(strings.Fields(a.Thesis)); w < 20 {
		errs = append(errs, fmt.Sprintf("thesis is %d words; write one paragraph of 60–120", w))
	}
	switch {
	case a.Compose != nil && a.Route != "" && a.Route != a.Compose.Name:
		errs = append(errs, `give "route" or "compose", not both`)
	case a.Compose != nil:
		errs = append(errs, a.Compose.Validate(m)...)
		if fixedStart != "" && a.Compose.Start != "" && a.Compose.Start != fixedStart {
			errs = append(errs, "the start is fixed: "+fixedStart)
		}
	case a.Route == "":
		errs = append(errs, `no "route" and no "compose"`)
	default:
		if _, ok := m.Get(a.Route); !ok {
			errs = append(errs, fmt.Sprintf("route %q is not on the menu", a.Route))
		}
	}
	return a, errs
}

// RouteFile is the directives file the choice plays, with its "_route" key: the menu route's
// own file, or the composition's.
func (ch Choice) RouteFile(menu *Menu) ([]byte, error) {
	if !ch.Composed {
		e, ok := menu.Get(ch.Route)
		if !ok {
			return nil, fmt.Errorf("route %q is not on the menu", ch.Route)
		}
		return os.ReadFile(e.Path)
	}
	return ch.Composition.File(menu.For(ch.FixedStart), ch.Persona, "Composed by the strategist: "+ch.Composition.Summary()+". "+ch.Thesis)
}

// liftMeta moves a start or perks the model wrote among the levers to where they belong, and
// drops a persona there: models mix the route's metadata into its levers.
func liftMeta(c *Composition) {
	for _, k := range []string{"start", "perks"} {
		if v, ok := c.Levers[k].(string); ok {
			if k == "start" && c.Start == "" {
				c.Start = v
			} else if k == "perks" && c.Perks == "" {
				c.Perks = v
			}
		}
		delete(c.Levers, k)
	}
	delete(c.Levers, "persona")
}
