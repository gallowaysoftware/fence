package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// DefaultRoute is what a run plays when the model's choice is unusable: the incumbent.
const DefaultRoute = "appraiser"

const chooseMaxTokens = 1500

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
	Persona     string       `json:"persona,omitempty"`
	Thesis      string       `json:"thesis"`
	// Menu is the route names the model was offered; FixedStart the start the match fixed.
	Menu       []string `json:"menu"`
	FixedStart string   `json:"fixed_start,omitempty"`
	Recent     []string `json:"recent,omitempty"`
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

// ChoosePrompt is the day-0 ask: the start (fixed or free), the menu with its lab record, the
// levers and the answer's shape.
//
// lean cuts the menu to one short line a route (its levers over the appraiser's and its wins)
// and the levers to their names, for a small model on a tight context (drift).
func ChoosePrompt(m *Menu, fixedStart string, recent []string, lean bool) string {
	var b strings.Builder
	b.WriteString(shopRules + "\n\n")
	if len(recent) > 0 {
		fmt.Fprintf(&b, "The stream's previous runs played, oldest first: %s. The audience has seen those; playing one again needs a reason of its own.\n\n", strings.Join(recent, ", "))
	}
	if fixedStart != "" {
		fmt.Fprintf(&b, "This match fixes your start: %s. Only routes that play it are on your menu.\n\n", fixedStart)
	} else {
		b.WriteString("You choose the start too: each route names the start and perks it plays.\n\n")
	}
	b.WriteString("THE MENU (every route was measured in our lab, the gym that runs the game's own rules):\n")
	if lean {
		b.WriteString(m.LeanText())
	} else {
		b.WriteString(m.Text())
	}
	b.WriteString("\nOr COMPOSE your own route: take a menu route as the base and change levers. A composed route has never been played; the lab numbers above are your only evidence. Levers:\n")
	if lean {
		b.WriteString(LeverNames() + "\n")
	} else {
		b.WriteString(LeverText())
	}
	b.WriteString(`
Answer with one JSON object:
- to play a menu route: {"route": "<name>", "thesis": "<one paragraph>"}
- to compose: {"compose": {"name": "<new_name>", "base": "<menu route>", "levers": {"<lever>": <value>, ...}}, "thesis": "<one paragraph>"} (add "start" and "perks" only to change the base's).
The thesis is one paragraph, 60–120 words, in your own voice: what this shop will be, why it beats the alternatives you passed over (name one), and what you will watch for. It is shown to the viewers.`)
	return b.String()
}

// Choose asks c for the day-0 route. The result is always playable: an unusable answer
// falls back to a menu route (the model's named route if it is on the menu, else
// DefaultRoute) and says why.
//
// recent are the routes the stream's previous runs played, which the model is asked to vary from.
func Choose(ctx context.Context, c llm.Client, menu *Menu, fixedStart string, recent []string, lean bool) Choice {
	m := menu.For(fixedStart)
	ch := Choice{FixedStart: fixedStart, Recent: recent}
	for _, e := range m.Entries {
		ch.Menu = append(ch.Menu, e.Name)
	}
	start := time.Now()
	defer func() { ch.MS = time.Since(start).Milliseconds() }()
	msgs := []llm.Message{{Role: llm.RoleUser, Content: ChoosePrompt(m, fixedStart, recent, lean)}}
	req := llm.Request{System: chooseSystem, Messages: msgs, Schema: chooseSchema, SchemaName: "route_choice", MaxTokens: chooseMaxTokens}
	var a answer
	var errs []string
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.Complete(ctx, req)
		if err != nil {
			errs = append(errs, err.Error())
			break
		}
		ch.Raw, ch.Model = resp.Text, resp.Model
		a, errs = parseAnswer(resp.Text, m, fixedStart)
		if len(errs) == 0 {
			break
		}
		// One repair round: the model sees its answer and what was wrong with it.
		req.Messages = append(slicesClip(msgs), llm.Message{Role: llm.RoleAssistant, Content: resp.Text},
			llm.Message{Role: llm.RoleUser, Content: "That answer cannot be played: " + strings.Join(errs, "; ") + ". Answer again with one corrected JSON object."})
	}
	ch.Errors = errs
	ch.Thesis = strings.TrimSpace(a.Thesis)
	if len(errs) == 0 && a.Compose != nil {
		base, _ := m.Get(a.Compose.Base)
		ch.Route, ch.Composed, ch.Composition = a.Compose.Name, true, a.Compose
		ch.Start, ch.Perks, ch.Persona = a.Compose.start(base), a.Compose.perks(base), base.Persona
		return ch
	}
	name := a.Route
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
	return ch
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
