# Strategy knowledge (build 26525552)

## PRIMER
- **Rent is the clock.** 200 cr due every 7th day, ×1.05/week on Mark Up, ×1.1 on Thin Margins and Red Ink. Only Price Gouge and Mark Up start with Rent Grace. Count the next rent as already spent. [code]
- **The buyout wins the run.** Mark Up 12k, Thin Margins 24k, Red Ink 36k. After day 50 the price rises 50 cr/day on Mark Up and Thin Margins, capped at +5k on Mark Up and +10k on Thin Margins. A Loan Shark mortgage costs more than paying cash. [code]
- **Produce, don't just trade.** On hard, resale earns little without a wholesale discount. Furnace ingots, water and wine pay. [owner][many]
- **Wine:** 2 small berries + yeast + Pure or HQ water gives Master Vintage. Wine gains +3 cr/night as it ages. Sell moldy and unripe berries as food. [code][owner]
- **Furnace:** 2 ore make a High ingot and 2 scrap a Low one. The junk module turns 6 junk into 4 scrap. Jackson buys materials. [many]
- **Dumping Ground nightly.** Each trip costs one random customer the next day unless you have Short Sleeper. Wednesday finds better loot but wounds more. Carry a flashlight, a bandage and search tools. [code][owner]
- **Speculate.** Market events appear on the calendar each morning before opening and last 1–3 days. Buy during a surplus and sell during a shortage. The 50 cr Shady Customer tip schedules an event in advance. [code][owner]
- **Window:** set it before opening. The category draws buyers, signs draw sellers, and value raises budgets. Contraband or a found stolen item in the window triggers an inspection that day. [many][code]
- **Heat:** stolen goods lose 18 heat/night (2–8 less for contraband) and are clean at 0. Inspections skip Smuggler's Bays. [code][owner]
- **Losses:** unpaid rent after grace; a case file of 300 cr or more (arrest) or 1,500 cr or more (execution); Critical contraband found without Clemency; a faction at −100. [code][many]

Tags: [code] verified in decompile/data · [owner] PLAYER-NOTES · [many] ≥2 independent sources · [one] single source · [contested]. S# → SOURCES.md.

## Early (days 1–14)
- Start as Apprentice: 550 cr, forgiving. Rock Bottom (0 cr) is a scenario, not a difficulty [many S14,S88].
- Join the Appraisal Board on day 8 (85 cr on Normal). It pays steady side income [many S7,S3].
- Buy insurance. Without it, burglary is 38% on night 1 and +5% each night after. With it the chance is 6% and stolen goods come back [many S52,S81].
- Fund Georgio. He repays more than you gave and leaves a Scavenger Token, which adds a 20%/day scavenger visit when shown in the window [many S40,S52,S72].
- Pick one niche, such as food, furnace or water. A mixed stock clogs storage [many S10,S42].

## Mid (days 15–50)
- **Wine** (Joe Wild 200 cr, winemaker on Fridays). Score each berry: small +2, normal +1, moldy −1. Add +1 for 2 or more berries, +1 for yeast, and +1/+1/0/−1/−2/−3 for Pure/HQ/Base/Ghost/Rust/Gutter water. A score of 6 or more is Master Vintage, worth +75% [code S56,S92].
- **Water:** call out mislabeled water to get a discount. Purify it and sell during a crisis (+50%) [many S39,S50,S66].
- **Inspections:** contraband is always found. A stolen item is found with chance equal to its heat, and Luck% waives the whole find. Low-level contraband gets a warning. Mid-level contraband and stolen goods are seized. High-level is seized plus a fine. Critical means arrest unless you hold Clemency (250 cr, +100 each rebuy, used up when it saves you) [code S89,S60].
- **Evidence:** contraband sold to the Revolution adds ⅓ of its value to your case file, to the Black Market ½, and to the Cartel 0 [code S86].
- **Business permit:** due from day 38, 1,500 cr on Normal. Each refusal adds to a fine [code S57].
- **Haggling:** one bargain per side per customer. A discount raises reputation with the customer's faction and a markup lowers it [many S59,S77].

## Late (days 50+)
- While saving for the buyout, stop buying machines and sell idle stock [many S8,S17].
- After the buyout there is no rent. Scale up production. Joe Wild then refers you to the Hypertech Smuggler [many S57,S65].

## Expert hypotheses (not yet tested in the gym)
- **Security VIP.** Security 100 ends inspections and burglary [code]. Whether racing to it pays is untested. One hard run reached it by day 56 with an alarm transmitter (+1 a night) and by reporting impostors [one PerunGamingAU; EXPERT-HYPOTHESES H17].
- **Renovation II early.** At 500+ attractiveness the third extra customer arrives [code]. A hard player regretted buying it late [one PerunGamingAU; H19].
- **Landlord vending (+150 a week, compounding).** The gym declines it. The one player who took it, on hard, took in 34–72 a night and called it disappointing [one PerunGamingAU; H18].

## Community vs code
- **Pure water in wine.** The in-game guide and videos say Pure Water "lacks minerals" or ruins the wine. In code it scores +1, the same as HQ [code vs S6,S41].
- **Lucky.** Wikis say +5 luck; the code gives +15 [code vs S79].
- **Rent example.** 200→209 on Mark Up is correct: (int)(200×1.05f)=209 in float math. The model checker wrongly called it wrong [code].
- **Clemency.** It is a consumable favor, not a permanent unlock as the model checker thought [code].
- **Contraband in the window** does not mean "instant arrest" [S17]. It triggers an inspection, and the punishment depends on the tier [code].
- **Smuggler's Bay** is not sold by Dr Jackson. It comes with the Fence start, from clients and from the Exchange [code].
