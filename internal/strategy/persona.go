package strategy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/show"
)

const (
	// MaxPersonaBytes bounds a persona without its notes (show.PersonaNotes): it rides in every
	// morning prompt.
	MaxPersonaBytes = 800
	// DefaultPersonaDir is where persona ids resolve, relative to the bridge's working
	// directory (the repository root).
	DefaultPersonaDir = "docs/personas"
)

// personaID is what an id may look like; anything else is refused, so an id taken from an
// event can never name a path outside the persona directory.
var personaID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// isPersonaPath tells a file path from a persona id.
func isPersonaPath(ref string) bool {
	return strings.ContainsAny(ref, `/\`) || strings.HasSuffix(ref, ".md")
}

// LoadPersona reads a persona: ref is an id resolved as dir/<id>.md, or a path to a file.
// The text is the file up to its notes, with its markdown title marks removed.
func LoadPersona(dir, ref string) (string, error) {
	path := ref
	if !isPersonaPath(ref) {
		if !personaID.MatchString(ref) {
			return "", fmt.Errorf("persona %q: not an id (lowercase letters, digits, - and _) or a path", ref)
		}
		if dir == "" {
			dir = DefaultPersonaDir
		}
		path = filepath.Join(dir, ref+".md")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("persona %q: %w", ref, err)
	}
	prompt := show.PersonaPrompt(string(raw))
	if len(prompt) > MaxPersonaBytes {
		return "", fmt.Errorf("persona %q: %d bytes before its notes, the limit is %d", ref, len(prompt), MaxPersonaBytes)
	}
	text := personaText(prompt)
	if text == "" {
		return "", fmt.Errorf("persona %q is empty", ref)
	}
	return text, nil
}

func personaText(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(strings.TrimLeft(strings.TrimRight(l, " \r"), "#"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// simRunPersona is the persona id a sim_run event names, "" when it names none.
func simRunPersona(data json.RawMessage) string {
	var d struct {
		Persona string `json:"persona"`
	}
	if json.Unmarshal(data, &d) != nil {
		return ""
	}
	return strings.TrimSpace(d.Persona)
}

// styleRule is added to the plan system prompt when the morning prompt carries a style.
const styleRule = `YOUR STYLE: the prompt ends with this shop's chosen way to play. The plan follows it: GOALS, FACTIONS, STOCK, KEEP, PRODUCTION and RISK pick what the style picks, push its engine and its factions, and never do what it says it never does, even when another line would earn a little more today. Break the style only when survival is at stake (rent that cannot otherwise be met, a faction within 20 of -100, an inspection or sentence that would end the run), and then name the break in GOALS. The style never overrides the mechanics or the layout above.`

func renderStyle(b *strings.Builder, persona string) {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return
	}
	fmt.Fprintf(b, "\nYOUR STYLE (the plan follows it unless survival is at stake):\n%s\n", truncateLines(persona, MaxPersonaBytes))
}

// routeRule is added to the plan system prompt when the run plays a route.
const routeRule = `THIS RUN'S ROUTE: the prompt names the route this shop plays, a tested way to win: its start, its engine and the base policy are already in force. Write the plan as that shop: GOALS, PRODUCTION and RISK speak of the route's own engine and signature in its own terms, and its base policy stands unless today's state gives a specific reason to change it. YOUR STYLE is the character playing the route (attitude, factions, what it never does); where the style's path differs from the route, the route wins.`

func renderRoute(b *strings.Builder, route string) {
	route = strings.TrimSpace(route)
	if route == "" {
		return
	}
	fmt.Fprintf(b, "\nTHIS RUN'S ROUTE (already in force):\n%s\n", route)
}

// style is the persona the plan follows. On a route the route's base policy buys the
// upgrades, so the persona's own buy order and pivots are left out: they would make
// lintUpgrades demand purchases the route was tested without.
func (in PlanInput) style() string {
	if strings.TrimSpace(in.Route) == "" {
		return in.Persona
	}
	lines := strings.Split(in.Persona, "\n")
	lines = slices.DeleteFunc(lines, func(l string) bool {
		l = strings.TrimSpace(l)
		return strings.HasPrefix(l, "Buy:") || strings.HasPrefix(l, "Pivot:")
	})
	return strings.Join(lines, "\n")
}
