package strategy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Aug presence (docs/re/AUGS.md §1.6, §4.1): hidden, but every change leaves a visible trace,
// so it is rebuilt exactly from the run's history: 15 on the intro day, + rise for each
// "an Aug visited your store yesterday" night-log line, − drop (floored at 0) for each good
// kill. At 100 the run ends on hard and challenge, and on any difficulty with Aug Menace.

const (
	augIntroDay      = 12
	augStartPresence = 15
	augGameOver      = 100
	// augAllClearDay: from here, presence exactly 0 with the business permit held 25 days
	// ends the aug threat for good.
	augAllClearDay = 62
)

type augRules struct{ rise, drop int }

var augByDifficulty = map[string]augRules{
	"cozy": {10, 25}, "normal": {10, 25}, "hard": {15, 10}, "challenge": {16, 8},
}

// augHistory is what the strategist has seen of the run's augs, by game day.
type augHistory struct {
	// escapes[d] is the escape lines in day d's morning night log (augs left alive on d-1).
	escapes map[int]int
	// kills[d] is the good kills on day d.
	kills map[int]int
}

func (h *augHistory) noteMorning(day int, nightLog []string) {
	n := 0
	for _, l := range nightLog {
		if isAugEscapeLine(l) {
			n++
		}
	}
	if h.escapes == nil {
		h.escapes = map[int]int{}
	}
	h.escapes[day] = n
}

func (h *augHistory) noteKill(day int) {
	if h.kills == nil {
		h.kills = map[int]int{}
	}
	h.kills[day]++
}

// isGoodKill reads the mod's `aug` record event (docs/PROTOCOL.md): event "good_kill". The
// event is sent for every shot, asked about or not; the matching outcome only when an
// aug_check decision existed, so kills are counted from the event alone.
func isGoodKill(data []byte) bool {
	var r struct {
		Event string `json:"event"`
	}
	return json.Unmarshal(data, &r) == nil && r.Event == "good_kill"
}

func isAugEscapeLine(l string) bool {
	l = strings.ToLower(l)
	return strings.Contains(l, "aug visited your store")
}

// AugTrack is the presence rebuilt from history, as of a morning.
type AugTrack struct {
	Escapes, GoodKills, Presence int
	// Partial: the history does not reach back to the intro day (a reload, a gap), so
	// Presence is a lower bound.
	Partial bool
}

// track rebuilds presence as of day; intro is the aug intro day. Within a day the order
// of a kill and an escape is not visible, so a day's kills are applied before the escapes
// its next morning reports.
func (h *augHistory) track(day, intro int, r augRules) AugTrack {
	t := AugTrack{Presence: augStartPresence}
	for d := intro; d <= day; d++ {
		if d > intro {
			for range h.escapes[d] {
				t.Escapes++
				t.Presence += r.rise
			}
		}
		for range h.kills[d] {
			t.GoodKills++
			t.Presence = max(0, t.Presence-r.drop)
		}
	}
	if _, ok := h.escapes[intro+1]; !ok && day > intro {
		t.Partial = true
	}
	return t
}

// escapesLeft is how many more escapes the run survives (docs/re/AUGS.md §5.2).
func escapesLeft(presence, rise int) int {
	return (augGameOver-presence+rise-1)/rise - 1
}

var augBands = []struct {
	min        int
	name, rule string
}{
	{3, "SAFE", "shoot only on a zero-false-positive tell (stillness, twitch, aug portrait, scanner)"},
	{2, "CAUTION", "shoot on a strong tell, or two weak ones plus a haggle refusal"},
	{1, "DANGER", "shoot when an aug is more likely than not; never threaten a suspected aug"},
	{0, "CRITICAL", "the next escape ends the run: scan or probe, then shoot on any real suspicion; a wrongful kill is recoverable, this is not"},
}

// augSetup is the shop's aug rules and intro day; ok false when the difficulty is unknown.
func (s *snapshot) augSetup() (r augRules, intro int, ok bool) {
	r, ok = augByDifficulty[strings.ToLower(s.storeExtra["difficulty"])]
	intro = augIntroDay
	if s.augMenace() {
		intro = 1
		if !ok {
			r, ok = augByDifficulty["normal"], true
		}
	}
	return r, intro, ok
}

// trackFor rebuilds the aug presence as of day for the shop s describes; nil when its
// difficulty is unknown or the augs have not arrived.
func (h *augHistory) trackFor(s *snapshot, day int) *AugTrack {
	r, intro, ok := s.augSetup()
	if !ok || day < intro {
		return nil
	}
	t := h.track(day, intro, r)
	return &t
}

func (s *snapshot) augMenace() bool {
	if s.hasPerk("aug_menace") {
		return true
	}
	for k, v := range s.storeExtra {
		if strings.Contains(strings.ToLower(k+" "+v), "aug_menace") || strings.Contains(strings.ToLower(k+" "+v), "aug menace") {
			return true
		}
	}
	return false
}

// augCondition is the aug loss condition, on difficulties where full presence ends the run.
func (s *snapshot) augCondition(day int) (lossCond, bool) {
	diff := strings.ToLower(s.storeExtra["difficulty"])
	menace := s.augMenace()
	rules, known := augByDifficulty[diff]
	if !menace && diff != "hard" && diff != "challenge" {
		return lossCond{}, false
	}
	if !known {
		rules = augByDifficulty["hard"]
	}
	intro := augIntroDay
	if menace {
		intro = 1
	}
	c := lossCond{key: "aug", refs: []string{"aug"}, rank: 50}
	if day < intro {
		c.text = fmt.Sprintf("aug presence: starts at %d on day %d; 100 ends the run", augStartPresence, intro)
		return c, true
	}
	t := AugTrack{Presence: augStartPresence, Partial: true}
	if s.aug != nil {
		t = *s.aug
	}
	left := escapesLeft(t.Presence, rules.rise)
	band := augBands[len(augBands)-1]
	for _, b := range augBands {
		if left >= b.min {
			band = b
			break
		}
	}
	src := fmt.Sprintf("%d escape(s) +%d each, %d good kill(s) -%d each", t.Escapes, rules.rise, t.GoodKills, rules.drop)
	if t.Partial {
		src += "; history incomplete, at least this"
	}
	rule := band.rule
	if !strings.EqualFold(s.storeExtra["aug_detection"], "tells") {
		// Detection is assumed perfect (docs/PROTOCOL.md): every aug is known at the counter.
		kills := (t.Presence + rules.drop - 1) / rules.drop
		rule = fmt.Sprintf("shoot every aug (a good kill: -%d presence, rep gains, no crime) unless the gun is unavailable; never threaten or serve one; %d good kill(s) reach 0, and from day %d presence 0 with the business permit held 25 days ends the augs for good", rules.drop, kills, augAllClearDay)
	}
	c.text = fmt.Sprintf("aug presence %d/100 (%s): %d more escape(s) survivable, %s: %s", t.Presence, src, max(left, 0), band.name, rule)
	c.rank = int64(max(left, 0)) * 3
	c.imminent = left <= 0
	return c, true
}

// renderAugs states the presence on every difficulty once the augs are introduced, and the
// all-clear target late in the run.
func (s *snapshot) renderAugs(b *strings.Builder, day int) {
	if s.aug == nil {
		return
	}
	if _, ok := s.augCondition(day); ok {
		return
	}
	fmt.Fprintf(b, "Augs: presence %d (no game over on this difficulty; %d escape(s), %d good kill(s)).", s.aug.Presence, s.aug.Escapes, s.aug.GoodKills)
	if day >= augAllClearDay-10 {
		fmt.Fprintf(b, " All-clear from day %d: presence exactly 0 and the business permit held 25 days end the augs for good.", augAllClearDay)
	}
	b.WriteString("\n")
}
