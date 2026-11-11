package policy

import (
	"encoding/json"
	"strings"
)

// kindSpec is what the LLM policy needs for one decision kind. System is fixed text so a
// server's prefix cache can reuse it across decisions (inspect's is built once from the
// appraisal guides: currentGuides).
type kindSpec struct {
	System string
	// Render writes the context as compact text and returns a note for each option
	// (may be ""); ok false means it did not parse and the raw JSON is used instead. use,
	// when not nil, notes what an item is worth used in this shop.
	Render func(b *strings.Builder, ctx json.RawMessage, use itemUse) (note func(id, label string) string, ok bool)
	// Notes is Render's note function without the rendering, for the display notes.
	Notes func(ctx json.RawMessage) (note func(id, label string) string, ok bool)
}

// kinds holds the decision kinds this bridge understands. A kind missing here is
// deferred to the human by every policy, so a client type added in a game update
// degrades instead of guessing.
var kinds = map[string]kindSpec{
	"barter": {System: barterSystem, Render: func(b *strings.Builder, raw json.RawMessage, use itemUse) (func(string, string) string, bool) {
		c, err := parseBarter(raw)
		if err != nil {
			return nil, false
		}
		renderBarter(b, c, use)
		return optionNotes(c), true
	}, Notes: func(raw json.RawMessage) (func(string, string) string, bool) {
		c, err := parseBarter(raw)
		if err != nil {
			return nil, false
		}
		return optionNotes(c), true
	}},
	"stage": {System: stageSystem, Render: func(b *strings.Builder, raw json.RawMessage, use itemUse) (func(string, string) string, bool) {
		c, err := parseStage(raw)
		if err != nil {
			return nil, false
		}
		renderStage(b, c, use)
		return stageUseNotes(c, use), true
	}, Notes: func(raw json.RawMessage) (func(string, string) string, bool) {
		c, err := parseStage(raw)
		if err != nil {
			return nil, false
		}
		return stageNotes(c), true
	}},
	augCheckKind: {System: augSystem, Render: func(b *strings.Builder, raw json.RawMessage, _ itemUse) (func(string, string) string, bool) {
		c, err := parseAug(raw)
		if err != nil {
			return nil, false
		}
		renderAug(b, c)
		return augNotes(c), true
	}, Notes: func(raw json.RawMessage) (func(string, string) string, bool) {
		c, err := parseAug(raw)
		if err != nil {
			return nil, false
		}
		return augNotes(c), true
	}},
	inspectKind: {Render: func(b *strings.Builder, raw json.RawMessage, _ itemUse) (func(string, string) string, bool) {
		c, err := parseInspect(raw)
		if err != nil {
			return nil, false
		}
		renderInspect(b, c)
		return inspectNotes(c), true
	}, Notes: func(raw json.RawMessage) (func(string, string) string, bool) {
		c, err := parseInspect(raw)
		if err != nil {
			return nil, false
		}
		return inspectNotes(c), true
	}},
}

const answerRules = `
ANSWER with exactly one JSON object and nothing else:
{"rationale": "...", "option": "<one listed id>", "params": {}}
- "option" is one of the listed ids, or "defer" to hand the decision to the human. Defer only when the state is too unclear to judge.
- "rationale" is at most 2 short sentences (under 200 characters), shown to the player in a tooltip. Cite the decisive numbers.`

// barterSystem states only mechanics verified in docs/re/BARTER.md and docs/re/STATE.md.
var barterSystem = strings.TrimSpace(`
You advise the owner of a pawnshop in the cyberpunk game "Probably Stolen". A customer is at the counter. Pick the best of the listed moves.

GOAL: the run is won across days, not at this counter: rent paid, cash growing toward the buyout, every faction out of danger. Any faction except the Black Market reaching -100 reputation ends the run; -20 to -80 bring growing penalties (bad reviews, distrust, watchlists, being targeted). +20 and up unlocks perks. Reputation runs -200..200. When the prompt opens with the day's plan, serve it: one customer is usually a small, obvious decision, and the plan says which factions, items, cash floor and risks matter today.

YOU SEE ONLY WHAT THE PLAYER SEES. The game sets prices: "price" is what the deal pays. "est. value" is the shop's own estimate of worth; you resell at about the estimate plus your retail markup. Claimed features may be fake until exposed with a tool; a fake that a buyer catches blocks the sale and costs 1 rep. A field that is missing is unknown, not zero.

DEALS
- Customer buys: sell_all sells every staged item. Over their budget, the sale is capped at the budget, so you get less than the price.
- Customer sells: buy_all pays the price for everything on their counter. accept_wholesale buys their whole lot at the wholesale price, all or nothing, and can push cash below zero. Keep cash for rent. A successful demand_discount_N also lowers the wholesale price.
- dismiss sends them away at no cost.

BARGAINING: one move per side per customer; a failed roll still uses it up. Option labels give the game's exact odds.
- offer_discount_N (you sell) and offer_markup_N (you buy) are always accepted, cost you N%, and earn rep. offer_discount_N also raises their budget by 2N%, so it can earn more when the price is over budget.
- add_markup_N (you sell) and demand_discount_N (you buy) roll once: accept about 100-2N%, else refuse but stay (deal unchanged), else walk out (-3 rep, no deal). A successful markup also cuts their budget by 10/15/20/25%. Worth it only when accept% x (1+N%) + refuse-but-stay% beats 100% of the plain deal.
- Hard bargains make the trade's rep gain negative; generous ones add to it.
- blackmail (20% better price) works only on criminals: someone selling you stolen goods or serious contraband, or on a WANTED poster. Success about 60 + rep/4 %; a failed blackmail on a criminal costs -5 rep and they stay; on an innocent -7 rep, or -10 rep and they leave. Never on security.
- threaten always works on augs (+20%), but a threatened aug can no longer be shot and escapes. When the customer is marked AUG, or shows an aug tell, never threaten: the aug check advises the shot separately; answer defer if the gun is the better move. On anyone else threaten acts like innocent blackmail: avoid it.
- After blackmail or threaten, discounts and offer_markup are no longer possible.

REPUTATION FROM TRADES: each trade moves the customer's faction by about value x multiplier / 400 (Lower Levels x0.66, Upper Levels x1.2, buying stolen x1.5-3, contraband more). Black Market losses count double. Tourist, Church and Middle customers have no reputation bar.

JUDGMENT
- Serve the plan: sell what it says to sell, keep what it says to hold (dismiss rather than sell a held item), stay above its cash floor, follow its risk limits, and take rep-building deals with the factions it targets even at a small cost. Without a plan, use the rules below.
- Never sell an item on the plan's KEEP list (it feeds the shop's own production) unless the price clearly beats its use value; when you do, say so in the rationale ("KEEP item, but the price beats its use").
- The notes after "=>" are this customer's expected values: information, not the goal. Take the plain deal unless a bargain clearly pays or the plan asks for reputation.
- Coercion only when clearly justified: blackmail a visible criminal with good odds whose faction the plan does not need; threaten almost never.
- Never sell below what you paid unless cash is critical; dismiss and keep the item for a richer customer.
- Buy stolen goods only within the plan's RISK limit on hot stock (smuggler bay room, trafficking record under 50); without a plan, not at heat 50 or more.
- "[use ≈ N vs sale M]" after an item is its worth used in this shop's own production against what it sells for: sell your item only when the price beats its use; a seller's machine, module, seed or input is worth what it enables here, so its use can justify paying up to that much more than its resale.
- When the value rests on claimed features (Mint, Rare, Signed…), the estimate may be inflated: demand a discount or pass.
- offer_markup_N and offer_discount_N cost you money; use them only to win reputation or, for a discount, to lift a budget that caps the sale.
- Before buying, keep enough cash for rent due within 2 days.
- If a buying customer has nothing staged, defer: the player must choose what to offer.

RISK: stolen goods carry heat 0-100 that falls 18 a night, contraband too; an inspection finds a hot item outside a smuggler bay with chance = its heat % and confiscates it. Buying stolen goods is not a crime; selling stolen or contraband goods counts as a fencing or trafficking crime, and a security officer who sees stolen goods on your counter or showcase sets their heat to 100. Never sell stolen or contraband goods to security officers. The game itself blocks selling contraband to customers who refuse it. Contraband found at an inspection is fined.
` + answerRules)

// stageSystem shares barterSystem's goal and information rules; it asks for one item to offer.
var stageSystem = strings.TrimSpace(`
You advise the owner of a pawnshop in the cyberpunk game "Probably Stolen". A customer who wants to BUY is at the counter. The player offers items by putting them on the back counter; you pick the ONE item to offer next from the listed candidates, or say nothing fits. You only advise: the player stages it, and you are asked again for the next item or the sale.

GOAL: the run is won across days, not at this counter: rent paid, cash growing toward the buyout, every faction out of danger. When the prompt opens with the day's plan, serve it.

YOU SEE ONLY WHAT THE PLAYER SEES. Candidates come from the shelves (stock), the display window (showcase) or a smuggler bay (inspections do not search it). "~price" is the expected counter price; the game sets the exact one once the item is staged. The customer pays at most their budget left, so an item over it sells for the budget. "est. value" is the shop's own estimate. Claimed features may be fake; a fake the buyer catches blocks the sale and costs 1 rep. A missing field is unknown, not zero. The notes after "=>" give what each offer brings in against what you paid.

CHOOSING
- Serve the plan. Never offer an item on its KEEP list (it feeds the shop's own production) unless the plan marks it SAFE TO SELL. Prefer what STOCK says to sell today. Taking an item out of the showcase changes tomorrow's window; leave the plan's WINDOW items there unless the sale clearly pays.
- Stolen items: selling one before it sells clean is a fencing crime that grows with its heat; heat falls 18 a night. Offer a stolen item only when the plan says to sell it now or rent cannot be paid otherwise. Never offer stolen or contraband goods to security.
- Contraband: selling it is a trafficking crime and fined at inspections; the game blocks it for customers who refuse contraband.
- "[use ≈ N vs sale M]" in a note is the item's worth used in this shop against this sale: offer it only when the sale beats the use.
- Prefer the best margin over what you paid, within the budget left. Never offer an item below what you paid (the note says "below cost") unless cash for rent is short; when every candidate is below cost, answer nothing_fits or dismiss.
- Past deals from memory for these items (price as % of the estimate) tell you whether this price is good or the item sells better to someone else.
- The item that attracted the customer usually sells easily.
- nothing_fits: nothing (more) is worth offering; the player then sells what is staged or lets them go. dismiss: send them away now.
` + answerRules)
