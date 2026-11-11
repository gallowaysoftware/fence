package show

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The stream's big beats (docs/STREAM-OPS.md "Show beats"): the appraisal board, the mortgage from
// signature to last payment, the gun, and the autopilot handing over to its human.

// boardMember is the appraisal board's client name in the game's English text; the live autopilot
// names the dialogue by client id instead.
const (
	boardMember      = "Appraiser Board Member"
	boardIntroID     = "appraisalBoardIntro"
	boardPromotionID = "appraisalBoardPromotion"
)

var (
	creditsRe = regexp.MustCompile(`\(-?(\d[\d,]*) Credits\)`)
	titleRe   = regexp.MustCompile(`promote you to ([A-Z][A-Za-z ]+?)[.!]`)
)

// speaker is who said the last line of a dialogue's history ("Appraiser Board Member: …").
func speaker(history []string) string {
	if len(history) == 0 {
		return ""
	}
	who, _, ok := strings.Cut(history[len(history)-1], ": ")
	if !ok {
		return ""
	}
	return who
}

// promotionTitle is the rank the board's offer names ("Master Appraiser"), "" when none.
func promotionTitle(history []string) string {
	for _, l := range history {
		if m := titleRe.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// choiceCost is the credits a choice's label names ("[Accept (-212 Credits)]"), 0 when none.
func choiceCost(label string) int64 {
	m := creditsRe.FindStringSubmatch(label)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64)
	return n
}

// boardChoice turns an answer to the appraisal board into its beat: joining, or taking a promotion.
// promotion is whether the dialogue is the promotion offer (the live log names it by id; the gym's
// by the choice's own words). Declining is no beat.
func (b *Builder) boardChoice(label string, promotion bool, history []string) []Event {
	l := strings.ToLower(label)
	cost := choiceCost(label)
	switch {
	case strings.Contains(l, "join the board"):
		b.appraisalRank = 0
		b.clock.Cash -= cost
		return []Event{{Kind: KindPurchase, Level: LevelMoment, Beat: BeatAppraisalJoin, Amount: -cost, Clock: b.Clock(),
			Text: fmt.Sprintf("Joined the appraisal board for %d: appraisal customers from now on.", cost)}}
	case promotion && strings.Contains(l, "accept"):
		b.appraisalRank = min(3, b.appraisalRank+1)
		b.clock.Cash -= cost
		to := fmt.Sprintf("rank %d", b.appraisalRank)
		if t := promotionTitle(history); t != "" {
			to = t
		}
		return []Event{{Kind: KindPurchase, Level: LevelMoment, Beat: BeatPromotion, Amount: -cost, Clock: b.Clock(),
			Text: fmt.Sprintf("Promoted by the appraisal board to %s, for %d: bigger appraisals, bigger fees.", to, cost)}}
	}
	return nil
}

// appraisal is one estimate given to an appraisal customer (the gym's `appraisal` event): the
// estimate and the fee are what the player saw; the slip's value is not used.
func (b *Builder) appraisal(e *logEvent) []Event {
	var d struct {
		Type     string `json:"type"`
		Estimate int64  `json:"estimate"`
		Fee      *int64 `json:"fee"`
		Refused  string `json:"refused"`
	}
	if json.Unmarshal(e.Data, &d) != nil || d.Estimate <= 0 {
		return nil
	}
	what := "Appraisal"
	if d.Type != "" {
		what += " (" + strings.ReplaceAll(d.Type, "_", " ") + ")"
	}
	if d.Fee == nil {
		b.appraisalBeat(nil)
		return []Event{{Kind: KindThought, Level: LevelTicker, Text: fmt.Sprintf("%s: estimate %d, refused as too %s.", what, d.Estimate, strings.TrimPrefix(d.Refused, "too_"))}}
	}
	b.clock.Cash += *d.Fee
	return append([]Event{{Kind: KindThought, Level: LevelTicker, Text: fmt.Sprintf("%s: estimate %d, accepted, fee %d.", what, d.Estimate, *d.Fee)}}, b.appraisalBeat(d.Fee)...)
}

type mortgageData struct {
	Price  int64 `json:"price"`
	Down   int64 `json:"down_payment"`
	Weekly int64 `json:"weekly"`
	Weeks  int   `json:"weeks"`
	Amount int64 `json:"amount"`
}

// mortgage handles the mortgage's money events: the signature, each weekly payment, the last one,
// and a buyout in cash. Payments fall on the rent day and replace the rent while it runs.
func (b *Builder) mortgage(kind string, raw json.RawMessage) ([]Event, bool) {
	var d mortgageData
	_ = json.Unmarshal(raw, &d)
	ev := func(level int, beat string, amount int64, format string, a ...any) ([]Event, bool) {
		return []Event{{Kind: KindMoney, Level: level, Beat: beat, Amount: amount, Clock: b.Clock(), Text: fmt.Sprintf(format, a...)}}, true
	}
	switch kind {
	case "mortgage_signed", "mortgage_taken":
		b.clock.Cash -= d.Down
		b.clock.OfferDown, b.clock.OfferWeekly = 0, 0
		if d.Weekly > 0 {
			b.clock.MortgageWeekly, b.clock.MortgageWeeksLeft = d.Weekly, d.Weeks
			b.mortgageWeeks = d.Weeks
		}
		if d.Down > 0 && d.Weekly > 0 {
			return ev(LevelMoment, BeatMortgageSigned, -d.Down, "Signed the mortgage: %s down, then %s a week for %d weeks, and the shop is ours.", num(d.Down), num(d.Weekly), d.Weeks)
		}
		return ev(LevelMoment, BeatMortgageSigned, 0, "Signed the mortgage.")
	case "mortgage_payment", "mortgage_paid":
		left := max(0, b.clock.MortgageWeeksLeft-1)
		b.clock.MortgageWeeksLeft = left
		if left == 0 {
			return ev(LevelMoment, BeatMortgagePayment, -d.Amount, "The last mortgage payment: %s. Nothing more owed on the shop.", num(d.Amount))
		}
		// Sixteen spoken payments in a row sounded alike in the full replay: the first, every
		// fourth and the last two are moments, the rest notable.
		level := LevelNotable
		if left == b.mortgageWeeks-1 || left%4 == 0 || left <= 1 {
			level = LevelMoment
		}
		return ev(level, BeatMortgagePayment, -d.Amount, "Mortgage payment made: %s. %s to go.", num(d.Amount), plural(left, "week"))
	case "mortgage_paid_off":
		b.clock.Owned, b.clock.MortgageWeekly, b.clock.MortgageWeeksLeft = true, 0, 0
		return ev(LevelNotable, "", 0, "The mortgage is paid off: the deed is ours.")
	case "buyout", "property_bought", "store_bought":
		b.clock.Cash -= d.Price
		b.clock.Owned = true
		if d.Price > 0 {
			return ev(LevelMoment, BeatBuyout, -d.Price, "Bought the shop outright: %s.", num(d.Price))
		}
		return ev(LevelMoment, BeatBuyout, 0, "Bought the shop.")
	}
	return nil, false
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// unholster is the gun coming out for a customer. The gym's aug check reads the truth
// (aug_detection "perfect"), which no player has: the entry is marked.
func (b *Builder) unholster(seq int64) []Event {
	text := "Gun out: the customer at the counter is an aug."
	if who := b.clients[seq].name; who != "" {
		text = fmt.Sprintf("Gun out: the %s is an aug.", strings.TrimPrefix(who, "The "))
	}
	ev := Event{Kind: KindThought, Level: LevelNotable, Text: text}
	if b.augDetection == "perfect" {
		ev.HiddenUsed = []string{"aug_truth"}
	}
	return []Event{ev}
}

// runRules notes what a run's start says about how the hands see.
func (b *Builder) runRules(raw json.RawMessage) {
	var d struct {
		AugDetection string `json:"aug_detection"`
	}
	if json.Unmarshal(raw, &d) == nil && d.AugDetection != "" {
		b.augDetection = d.AugDetection
	}
}

// handoverText is what the viewer reads when the autopilot stops for its human.
func handoverText(state, reason string) string {
	reason = strings.TrimSuffix(strings.TrimSpace(reason), ".")
	switch {
	case state == "handover" && reason != "":
		return "Over to the human: " + reason + "."
	case reason != "":
		return "The autopilot stopped (" + reason + "). Over to the human."
	}
	return "The autopilot stopped. Over to the human."
}
