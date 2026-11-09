# Simulator specification

Build: Steam `26525552`, `Application.version` 0.46D, `Assembly-CSharp.dll` sha256
`725cb999…7b1d05`. Rules are cited to the game's own types and methods (signature plus our
paraphrase); no decompiled code is reproduced. Values marked **[data]** come from
`var/gamedata/*.json`, produced by `sim/extract` from the local install and never committed.
Bargaining is specified in `docs/re/BARTER.md`, the day snapshot and its readable fields in
`docs/re/DAY.md`, visibility in `docs/re/STATE.md`; this document does not repeat them.

## 0. How the data is obtained, and why that changes the plan

`docs/research/SOLVER.md` §5 judged running the game's code headless non-viable. That verdict
no longer holds. `sim/extract` (C#, net10) now runs the unmodified game logic in a plain .NET
process:

| Obstacle (SOLVER §5) | What `sim/extract/Headless.cs` does |
|---|---|
| Unity internal calls throw outside the player | Every body-less (`extern`/InternalCall/P/Invoke) method in the Unity and third-party assemblies is rewritten with Mono.Cecil to return its default; methods of scene objects (`Component`, `GameObject`, …) in the engine modules and of presentation assemblies (UGUI, TextMeshPro, DOTween, Febucci, Unity.Localization) likewise. A method that returns a concrete engine object returns an unattached instance instead of null, so chains such as `x.gameObject.name = …` stay no-ops; the fake still compares equal to `null`. |
| Item constructors render sprites | `TreeNodeRender.Instantiate*` return unattached render nodes; `RenderHandler` sprite loaders return empty sprites; the game's own draw methods on `PixelElement` types (`Validate*`, `Toggle*`, `Refresh*`, …) are no-ops; `GameItem.BuildShapeFromSprite` returns 1×1. |
| UI singletons | `*UIManager`, `*UIController`, `Item*Handler`, `Store*` behaviours and a short list of others have every method stubbed; their singletons are created with their field initializers, and their editor-wired references become unattached fakes. Logic singletons (`EmporiumEntry`, `ModHook`, `EventTriggerManager`, `ItemBehaviourManager`, `TableMaster`, `GameMaster`) run their real `Awake`/`Start`. |
| Localization | `LocHelper.Get` returns the key; tables therefore carry loc keys, not English. |
| `UnityEngine.Random` is native | `Random.Range`/`value` are backed by a seedable `System.Random`; `RNG.random` is seeded too. Extraction is byte-identical across runs. |
| Saves | `SaveFiles`, `ES3*`, `EmporiumAnalytics`, `Steam*` and `PlayerStore.SaveGame/InitialSave` are stubbed; `Application.persistentDataPath` is null, so any missed write throws instead of touching real saves. |

With that, `EmporiumEntry.Start` runs the game's real new-game path (`PlayerStore.StartNewGame`,
`AddStartingItem`, perks), and the real day loop runs: `PlayerStore.OnShutterOpened` →
`StoreClientManager.GenerateClient` → `EndDay` → `EndNight` → `BeginDay`. **1000 idle days take
176 ms (~340 000 days/min).** Real trades also run through the game's funnels:
`PlayerStore.BuyItem` / `SellItem` move cash, budgets and ownership headless (`extract probe`).

What still does not run headless: anything that resolves through a dialogue choice the UI
would present (rent payment, inspection accept/bribe, story choices, wanted reports) — the
extractor plays dialogue `startAction`/`endAction` chains but cannot click choices yet — and 4
of 575 item factories (`alarm_module_transmitter`, `marking_pen`, `toolbox`, `trashcan`).

## 1. Data tables (`var/gamedata`)

Run `dotnet run --project sim/extract -c Release` (≈ 3 s), then `… -- stats`. Every file
carries `source.steam_buildid`, `source.assembly_csharp_sha256`, `source.extractor_version`.

| File | Produced by | Content | Fidelity |
|---|---|---|---|
| `items.json` | every `ItemDirectory` factory (575 ids in 21 directories, `DirectoryMaster.Create<GameItem>`), 24 seeded samples each | 571 items: directory, loc name key, types, tags (with values: power, manufacturer, contraband level, …), `unitValue`/`unitBaseValue`/`lateUnitValue`, `GetBaseValue`, `GetNegociatedValue`, contraband level, features with stage/type/tool/fake and real modifier ranges/condition distributions | **exact** for factory-time state; features applied later (stolen, client, bargain, market) are rules, see §5.3 |
| `clients.json` | every `StoreClientListDict.storeClientDict` factory (276), 12 samples × 2 phases (day 1; day 30 with every `PlayerStore.is*Introduced` flag set) | faction, cooldown, intent mix, budget range, buying/black tags and ids, no-contraband and wholesale rates, **stock actually placed on the counter** by playing the main dialogue's actions (items, count, value, stolen share, heat), full first sample (price modifiers, rep multipliers, flags, …) | **exact** blueprint; stock is an empirical sample of the real code (12 draws) |
| `client_pools.json` | static `RandomClientPool` fields | the 4 faction pools: factory per entry, tier weights `[<400, 400–649, ≥650]`, faction chance function | **exact** |
| `normal_mix.json` | `StoreClientManager.PickClient`, 2000 draws per cell, cooldowns cleared | client and faction mix over attractiveness {100,250,450,700} × reputation {−50,0,30,60} (all factions), plus `GetNormalCustomerCount` per cell | **exact distribution** (sampled); cooldowns are a rule (§4.2) |
| `days.json` | the real loop for 60 days, idle policy (open, dismiss all, close, sleep) | per day: cash, attractiveness, normal count, full queue (id, faction, intent, budget) | **exact for the idle policy only**; validation baseline, not a model |
| `reputation.json` | `PlayerStore.storeReputations` after `StoreReputation.InitReputation` | 5 factions, 41 perks (id, threshold, polarity), report penalty, loss multiplier | **exact** (perk *effects* are rules, §7) |
| `upgrades.json` | `PlayerStore.networkUpgrade` (`NetworkUpgrade.InitUpgradeDict`) | 54 upgrades: cost, `GetCost()`, prerequisites, tab, state at start; `UnlockActions` keys | **exact** numbers; effects are code (§9) |
| `services.json` | `PlayerStore.storeService` after `StoreService.InitService` + `ApplyDifficultyStart` | 13 services: cost, unlocked, in use | **exact** start state; cost updates are rules |
| `difficulty.json` | `Difficulty.GetAll()`, `Difficulty.Effects` | cozy/normal/hard/challenge `DifficultyRules` (37 fields: retail markup, rent multiplier, property price, mortgage, rep multipliers, favor rate, …) | **exact** |
| `market_events.json` | `StoreEventManager` blueprint lists + every `EventList*` factory | 15 normal + 13 cosmetic blueprints with odds; per event: duration, cooldown, importance, market modifiers (`NegociationData` item type → %), on-active/add-client action ids | **exact** |
| `afterhours.json` | `AfterhourEvents` registry after `VanillaAfterhourEvents.RegisterAll` | 7 event definitions | **exact** definitions; outcomes are code |
| `starting_perks.json` | every `StartingPerkList` factory | 48 starting perks | **exact** |
| `constants.json` | constants and static primitive tables of 39 economic types | e.g. `StolenHelper.DEFAULT_HEAT_LOSS` 18, `StoreReputation.REP_TRADE_DIVISOR` 400, `StoreClientManager` attractiveness and budget thresholds, `PlayerStore` property/aug/permit days, `ShowcaseHelper` client id tables, `PriceHelper` tables | **exact** |
| `extract_report.json` | — | every step or factory that failed, with the exception | — |

`extract stats` prints the summary used to compare against recorded play (item counts and value
quantiles, client intents/budgets/stock by phase, pool sizes, the normal-mix grid, the idle-day
queue profile, table sizes) and fails if the files mix game builds.

## 2. Conventions every port needs

- **RNG.** `RNG.Roll(int c)` is `Random.Range(0,100) < c`; `Roll(float)` uses 0f–100f.
  `GetRandomInt(a,b)` includes both ends. `RNG.GetOdd(List<int>)` draws from `[0, Σw]`
  *inclusive*, so a draw of Σw falls through to index 0: **the first entry of every weighted
  table gets one extra unit of weight.** Copy it. Rolls whose result is ignored still consume
  the stream (e.g. `Roll(100)` in `GetNormalCustomerCount`); that matters only to the oracle.
- **Rounding.** `Math.Round` / `Mathf.RoundToInt` round half to even; `RNG.MultiplyAndRound`
  uses it. `PlayerStore.RentIncrease` truncates a float32 product. Several multipliers are float
  literals widened to double (1.2 is 1.2000000476837158, 0.67 is 0.66999995…, 1.33 is
  1.33000004…); reproduce them bit for bit where the oracle compares exactly.
- **Calendar.** Weekday is `day % 7`, 0 = Sunday (`AfterhourWeekday`). Rent day is `day % 7 == 0`.
  New game: day 1, cash 600, rent 200, `dayUntilRent` 6 (`PlayerStore.StartNewGame`).
- **Introductions.** Much content is gated by `PlayerStore.is*Introduced` flags set by story
  clients (stolen goods on day 13 via `StoreShutterButton`, aug on day 12, …). The simulator
  carries them as state; `clients.json` samples both before and after.

## 3. Day structure

States `MORNING → OPEN → CLOSED → MORNING` (`StoreShutterButton.OpenShutter`,
`PlayerStore.EndDay`, the sleep callback). There is no clock: the day ends when the player
closes (`docs/re/TIME.md`); `PlayerStore.CanEndDay` only requires every queued/current client to
be dismissable.

**Open** (`StoreShutterButton.OpenShutter`)
1. `PlayerStore.OnShutterOpened`: newspaper unlocked from day 8; `CheckCrime` (indirect crime
   trackers, §6.4); `SoldItem.HandleSoldItem`; `StoreClientManager.GenerateClient` (§4);
   `HandleFutureClientQueue` (every deferred visitor's countdown −1, those at ≤ 0 join the queue).
2. State OPEN. `OnShutterDoorOpenMorning`: newspaper item, gun permits by Sec rep 20/50/80,
   intro items, welfare stamps, shareholder dividends (§9), lottery coin, nuclear waste back on
   the counter.
3. `OnLateShutterOpen`: `LLTIPS` 4 % rumor event; the −100-perk visitors (§12); Lou.

**Serve** — `docs/re/BARTER.md`. Unserved clients are dropped at close.

**Close** (`StoreShutterButton.EndAndClose` → `PlayerStore.EndDay`): one outing allowed tonight;
non-owned counter items removed; `DismissClientStack`; `UpdateServiceCost` — **service prices are
fixed at close** (insurance on net worth at that moment).

**After hours** (§10), at most one outing; it costs one normal customer tomorrow unless
`ShortSleeper`.

**Sleep** (`EndOfDayUIManager.OnEndDayButtonClick`). The tablet forces mandatory services on and
cooled-down ones off and **disables sleep while selected services cost more than cash**, so
services never drive cash negative. Then `PlayerStore.EndNight` in this order:
1. dismiss any client; `RunStats.OnDayEnded`; projector bonus reset; reactor; power state
   (`POWER_SERVICE` in use, or `POWER_HIJACK`, or the reactor perk).
2. item pass 1 (`onCycleEndEarlySlotItemFunc`).
3. charge every unlocked, in-use service (`cash -= cost`, cooldown reset); Wild favor refund
   `cost × wildServiceFavorRefund` for insurance, referrals, fixer, criminal network.
4. fixer (`SecData.OnFixerUsed`); janitorial (§4.3); power panel; burglary (§9.3); referral logs;
   vandalism (not vandalized for ≥ 2 days: per faction with rep ≤ −20, roll `|rep|/5` %; first hit
   vandalizes; plus scripted vandals); story night hooks; warnings (inspection early warning
   3/2/1 days, …); faction operations (`StoreOperationManager.OnDayStart`); expeditions; health
   (`HealthData.OnSleep`: wounds, addiction, sentinel pool, lottery draw on `day % 7 == 5`).
5. phone, `ClearFrontInv`, debris, **nightly chemistry**, hopper window 1.
6. item pass 2: for each owned item `StolenHelper.UpdateStolenItem` (§6.1), then
   `onCycleEndSlotItemFunc` (vending, rechargers, hydroponics, most machines); hopper window 2;
   item pass 3; hopper window 3.

Then `StoreEventManager.OnDayEnd` (event durations −1, expired ones start their cooldown) and
`PlayerStore.BeginDay`: `StoreStation.StartDay` (day +1, **market events roll**, §8.4; water
purities re-rolled; faction influence; store requests; upgrade cooldowns −1), `dayUntilRent −1`,
mislabelled vending sales settled, `SecData.OnNewDay` (§6.4), after-hours events (§10),
commissary restock. Finally game-over checks (`reactor`, `aug`) and the morning report.

**Fidelity:** exact port of the phase order and every listed rule. Per-item night callbacks
(chemistry, hydroponics, machines, vending, hoppers) are closures on items: **approximated** in Go
(or delegated to the oracle) — see §13.

## 4. Customer generation

### 4.1 `StoreClientManager.GenerateClient`, in order

Clients enter through `AddClient` (append + `TrackClient`; drops nulls and Known-Snitch/Banned-
Commissary blocks), `AddNextClient` (front), `TryAddClient` (only if off cooldown and not
already queued). `ClientGenCheck` near the end removes duplicate identifiers (first survives).

| # | Step | Adds |
|---|---|---|
| 0 | `StorylineSchedule.Resolve`, Eleanor bail migration, mod hook | — |
| 1 | `HandleShowCaseSecurity` | inspection if the showcase holds contraband, or a stolen item passing `Roll(heat)` (heat then set to 100) |
| 2 | `HandleImportantScriptedClient` | loan-shark intro (cash ≥ mortgage down payment, property unpaid); permit revocation (gun permit held, Sec rep < 0/30/60); Li, John Smith returns |
| 3 | `HandleWildBailout` | once per run on cozy/normal (not Rock Bottom) when 8 days of net worth fall |
| 4 | `HandleCartelClient` | Drug Dealer start only |
| 5–7 | Eleanor, display-case visitors (§4.4), chemical feedback | |
| 8 | `HandleSkipClient` | skip-intro day 1–5 roster |
| 9 | `HandleAlwayClient` | calendar visitors: landlord wholesale day 18, Juan `d%8==1`, Lou chain, lottery rep `d%7==4`, mentor (permit day 24, shotgun day 12), business-permit official from day 38 on `d%7==3`, blackmail/scalper/case-enjoyer perk clients |
| 10 | `HandleClientFromEvent` | each active event's `addClientFromEventActionId` (`StoreEventActionDict`) |
| 11 | `HandleClientFromIntro` | fixed tutorial roster days 1–14 (see `days.json` days 1–14) |
| 12 | `HandleInspectionClient` | §6.3 |
| 13 | `HandleSupplierClient` | `supplierIntro` day 7, then `supplier` every `d%7==0` |
| 14 | `HandleContentUnlockClient` | reminders; flexi buyer `d%7==6` (and `==3` from day 50); retired specialists on fixed weekdays once unlocked; landlord vending pitch days 43/57/71/85 |
| 15 | `HandleCharacterLine` | Georgio, Officer Lun, Eleanor, Serpico storylines |
| 16 | `HandleExpeditionClient` | hirelings, with `sign_expedition` displayed |
| 17 | `HandleInformationClient` | after day 13: certain at ≥ 7 days since the last, 25 % at ≥ 4 |
| 18 | `HandleInventorClient` | inventor `d%7==5` from day 12 (not with `LLAVOIDED`); storage salesman by owned-bay/day/cash rule |
| 19 | `HandleMinorClient` | `n = day − 8`; n ≤ 2 none; one client certain for n ≤ 7, 50 % ≤ 15, 25 % ≤ 25, 10 % after; drawn without replacement from 21 one-shot ids (`StoreClientListMinor.InitMinorClientList`) |
| 20 | `HandleAugClient(early)` | §4.6; skipped on a 50 % roll |
| 21 | `HandleWantedClient` | §4.6 |
| 22 | `HandleNormalClient` | §4.2 |
| 23 | `HandleAugClient()` | late call if the early one did nothing |
| 24 | `HandlePerkClient` | `SEC_SURPLUS` `%7==3`; `SEC_REQUISITION` `%7==4`; `LLRAREGOOD` `%7∈{1,5}` (50 % rare merchant, else 25 % gunsmith, else 25 % chemist); `BM_RELIABLE` thief `%7==1` + 20 %/day petty thief; `UL_HOUSEHOLD_NAME` 20 % extra UL; `UL_DISCERNING_TASTE` (§4.4); `UL_CHARITY`; `REV_QUARTERMASTER` `%7==0`; `REV_RAID_LOOT` `%7==5` |
| 25 | `HandleMarketClient` | **never fires in this build**: `PlayerStore.GetMarketModifierType` filters on `NegociationData.identifier`, which is a runtime GUID, not the item type (verified in `market_events.json`), so no shortage/surplus client is ever added |
| 26 | `HandleServiceClient` | buy referral → `PickShopperClient`; sell referral → `PickSupplierClient`; criminal network → BM rep +2 and 50/50 `criminalBuyerThief`/`criminelMerc`; hypertech smuggler |
| 27 | Sawyer Crew, Eleanor's uncle | |
| 28 | `CheckRentDay` | on `d%7==0`: Georgio payback if short, merchant (if not queued), then `landlord` (or `loan_shark_collect` with a mortgage); mortgage offer `loan_shark2`; property paid → merchant only |
| 29 | `ClientGenCheck` | dedupe |
| 30 | John Smith vandal (front), Lou bomb/operative visits | |

`HandleClientFromStoryLine` and `HandleAdapativeClient` are never called.

**Fidelity:** exact port of the order and of each calendar/flag rule. The story chains (Lou,
Eleanor, Serpico, Georgio, Bailey/Butcher, cartel) are **replayed from extracted schedules**: they
are scripted, mostly fixed-day, and `days.json` shows the idle-policy schedule; their *choices*
are out of scope for the first simulator (§14).

### 4.2 Normal customers

`GetNormalCustomerCount`: 0, −1 `LLPOORREVIEWS`, +1 `LLREFERAL`, +1 each at attractiveness ≥ 200,
≥ 250, ≥ 500 (all void while vandalized), −1 Side Street, +1 Prime Real Estate, −1 if the player
went out last night (not Short Sleeper); clamp 0–6. Default attractiveness 250 gives **2**; below
200 gives 0. **[data]** `normal_mix.json` has the count at 16 (attractiveness, rep) cells.

`HandleNormalClient`: cooldown counters −1 (once a day, here); display-case predictions roll
first and each success uses a slot (§4.4); during intro days (≤ 13 without skip-intro) stop;
otherwise `PickClient` for the remaining slots, minus one if a wanted client came today.

`PickClient`: faction pools tried in order, each with its own roll — BM, then UL, then Rev,
else Lower Level. P(BM) = b, P(UL) = (1−b)u, P(Rev) = (1−b)(1−u)r.

| Chance | Value |
|---|---|
| `GetBlackMarketClientChance` | BM rep < 0 → 4; 0–24 → 7; 25–49 → 14; ≥ 50 → 17 |
| `GetUpperLevelClientChance` | 8, −3 with `UL_TOURISTTRAP`, +4 at UL ≥ 25, +3 at ≥ 50 |
| `GetRevolutionClientChance` | 0 with `REV_BLACKLISTED`; Rev rep < 0 → 2, ≥ 0 → 3, ≥ 25 → 6, ≥ 50 → 9 |

Then `RandomClientPool.Pick(GetAttractivenessTier())` — tier 2 at attractiveness ≥ 650, 1 at
≥ 400, else 0 — a `GetOdd` over the entry weights for that tier. Up to 50 draws, each
*constructing* the client, keeping the first that `IsClientValid` (not on cooldown, not blocked);
a non-LL faction that fails falls back to 50 draws from `generalPool`. **[data]** pool entries
and tier weights: `client_pools.json` (generalPool 18 entries, BM 4, UL 4, Rev 1).

Cooldowns: `TrackClient` sets `clientTracker[id] = cooldown` (default 1) when a client is picked
or added; counters fall once a day; on cooldown while ≠ 0. So a client tracked on day d is next
pickable on day d + cooldown. Non-default: merchant, scavInspectable, brokeUpperLevel 3; thieves
2; foodBankBuyer 6; discerning shopper 5 from arrival (`cooldownStartsOnArrival`).
**[data]** per-client `cooldown` in `clients.json`.

**Fidelity:** exact port (count, chances, tiers, `GetOdd` bias, cooldowns) on extracted pools;
validated against `normal_mix.json`.

### 4.3 Store attractiveness

`GetCurrentStoreAttractiveness = baseStoreAttractiveness − GetTrashAttPenalty + projectorAttractivenessBonus`.
Base 250, +150 per `RENOVATION1/2/3`. Trash penalty `(int)(base × garbage × 5 / 100)` when
`garbageLevel > 2`. Garbage +2 per night (cap 10) once the janitorial service exists and is
not used; using it resets garbage and vandalism. Projector: `100 + quality/perf bonuses`, reset
each night, so only while a powered projector runs. Showcase, perks and rep do **not** feed it.
**Fidelity:** exact port.

### 4.4 Display case and attracted clients

`HandleShowcaseClient` (skipped day 1 and below attractiveness 100): rank the showcase's major
types by summed negotiated value (water ration counts as water; energy cell as material), take
the top `GetShowcaseTypeVisitorCount()` (1, +1 buy referral in use, +1 Showcase I, +1 Showcase
II, reduced while vandalized, min 1) and add one **guaranteed** client per type via
`ShowcaseHelper.CreateClientByStockType`: tier 3 at type value ≥ 350 and day ≥ 14 (household
300), tier 2 at ≥ 145 or attractiveness ≥ 400, else a random tier-1 id. **[data]** the id tables
are in `constants.json` (`ShowcaseHelper.t1*ClientIds`, …).

Predictions (`GetDisplayCasePrediction`, rolled in `HandleNormalClient`, each success uses a
normal slot): gun permits (base 50/40/35 % scaled by days missed and the gun market), scav token
20 % each, signs (`sign_water`, `sign_food`, … 75 %; firearm 40 %; heavy 25 %; scav vs thief
seller weighted `max(0, LL+25)` : `max(0, BM)`), appraisal certificate, Black Lamp invitations.

Discerning shopper: `UL_DISCERNING_TASTE`, day > 13, a luxury item on display, chance
`1 + max(0, ULrep)/5` %, budget 750 + rand(0..250) + day.

`PickShopperClient` (buy referral): the type with the most items in the **main inventory**, tier
judged on showcase value.

**Fidelity:** exact port.

### 4.5 What customers want and pay

Budget: `StoreClient.SetBudget(b)` then `ApplyBudgetModifier`, each bonus a fraction of the
pre-modifier B: `UL_TOP_REVIEW` +0.25B, `SEC_RELIABLE_VENDOR` +0.5B, Sycophant on UL +0.1B,
endorsement tier t +0.25tB, and the highest attractiveness bracket ≥ 900/750/600/450/300 →
+1.0/0.75/0.5/0.25/0.1 B. `SetClientBudget`/`OverrideBudget` skip modifiers (junker).
During a visit: buying from the client raises its budget by `cost × budgetConversionRate/100`
(default 50); selling to it lowers the budget and caps the payout at the remaining budget
(`PlayerStore.SellItem`). Budget formulas per client are code, e.g. merchant
`200 + rand(0..100) + 3·day`, shady merchant `250 + rand(0..50) + 3·day`, UL visitor
`rand(250..300) + 2·day`, revolutionary `rand(150..200) + 2·day`. **[data]** `clients.json`
budget ranges on day 1 and day 30.

Acceptance (`StoreClient.IsClientBuyingThisItem`): refused for `NOT_FOR_RESALE`, black ids/tags,
contraband when `noAcceptingContraband` (forced for security; `Roll(25 − LL/4)` for scavs,
`Roll(35)` for LL citizens; only BUY/SELLNBUY keep it), the variety rule, chemical grade below
`acceptedChemicalProductGrade`, hazardous waste, luxury to a non-UL water buyer, a refused child;
then accepted by `ALL_TYPE`, the id list, `ALL_TYPE_EXCEPT_CONTRABAND`, any buying tag, or the
`willAcceptItem` predicate. BUY and SELLNBUY buy; SELL and SELLNBUY sell; other intents are their
own flows (BARTER.md, coverage table).

Price-affecting client fields: `acceptRetailMarkup` (default true; false for merchant and shady
merchant) adds `+retailMarkup` % to staged owned items; `clientItemFeatureBuying` adds catalogue
features (§5.3) to items the player buys from the client, e.g. `merchantDiscountBuy` −10/15/20/25
% by difficulty, `premiumBuy` +50 % × `supplierMarkupMultiplier`; `tradeReputationMultiplier`
scales rep (§7); wholesale (`isOfferWholesale`, `wholesaleDiscount` default −15) buys every
non-owned item at `Σ negotiated + Σ·disc/100` (integer division), discounts by client (scavs
−15..−25, thief-generic −15, petty thief −10, broke UL −35, …). `sellPriceModifier` is never
read; `clientItemFeatureSelling` does not exist in this build.

**Fidelity:** exact port of modifiers, acceptance and wholesale; per-client fields **[data]**.

### 4.6 What sellers bring

Stock is added in the main dialogue's **end action**, i.e. when the client is served, using the
reputation and day at that moment. `PlayerStore.TryAddDirectSellingItemToTable` skips items when
the counter is full, withholds contraband from LL with `LLDISTRUSTED` and BM with
`BM_DISTRUSTED`, sets ownership, and makes the item stolen (§6.1) only once stolen goods are
introduced. Stock helpers are independent per-item rolls plus a guaranteed item: scav generic
main `25 + max(−15, LL/2)` %, rare `10 + max(−5, LL/5)` %; thief generic the same with BM rep and
stolen; thief/petty thief base % × BM stock multiplier (BM ≥ 100 → 2.5, ≥ 80 → 1.75, ≥ 60 → 1.5,
≥ 40 → 1.25); miner 3–4 ore + day-gated extras; supplier a fixed list extended by unlocks.

**[data]** `clients.json` → per client and phase: `stock_count`, `stock_value`, `stock_items`
frequencies, `stock_stolen_share`, `stock_heat`, and `stock_features` (conditions and fakes as
spawned: e.g. `fakeGenuine` always fake, water purity mislabelled ~10 %). Fidelity: **sampled
from the real code** at two (day, flags) points with rep 0; rep-scaled chances are ported
formulas, so the Go sim should scale the sampled draw rates by the rep factor above rather than
re-sample per rep (approximation; the oracle can sample per rep bucket when needed).

### 4.7 Aug, wanted, supplier

Aug: introduced day 12 (1 with Aug Menace); guaranteed `aug_med` at intro + 1 and + 6, then
every rand(6..8) days; 80 % disguised as a material/medical buyer, else 50/50 `aug_med` /
`aug_material`; tier lerps from mostly tier 3–4 to mostly tier 1 over 100 days
(`AugHelper.RollAugTier`). Wanted: one per day at most after the intro days, chance
`min(100, 4 + 2·(d−1))` (100 after 16 days without one), uniform over the remaining posters;
report rewards Bailey 300, Butcher 450, Lafleur 100, Odin 300, Lou 1500, Li 400, Juan 125, John
Smith 125, Leroy 300 (×0.75 Convict, ×1.5 Model Citizen), Sec +5, the client's faction −6;
wrongful report Sec −8 and a 50 fine. Supplier: LL, SELL, `premiumBuy`, no wholesale;
`PickSupplierClient` uniform over miner, shady pharma, chemist, farmer. **Fidelity:** exact port.

## 5. Items and prices

### 5.1 The item

**[data]** `items.json`: 571 ids with `unitValue` (the base price), `unitBaseValue` (power
sources only), `lateUnitValue`, types (each type is also a tag), tags with values, contraband
level, and factory-time features. Base values: median 50, p75 150, max 15 000; 56 contraband
items. Store pricing never reads `unitCount` (only the legacy RPG `GameItem.GetValue` does).

### 5.2 Price pipeline

`GameItem.ComposeStagedValue`: value = round(((max(0, B·(1+I/100)) + W) · max(0, 1+M/100) + misc) · F),
floored at 0 by `CheckValueSafe`, where B = `unitValue` + power value
(`PowerHelper.GetPowerSourceItemValue`), I = Σ Innate feature mods, M = Σ Market mods, F = Π over
Final features of `max(0, 1+mod/100)`, W = liquid container base price, misc = `lateUnitValue` +
children's values (unless `IGNORE_CHILD_VALUE`; locked children excluded; children inherit the
parent's temporary client market mods). Stage: conditions are Innate; client, market, stolen,
contraband and retail features are Market; bargain/blackmail/threaten are Final
(`ItemFeature.GetValueStage`, with legacy overrides).

| Getter | Used for | Event modifiers | Retail markup |
|---|---|---|---|
| `GetNegociatedValue` | **the traded price** | as event features added when the item enters the counter | only via the `retailMarkUp` feature |
| `GetCurrentValue(…)` | tooltip estimate | adds every active event's `NegociationData.value` whose item type the item has | when staged, owned, and the client accepts markup |
| `GetBaseValue` | stolen heat bands | none, no features | no |

Market events are **[data]** (`market_events.json`). Retail markup is `Difficulty.retailMarkup`
(cozy 20, normal 15, hard/challenge 0, i.e. no markup). **Fidelity:** exact port.

### 5.3 Features, conditions, fakes and exposure

A feature is either plain (a fixed pre-expose modifier; catalogue markups and discounts, which
are always "matched" and cannot be exposed) or a condition (shows `fakeCondition.modValue`
until exposed, then `realCondition.modValue`). The **price uses the fake condition until
exposure.** Exposing runs the condition's `onActivateActionId`, which for cigarettes, stamps and
injectors rewrites `unitValue` (counterfeit cigarette 50, genuine 100; counterfeit stamp 8;
injector genuine 160 / expired 80 / counterfeit 40 / unusable 8).

Condition ladders (`ItemConditionList`, % modifier): equipment new +25 … poor −25; water pure
+100, high +50, base 0, ghost −33, rust −75, gutterflow −90; chemical lab-pure +135 … starch −95;
wine master +75 … undrinkable −100; ingot perfect +200 … very low −25; freshness +15 … spoiled
−50; rusted −40, dented −5, expired −10, replica −75, open box/sealed +10; shabby −75,
second-hand −50. Fake/real pairs are chosen by per-family helpers (`PreBuiltItemHelper`,
`InsCigaretteHelper`, `InsInjectorHelper`, `InsStampHelper`, `WaterFeatureHelper`,
`ChemicalProductHelper`, `IngotPurityHelper`) and by the client factories that spawn them.

Exposure:
- **By customers** (`StoreClient.OnItemPlaced`, `PlayerStore.TryReinspectBeforeSale`): possible
  when the feature is a mismatch whose real value is cheaper and the client's
  `canClientExposeFeature` delegate covers that family (scavs: flashlight water, cigarettes,
  injectors, chem if `CanExposeChem`; LL/BM/security: strip water too; tourists and UL: narrower).
  Effect: confront dialogue, **faction rep −4** (scaled, §7; not for augs), price drops to real.
  `PlayerStore.CanSellThisItem` refuses the sale while a feature is still exposable, so **a buyer
  always catches a catchable fake before paying**.
- **By the player**: scanners and inspection helpers (`WaterScannerUIManager`,
  `ChemScannerUIManager`, cigarette/stamp/injector helpers) expose with no rep change.
  `ItemFeature.SetTool` has no callers, so the `tool` field in our tables is always empty and
  `NegociationHelper` / `WillClientExposeFeature` are dead code.

Feature catalogue (`ItemFeatureList`, Market stage): retail markup +retailMarkup; UL tips +15;
perfumes ±15/±25; addiction ±8 % per withdrawal; awful negotiator ±5; stolen discount when buying
−10/−15/−20/−25 at heat ≤ 25/≤ 50/≤ 75/above; contraband markup on owned items +35/+70/+100/+150
(low → critical); restricted (permit) half of that; merchant discount; supplier premiums;
variety and tourist +15; first-time discount −`newCustomerDiscount`; vending +10. Temporary
features are removed when the item leaves the counter.

**Fidelity:** exact port of the rules and the catalogue; which items carry which fake/real
pairs is **[data]** (`items.json` factory features + `clients.json` `stock_features`).

### 5.4 Appraisal

`AppraisalHelper.GetCorrectBaseValue` is 0 unless tagged `APPRAISAL_ITEM`, else the registry's
per-type formula (hand, ribwich, plasma fuel). Not read; appraisal is advise-only in the plan.
**Fidelity:** not modelled in v1.

## 6. Heat, contraband, inspection, crime

### 6.1 Stolen goods (`StolenHelper`)

Heat (tag `STOLEN_VALUE_INT`) from `GetBaseValue`: ≤ 17 not stolen, < 50 → 25, < 85 → 50, < 140 →
75, else 100 (`InitStolenItem`). Nightly −18, less 2/4/6/8 for contraband low/mid/high/critical;
at 0 the item is clean (`UpdateStolenItem`). Found chance = heat %. Buying a stolen item
multiplies the rep gain by 1.5/2/2.5/3 (heat ≤ 25/50/75/else). Selling one commits FENCING crime
(§6.4). A security client rolls `Roll(heat)` for stolen items on the counters or showcase; a hit
sets heat 100 and starts an inspection. **Fidelity:** exact port.

### 6.2 Contraband

`CONTRABAND_LEVEL` NONE/LOW/MID/HIGH/CRITICAL (0–4); serious = MID+. Found by an inspection when
flaggable (contraband with level, or stolen) and not behind a lock, hidden, or locked tag; hidden
stash searched only when `StorePoster.isPulled`; smuggler bays never descended. **[data]** levels
per item in `items.json`.

### 6.3 Inspections

Scheduled (`HandleInspectionClient`): inactive with `SEC_VIP` or total trafficking crime < 50;
otherwise a countdown drawn uniformly from `ComputeInspectionCooldownRange` by trafficking: ≥ 750
→ 1–2, ≥ 600 → 2, ≥ 450 → 2–3, ≥ 300 → 3, ≥ 150 → 3–4, else 3–5; `SEC_TRUSTED` +2/+4,
`SEC_THOROUGH_INSPECTION` −1; min ≥ 1, max ≥ 2. Plus the showcase inspection (§4.1 step 1) and
security clients reacting to contraband on the counter.

`ContrabandHelper.StartInspection(personality, fromShowcase)`: lazy inspectors (personality 2)
skip the trash; stolen non-contraband items stay found with probability heat %; a successful
`HealthData.RollLuck` voids the whole find; severity = Σ (1 + level), stolen items +1..+4 by base
value, cap 25. Outcome by the worst item: **CRITICAL** → arrest (game over) unless `SEC_GRACE`
(single use; Sec rep −sev); **HIGH** → Sec −sev and a fine = Σ raw `unitValue` of found items,
unaffordable → grace or arrest; **MID / STOLEN** → Sec −sev, items seized; **LOW** → warning; none
found → Sec +3 (when not from the showcase, nothing concealed, no luck, genuine inspector).
Seized: everything found except LOW. Bribe (personality 1 only):
`clamp(100 + 2·day − 2·SecRep, 40, 400)`, which accrues toward BRIBERY crime. Personality is 0
once BRIBERY ≥ 500, else uniform 0–2. (`PlayerStore.StartInspection` / `PaidOrForeclose` /
`GenerateContrabandFineFromItem` are dead: nothing calls them.)

### 6.4 Crime and evidence (`SecData`)

Selling: stolen → FENCING += negotiated × heat/100; contraband → DRUG/WEAPON/CARD/CONTRABAND
TRAFFICKING += negotiated; divided by 2 for BM buyers, 3 for Rev, 2 for LL with `LLTIGHTLIPS`, 0
for the cartel; ×0.67 Intimidating, ×1.33 Blabbermouth. Indirect trackers at shutter open
(`CheckCrime`): dirty/mislabelled water, bad chemistry, bad ammo, bribes convert into
ENDANGERMENT/FRAUD/BRIBERY in chunks.

`OnNewDay`: every crime −20 while evidence < 80; evidence += by total T: < 100 → 0, < 300 → 2,
< 850 → 6, < 1200 → 9, < 1500 → 12, < 2500 → 20, else 33 (`SEC_TRUSTED` ½, `SEC_PRIORITY` ×1.5
else `SEC_WATCHLIST` ×1.2, `UL_PETITION` ×1.5). At 100 a sentencing officer comes: T < 300 → fine
T (Sec −10, record reset) or arrest if unaffordable; T < 1500 → arrest; else execution.
`EVIDENCE_REMOVAL` resets evidence; the fixer service cuts crimes nightly. **Fidelity:** exact port.

## 7. Reputation and perks

Rep is a double in [−200, 200] per faction (LL, UL, SEC, BM, REV; CARTEL for the Drug Dealer
start). No daily drift. `StoreReputation.ModReputation(double)`: Blabbermouth halves BM/REV
gains; Sycophant ×1.25 UL gains; **BM losses ×2**; `UL_PETITION` doubles SEC losses; then gains ×
`reputationGainMultiplier` (1.5/1/0.75/0.5) and losses × `reputationLossMultiplier`
(0.5/1/1.25/2); clamp. Static `ModReputation(faction, int)` also adds Wild favor +40 per positive
LL point. Report penalty starts −8, worsens by 2 per report to −20.

`OnItemTraded(faction, value, original, client, isPlayerSold, item)`: favor += round(|original| ×
`tradeFavorRate`); m = client trade multiplier (+1 when selling capped by budget); value =
round(pre-bargain value × m); buying stolen × heat bonus; buying serious contraband × (level/2+1);
LL × 0.66, UL × 1.2, Diplomatic × 1.33 on gains; then `ModReputation(value / 400)`.

Perks unlock live at their threshold (`UpdateReputation`); **[data]** `reputation.json`. Effects
(call sites):

| Perk | Effect |
|---|---|
| LL 20 REFERAL / −20 POORREVIEWS | ±1 normal customer |
| LL 40 RAREGOOD | rare merchant / gunsmith / chemist on `%7∈{1,5}` |
| LL 60 TIPS | 4 % rumor event (future market event known 2–3 days ahead) |
| LL 80 TIGHTLIPS | LL sale crime halved |
| LL 100 PILLAR | upgrades ×0.85; −15 % buying from the inventor and LL sellers |
| LL −40 DISTRUSTED | LL contraband offers withheld |
| LL −60 TARGET | insurance ×2, break-in +33 % |
| LL −80 AVOIDED | no inventor, supplier calls, Dr Jackson |
| LL −100 LYNCH | game over (mob visitor) |
| UL 0 DISCERNING_TASTE | discerning shopper (§4.4) |
| UL 20 TIPS | +15 % on sales to UL |
| UL 40 CHARITY / 80 HOUSEHOLD_NAME | charity client / 20 % extra UL client |
| UL 100 TOP_REVIEW | all budgets +25 % |
| UL −40 TOURISTTRAP / −60 PETITION | UL chance −3 / SEC losses ×2, evidence ×1.5 |
| SEC 20 SURPLUS / 0 REQUISITION | surplus merchant `%7==3` / requisition officer `%7==4` |
| SEC 40 PROTECTION | break-in −16 % |
| SEC 60 TRUSTED | evidence ½, inspection cooldown +2/+4 |
| SEC 80 RELIABLE_VENDOR | all budgets +50 % |
| SEC 100 VIP | no scheduled inspections, no break-ins |
| SEC −20 WATCHLIST / −60 PRIORITY | evidence ×1.2 / ×1.5 |
| SEC −40 ALARM_IGNORED / −80 THOROUGH_INSPECTION | security button dead / cooldown −1 |
| SEC −100 IMMEDIATE_ARREST | game over |
| BM 40 RELIABLE | thief `%7==1`, 20 % petty thief/day, richer stock |
| BM −40 DISTRUSTED / −60 UNINSURABLE | contraband offers withheld, janitor ×2 / no insurance |
| REV 20 REQUEST / 40 QUARTERMASTER / 80 RAID_LOOT | request board / quartermaster `%7==0` / raider `%7==5` |
| REV −40 BLACKLISTED / −100 HIT | no Rev clients / bombing (game over) |
| CARTEL 25/50/100, −100 | stipend 150 + 4·rep / cheaper chemist / exit allowed / game over |
| *"Reveals operations"* (UL 60, SEC 60, BM 60, REV 60) | information only |

**Fidelity:** exact port.

## 8. Money

### 8.1 Rent
On `d%7==0` the landlord (not dismissable) offers: **pay** (`CanPayRent`: cash ≥ rent + arrears;
pays both, `RentIncrease`); **don't pay** — with `LANDLORD_GRACE` owned (state 2) the grace is
consumed, arrears += rent, rent increases; without it, **game over `rent`**; **buy out** when
`CanBuyOutStore`. Rent × `rentIncreaseMultiplier` (1.025/1.05/1.1/1.1) on **every** landlord
visit, truncated. Start 200 (+40/+50 by start, +40 Bad Lease, +75 Prime Real Estate).

### 8.2 Mortgage and buyout
Buyout price `GetStoreBuyPrice` = `propertyBasePrice` + min((day − 50) × perDay, cap) after day
50 (DAY.md). Buying out ends the run as `buyout` (continue-play offered). Mortgage: down payment
`max(500, RoundToInt(price × share / 500) × 500)`; weekly
`ceil((L + L·r/100/52·W) / W)` in integer arithmetic; offered on rent day at cash ≥ 2× down
payment; then the loan shark replaces the landlord: pay (weeks −1; last week → `buyout_mortgage`)
or refuse → repossession of items worth the payment (term does not shrink) or game over `repo`.
Normal: 12 × 930 after a 2000 down payment. There are **no player loans**.

### 8.3 Other flows
Fines (§6.3/6.4), bribes, wanted rewards (§4.7), shareholder dividends (§9.1), Wild favor (a
second currency spent before cash on upgrades: trade × `tradeFavorRate`, service refunds, LL rep
gains, network intro, one bailout of 250), commissary (stamp tokens; fixed catalogue whose
restock is driven by market events), vending (accrues on the machine, collected by the player).

### 8.4 Market events (`StoreEventManager.OnDayStart`, each morning)
Queued events activate; per-id cooldowns −1; then a **normal roll**: skipped on days ≤ 5 or with
≥ 3 non-cosmetic events active; chance 50/35/25 % with 0/1/2 active; pick by `GetOdd` over
`normalEventBlueprints` odds excluding ids on cooldown; activates immediately (cooldown =
duration + cooldown). Cosmetic events fill to 2 active (no price effect). Normal events are never
known ahead except via rumors (`LLTIPS`), information dealers, faction operations and story.
Same-id reactivation extends; opposing events coexist. `threatEventBlueprints` is empty.
**[data]** odds, durations, cooldowns, modifiers: `market_events.json` (15 normal events, e.g.
`badNutrifruitHarvest` FOOD +50 for 3 days at odds 15). **Fidelity:** exact port.

## 9. Upgrades, services, machines

### 9.1 Upgrades (`NetworkUpgrade`)
State 0 not bought, 1 bought consumable ready to use, 2 unlocked (`NetworkUpgrade.IsUnlocked`, `Unlock`; matches DAY.md). `GetCost` = base ×
`upgradePriceMultiplier` × 0.85 (`LLPILLAR`) × 0.9/1.1 (Wild favorite/bad side). Paid with favor
first, then cash. **[data]** 54 upgrades, costs and prerequisites in `upgrades.json`. Effects that
matter economically: `LANDLORD_GRACE` 100 (×2 per purchase, cooldown 15); `SEC_GRACE` 250 (+100
per purchase); `EVIDENCE_REMOVAL` 500 (cooldown 14); `INSPECTION_INFORMANT` → early-warning
service; `WILD_FIXER` I–III (fee 25/45/70; crimes −15/20/25 % then −50/150/400 nightly);
`BUY_REFERAL`/`SELL_REFERAL` 200 → +1 shopper / +1 supplier per day; `CRIMINEL_NETWORK` 350 → +2 BM
rep and a criminal client; `POWER_HIJACK` I–III → free power; `SHOWCASE_I/II` → bigger case and +1
attracted visitor each; `RENOVATION1–3` → +150 attractiveness each; `ENDORSEMENT1–3` (after the
buyout) → budgets +25 % per tier; supplier contacts and `*_SHAREHOLDER` (wholesale −35 %, weekly
dividend items); production-line unlocks (water, aquaculture, farming, gunsmith, chemist,
rancher, wine, vending) grant items and a teacher client.

### 9.2 Services
**[data]** `services.json` (13). Costs recomputed at close: power (day/10 + 5) ×
`powerCostMultiplier` (0 when hijacked or blacked out); janitorial day/10 + 3 (+72 while
vandalized; ×2 `BM_DISTRUSTED`); insurance (netWorth/400 + 3) × multipliers (×2 `LLTARGET`);
referrals 10; criminal network 45; fixer 25/45/70; hypertech smuggler 500. The difficulty start
cost bonuses are dead (overwritten by the first `UpdateCost`).

### 9.3 Burglary
Uninsured: chance 33 (+33 `LLTARGET`, −16 `SEC_PROTECTION`, none with `SEC_VIP`) + 5 × nights
uninsured; an alarm may stop it; a random repo-eligible item is destroyed. Insured: counter reset,
no loss, no payout.

### 9.4 Machines and production
Hydroponics, chemistry, hoppers, rechargers, vending sales and similar run as per-item night
callbacks (§3). **Fidelity:** not ported in v1; model the store as a trader. Runs that rely on
production are the domain of the oracle (§15).

## 10. After hours

One outing per night (not seriously wounded, no crackdown). Locations (`AfterhourLocation`):
dumping ground (scavenging), commissary (stamp purchases), Dr Jackson (a seller; closed Mondays
unless special stock). Weekday events (`VanillaAfterhourEvents.RegisterAll`, **[data]**
`afterhours.json`): Wednesday fresh dump (junk −50 %, injury +25 %), Sunday Jackson −10 %, Friday
commissary restock, Monday special stock. Scavenging (`ScavHelper.ScavengeDumpingGrounds`): 5
attempts (perks ±2); per attempt minor wound (5 + n) %, else major (1 + 0.25n) %, halved by a
flashlight, voidable by luck; one loot item from `SpawnDumpingGroundLoot` plus small tool-gated
extras. Wounds: minor +2, major +5, > 5 blocks outings, heal −4/night when stable. Expeditions
(`ExpeditionData`) were not read. **Fidelity:** exact port of the schedule and wound math; loot
tables **approximated** until extracted (a `SpawnDumpingGroundLoot` sampler is a small addition
to `sim/extract`).

## 11. Difficulty and starting perks

**[data]** `difficulty.json`: cozy/normal/hard/challenge, 37 rule fields, each consumed as cited
above (retail markup, rent multiplier, grace at start, power/insurance multipliers, permit fee,
first-time discount, upgrade multiplier, supplier/inventor/retired markups, merchant discount,
scalper, property price/increase/cap, mortgage, rep gain/loss, favor rate and refund, aug
presence, Lou's demand, bailout and reminders). **[data]** `starting_perks.json` (48). Economic
ones: +150 cash (saving); rent +40/+75 (bad lease / prime real estate, which also gives +1
customer); −1 customer (side street); no bargaining (awful negotiator); rep ×1.33 (diplomatic);
UL budget +10 % (sycophant); upgrades and insurance ×0.9/×1.1, ×0.5/×1.5 (Wild favorite/bad side);
free janitor (Sawyer crew); never burgled (alert); crime ×0.67/×1.33; luck ±15.

## 12. Game over

| Ending | Trigger |
|---|---|
| `rent` | refuse rent without Landlord Grace |
| `buyout` / `buyout_mortgage` | buy out / pay the last mortgage week (the win) |
| `repo` | refuse a mortgage payment with too little repo-eligible stock |
| `arrest` | CRITICAL find or unaffordable HIGH fine without SEC_GRACE; unaffordable sentencing fine; sentencing at crime 300–1499 |
| `execution` | sentencing at crime ≥ 1500 |
| `mob` / `sec` / `hitman` / `cartel_retired` / `bombed` | −100 perk visitors (`OnLateShutterOpen`; LL, SEC, UL, CARTEL, REV) |
| `lou_bombed`, `lou_bombed_debt`, `sentinel`, `reactor`, `aug` | story, health, reactor perk, aug presence ≥ 100 on hard/challenge |

## 13. Fidelity summary

| Subsystem | Plan | Status of data |
|---|---|---|
| Day phase order, night order | exact port | — |
| Bargaining | exact port (BARTER.md) | — |
| Item base values, types, tags, contraband levels | extracted | **exact** (571/575; 4 machine/tool factories fail) |
| Price pipeline, features, catalogue, conditions | exact port | condition ladders read from code; per-item factory features **exact** |
| Fake/real pairs on spawned stock | extracted (sampled) | 12 draws × 2 phases per client |
| Normal-customer count, faction chances, tiers, cooldowns | exact port | pools **exact**; mix grid **exact (sampled)** |
| Scripted/story/calendar clients | exact port of schedules; story choices not modelled | idle-policy schedule in `days.json` |
| Client blueprints (budget, tags, flags, wholesale) | extracted + formulas | **exact** per factory (276) |
| Seller stock | extracted distributions, rep scaling ported | **sampled**, rep 0 only |
| Market events | exact port | **exact** |
| Stolen heat, contraband, inspection, crime, evidence | exact port | constants **exact** |
| Reputation, perks | exact port | perk list **exact** |
| Rent, mortgage, buyout, services, burglary | exact port | **exact** start values, difficulty rules |
| Upgrades | exact port of economic effects | costs/prereqs **exact** |
| After hours | schedule and wounds exact; loot approximated | event definitions **exact** |
| Production machines, chemistry, hydroponics, vending | not modelled (oracle) | — |
| Appraisal service, gunsmith orders, procurement, barter clients, info dealers | not modelled in v1 | client definitions extracted |
| Story choices (Lou, Eleanor, Serpico, cartel, wanted reporting beyond rewards) | not modelled in v1 | — |
| RNG stream | not reproducible against the real game (unseeded); our own seeded streams | oracle is seeded |

## 14. Biggest unknowns

1. **Dialogue-choice flows headless.** Rent, inspections, bribes, reports and story choices go
   through `Dialogue.AddChoice`; the extractor cannot click them yet. Until it can, those rules
   are ported from reading, and only validated against recorded play.
2. **Seller stock vs reputation.** Stock is sampled at rep 0; BM/LL scaling is ported but not yet
   checked against the real draw at high rep.
3. **Night production.** Chemistry, hydroponics, hoppers and machines are item closures; the Go
   sim ignores them, which under-values production-heavy starts (moonshiner, farmer, chemist).
4. **Human-policy realism of the idle baseline.** `days.json` is an idle player; the real queue
   depends on attractiveness, perks and story flags the player changes. The comparison that
   matters is against recorded runs (phase D).
5. **Unread code:** `Expedition.ExecuteExpedition`, `StoreOperationManager` cadence,
   `StoreEventActionDict` event clients, `AugHelper` presence growth, `LotteryHelper`, appraisal
   helpers, `StoreClientData.Can*` story predicates.
6. **Patch churn.** The game patches often; every table records the Assembly-CSharp hash, and
   `extract stats` refuses mixed builds. Rules in Go need a per-build review of the cited methods
   (extend `make compat` to diff their IL).

## 15. Proposed Go simulator

Two engines, one contract. The **Go simulator** is the fast, inspectable model that search and
learning run on. The **headless game** (`sim/extract`, later a `sim/oracle` mode) is the ground
truth: it produces the tables, and it differential-tests the Go rules. Neither is a fallback
for the other.

```
var/gamedata/*.json ──► internal/sim/gamedata ──┐
                                                ▼
 internal/sim/{item,client,gen,barter,rep,sec,econ,events,night}  ◄── rules: ported from §3–§12
                                                │
                     internal/sim (State, Day, Step) ── seeded rng streams per subsystem
                                                │
             internal/sim/play: the mod's side of the protocol, in process
               builds protocol.DecideRequest / day_snapshot / outcome events from State
                                                │
              internal/policy (Heuristic, LLM, solver) · internal/strategy (planner)
                                                │
                     cmd/fencesim: N runs × seeds → trajectories in the fencedata format
```

| Package | Responsibility | Consumes |
|---|---|---|
| `internal/sim/gamedata` | Load and validate `var/gamedata`; refuse files whose `assembly_csharp_sha256` differs from the installed game unless `-allow-stale`; typed views (items by id and type, client blueprints, pools, mix grid, perks, upgrades, services, difficulty, events, constants) | all JSON tables |
| `internal/sim/rng` | One PCG stream per subsystem (generator, bargain, inspection, night, events, stock) derived from the run seed, so a policy change in one place does not reshuffle the rest; replayable | — |
| `internal/sim/item` | Item instance (id, unitValue, features with fake and real modifiers, tags, heat, contraband level, bought price, owner); the three-stage price (§5.2) and the client/bargain/stolen/market features | `items.json`, `market_events.json` |
| `internal/sim/client` | Blueprint sampling: exact ports for the ~15 high-traffic factories (merchant, scav general, crate, chemist, junker, thief, petty thief, shady merchant, upper visitors, revolutionary, supplier, landlord, security), empirical draws from `clients.json` for the long tail (stock items, budgets by day) | `clients.json` |
| `internal/sim/gen` | The daily queue (§3): normal count, faction choice, tier pick from pools, cooldowns, scripted/story schedule (§4), inspection, rent, suppliers, events | `client_pools.json`, `normal_mix.json`, `constants.json` |
| `internal/sim/barter` | `docs/re/BARTER.md` exactly: one bargain per side, coercion, odds, walk-outs, budget effects, rep multipliers | — |
| `internal/sim/rep`, `sec`, `econ`, `events`, `night` | reputation and perks (§7), heat/crime/inspection (§6), rent/mortgage/buyout/services/upgrades (§8–§9), market events (§8.4), the night (§3) | the matching tables |
| `internal/sim` | `State` (everything in `docs/re/DAY.md` §1 plus hidden truths), `NewRun(seed, difficulty, start)`, the phase machine `Morning → Open → Customer* → Close → AfterHours → Night → Sleep`, `Step(Action) (Observation, Outcome)`, game-over checks | — |
| `internal/sim/play` | Plays the mod: from `State` it builds exactly the `protocol.DecideRequest` the live mod would send (same `context`, `options`, `hidden`), validates the policy's `Choice` against the options as the mod does, applies it; emits `day_snapshot` and outcome events the strategist and memory already consume | `internal/protocol` |
| `cmd/fencesim` | Batch runner: runs × seeds × policy; writes trajectories in the `cmd/fencedata` export shape so offline learning sees simulated and real play the same way; `-compare` prints the `extract stats` metrics for the simulated runs | — |

**How policies plug in.** A policy already implements
`policy.Policy.Decide(ctx, *protocol.DecideRequest) Decision`, and the strategist consumes
`event.day_snapshot`. `internal/sim/play` is a second implementation of the mod's side (the
first being `internal/fakemod`), so `Heuristic`, `LLM`, a future per-customer solver and the
strategist run unchanged: the simulator calls `Decide` in-process instead of over HTTP, at
µs per decision for code policies. Night decisions (services, upgrades, after-hours) become
new `kind`s in the same protocol, added to the mod and the simulator together, so phase N of
`docs/PLAN.md` gets a simulator from the start. LLM policies run against the simulator at LLM
speed; the "thousands of days per minute" budget applies to code policies, rollouts and
learned policies.

**Validation, in order.**
1. Unit: each ported formula against the headless game on the same inputs (an `extract oracle`
   mode that evaluates `GetNegociatedValue`, `OnItemTraded`, `UpdateStolenItem`,
   `SecData.OnNewDay`, `GetNormalCustomerCount`, `ComputeInspectionCooldownRange` on generated
   inputs and writes cases to `var/gamedata/oracle/*.json`; Go table tests replay them).
2. Generator: simulated queues vs `days.json` (idle policy) and vs `normal_mix.json`, by
   chi-square on client-id frequencies.
3. Whole game: simulated runs vs recorded human runs (phase D) on cash/day, customers/day,
   rep drift, inspections per week — the `extract stats` metrics.

**Decided 2026-11-09: the real game is the primary backend.** `sim/gym` (see its README) plays the
installed game's own code headless through the mod's side of the protocol; the Go port below is
no longer the plan of record. The original note follows.

**Alternative worth keeping open: drive the real game.** The headless game already does 340 000
idle days/min and real trades. Making it fully playable means a small action API over
`PlayerStore` (`BuyItem`, `SellItem`, bargain methods with the RNG seeded, dismiss) and a
headless dialogue driver that picks `Dialogue` choices by index. That would be exact by
construction, including the 270 client scripts the Go port approximates, at the cost of .NET in
the loop, static state that must be reset per run (one process per run is simplest), and
breakage on game patches. Recommended order: build the Go simulator for the economic core now;
prototype the dialogue driver in `sim/extract` as the oracle; decide later whether the oracle
becomes a second simulator backend behind the same `internal/sim/play` contract.
