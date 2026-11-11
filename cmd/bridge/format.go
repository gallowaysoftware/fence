package main

import (
	"os"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/route"
)

// formatRoute is the base policy and route text of a run on a stream format: the route's
// directives with the format's handicaps laid over them and locked, and the route's brief
// followed by the format.
func formatRoute(name, routeRef string) (base, text string, err error) {
	f, err := format.Load("", name)
	if err != nil {
		return "", "", err
	}
	path, err := f.RouteFile(routeRef)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if b, err = f.Apply(b); err != nil {
		return "", "", err
	}
	r, err := route.Parse(path, b)
	if err != nil {
		return "", "", err
	}
	return r.Base, r.Text() + "\n" + f.StrategistText(), nil
}
