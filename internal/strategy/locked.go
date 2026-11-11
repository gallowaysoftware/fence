package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// lintLocked drops changes to the keys the base policy locks (a stream format's handicaps,
// internal/format): the executor ignores them, so the panel would show a plan that is not played.
func lintLocked(d *protocol.Directives, base string) []string {
	var b struct {
		Locked []string `json:"locked"`
	}
	if json.Unmarshal([]byte(base), &b) != nil || len(b.Locked) == 0 {
		return nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	var fixes []string
	for _, path := range b.Locked {
		keys := strings.Split(path, ".")
		obj := m
		for _, k := range keys[:len(keys)-1] {
			if obj, _ = obj[k].(map[string]any); obj == nil {
				break
			}
		}
		last := keys[len(keys)-1]
		if _, ok := obj[last]; !ok {
			continue
		}
		delete(obj, last)
		if len(keys) > 1 && len(obj) == 0 {
			delete(m, keys[0])
		}
		fixes = append(fixes, fmt.Sprintf("%s: locked by the run's format; the base stands", path))
	}
	if len(fixes) == 0 {
		return nil
	}
	out, _ := json.Marshal(m)
	var nd protocol.Directives
	if json.Unmarshal(out, &nd) == nil {
		*d = nd
	}
	return fixes
}
