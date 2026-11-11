package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/internal/show"
)

// formatPublish adds the format's verdict to a run's end and, when record is set, appends the
// attempt to the local record the card reads.
func formatPublish(pub func(show.Event) show.Event, p *show.Player, record string, log *slog.Logger) func(show.Event) show.Event {
	if p == nil || p.Format == nil {
		return pub
	}
	return func(ev show.Event) show.Event {
		if a, ok := show.FormatEnd(&ev, p, "stream"); ok && record != "" {
			if a.Run == "" {
				a.Run = fmt.Sprintf("%s:s%d", p.ID, p.Seed)
			}
			if _, err := format.Append(record, a); err != nil {
				log.Warn("format record", "err", err)
			}
		}
		return pub(ev)
	}
}

// formatBase is the executor's base policy on a format: the route's directives with the handicaps
// laid over them, for the standing plan's goals.
func formatBase(dir, name, routeRef string) (*protocol.Directives, error) {
	f, err := format.Load(dir, name)
	if err != nil {
		return nil, err
	}
	path, err := f.RouteFile(routeRef)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if b, err = f.Apply(b); err != nil {
		return nil, err
	}
	base, err := route.StripMeta(b)
	if err != nil {
		return nil, err
	}
	var d protocol.Directives
	if err := json.Unmarshal([]byte(base), &d); err != nil {
		return nil, fmt.Errorf("format %s: %w", name, err)
	}
	return &d, nil
}
