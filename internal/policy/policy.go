// Package policy turns a decide request into a choice. Every policy always produces a
// Decision; failure is expressed as a defer, never as an error.
package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

type Policy interface {
	Name() string
	Decide(ctx context.Context, req *protocol.DecideRequest) Decision
}

type Decision struct {
	Choice    protocol.Choice
	Rationale string
	Source    protocol.Source
	// Notes maps option ids to a short display note; nil when there are none.
	Notes map[string]string
	Trace Trace
}

// Trace is what the decision log needs to reconstruct how a choice was reached.
type Trace struct {
	System   string    `json:"system,omitempty"`
	Prompt   string    `json:"prompt,omitempty"`
	Attempts []Attempt `json:"attempts,omitempty"`
	Note     string    `json:"note,omitempty"`
}

type Attempt struct {
	Raw        string `json:"raw"`
	Reasoning  string `json:"reasoning,omitempty"`
	Model      string `json:"model,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	Error      string `json:"error,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
}

// OptionNotes returns a short note per choosable option of req (e.g. "expected 333 (vs
// 340 plain)"), from the visible context and the labels only. It is nil when the kind is
// unknown or the context does not parse.
func OptionNotes(req *protocol.DecideRequest) map[string]string {
	spec, ok := kinds[req.Kind]
	if !ok || spec.Notes == nil || len(req.Context) == 0 {
		return nil
	}
	note, ok := spec.Notes(req.PolicyView().Context)
	if !ok {
		return nil
	}
	var out map[string]string
	for _, o := range req.Options {
		if !o.Choosable() {
			continue
		}
		// The prompt form spells out the roll after " = "; the display keeps the headline.
		head, _, _ := strings.Cut(note(o.ID, o.Label), " = ")
		if head = ClipNote(head); head == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[o.ID] = head
	}
	return out
}

// ClipNote trims s to protocol.MaxOptionNote bytes, never inside a UTF-8 sequence.
func ClipNote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= protocol.MaxOptionNote {
		return s
	}
	cut := protocol.MaxOptionNote - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " ") + "…"
}

func Fallback(reason string) Decision {
	return Decision{Choice: protocol.Defer(), Rationale: reason, Source: protocol.SourceFallback, Trace: Trace{Note: reason}}
}

// ValidateChoice checks c against the options the mod enumerated. The error text is
// fed back to the model in the repair round, so it says what would be valid.
func ValidateChoice(opts []protocol.Option, c protocol.Choice) error {
	if c.Option == protocol.OptionDefer {
		if len(c.Params) > 0 {
			return errors.New(`option "defer" takes no params`)
		}
		return nil
	}
	i := slices.IndexFunc(opts, func(o protocol.Option) bool { return o.ID == c.Option })
	if i < 0 {
		ids := make([]string, 0, len(opts)+1)
		for _, o := range opts {
			if o.Choosable() {
				ids = append(ids, strconv.Quote(o.ID))
			}
		}
		ids = append(ids, strconv.Quote(protocol.OptionDefer))
		return fmt.Errorf("option %q does not exist; valid options are %s", c.Option, strings.Join(ids, ", "))
	}
	opt := opts[i]
	if !opt.Choosable() {
		return fmt.Errorf("option %q has a parameter type this bridge cannot fill", opt.ID)
	}
	var errs []error
	for _, name := range sortedKeys(opt.Params) {
		p := opt.Params[name]
		raw, ok := c.Params[name]
		if !ok {
			errs = append(errs, fmt.Errorf("param %q is required for option %q", name, opt.ID))
			continue
		}
		// Only "int" is Supported today; a new param type adds a case here.
		v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("param %q must be an integer, got %s", name, raw))
			continue
		}
		if v < *p.Min || v > *p.Max {
			errs = append(errs, fmt.Errorf("param %q = %d is outside [%d, %d]", name, v, *p.Min, *p.Max))
		}
	}
	for _, name := range sortedKeys(c.Params) {
		if _, ok := opt.Params[name]; !ok {
			errs = append(errs, fmt.Errorf("param %q is not declared by option %q", name, opt.ID))
		}
	}
	return errors.Join(errs...)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
