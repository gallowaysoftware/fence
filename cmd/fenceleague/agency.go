package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

// Values of Player.ChooseRoute: the model picks the run's route before day 1 (internal/agency).
const (
	ChooseAny   = "any"   // any menu route or a composition, with its own start
	ChooseStart = "start" // only routes that play the match's start
)

const composedFile = "route.json"

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
	recent := agency.Recent(l.Dir, dir)
	ask := agency.Ask{Recent: recent, Lean: m.Player.AgencyLean}
	if id := m.Player.Persona; id != "" {
		text, err := strategy.LoadPersona(strategy.DefaultPersonaDir, id)
		if err != nil {
			return m, agency.Choice{}, err
		}
		ask.Persona, ask.PersonaText = id, text
	}
	if len(recent) > 0 {
		ask.Exclude = recent[len(recent)-1]
	}
	if m.Player.ChooseRoute == ChooseStart {
		ask.FixedStart = m.Start
	}
	var f format.Format
	if m.Format != "" {
		if f, err = format.Load("", m.Format); err != nil {
			return m, agency.Choice{}, err
		}
		ask.Format = &f
	}
	ch := agency.Choose(ctx, client, menu, ask)
	if ctx.Err() != nil {
		return m, ch, ctx.Err()
	}
	if err := writeJSON(filepath.Join(dir, agency.ChoiceFile), ch); err != nil {
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
	if m.Format != "" {
		path, err := f.Materialize(played.RoutePath, filepath.Join(dir, "format"))
		if err != nil {
			return m, ch, err
		}
		played.RoutePath = mustAbs(path)
	}
	return played, ch, nil
}
