package policy

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// obj is a JSON object that keeps key order. Grammar-constrained local models emit keys
// in schema order, and "rationale" first lets them reason before committing.
type obj []kv

type kv struct {
	k string
	v any
}

func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(e.k)
		b.Write(k)
		b.WriteByte(':')
		v, err := json.Marshal(e.v)
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// choiceSchema is deliberately looser than ValidateChoice: the root stays a plain object
// (OpenAI strict mode rejects a root anyOf), so which params go with which option is
// checked after decoding, not by the schema.
func choiceSchema(opts []protocol.Option, bounds bool) (json.RawMessage, error) {
	ids := []string{}
	var branches []any
	seen := map[string]bool{}
	for _, o := range opts {
		if !o.Choosable() {
			continue
		}
		ids = append(ids, o.ID)
		props := obj{}
		required := []string{}
		for _, name := range sortedKeys(o.Params) {
			p := o.Params[name]
			ps := obj{{"type", "integer"}, {"description", fmt.Sprintf("integer from %d to %d", *p.Min, *p.Max)}}
			if bounds {
				ps = append(ps, kv{"minimum", *p.Min}, kv{"maximum", *p.Max})
			}
			props = append(props, kv{name, ps})
			required = append(required, name)
		}
		branch := obj{{"type", "object"}, {"properties", props}, {"required", required}, {"additionalProperties", false}}
		key, err := json.Marshal(branch)
		if err != nil {
			return nil, err
		}
		if !seen[string(key)] {
			seen[string(key)] = true
			branches = append(branches, branch)
		}
	}
	ids = append(ids, protocol.OptionDefer)
	empty := obj{{"type", "object"}, {"properties", obj{}}, {"required", []string{}}, {"additionalProperties", false}}
	if k, _ := json.Marshal(empty); !seen[string(k)] {
		branches = append(branches, empty)
	}
	var params any = obj{{"anyOf", branches}}
	if len(branches) == 1 {
		params = branches[0]
	}
	s := obj{
		{"type", "object"},
		{"properties", obj{
			{"rationale", obj{{"type", "string"}}},
			{"option", obj{{"type", "string"}, {"enum", ids}}},
			{"params", params},
		}},
		{"required", []string{"rationale", "option", "params"}},
		{"additionalProperties", false},
	}
	return json.Marshal(s)
}
