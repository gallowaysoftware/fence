package expert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const gameBrief = `Probably Stolen is a shop sim on a space station. Each day the player buys items from
sellers and sells to buyers at a counter (haggling, judging stolen goods by heat), keeps six
faction reputations (Lower Levels, Upper Levels, Black Market, Security, Cartel, Tourists),
pays weekly rent that rises, and at night buys services (insurance, janitorial), Wild's Network
upgrades (showcase, renovations, referrals, landlord grace, permits, trades like water trader,
winemaker, retired farmer/gunsmith/chemist, aquaculture, vending machine) with cash or Wild favor,
and goes on outings (dumping ground scavenging). Engines: wine, water purifying, furnace/ores,
chemistry, guns, rats/fish, vending. Starts: Apprentice, Water Merchant, Fence, Farmer, Gunsmith,
Chemist, Rat Wrangler, Aquafarmer, Scavenger, Rock Bottom. Difficulties: Price Gouge (cozy),
Mark Up (normal), Thin Margins (hard), Red Ink (challenge). Win: buy the shop (deed) or pay off
the mortgage.`

const ExtractSystem = `You read auto-generated transcripts of a skilled player narrating their own play of a
video game and turn the narration into structured decision records. Report only what the
player says or clearly does; never invent days, numbers or outcomes. Paraphrase in your own
words; do not copy sentences. Answer with one JSON object only.`

func ExtractPrompt(v Video, chunk, of int, text string) string {
	return fmt.Sprintf(`%s

Video: %q by %s. Part %d of %d of the transcript. Lines start with [m:ss] timestamps.

Extract every STRATEGIC decision the player makes or explains: buying/skipping an upgrade or
machine, what stock to buy or refuse and at what price, wholesale lots, pricing and haggling
policy, faction choices, handling stolen/hot items and contraband, inspections, services,
production engine steps, rent and cash management, night outings, story NPC choices. Skip
routine single-customer trades unless the player states a general rule from them.

Return:
{"decisions":[{"at":"m:ss timestamp of the passage","day":<in-game day if stated or clearly
implied, else 0>,"category":one of %s,"situation":"state that prompted it (cash, rent due,
rep, stock, event)","decision":"what they chose","reason":"their stated reason, empty if none",
"outcome":"what happened as a result if mentioned, else empty","numbers":"prices, cash, rep
values stated, else empty"}],
 "facts":{"start":"start type if stated","difficulty":"if stated","perks":[starting perks],
"engines":[income engines running],"bought":[{"item":"upgrade/machine/service","day":<day or 0>,
"cost":<or 0>}],"turning_points":["short event with day"],"first_day":<earliest day seen or 0>,
"last_day":<latest day seen or 0>,"ending":"buyout/mortgage/game over with day, if shown",
"cash_notes":["day N: cash X / rent Y / net worth Z" as stated]}}

Transcript:
%s`, gameBrief, v.Title, v.Creator, chunk, of, strings.Join(Categories, "|"), text)
}

// ParseExtraction decodes a model answer; invalid decisions are dropped and counted.
func ParseExtraction(out string, v Video, at string) (Extraction, int, error) {
	var e Extraction
	obj, err := llm.ExtractJSON(out)
	if err != nil {
		return e, 0, err
	}
	var raw struct {
		Decisions []json.RawMessage `json:"decisions"`
		Facts     json.RawMessage   `json:"facts"`
	}
	if err := json.Unmarshal(obj, &raw); err != nil {
		return e, 0, err
	}
	if len(raw.Facts) > 0 {
		if json.Unmarshal(raw.Facts, &e.Facts) != nil {
			e.Facts = Facts{} // loosely shaped facts are not worth a retry
		}
	}
	dropped := 0
	for _, r := range raw.Decisions {
		var d Decision
		if json.Unmarshal(r, &d) != nil {
			var loose map[string]any
			if json.Unmarshal(r, &loose) != nil {
				dropped++
				continue
			}
			if day, ok := loose["day"].(string); ok {
				fmt.Sscan(day, &d.Day)
				delete(loose, "day")
			}
			b, _ := json.Marshal(loose)
			if json.Unmarshal(b, &d) != nil {
				dropped++
				continue
			}
		}
		d.VideoID, d.Creator = v.ID, v.Creator
		if d.At == "" {
			d.At = at
		}
		if d.Validate() != nil {
			dropped++
			continue
		}
		e.Decisions = append(e.Decisions, d)
	}
	return e, dropped, nil
}

// Ask sends one prompt and decodes the first JSON object of the answer, retrying once.
func Ask(ctx context.Context, c llm.Client, system, user string, maxTokens int, out any) (string, error) {
	req := llm.Request{System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxTokens: maxTokens}
	var lastErr error
	var text string
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.Complete(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		text = resp.Text
		obj, err := llm.ExtractJSON(text)
		if err == nil {
			if err = json.Unmarshal(obj, out); err == nil {
				return text, nil
			}
		}
		lastErr = err
		req.Messages = append(req.Messages,
			llm.Message{Role: llm.RoleAssistant, Content: truncate(text, 2000)},
			llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("That was not valid JSON for the requested shape (%v). Answer again with only the JSON object.", err)})
	}
	return text, lastErr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

const SummarySystem = `You are a strategy analyst for a shop-management game. From structured decision
records extracted from one player's narrated runs, describe their strategy in your own words.
Be concrete (days, prices, thresholds) where the records give them; never invent numbers.
Answer with one JSON object only.`

// CreatorSummary is the per-creator digest.
type CreatorSummary struct {
	Creator      string   `json:"creator"`
	Start        string   `json:"start"`
	Engines      string   `json:"engines"`
	UpgradeOrder []string `json:"upgrade_order"`
	RiskPolicy   string   `json:"risk_policy"`
	CashPolicy   string   `json:"cash_policy"`
	Habits       []string `json:"habits"`
	Mistakes     []string `json:"mistakes,omitempty"`
}

func SummaryPrompt(creator string, runs map[string]Facts, ds []Decision) string {
	rb, _ := json.Marshal(runs)
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "- [%s day %d %s] situation: %s | decision: %s", d.At, d.Day, d.Category, d.Situation, d.Decision)
		if d.Reason != "" {
			b.WriteString(" | why: " + d.Reason)
		}
		if d.Outcome != "" {
			b.WriteString(" | outcome: " + d.Outcome)
		}
		if d.Numbers != "" {
			b.WriteString(" | numbers: " + d.Numbers)
		}
		b.WriteByte('\n')
	}
	return fmt.Sprintf(`%s

Player: %s. Run facts per video (merged from transcript parts):
%s

Decision records:
%s
Return {"creator":%q,"start":"start type and perks and difficulty","engines":"income engines in the
order they came online, with days","upgrade_order":["upgrade (day)"...],"risk_policy":"how they
treat stolen goods, contraband, inspections, factions, haggling risk","cash_policy":"rent buffer,
when they spend vs save, buyout/mortgage timing","habits":["8-15 recurring, concrete habits, each
one sentence, with thresholds where stated"],"mistakes":["things they regretted or that cost them"]}`,
		gameBrief, creator, rb, b.String(), creator)
}

const HabitsSystem = `You compare strategies of several skilled players of a shop-management game and
find the habits that recur across them, and where they disagree. Be concrete. Answer with one
JSON object only.`

// Habit is a cross-creator pattern.
type Habit struct {
	Habit    string   `json:"habit"`
	Who      []string `json:"who"`
	Evidence string   `json:"evidence"`
	Contra   string   `json:"contra,omitempty"`
}

func HabitsPrompt(sums []CreatorSummary) string {
	b, _ := json.MarshalIndent(sums, "", " ")
	return fmt.Sprintf(`%s

Per-player strategy summaries:
%s

Return {"habits":[{"habit":"one concrete sentence with thresholds where possible","who":[players
who show it],"evidence":"days/numbers from the summaries","contra":"players who do the opposite,
if any"}]} with 10-25 habits, the most widely shared first. Include habits shared by only one
player only if that player is clearly the strongest or the habit is very specific.`, gameBrief, b)
}
