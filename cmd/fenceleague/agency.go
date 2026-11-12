package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/llm"
)

// Values of Player.ChooseRoute: the model picks the run's route before day 1 (internal/agency).
const (
	ChooseAny   = "any"   // any menu route or a composition, with its own start
	ChooseStart = "start" // only routes that play the match's start
)

const (
	choiceFile   = "choice.json"
	composedFile = "route.json"
)

// chooseRoute asks the player's model for the match's route and returns the match as it is
// played: start, perks, persona and directives of the route chosen. m keeps the route slot it
// was scheduled as, so its id and the report's pairing stay those of the slot.
func (l *League) chooseRoute(ctx context.Context, m Match, ep *Endpoint, model, dir string) (Match, agency.Choice, error) {
	menu, err := agency.LoadMenu("")
	if err != nil {
		return m, agency.Choice{}, err
	}
	opts, err := ep.openAIOptions()
	if err != nil {
		return m, agency.Choice{}, err
	}
	client := &llm.OpenAI{BaseURL: l.proxy.BaseURL(m.ID()), Model: model, OpenAIOptions: opts}
	fixed := ""
	if m.Player.ChooseRoute == ChooseStart {
		fixed = m.Start
	}
	ch := agency.Choose(ctx, client, menu, fixed, l.recentChoices(dir), m.Player.AgencyLean)
	if ctx.Err() != nil {
		return m, ch, ctx.Err()
	}
	if err := writeJSON(filepath.Join(dir, choiceFile), ch); err != nil {
		return m, ch, err
	}
	l.Log.Info("day-0 choice", "match", m.ID(), "route", ch.Route, "composed", ch.Composed, "fallback", ch.Fallback, "thesis", ch.Thesis)
	played := m
	played.id = m.ID()
	played.Start, played.Perks, played.Persona = ch.Start, sortedPerks(ch.Perks), ch.Persona
	if ch.Composed {
		b, err := ch.RouteFile(menu)
		if err != nil {
			return m, ch, fmt.Errorf("composed route: %w", err)
		}
		path := filepath.Join(dir, composedFile)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return m, ch, err
		}
		played.RoutePath = mustAbs(path)
	} else {
		e, _ := menu.Get(ch.Route)
		played.RoutePath = mustAbs(e.Path)
	}
	return played, ch, nil
}

// recentChoices are the routes the league's other matches chose, oldest first: the stream's
// previous runs, which the model is asked to vary from.
func (l *League) recentChoices(own string) []string {
	paths, _ := filepath.Glob(filepath.Join(l.Dir, "*", choiceFile))
	type c struct {
		at    time.Time
		route string
	}
	var cs []c
	for _, p := range paths {
		if filepath.Dir(p) == own {
			continue
		}
		var ch agency.Choice
		fi, err := os.Stat(p)
		if err != nil || readJSON(p, &ch) != nil || ch.Route == "" {
			continue
		}
		cs = append(cs, c{fi.ModTime(), ch.Route})
	}
	slices.SortFunc(cs, func(a, b c) int { return a.at.Compare(b.at) })
	var out []string
	for _, x := range cs {
		out = append(out, x.route)
	}
	return out
}
