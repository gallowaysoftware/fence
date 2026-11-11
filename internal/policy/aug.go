package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// The aug_check decision (docs/PROTOCOL.md, docs/re/AUGS.md §5): an aug, or in tells mode a
// suspected one, is at the counter and the gun can be raised. Advice only: the mod never
// raises or fires the gun, scans, probes or threatens on this kind.
const (
	augCheckKind      = "aug_check"
	optShoot          = "shoot"
	optScan           = "scan"
	optProbeMarkup    = "probe_markup_10"
	optProbeDiscount  = "probe_discount_10"
	optThreaten       = "threaten"
	optServe          = "serve"
	optDismissAug     = "dismiss"
	detectionPerfect  = "perfect"
	detectionTells    = "tells"
	scanAugDetected   = "AUG DETECTED"
	scanNoAugDetected = "No Aug detected"
)

type augCtx struct {
	Detection    string     `json:"detection"`
	Client       *augClient `json:"client"`
	GunRaised    *bool      `json:"gun_raised"`
	ThreatenUsed *bool      `json:"threaten_used"`
	Run          *augRun    `json:"run"`
}

type augClient struct {
	Name     string   `json:"name"`
	Faction  string   `json:"faction"`
	Intent   string   `json:"intent"`
	Wants    []string `json:"wants"`
	Budget   *int64   `json:"budget"`
	Aug      *bool    `json:"aug"`
	Tells    []string `json:"aug_tells"`
	Portrait string   `json:"portrait"`
	Idle     string   `json:"idle"`
	Lines    []string `json:"lines"`
	Haggle   string   `json:"haggle"`
	Scan     string   `json:"scan"`
}

type augRun struct {
	Day               *int64 `json:"day"`
	DaysSinceIntro    *int64 `json:"days_since_intro"`
	Difficulty        string `json:"difficulty"`
	PresenceRise      *int64 `json:"presence_rise"`
	KillPresenceDrop  *int64 `json:"kill_presence_drop"`
	GameOverAt100     *bool  `json:"game_over_at_100"`
	AugMenace         *bool  `json:"aug_menace"`
	KillReward        *int64 `json:"kill_reward"`
	ManslaughterCrime *int64 `json:"manslaughter_crime"`
}

func parseAug(raw json.RawMessage) (*augCtx, error) {
	var c augCtx
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Client == nil {
		return nil, errors.New("aug_check context has no client")
	}
	return &c, nil
}

// augBelief is what the visible state proves: known says whether it settles the question.
type augBelief struct {
	known, aug, stated bool
	why                string
}

// belief reads only zero-false-positive evidence (AUGS.md §2): the stated status in perfect
// mode, a scanner verdict, or a tell no human ever shows. A haggle refusal or the shopping
// pattern is suspicion, not proof.
func (c *augCtx) belief() augBelief {
	cl := c.Client
	switch {
	case cl.Scan == scanAugDetected:
		return augBelief{known: true, aug: true, why: "the scanner says AUG DETECTED"}
	case cl.Scan == scanNoAugDetected:
		return augBelief{known: true, why: "the scanner says no aug"}
	case cl.Aug != nil && *cl.Aug:
		return augBelief{known: true, aug: true, stated: true, why: "a confirmed aug"}
	case cl.Aug != nil:
		return augBelief{known: true, stated: true, why: "not an aug"}
	case len(cl.Tells) > 0:
		return augBelief{known: true, aug: true, why: "a tell no human shows: " + cl.Tells[0]}
	}
	return augBelief{why: "no definitive tell"}
}

func (c *augCtx) rise() string {
	if c.Run != nil && c.Run.PresenceRise != nil {
		return fmt.Sprintf("presence +%d", *c.Run.PresenceRise)
	}
	return "presence rises"
}

func (c *augCtx) gameOver() bool {
	return c.Run != nil && (isTrue(c.Run.GameOverAt100) || isTrue(c.Run.AugMenace))
}

func augNotes(c *augCtx) func(id, label string) string {
	b := c.belief()
	crime := "manslaughter"
	if c.Run != nil && c.Run.ManslaughterCrime != nil {
		crime = fmt.Sprintf("manslaughter (crime %d)", *c.Run.ManslaughterCrime)
	}
	return func(id, _ string) string {
		switch {
		case b.known && b.aug:
			switch id {
			case optShoot:
				return "confirmed aug: a good kill"
			case optServe, optDismissAug:
				return "lets a confirmed aug go: " + c.rise()
			case optThreaten:
				return "forfeits the shot: the aug escapes, " + c.rise()
			case optScan, optProbeMarkup, optProbeDiscount:
				return "no need: the aug is confirmed"
			}
		case b.known:
			switch id {
			case optShoot:
				return "a human: " + crime
			case optServe:
				return "a human: serve as usual"
			case optThreaten:
				return "a human: -7 rep, or -10 and they leave"
			}
		default:
			switch id {
			case optShoot:
				return "no definitive tell: a wrongful kill is " + crime
			case optScan:
				return "exact, and the gun stays available"
			case optProbeMarkup, optProbeDiscount:
				return "an aug always refuses and stays; a refusal is suspicion, not proof"
			case optThreaten:
				return "exact, but forfeits the shot"
			case optServe, optDismissAug:
				return "if an aug: " + c.rise()
			}
		}
		return ""
	}
}

func renderAug(b *strings.Builder, c *augCtx) {
	cl := c.Client
	mode := c.Detection
	if mode == "" {
		mode = detectionPerfect
	}
	fmt.Fprintf(b, "Aug check (detection: %s)\n", mode)
	fmt.Fprintf(b, "Customer: %s", cl.Name)
	if cl.Faction != "" {
		fmt.Fprintf(b, " (bubble suggests %s)", factionName(cl.Faction))
	}
	if cl.Intent != "" {
		fmt.Fprintf(b, ", wants to %s", strings.ReplaceAll(cl.Intent, "_", " and "))
	}
	if len(cl.Wants) > 0 {
		fmt.Fprintf(b, " %s", strings.Join(cl.Wants, ", "))
	}
	if cl.Budget != nil {
		fmt.Fprintf(b, ", budget %d", *cl.Budget)
	}
	b.WriteString("\n")
	if cl.Aug != nil {
		if *cl.Aug {
			b.WriteString("- STATUS: AUG (confirmed)\n")
		} else {
			b.WriteString("- STATUS: human (not an aug)\n")
		}
	}
	for _, t := range cl.Tells {
		fmt.Fprintf(b, "- TELL: %s\n", t)
	}
	if cl.Portrait != "" {
		fmt.Fprintf(b, "- portrait: %s\n", cl.Portrait)
	}
	if cl.Idle != "" {
		fmt.Fprintf(b, "- idle: %s\n", cl.Idle)
	}
	for _, l := range cl.Lines {
		fmt.Fprintf(b, "- said: %q\n", l)
	}
	if cl.Haggle != "" {
		fmt.Fprintf(b, "- haggle: %s\n", cl.Haggle)
	}
	if cl.Scan != "" {
		fmt.Fprintf(b, "- scanner: %s\n", cl.Scan)
	}
	if isTrue(c.GunRaised) {
		b.WriteString("Gun: raised (lowering it without a shot sends the customer away)\n")
	}
	if isTrue(c.ThreatenUsed) {
		b.WriteString("Threaten already used.\n")
	}
	if r := c.Run; r != nil {
		var parts []string
		if r.Day != nil {
			p := fmt.Sprintf("day %d", *r.Day)
			if r.DaysSinceIntro != nil {
				p += fmt.Sprintf(" (%d since the augs began)", *r.DaysSinceIntro)
			}
			parts = append(parts, p)
		}
		if r.Difficulty != "" {
			parts = append(parts, r.Difficulty)
		}
		if r.PresenceRise != nil {
			parts = append(parts, fmt.Sprintf("each escape +%d presence", *r.PresenceRise))
		}
		if r.KillPresenceDrop != nil {
			parts = append(parts, fmt.Sprintf("each good kill -%d", *r.KillPresenceDrop))
		}
		if c.gameOver() {
			parts = append(parts, "presence 100 ENDS THE RUN")
		} else {
			parts = append(parts, "full presence does not end the run")
		}
		if r.KillReward != nil {
			parts = append(parts, fmt.Sprintf("kill reward about %d", *r.KillReward))
		}
		if r.ManslaughterCrime != nil {
			parts = append(parts, fmt.Sprintf("a wrongful kill adds manslaughter %d", *r.ManslaughterCrime))
		}
		fmt.Fprintf(b, "Run: %s.\n", strings.Join(parts, "; "))
	}
	bl := c.belief()
	switch {
	case bl.known && bl.aug:
		fmt.Fprintf(b, "RULE CHECK: %s; shooting is a good kill.\n", bl.why)
	case bl.known:
		fmt.Fprintf(b, "RULE CHECK: %s; shooting is a wrongful kill.\n", bl.why)
	default:
		b.WriteString("RULE CHECK: no definitive tell; shooting risks a wrongful kill. Scan if you can; the plan's aug band says how much suspicion justifies a shot.\n")
	}
}

func augHeuristic(req *protocol.DecideRequest) Decision {
	c, err := parseAug(req.Context)
	if err != nil {
		return heuristicDefer("aug_check context does not have the expected shape")
	}
	opts := inspectOptions(req)
	has := func(id string) bool { _, ok := opts[id]; return ok }
	b := c.belief()
	switch {
	case b.known && b.aug && has(optShoot):
		why := "Confirmed aug and the gun is ready: a good kill"
		if r := c.Run; r != nil && r.KillReward != nil && r.KillPresenceDrop != nil {
			why += fmt.Sprintf(" pays about %d and drops presence %d", *r.KillReward, *r.KillPresenceDrop)
		}
		if !b.stated {
			why = strings.Replace(why, "Confirmed aug", "Aug proven ("+b.why+")", 1)
		}
		return choose(optShoot, why+".")
	case b.known && has(optServe):
		if b.aug {
			return choose(optServe, "An aug, but the gun is not available: serve them.")
		}
		return choose(optServe, "Not an aug ("+b.why+"): serve them normally.")
	case !b.known && has(optScan):
		return choose(optScan, "Suspicious but no definitive tell: the scanner answers exactly and keeps the shot.")
	case !b.known && has(optServe):
		return choose(optServe, "No definitive tell: a wrongful kill costs more than one escape ("+c.rise()+").")
	}
	return heuristicDefer("no option fits what is known")
}

// checkAugChoice refuses a shot nothing visible supports (AUGS.md §5.1): a customer stated or
// scanned human, or one with no tell, no scanner verdict and no haggle refusal at all. How
// much suspicion a shot needs short of proof is the plan's aug band, not this check.
func checkAugChoice(req *protocol.DecideRequest, choice protocol.Choice) error {
	if choice.Option != optShoot {
		return nil
	}
	c, err := parseAug(req.Context)
	if err != nil {
		return errors.New("the aug_check context does not parse, so a shot cannot be justified")
	}
	b := c.belief()
	if b.known && !b.aug || !b.known && c.Client.Haggle == "" {
		return fmt.Errorf("option %q is not justified: %s; shooting a human is manslaughter. Pick %q or %q, or %q if offered", optShoot, b.why, optServe, optDismissAug, optScan)
	}
	return nil
}

// augSystem states the rules from docs/re/AUGS.md in our own words.
var augSystem = strings.TrimSpace(`
You advise the owner of a pawnshop in the cyberpunk game "Probably Stolen". Augs (augmented people posing as customers) come about once a week; the owner has a shotgun. Decide what to do with the customer at the counter. You perceive like an expert player: the prompt states the customer's aug status when detection is "perfect", or lists every visible tell in words when it is "tells".

CONSEQUENCES
- shoot an aug: a good kill. Cash about 100 + days since the augs began (max 250), rep +2 Lower Levels, +2 Black Market, +2 Revolution, +8 Security, and presence drops (25 on cozy/normal, 10 on hard, 8 on challenge).
- shoot a human: MANSLAUGHTER crime of 250 (before day 25), 500 (from day 25) or 750 (from day 50) feeding the Security investigation, rep -15 Lower Levels, -15 Security, -10 Revolution, -10 Black Market (Black Market losses count double). No game over, but it can push Security toward a gun-permit revocation and the investigation into its fast band.
- every other exit of an aug counts as LEFT ALIVE: serving, dismissing, reporting, threatening, raising the gun and lowering it. Presence rises (10 on cozy/normal, 15 hard, 16 challenge). On hard, challenge, or with Aug Menace, presence 100 ends the run at the next sleep; on cozy/normal it only delays the all-clear.
- threaten: an aug always accepts (+20% deal) and a human always denies (-7 rep, or 50% -10 and they leave), so it is exact, but a threatened customer can no longer be shot: the aug escapes.
- probe (10% markup or demanded discount): an aug always refuses and stays; a human accepts, refuses-and-stays (8-20%) or walks out (-3 rep). A refusal is suspicion, not proof; an acceptance or a walk-out proves human.
- scan (aug scanner, 25 energy): exact, and the gun stays available.

TELLS THAT NEVER APPEAR ON HUMANS: perfectly still (no bob at all), a fast side-to-side twitch, an aug portrait (unusual pupils, uncanny expression, visible chip), the aug-only stiff/robotic lines, a generic "Upper Level Citizen" or "Thief" talking in a plain bubble, the scanner's AUG DETECTED. Augs buy medical supplies, or materials and energy cells, with budgets of 150-250: that pattern alone is suspicion only. Outfits from another trade are a weak tell.

JUDGMENT
- A confirmed aug (stated, scanned, or a tell above) with the gun available: shoot. Never threaten or serve one.
- A human (stated or scanned): serve normally.
- Uncertain: scan if offered. Otherwise follow the plan's aug band: with escapes to spare, shoot only on proof and serve the rest; when the next escape ends the run, a haggle refusal on top of the shopping pattern justifies the shot, because a wrongful kill is recoverable and the game over is not. Never shoot on the shopping pattern alone.
` + answerRules)
