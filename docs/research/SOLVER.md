# Solving Probably Stolen instead of (or under) an LLM

Build 26525552 / 0.46D. Game claims cite the decompiled type and method (`Type.Method`,
`File.cs:line` where a line matters). "Unverified" means read in code, not yet seen in game.
Numbers marked *est.* are reasoning, not measurement. Our memory DB holds **0 decisions** today
(`var/memory.db`), so no estimate below is backed by our own play data yet.

## Verdict

- **One customer is small and fully computable.** About 10³ to 10⁶ leaves in the worst case,
  with every probability given by a formula in the code. Exact expectimax solves it in
  milliseconds. Do it now: it replaces the heuristic's greedy one-move pick.
- **A whole run cannot be solved.** The state space is far beyond 10¹⁰⁰. Some information is
  hidden: the inspection countdown, the customer queue, true item condition, and aug status.
  The game's RNG is unseeded and global. Plain A* does not apply (see §2), and neither does
  exact dynamic programming.
- **The multi-day parts can be approximated well.** The method is **rollout**: improve the
  existing heuristic by simulating a few days ahead from sampled futures. It runs on a
  **small Go simulator of the economic core**, which we write ourselves from the decompile and
  calibrate on recorded play. It does not run the game's own code headless: that code is welded
  to Unity rendering, localization and UI singletons (§5).
- **The LLM still has work.** It covers story and dialogue clients, the 49 upgrades, appraisal
  judgement, rare client kinds, and narrating why. The solver supplies the numbers it reasons
  over.

## 1. The decision problem

### 1.1 One customer (a "turn")

| Element | Choices | Evidence |
|---|---|---|
| **Staging** (the player's choice, not yet automated) | which owned items go on the back counter. A BUY client buys what is there, capped at budget | `PlayerStore.AddedItemToWeightedArea`, `PlayerStore.SellItem` (cap), `StoreClient.IsClientBuyingThisItem` |
| **Selecting** (client SELL) | which of their items to drag to the back counter. `OnBuyAllClick` buys all un-owned items there; wholesale takes everything | `NegociationUIManager.OnBuyAllClick`, `OnWholesaleClick`, `PlayerStore.GetWholesalePrice` |
| **Coercion** (≤ 2, one-shot each) | Blackmail 20 %, Threaten 20 % (once aug is introduced) | `BargainUIManager.Blackmail`, `ThreatenAug`, `CanBlackmail`, `CanThreatenAug` |
| **Side bargain** (one per side) | sell side: Offer Discount or Add Markup at 10/25/50/75. Buy side: Offer Markup or Demand Discount. Discount offers are blocked after coercion | `CanOfferDiscount`, `CanOfferBuyingMarkup`, `PRICE_PERCENTS` |
| **Terminal** | sell / buy / wholesale / dismiss | `OnSellAllClick`, `OnBuyAllClick`, `TryPlayerDismissCurrentClient` |
| **Appraisal** (only with a tool) | expose a feature, which changes price and truth | `ItemFeatureUI.ExposeFeature` (advise-only per `docs/PLAN.md`) |

**Chance nodes.** All of them come from known formulas (`docs/re/BARTER.md`):

| Node | Outcomes | Code |
|---|---|---|
| Add Markup / Demand Discount | accept `100−2p+rep/6+event+dm`; stay `100−1.2p+…`; otherwise walk (−3 rep) | `BargainUIManager.OfferMarkup`, `DemandBuyingDiscount`, rolled with `RNG.GetRandomInt(0,99)` (`BargainUIManager.cs:830,1023`) |
| Blackmail on a criminal | `60+rep/4+event+dm` success; fail −5 rep and the client stays | `Blackmail`, `GetFactionRepBonusBlackmail` |
| Blackmail an innocent / Threaten a non-aug | 50 %: −10 rep and the client walks; 50 %: −7 rep | `HandleInnocentBlackmail`, `HandleFalseThreaten` (`RNG.Roll(50)`, `:948,:1082`) |
| Offer Discount / Offer Markup | deterministic. A discount raises the budget by 2p %; a successful markup cuts it 10–25 % | `GetBargainSuccessChance` = 100, `GetMarkupBudgetCutPercent` |
| Buyer catches a fake feature | **deterministic given hidden truth**: no roll | `StoreClient.CanClientExposeThisFeature` (`IsFeatureMatch`, `IsRealFeatureCheaper`) |

**Size.** Sell side: about 5 coercion orders × 3² outcomes × 9 side moves × 3 outcomes × 2
terminals ≈ **2.4 × 10³ leaves per staging subset**. With k candidate items there are 2^k
subsets, so k = 10 gives ≈ 2.5 × 10⁶ leaves before pruning. Budget and interest filters cut
most subsets. The buy side is similar. Depth is ≤ 4 player moves, or ≤ 7 for SELLNBUY.
**Exact solution is cheap.**

### 1.2 One day (a "meta-turn")

| Decision | Size | Evidence |
|---|---|---|
| Customers served | 0–6 "normal" (attractiveness, perks, vandalism) plus scripted, supplier, info, inspection, wanted and aug clients | `StoreClientManager.GetNormalCustomerCount` (clamp 0–6), `GenerateClient` (≈ 30 `Handle*` sources) |
| Display case / stock mix | drives the "attracted" clients | `ShowcaseHelper.GetDisplayCasePrediction`, `PickShopperClient` |
| Nightly services (on/off) | 7+ services, cost paid nightly | `PlayerStore.EndNight` (service loop), `StoreServiceSheet` |
| Upgrades | 49 network upgrades with prerequisites | `NetworkUpgradeList` (49 `new NetworkUpgrade`) |
| Afterhour locations, expeditions | 4 locations with weekday events | `AfterhourLocation` subclasses, `VanillaAfterhourEvents.RegisterAll`, `ExpeditionData` |
| What to hold, hide or trash | hidden storage avoids inspection | `ContrabandHelper.StartInspection`, `IsConcealedFromInspection` |
| Close the shutter (end the day) | ending early forfeits the rest of the queue | `PlayerStore.EndDay` → `DismissClientStack` |
| ~~Retail markup~~ | **not a player choice**: set from difficulty (15 % on Normal) | `StoreService.cs:217`, `Difficulty` rules, `PlayerStore.IsCurrentClientAcceptMarkup` |

### 1.3 State carried across days

| State | Dynamics | Evidence |
|---|---|---|
| Cash | trades, rent, services, fines | `PlayerStore.playerCash` |
| Rent | 200 at start, due every 7 days, ×1.05 after each due date. A miss rolls into `temporaryRent` and uses up Landlord Grace | `OnRentPaid`, `OnRentNotPaid`, `RentIncrease`, `Difficulty` (`rentIncreaseMultiplier = 1.05`) |
| **Goal: buyout** | 12 000 (Normal), +50/day after day 50, capped at +5 000 | `PlayerStore.GetStoreBuyPrice`; `ExecuteGameOver("buyout")` |
| Ruin states | arrest, rent, repo (loan shark), hitman (UL −100), mob (LL −100), sec, … (13 endings) | `PlayerStore.ExecuteGameOver` callers |
| 5–6 reputations (−200…200, double) | trade rep = value × mult / 400 × faction factor. Bargains shift mult (−5…+2.9) | `StoreReputation.OnItemTraded`, `BargainUIManager.ComputeDirectionalMultiplier` |
| Perk thresholds | about 40 perks at ±20/40/60/80/100 that change traffic, customer mix, inspections and endings | `StoreReputationPerk`, `StoreReputation` perk loop (`:145–156`) |
| Customer mix by rep | BM chance 4/7/14/17 % at rep <0/<25/≥25/≥50; UL 8→12→15; Rev 2/3/6/9 | `StoreClientManager.GetBlackMarketClientChance`, `GetUpperLevelClientChance`, `GetRevolutionClientChance` |
| Inventory | items from **563 ids** (`Add("…")` in 31 `ItemDirectory` subclasses). Each carries features (claimed vs real), heat and the price paid | `ItemDirectory`, `GameItem.itemFeatures`, `BOUGHT_PRICE_TAG` |
| Stolen heat | 25/50/75/100 by base value; −18 per night (−2…−8 less for contraband); 0 = clean | `StolenHelper.InitStolenItem`, `UpdateStolenItem` (called from `EndNight`) |
| Crime record | sale of stolen goods → FENCING = value × heat %; contraband sale → *_TRAFFICKING. Each type decays 20/day while evidence < 80 | `SecData.CheckSoldContraband`, `CommitCrime`, `OnNewDay` |
| Inspection clock | starts once trafficking + fencing ≥ 50. Countdown is uniform in [min,max] = 3–5 days, shrinking to 1–2 at ≥ 750. SEC perks shift it; SEC_VIP disables it | `StoreClientManager.ComputeInspectionCooldownRange`, `ResetInspection`, `HandleInspectionClient` |
| Story / unlock flags | hundreds of bools and counters (`is*Introduced`, storyline schedule, Lou, Eleanor, …) | `PlayerStore` fields, `StoreClientData`, `StorylineSchedule` |
| Garbage, power, health, wild favor | attractiveness penalty, afterhour debuffs, a second currency at 5 % of trade value | `GetTrashAttPenalty`, `HealthData`, `OnItemTraded` (`tradeFavorRate`) |

## 2. Chance and hidden information

**RNG.** Every game roll goes through `RNG` (1 821 call sites), which wraps
**`UnityEngine.Random`**: one global stream that Assembly-CSharp never seeds (no
`InitState` or `Random.state` anywhere). `RNG.GetRandomDouble` uses a time-seeded
`System.Random`. Cosmetic draws share the stream (`BargainUIManager.cs:1628` picks a
dialogue line). So rolls are **not reproducible per save**: reloading gives fresh rolls, and
we cannot replay the game's stream.

| Hidden from the player | Kind | Evidence |
|---|---|---|
| Queue identities | chance, revealed on arrival | `clientStack` (`[ES3NonSerializable]`) |
| Inspection countdown | chance with a **tiny belief**: uniform on [min,max], conditioned on days since the last inspection | `ComputeInspectionCooldownRange`; announced only with the early-warning service (`HandleInspectionEarlyWarning`) |
| Which stolen items an inspector finds | chance: kept with probability `heat %`; contraband is always found unless concealed; Luck can void the whole find | `ContrabandHelper.StartInspection`, `StolenHelper.GetFoundChance`, `HealthData.RollLuck` |
| True item condition / appraisal value | fixed hidden fact, revealed by tools | `ItemFeature.realCondition`, `AppraisalHelper.GetCorrectBaseValue` |
| Aug, stealth-wanted | fixed hidden fact (animation tells, aug scanner) | `StoreClient.isAug`, `isStealthWanted` |

**Classification.** This is a single-agent **POMDP with chance nodes**: nature rolls the dice,
and some facts stay hidden. The customers are scripted samplers, not opponents. Plain A*
fails on three counts:

1. **Stochastic transitions.** A move leads to a distribution of states, so the answer is a
   policy (a tree), not a path. The stochastic generalizations, AO*/LAO* (Hansen & Zilberstein
   2001), still need an enumerable state space.
2. **No single goal.** Buyout is a goal, but the real objective is "reach buyout without ruin",
   that is, maximize a probability or an expected value. No admissible "distance to go" exists
   when a roll can end the run.
3. **Hidden state.** You plan over beliefs, not states.

A* does fit **deterministic sub-problems**. Example: the cheapest schedule to cool a set of
stolen items before a known date.

## 3. State-space size

| Scope | Estimate | Reasoning |
|---|---|---|
| One customer | 10³–10⁶ leaves | §1.1 |
| One day | ≈ 10³⁰–10⁴⁰ paths *est.* | about 8 customers × (10³ decisions × 10² chance outcomes) each. Not enumerable, but each decision is local |
| Run, ignoring inventory and flags | ≈ 10²³ states *est.* | cash ~10⁴ × 6 reputations ~10¹⁵ × crime/evidence ~10⁴ × rent phase |
| Full run | ≫ 10¹⁵⁰ *est.* | inventory of ~40 items drawn from thousands of (id, feature, heat) variants, plus hundreds of story flags |

Exact dynamic programming over the run is out by more than 100 orders of magnitude.
**Abstraction plus sampling is the only route.**

## 4. Where exact solving is cheap and valuable

| Sub-problem | Method | The heuristic today (`internal/policy/heuristic.go`) |
|---|---|---|
| Bargain sequence for one customer | exact expectimax over §1.1 using the game's own odds | greedy: best expected value of the *first* move only; ignores that a spent slot forecloses later moves |
| Coercion stacking | Blackmail (−20 %) then Demand Discount still allowed (`CanDemandBuyingDiscount` survives coercion) | never offers blackmail or threaten |
| Rep cost of hard bargains / value of nice ones | leaf value includes Δrep × λ_faction (`ComputeDirectionalMultiplier`) | a flat 5 % "edge" fudge; never picks Offer Markup, which buys rep |
| Staging / selecting | knapsack over owned items × budget. Offer Discount raises the budget 2p %, so "discount to fit more" is a real option | none: the human stages |
| Hot goods | value = discount (10–25 %) − P(found at inspection) × loss − fencing crime if sold hot | flat rule: dismiss at heat ≥ 50 |
| Rent | cash constraint against a known schedule | reserve rent only when due within 2 days |
| Customer-level optimal stopping | sell now vs hold for a better buyer (a reservation price per item) | sells whenever above the price paid |

The per-turn part needs **a leaf value function** V(cash, inventory, rep, crime). A good start
is linear: `cash + Σ reservation(item) + Σ λ_f·rep_f − κ·crime − ruin penalty`. The λ, κ and
reservation prices are what the rollouts in §6 estimate.

## 5. The simulator question

### (b) The game's own code, headless

The C# test host already loads Assembly-CSharp (`ScannerConformanceTests`). For simulation:

| Type | Base | Coupling | Usable headless? |
|---|---|---|---|
| `PlayerStore` | plain singleton | **253** `*UIManager/EmporiumEntry/…Instance` refs; inventories live in `EmporiumEntry : MonoBehaviour` | no |
| `StoreClientManager` | plain | 3 UI refs, but generation calls `StoreClientList*` (27 files, 17.5 k lines) → item factories | no (see `GameItemElement`) |
| `StoreClient` | plain | 22 UI refs; creation goes through `LocHelper` → Unity Localization `LocalizationSettings.StringDatabase` (`LocHelper.cs:29`) | with stubs |
| `BargainUIManager` | **MonoBehaviour** | rolls inline in instance methods that start dialogues | no; reimplement the formulas (~150 lines) |
| `GameItem` | abstract plain; base constructor clean (`GameItem.cs:162`) | every real item is a `GameItemElement`, whose constructor loads sprites and instantiates render nodes (`GameItemElement.cs:87–107`) | only through a non-rendering subclass |
| `StoreReputation` | plain | 3 UI refs; `OnItemTraded` mostly static math | yes, with small stubs |
| `SecData`, `StolenHelper` | plain | 0 UI refs | yes |
| `StoreStation` | plain singleton | — | yes |
| `RNG` → `UnityEngine.Random.Range` | native internal call | throws outside the Unity runtime; must be redirected with Cecil or Harmony to a seeded shim | possible, and it buys determinism |

**Verdict:** not viable as the day or run simulator. Making `GenerateClient` → item spawn →
sale run headless means stubbing rendering, localization, UI singletons and the scene
inventories: a long tail of null references that re-opens on every patch. It **is** a good
**test oracle**. Differential-test the Go simulator's heat, crime and rep functions against
`StolenHelper`, `SecData` and `StoreReputation`, run in the existing test host with
`UnityEngine.Random` shimmed.

### (a) Reimplement the economic core in Go: **recommended**

Scope, *est.* 1.5–2.5 k lines:

- bargain formulas (150)
- rep and perk thresholds (250)
- heat, crime and inspection hazard (250)
- rent, buyout and services (150)
- the customer generator (600–1 000)
- the day loop (300)

The trick is to model **price changes, not prices**. Take item prices as the game displays them
(`GetNegociatedValue`). Model only what changes them: bargain percentages, stolen discount
features, market events. The 563-item value pipeline then never needs porting.

Brittleness with patches several times a week: the formulas and thresholds are a handful of
constants. Put them in `compat/` alongside the hook manifest, and let `make compat` diff the
cited methods' IL per build.

### (c) Learn the parts we cannot read cheaply

The customer generator is 17.5 k lines of hand-written pools, so learn its *outputs*: per
(faction, attractiveness tier, intent), the budget distribution, interest tags, and the item
and price distribution. Take the *structure* from the code: faction chances, `GetNormalCustomerCount`,
the tier cutoffs at 400 and 650.

Data needed *est.*: about 9 factions × 3 tiers × 3 intents ≈ 80 cells × 30–50 samples ≈
**3–5 k customers ≈ 10 runs of 50 days**. Memory does not record the arriving offer as a row
yet (only the decision context), so add a `customers` table now.

**Recommendation:** (a) + (c), with (b) as the oracle. Prefer it over the alternatives because
it is the only option that is fast (µs per step, needed for search), stable across patches,
and buildable incrementally.

## 6. What only multi-day planning captures

| Lever | Mechanism (code) | Rough magnitude *est.* |
|---|---|---|
| Rep perks as investments | LL 20 Referral: +1 normal customer/day (`GetNormalCustomerCount`, max 6) | +15–20 % traffic for the rest of the run |
| Rep perks as investments | BM/UL ≥ 25: richer customer mix (chances above) | — |
| Rep perks as investments | negative thresholds are endings (Hitman, Lynch) | the whole run |
| Hot-goods arbitrage | buy at −10…−25 %, hold ⌈heat/18⌉ nights (≤ 6) and sell clean: no FENCING crime, so the inspection cadence stays slow | 10–25 % margin on a stolen-goods stream, net of seizure risk (heat % at each inspection) |
| Contraband vs inspection timing | Critical contraband found = arrest (`StartInspection` → `SecurityDialogInspectionArrest`) unless SEC_GRACE. Belief over the countdown is uniform in [min,max] | avoids run-ending risk; the heuristic has no model of this |
| Crime budget | each crime type decays 20/day; the trafficking total sets cadence (≥ 150/300/450/600/750) | lets a planner pace contraband sales |
| Reservation prices | hold an item for a buyer whose interests and budget fit vs sell now | 5–20 % on the sell side (unmeasured) |
| Rent and buyout | a known schedule; the buyout price climbs after day 50 | earlier wins, fewer ruin paths |

## 7. Algorithm families, mapped onto this game

The setting is one player against nature (dice), with some cards face-down. Each family below
starts with a one-line plain-English summary.

| Family | In plain English | Technical sketch | Fit here |
|---|---|---|---|
| **Expectimax** (backgammon-style chance-node search; Michie 1966, Ballard 1983 *-minimax) | Try every move, and for every dice outcome weigh the result by its probability; pick the best average. | depth-limited tree of max and chance nodes; leaves scored by a value function | **Yes, within a customer**: exact and cheap (§1.1). Too deep beyond a day |
| **MDP dynamic programming / value iteration** (Bellman; Sutton & Barto ch. 4) | Write down every possible situation and work backwards to the best move in each. | sweep V(s) ← max_a Σ P(s'\|s,a)[r + V(s')] over all states | **No**: about 10²³ states even ignoring inventory (§3). Works on tiny abstractions (e.g. rent and cash only) |
| **Monte Carlo Tree Search** (Kocsis & Szepesvári 2006 UCT; Browne et al. 2012 survey) | Instead of trying everything, play many quick random games from here and spend more effort on moves that keep looking good. | UCB-guided tree growth + random rollouts | Needs a fast simulator (§5a). Useful for day-level choices; the per-turn tree does not need it |
| **Determinization / PIMC** (Ginsberg's GIB for bridge, 2001; Long et al. 2010) | Guess the hidden cards many times, solve each guess as if you could see everything, then vote. | sample a full world consistent with observations, solve it with perfect information, average | Good fit: hidden info here **reveals itself fast** (the next customer arrives; the inspection happens), which Long et al. identify as when PIMC works well. **Flaw: strategy fusion** (Frank & Basin 1998): each sample "knows" the future, so it over-values plans that need hindsight, e.g. holding contraband "because no inspection comes" in that sample |
| **Information-Set MCTS** (Cowling, Powley & Whitehouse 2012) | Like MCTS, but every simulation re-guesses the hidden cards, and statistics are kept per "what I know", not per "what is true". | one tree over information sets, a new determinization per iteration | Fixes strategy fusion. Worth it only if PIMC/rollout shows hindsight bias in testing |
| **POMCP** (Silver & Veness 2010) | MCTS that keeps a cloud of guesses about the hidden state and updates them as evidence arrives. | particle-filter belief + UCT; needs only a black-box simulator | Principled fit. Our belief is tiny (the inspection countdown, aug status, item truth), so a full particle filter is overkill; sampling the belief directly is enough |
| **Rollout / approximate DP** (Tesauro & Galperin 1996; Bertsekas 2020) | Take a decent rule of thumb, try each option now, then let the rule of thumb play out the next few days in simulation; pick the option that ends best. Guaranteed no worse than the rule of thumb (under exact simulation). | one step of policy iteration by Monte Carlo; Tesauro & Galperin cut base-player error up to 5× in backgammon | **Best first fit**: we *have* a base policy (`Heuristic`), the payoff is large, and it is simple to build |
| **Learned value functions** (TD-Gammon, Tesauro 1995; AlphaZero, Silver et al. 2018; MuZero, Schrittwieser et al. 2020) | Let the program play itself millions of times and learn a score for "how good is this situation"; then search a little with that score. | TD(λ) or fitted value iteration; a network or linear V; search guided by V | Later. Needs millions of simulated days, so only on the Go simulator; quality is capped by the simulator's fidelity. MuZero learns the model too, but needs far more data than we can collect from a real-time game |
| **CFR / equilibrium solving** (Zinkevich et al. 2007) | Find strategies no opponent can exploit. | regret minimization over the game tree | **Does not apply**: there is no adversary. Customers are scripted dice, not strategic, and with known probabilities the "best response to nature" is just expected-value maximization |

## 8. Recommended architecture

```
mod ─decide→ bridge
               ├─ per-customer EXACT expectimax  (internal/solver, Go, <10 ms)
               │     leaves scored by V(cash, inv, rep, crime)          ← λ, κ, reservation prices
               ├─ day-level ROLLOUT over internal/sim (Go)               ← fitted from memory DB
               │     K sampled futures × H days, base policy = expectimax+V
               │     belief: inspection countdown, aug, item truth, queue (sampled)
               └─ LLM: story/dialogue/unique clients, upgrades, appraisal, narration,
                       and any case the solver flags as off-model; gets the EV table as option_notes
```

What it beats the LLM at:

- arithmetic over odds
- staging knapsacks
- consistent risk pricing of heat, crime and inspections
- rent timing

Where the LLM still helps:

- the ~30 scripted client sources and story branches
- upgrade choices whose effects are prose
- reading tells (aug, impostor)
- explaining itself

The runtime budget fits: the deadline is 10 s, and K = 64 futures × 7 days × ~10 customers
× ~50 µs per expectimax step ≈ 0.2 s *est.*

### Phases

| Phase | Deliverable | Effort *est.* | Exit criterion |
|---|---|---|---|
| **S0 Verify and record** | In game: confirm the bargain odds against `bargain_stats`; confirm staging via `GraphUtils.TryAcceptAll` (open in BARTER.md); add a `customers` table (faction, intent, budget, interests, offered items and prices, tier) and an inspection log | 2–3 days | 2 runs logged; observed accept rates inside the 95 % CI of the formulas |
| **S1 Exact per-customer solver** | `internal/solver`: expectimax over coercion → side bargain → trade, with staging/selecting knapsack and a linear V; replaces the greedy `pick` | 3–5 days | table-driven tests against hand-solved trees; offline, scores ≥ the heuristic on every recorded decision |
| **S2 Simulator** | `internal/sim`: §5a scope, with the generator fitted from S0 data; differential tests of heat, crime and rep against the game's own classes in the C# test host | 1.5–2 weeks | simulated days match recorded ones on cash/day, customers/day and rep drift (distribution-level) |
| **S3 Rollout** | day-level decisions (hot goods, hold vs sell, contraband stock vs inspection belief, services, rent reserve) by rollout of the S1 policy; λ/κ fitted by regression on rollout returns | 1 week | in simulation, rollout beats the base policy on P(buyout by day N) and mean cash; then A/B in game |
| **S4 (optional)** | fitted V by TD/self-play on `sim`; IS-MCTS only if S3 shows hindsight bias | 2+ weeks | measurable gain over S3 in simulation and in game |

## Sources

- Tesauro & Galperin, *On-line Policy Improvement using Monte-Carlo Search*, NIPS 1996 — https://proceedings.neurips.cc/paper/1996/hash/996009f2374006606f4c0b0fda878af1-Abstract.html
- Bertsekas, *Rollout, Policy Iteration, and Distributed Reinforcement Learning*, Athena Scientific 2020 — https://web.mit.edu/dimitrib/www/dp_rollout_book.html
- Silver & Veness, *Monte-Carlo Planning in Large POMDPs* (POMCP), NIPS 2010 — https://neurips.cc/virtual/2010/poster/2145
- Cowling, Powley & Whitehouse, *Information Set Monte Carlo Tree Search*, IEEE TCIAIG 4(2) 2012 — https://eprints.whiterose.ac.uk/75048/
- Long, Sturtevant, Buro & Furtak, *Understanding the Success of Perfect Information Monte Carlo Sampling in Game Tree Search*, AAAI 2010 — https://ojs.aaai.org/index.php/AAAI/article/view/7562
- Ginsberg, *GIB: Imperfect Information in a Computationally Challenging Game*, JAIR 14, 2001
- Frank & Basin, *Search in Games with Incomplete Information: A Case Study Using Bridge Card Play*, Artificial Intelligence 100, 1998 (strategy fusion)
- Kocsis & Szepesvári, *Bandit Based Monte-Carlo Planning* (UCT), ECML 2006
- Browne et al., *A Survey of Monte Carlo Tree Search Methods*, IEEE TCIAIG 4(1) 2012
- Tesauro, *Temporal Difference Learning and TD-Gammon*, CACM 38(3) 1995
- Silver et al., *A general reinforcement learning algorithm that masters chess, shogi, and Go through self-play* (AlphaZero), Science 362, 2018
- Schrittwieser et al., *Mastering Atari, Go, chess and shogi by planning with a learned model* (MuZero), Nature 588, 2020
- Zinkevich et al., *Regret Minimization in Games with Incomplete Information* (CFR), NIPS 2007
- Hansen & Zilberstein, *LAO\*: A heuristic search algorithm that finds solutions with loops*, Artificial Intelligence 129, 2001
- Sutton & Barto, *Reinforcement Learning: An Introduction*, 2nd ed., 2018 — http://incompleteideas.net/book/the-book-2nd.html
