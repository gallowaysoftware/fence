package show

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// PlanInput is what is known about one day's plan.
type PlanInput struct {
	Day int
	// Text is the strategist's prose; "" when only the directives were recorded.
	Text       string
	Directives *protocol.Directives
	// FromModel are the directive keys the model itself set; Fixes are the bridge's
	// corrections to them.
	FromModel []string
	Fixes     []string
	// Why is the model's reason for each key it set ("*": one for all).
	Why map[string]string
	// Failed is set when the model's plan failed and yesterday's stands.
	Failed bool
	// Extra are instructions in words that Directives cannot hold (the gym's own levers).
	Extra []string
}

// ReadBridgeLog collects each day's plan from a bridge's slog text output (bridge.log in
// a league match). The bridge logs the directives and its corrections; the prose plan and
// the model's reasons only since it logs plan= and why= (docs/SHOW.md).
func ReadBridgeLog(r io.Reader) (map[int]*PlanInput, error) {
	out := map[int]*PlanInput{}
	get := func(day int) *PlanInput {
		p := out[day]
		if p == nil {
			p = &PlanInput{Day: day}
			out[day] = p
		}
		return p
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, "day=") {
			continue
		}
		kv := logfmt(line)
		day, err := strconv.Atoi(kv["day"])
		if err != nil || day <= 0 {
			continue
		}
		switch kv["msg"] {
		case "day plan written":
			if t := kv["plan"]; t != "" {
				get(day).Text = t
			}
		case "directives":
			var d protocol.Directives
			if json.Unmarshal([]byte(kv["json"]), &d) == nil {
				get(day).Directives = &d
			}
			var why map[string]string
			if json.Unmarshal([]byte(kv["why"]), &why) == nil && len(why) > 0 {
				get(day).Why = why
			}
		case "directives fixed":
			p := get(day)
			p.FromModel = strings.Fields(strings.Trim(kv["from_model"], "[]"))
			p.Fixes = splitFixes(kv["fixes"])
		case "day plan failed; keeping the previous plan":
			get(day).Failed = true
		}
	}
	return out, sc.Err()
}

// logfmt parses one slog TextHandler line: key=value pairs, values bare or Go-quoted.
func logfmt(line string) map[string]string {
	kv := map[string]string{}
	for i := 0; i < len(line); {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		eq := strings.IndexByte(line[i:], '=')
		if eq < 0 {
			break
		}
		key := line[i : i+eq]
		i += eq + 1
		if i < len(line) && line[i] == '"' {
			j := i + 1
			for j < len(line) && line[j] != '"' {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(line) {
				break
			}
			v, err := strconv.Unquote(line[i : j+1])
			if err != nil {
				v = line[i+1 : j]
			}
			kv[key] = v
			i = j + 1
			continue
		}
		j := strings.IndexByte(line[i:], ' ')
		if j < 0 {
			j = len(line) - i
		}
		kv[key] = line[i : i+j]
		i += j
	}
	return kv
}

// fixStart finds where one correction begins in the bridge's space-joined list: a directive
// key followed by a colon, or by an optional value and "dropped"/"reverted".
var fixStart = regexp.MustCompile(`(?:^|\s)(?:cash_floor|buy|sell|bargain|window|services|upgrades_buy|permit|production|augs|scavenge|engine|mortgage[a-z_]*)(?:\.[a-z_]+)?(?::|(?: \S+)? (?:dropped|reverted))`)

func splitFixes(s string) []string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"))
	if s == "" {
		return nil
	}
	idx := fixStart.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return []string{s}
	}
	var out []string
	for k, m := range idx {
		end := len(s)
		if k+1 < len(idx) {
			end = idx[k+1][0]
		}
		if t := strings.TrimSpace(s[m[0]:end]); t != "" {
			out = append(out, t)
		}
	}
	if pre := strings.TrimSpace(s[:idx[0][0]]); pre != "" {
		out = append([]string{pre}, out...)
	}
	return out
}
