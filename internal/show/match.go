package show

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/route"
)

// Match is a cmd/fenceleague match directory as the show reads it.
type Match struct {
	Dir    string
	Player Player
	// LogFile is the bridge's decision log for the run; BridgeLog its slog output.
	LogFile   string
	BridgeLog string
	// Listen is the bridge's address, for /v1/summary while the match runs.
	Listen string
	Plans  map[int]*PlanInput
	// Whys are the gym executor's per-customer reasons, by day and client seq.
	Whys map[int]map[int64]string
	// Base is the bridge's -base-directives: the executor's standing plan.
	Base *protocol.Directives
	// Ending and Days come from result.json once the match has finished.
	Ending   string
	Days     int
	NetWorth int64
}

type matchJSON struct {
	Match  string `json:"match"`
	Player struct {
		ID       string `json:"id"`
		Endpoint string `json:"endpoint"`
		Mode     string `json:"mode"`
	} `json:"player"`
	Model      string   `json:"model"`
	Seed       int64    `json:"seed"`
	Start      string   `json:"start"`
	Perks      string   `json:"perks"`
	Difficulty string   `json:"difficulty"`
	Format     string   `json:"format"`
	BridgeArgs []string `json:"bridge_args"`
}

// LoadMatch reads what the show needs from a match directory. Only the decision log is
// required; the bridge log, gym summary and result enrich it when present.
func LoadMatch(dir, personaDir string) (*Match, error) {
	m := &Match{Dir: dir}
	var mj matchJSON
	if b, err := os.ReadFile(filepath.Join(dir, "match.json")); err == nil {
		if err := json.Unmarshal(b, &mj); err != nil {
			return nil, fmt.Errorf("%s/match.json: %w", dir, err)
		}
	}
	m.Player = Player{ID: mj.Player.ID, Model: mj.Model, Endpoint: mj.Player.Endpoint, Difficulty: mj.Difficulty,
		Start: mj.Start, Seed: mj.Seed, Planner: mj.Player.Mode}
	if mj.Perks != "" && mj.Perks != "none" {
		m.Player.Perks = strings.Split(mj.Perks, "+")
	}
	if m.Player.ID == "" {
		m.Player.ID = filepath.Base(dir)
	}
	logDir := filepath.Join(dir, "log")
	var routeRef, baseFile string
	for i, a := range mj.BridgeArgs {
		if i+1 >= len(mj.BridgeArgs) {
			break
		}
		switch a {
		case "-persona":
			m.Player.Persona = mj.BridgeArgs[i+1]
		case "-route":
			routeRef = mj.BridgeArgs[i+1]
			m.Player.SetRoute(routeRef)
		case "-listen":
			m.Listen = mj.BridgeArgs[i+1]
		case "-log-dir":
			logDir = mj.BridgeArgs[i+1]
		case "-base-directives":
			baseFile = mj.BridgeArgs[i+1]
		}
	}
	m.Base, _ = BaseDirectives(routeRef, baseFile)
	// A replay of a format run shows the format; an unreadable one is left off the card.
	_ = m.Player.SetFormat("", mj.Format, format.DefaultRecord, filepath.Base(filepath.Clean(dir)))
	if m.Player.Persona != "" {
		m.Player.SetPersona(personaDir, m.Player.Persona, "")
	}
	if !filepath.IsAbs(logDir) {
		logDir = filepath.Join(dir, "log")
	}
	m.LogFile = NewestLog(logDir)
	if m.LogFile == "" {
		return nil, fmt.Errorf("%s: no decision log in %s", dir, logDir)
	}
	if f, err := os.Open(filepath.Join(dir, "bridge.log")); err == nil {
		m.BridgeLog = f.Name()
		m.Plans, err = ReadBridgeLog(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.BridgeLog, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "gym.summary.json")); err == nil {
		m.Whys = gymWhys(b)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "result.json")); err == nil {
		var r struct {
			Ending     string  `json:"ending"`
			DaysPlayed int     `json:"days_played"`
			NW         []int64 `json:"net_worth_by_day"`
		}
		if json.Unmarshal(b, &r) == nil {
			m.Ending, m.Days = r.Ending, r.DaysPlayed
			if len(r.NW) > 0 {
				m.NetWorth = r.NW[len(r.NW)-1]
			}
		}
	}
	return m, nil
}

// Builder returns a builder primed with the match's player, plans and reasons.
func (m *Match) Builder() *Builder {
	b := NewBuilder(m.Player)
	b.Base = m.Base
	if m.Plans != nil {
		b.Plans = func(day int) *PlanInput { return m.Plans[day] }
	}
	if m.Whys != nil {
		b.Why = func(day int, seq int64) string { return m.Whys[day][seq] }
	}
	return b
}

// NewestLog is the most recently modified decision log (*.jsonl) in dir, "" for none.
func NewestLog(dir string) string {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	var best string
	var bestT int64
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if t := st.ModTime().UnixNano(); best == "" || t > bestT {
			best, bestT = p, t
		}
	}
	return best
}

func gymWhys(b []byte) map[int]map[int64]string {
	var g struct {
		Days []struct {
			Day       int `json:"day"`
			Customers []struct {
				Seq int64  `json:"seq"`
				Why string `json:"why"`
			} `json:"customers"`
		} `json:"player_days"`
	}
	if json.Unmarshal(b, &g) != nil {
		return nil
	}
	out := map[int]map[int64]string{}
	for _, d := range g.Days {
		for _, c := range d.Customers {
			if c.Why == "" {
				continue
			}
			if out[d.Day] == nil {
				out[d.Day] = map[int64]string{}
			}
			out[d.Day][c.Seq] = c.Why
		}
	}
	return out
}

// LoadPersonaText reads docs/personas/<id>.md (or a path).
func LoadPersonaText(dir, id string) (string, error) {
	if id == "" {
		return "", errors.New("no persona")
	}
	p := id
	if !strings.ContainsAny(id, "/.") {
		p = filepath.Join(dir, id+".md")
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

// Board reads finished matches into scoreboard rows, oldest first.
func Board(dirs []string, personaDir string) []BoardRow {
	var rows []BoardRow
	for _, d := range dirs {
		m, err := LoadMatch(d, personaDir)
		if err != nil || m.Ending == "" {
			continue
		}
		rows = append(rows, BoardRow{Run: filepath.Base(d), Player: m.Player.ID, Model: m.Player.Model,
			Persona: m.Player.PersonaName, Difficulty: m.Player.Difficulty, Ending: m.Ending, Days: m.Days, NetWorth: m.NetWorth, Won: Won(m.Ending)})
	}
	slices.SortStableFunc(rows, func(a, b BoardRow) int { return strings.Compare(a.Run, b.Run) })
	return rows
}

// BaseDirectives is the executor's base policy as the bridge reads it: the -base-directives
// file, else the -route's directives; nil when neither is given.
func BaseDirectives(routeRef, file string) (*protocol.Directives, error) {
	var b []byte
	switch {
	case file != "":
		var err error
		if b, err = os.ReadFile(file); err != nil {
			return nil, err
		}
	case routeRef != "":
		r, err := route.Load("", routeRef)
		if err != nil {
			return nil, err
		}
		b = []byte(r.Base)
	default:
		return nil, nil
	}
	var d protocol.Directives
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
