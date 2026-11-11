using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // What the executor knows about the day it is playing: the plan in force and the running totals the
    // plan's limits need. One per load context, reset each morning by the episode.
    internal sealed class PlayerDay
    {
        public static PlayerDay Current;

        public int Day, CashAtMorning, Spent, Sold, Bought, SoldItems;
        public int RentDue, RentDaysLeft;
        // Short of cash for the next payment, or the stock grid nearly full (engine deliveries would be
        // turned away): sell down harder, buy only at deeper discounts.
        public bool Liquidating => PlayerStore.Instance != null && (PlayerStore.Instance.playerCash < CashTarget || PlayerStock.FreeCells() < (Directives.Current.Raw.Value<int?>("min_free_cells") ?? 36));
        // The cash level a rent payment (or the planner's sell_down_to_cash) needs.
        public int CashTarget;
        public int Floor;
        // Net worth gained per week over the last four (rent and services already paid), or null early on.
        public int? NetGainWeek;
        // The same over the last four weeks only (null before day 29): the cash-buyout projection reads it.
        public int? NetGainWeek4;
        public bool SavingForBuyout;
        // `buyout_save` cuts in force today (upgrades, inputs, vending, insurance), or null.
        public HashSet<string> BuyoutCuts;
        public bool Cuts(string what) => BuyoutCuts != null && BuyoutCuts.Contains(what);
        // Georgio's early payback: spend until cash is under this (null: off).
        public int? SpendDownTo;
        // The rent reserve without the plan's cash_floor (engine inputs answer to it); spend on inputs today.
        public int Rent2Floor;
        public HashSet<string> RisingTypes = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        // The type the window concentrates on (window_mode: concentrate): restocked like a preferred type.
        public string ShowType;
        public Func<GameItem, bool> Reserved = _ => false;
    }

    // The competent executor (`-human player`): every choice the gym leaves to the human, steered by a
    // Directives object (Directives.Current) and falling back to rules measured in the gym
    // (docs/research/EXPERIMENTS.md). The counter half is PlayerPolicy, answered in-process or by a bridge.
    internal sealed class PlayerRole : KnobHumanRole
    {
        public PlayerRole(Defaults defaults, Knobs knobs) : base(defaults, knobs) { }

        public override string Name => "player";

        static Directives D => Directives.Current;
        static PlayerDay P => PlayerDay.Current;

        public static int Est(GameItem i) => (int)i.GetCurrentValue(false, true, true, false, 0L);
        public static int Heat(GameItem i) => StolenHelper.IsStolenItem(i) ? i.GetTagReadonly("STOLEN_VALUE_INT").GetInt() : 0;
        static bool Contraband(GameItem i) => ContrabandHelper.GetContrabandLevelInInt(i) > 0;
        public static int Cells(GameItem i)
        {
            try { return Math.Max(1, i.GetItemSize()); }
            catch (Exception) { return 1; }
        }

        public static bool TypeIs(GameItem i, IEnumerable<string> types)
        {
            var mine = i.GetGameItemType();
            return types.Any(t => mine.Any(m => string.Equals(m, t, StringComparison.OrdinalIgnoreCase)) || string.Equals(i.identifier, t, StringComparison.OrdinalIgnoreCase));
        }

        // Never staged for a buyer: held by the plan, an engine's input, or (by default) still hot.
        public static bool IsSign(GameItem i) => (i.identifier ?? "").StartsWith("sign_", StringComparison.Ordinal);

        // A condition shown better than it is and not yet exposed: what any buyer able to check it would find
        // (StoreClient.CanClientExposeThisFeature without the buyer's capacity).
        public static bool HidesFake(GameItem i) =>
            new[] { i }.Concat(StoreClient.GetInspectableChildren(i)).Any(g => g.itemFeatures.Any(f => !f.isDisabled && !f.IsExposed() && !f.IsFeatureMatch() && !f.IsRealFeatureCheaper() && (f.useCondition || f.isExposable)));

        public static bool Keep(GameItem i, StoreClient c) => KeepWhy(i, c) != null;

        public static string KeepWhy(GameItem i, StoreClient c)
        {
            if (IsSign(i)) return "sign";
            if (Appraisal.IsKit(i) || Labels.IsTool(i)) return "tool";
            if (Storage.On && Storage.IsBay(i)) return "tool";
            // `inspect`: the magnifier is the inspection tool, not stock.
            if (i.identifier == "magnifier" && D.Raw.Value<string>("inspect") != null) return "tool";
            // Water sold under a label its true grade does not match is exposed by the buyer's test (rep loss,
            // and a crime tracker); a bought container's label is the seller's word. Only matching labels sell.
            if (i.IsTag("LIQUID_CONTAINER_TAG") && !WaterHelper.IsEmpty(i) && i.identifier != "wine_bottle"
                && WaterFeatureHelper.GetWaterFeature(i) is ItemFeature wf
                && WaterFeatureHelper.GetPurityArrayIndex(wf.customIntValue1) < WaterFeatureHelper.GetPurityArrayIndex(WaterHelper.GetWaterPurity(i))) return "water_label";
            // Placing an item the buyer can expose (a fake seal, an unusable injector, a mislabeled jug) costs
            // its faction's rep at once, sale or not. Reads the true condition, as the water check above does.
            if (c != null && c.CanClientExposeAnyFeatureDeep(i)) return "exposable";
            if (D.HoldUids.Contains(i.uniqueId) || P.Reserved(i)) return "reserved";
            if (Contraband(i) && !(D.Raw["sell"]?.Value<bool?>("sell_contraband") ?? false)) return "contraband";
            // `wine_min_age`: finished wine ages +3 a night x its grade; held until this many nights unless the
            // shop is short of cash.
            // At most `wine_hold_max` bottles (default 20) are held back, the youngest.
            if (WineAging(i)) return "wine_aging";
            if (Contraband(i) && (c.isSecurity || c.noAcceptingContraband)) return "contraband";
            // Water held through a surplus (an event pushing its price down), sold into shortages.
            if (Production.WaterSellOn && i.GetGameItemType().Contains("WATER") && i.GetCurrentValue(false, true, true, false, 0L) < i.GetCurrentValue(false, true, false, false, 0L)) return "water_surplus";
            if (StolenHelper.IsStolenItem(i) && (c.isSecurity || !(D.SellHot ?? false))) return "stolen";
            return null;
        }

        static bool WineAging(GameItem i) => D.Raw.Value<int?>("wine_min_age") is int age && i.identifier == "wine_bottle" && WineHelper.IsFinishedWine(i)
            && i.GetTagReadonly("AGE_INT").GetInt() < age && !(P?.Liquidating ?? false) && Production.AgingRank(i) < (D.Raw.Value<int?>("wine_hold_max") ?? 20) && WineHoldCashOk();

        // `wine_hold_cash: C`: wine is held to age only while cash covers the rent, the floor and C.
        public static bool WineHoldCashOk() => !(D.Raw.Value<int?>("wine_hold_cash") is int c) || PlayerStore.Instance.playerCash >= PlayerStore.Instance.GetRentValue() + Math.Max(P?.Floor ?? 0, 0) + c;

        public override IList<GameItem> StageForBuyer(StoreClient client, IList<GameItem> owned)
        {
            bool liq = P.Liquidating;
            int max = liq ? 12 : 8;
            var wanted = owned.Where(i => client.IsClientBuyingThisItem(i, false) && !Keep(i, client)).ToList();
            // `hold_for` (gym): {types, k, faction}: the k most valuable of each type are kept for that faction's
            // buyers (Upper Levels by default: +15 % tips once UL_TIPS is unlocked, GameItem's selling features),
            // unless the shop is short of cash.
            if (D.Raw["hold_for"] is JObject hf && !liq && client.clientFaction != (hf.Value<string>("faction") ?? "FACTION_UPPER_LEVEL"))
            {
                int k = hf.Value<int?>("k") ?? 3;
                var held = new HashSet<GameItem>();
                foreach (var t in (hf["types"] as JArray ?? new JArray()).Select(x => (string)x))
                    foreach (var i in owned.Where(i => i.IsTag(t) && !StolenHelper.IsStolenItem(i) && !Contraband(i) && !P.Reserved(i) && !IsSign(i)).OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).Take(k))
                        held.Add(i);
                wanted = wanted.Where(i => !held.Contains(i)).ToList();
            }
            long budget = client.useClientBudget ? client.GetBudget() : long.MaxValue;
            // A buyer pays the counter price up to their budget; a discount raises the budget by twice its
            // percent, so up to ~1.5x the budget is still worth staging.
            long room = budget == long.MaxValue ? long.MaxValue : (long)(budget * (liq ? 1.5 : 1.0));
            var picked = new List<GameItem>();
            long sum = 0;
            // `stage_variety` (gym): a variety buyer takes one of each item id (StoreClient.SetVariety, the +15 tag on
            // placed goods), so a second copy only takes a slot from something it would buy.
            bool variety = client.isRequireVariety && (D.Raw.Value<bool?>("stage_variety") ?? false);
            foreach (var i in wanted.OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.GetNegociatedValue() / (double)Cells(i)).ThenBy(i => i.uniqueId))
            {
                if (picked.Count >= max) break;
                long v = i.GetNegociatedValue();
                if (sum + v > room) continue;
                if (variety && picked.Any(x => x.identifier == i.identifier)) continue;
                picked.Add(i);
                sum += v;
            }
            Breadth.Saw(client, P.Day);
            DemandTrace.Buyer(client, owned, picked, room);
            defaults.Take("player:stage_for_buyer", picked.Count + " of " + wanted.Count + (liq ? " (liquidating)" : ""));
            return picked;
        }

        public override IList<GameItem> StageFromSeller(StoreClient client, IList<GameItem> theirs, int cash)
        {
            DemandTrace.Seller(client, theirs);
            // An engine's inputs on the counter: buy those (as many as the engine wants) and nothing else.
            if (Environment.GetEnvironmentVariable("FENCE_WATER_DEBUG") != null)
                foreach (var w in theirs.Where(x => (x.identifier ?? "").Contains("water")))
                    Console.Error.WriteLine($"water-offer {w.identifier} purity={(w.IsTag("LIQUID_CONTAINER_TAG") ? WaterHelper.GetWaterPurity(w) : -1)} ml={(w.IsTag("LIQUID_CONTAINER_TAG") ? WaterHelper.GetCurrentCapacityML(w) : -1)} est={Est(w)} wants={Production.WantsWater(w)}");
            var wanted = new Dictionary<string, int>(StringComparer.Ordinal);
            var inputs = new List<GameItem>();
            foreach (var i in theirs.Where(i => Production.IsInput(i.identifier)).OrderBy(i => i.uniqueId))
            {
                if (Production.WantsWater(i) || Production.WantsEmptyVessel(i, inputs)) { inputs.Add(i); continue; }
                if (i.identifier == "energy_credit" && !Production.Charged(i)) continue;
                if (!wanted.TryGetValue(i.identifier, out int n)) wanted[i.identifier] = n = Production.Wants(i.identifier);
                if (n <= 0) continue;
                inputs.Add(i);
                wanted[i.identifier] = n - 1;
            }
            if (inputs.Count > 0)
            {
                defaults.Take("player:stage_inputs", inputs.Count + " of " + theirs.Count);
                return inputs;
            }
            var avoid = D.AvoidTypes;
            var picked = theirs.Where(i => !(avoid.Count > 0 && TypeIs(i, avoid))).OrderBy(i => i.uniqueId).ToList();
            if (Breadth.On && picked.Count > 0)
            {
                // A lot cheap enough to buy whole stays whole; otherwise only the short types are staged (the whole
                // lot can still go wholesale).
                bool cheap = picked.Sum(i => (long)i.GetNegociatedValue()) <= picked.Sum(i => (long)Est(i)) * (D.SingleMaxPct ?? 85) / 100.0;
                var want = cheap ? new List<GameItem>() : Breadth.Pick(picked, P.Day);
                if (want.Count > 0 && want.Count < picked.Count)
                {
                    defaults.Take("player:stage_breadth", want.Count + " of " + theirs.Count);
                    return want.OrderBy(i => i.uniqueId).ToList();
                }
            }
            defaults.Take("player:stage_from_seller", picked.Count + " of " + theirs.Count);
            return picked;
        }

        public override IList<GameItem> Window(IList<GameItem> owned, int capacityHint)
        {
            bool avoidStolen = D.AvoidStolen ?? true;
            // A display buyer lifts the case's items to the counter and inspects them like any buyer, so an
            // undisclosed fake in the case costs rep the same way.
            var pool = owned.Where(i => !Contraband(i) && !(avoidStolen && StolenHelper.IsStolenItem(i)) && !P.Reserved(i) && !D.HoldUids.Contains(i.uniqueId) && !HidesFake(i) && !WineAging(i)).ToList();
            List<GameItem> picked;
            if (D.ShowUids != null)
                picked = pool.Where(i => D.ShowUids.Contains(i.uniqueId)).Take(capacityHint).ToList();
            else if ((D.Raw.Value<string>("window_mode") ?? "types") == "concentrate")
            {
                // The day's display buyer follows the case's top type by summed value, and that type at 350
                // or more (from day 14) brings its tier-3 buyer (stockpiler, chef, connoisseur…) with the big
                // budget (docs/re/WINDOW.md, ShowcaseHelper.CreateClientByStockType). Fill the case with the
                // type that gets closest, most valuable first; the other types' best items take what room is left.
                string top = WindowTypes.OrderByDescending(t => pool.Where(i => i.GetGameItemType().Contains(t)).OrderByDescending(i => i.GetNegociatedValue()).Take(6).Sum(i => (long)i.GetNegociatedValue())).First();
                if (P != null) P.ShowType = top;
                picked = pool.Where(i => i.GetGameItemType().Contains(top)).OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).ToList();
                foreach (var type in WindowTypes.Where(t => t != top))
                {
                    var best = pool.Where(i => !picked.Contains(i) && i.GetGameItemType().Contains(type)).OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (best != null) picked.Add(best);
                }
                picked = picked.Take(capacityHint).ToList();
            }
            else
            {
                picked = new List<GameItem>();
                foreach (var type in WindowTypes)
                {
                    var best = pool.Where(i => !picked.Contains(i) && i.GetGameItemType().Contains(type))
                        .OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (best != null && picked.Count < capacityHint) picked.Add(best);
                }
            }
            // `window_signs` (gym): these supplier signs go in the case first; each brings its trade's wholesale
            // seller three openings in four, in place of a random normal customer (docs/re/WINDOW.md).
            if (WindowSigns is List<string> signs)
            {
                var shown = pool.Where(i => signs.Contains(i.identifier)).GroupBy(i => i.identifier).Select(g => g.OrderBy(i => i.uniqueId).First()).OrderBy(i => signs.IndexOf(i.identifier)).ToList();
                picked = shown.Concat(picked.Where(i => !shown.Contains(i))).Take(capacityHint).ToList();
            }
            // The certificate on display is what brings appraisal clients (ShowcaseHelper.GetDisplayCasePrediction).
            if (Appraisal.Certificate() is GameItem cert)
            {
                picked.Remove(cert);
                picked.Insert(0, cert);
                if (picked.Count > capacityHint) picked.RemoveAt(picked.Count - 1);
            }
            defaults.Take("player:window", picked.Count + " items");
            return picked;
        }

        public override int Dialogue(DialogueView v)
        {
            int appraisal = Appraisal.Dialogue(v, P?.Floor ?? 0);
            if (appraisal >= 0) return Took("player:appraisal", v, appraisal);
            // The landlord's vending offers add 150 a week to the rent, compounding; the executor does not
            // stock a vending machine, so it is pure cost.
            // `rent: skip` (one day's directive): let Landlord Grace absorb this rent on purpose (only while it is owned).
            if (v.ClientId == "landlord" && D.Raw.Value<string>("rent") == "skip" && NetworkUpgrade.IsLandlordGraceUnlocked())
            {
                var skip = v.Choices.FirstOrDefault(c => c.Enabled && (c.Label ?? "").IndexOf("Don't pay", StringComparison.OrdinalIgnoreCase) >= 0);
                if (skip != null) return Took("player:rent_skip", v, skip.Index);
            }
            if (v.ClientId == "landlordVending")
            {
                // `landlord_vending: "machine"`: take his vending machine (+150 weekly rent) and stock it (Vending.cs).
                // Choices: the water fountain and the machine while each is not yet owned, then decline.
                var on = v.Choices.Where(c => c.Enabled).ToList();
                // `fountain: "landlord"`: his water fountain, the first choice while it is not owned.
                if (Fountain.TakesLandlords && !PlayerStore.Instance.isVendingFountainUnlocked && on.Count >= 2)
                    return Took("player:accept_fountain", v, on[0].Index);
                if (D.Raw.Value<string>("landlord_vending") == "machine" && !PlayerStore.Instance.isVendingMachineUnlocked && on.Count >= 2)
                    return Took("player:accept_vending_machine", v, on[on.Count - 2].Index);
                var no = v.Choices.LastOrDefault(c => c.Enabled && (c.Text ?? "").IndexOf("Decline", StringComparison.OrdinalIgnoreCase) >= 0);
                if (no != null) return Took("player:decline_vending", v, no.Index);
            }
            // The permit official: buy / bribe (the episode donates goods) / refuse; "later" whenever offered.
            // Georgio's loan (he repays four times as much on a rent day you are short): `georgio: lend_25 |
            // lend_50 | no`.
            // Default lend_50: 27/30 buyouts against 24/30 lending 25 % (paired, 1000-day horizon, normal).
            string g = D.Raw.Value<string>("georgio") ?? (D.Raw["georgio_loan"]?.ToString() switch { "25" => "lend_25", "50" => "lend_50", "decline" => "no", _ => "lend_50" });
            if (v.ClientId == "georgioLoan" && g != null)
            {
                var enabled = v.Choices.Where(x => x.Enabled).ToList();
                var pick = g == "lend_50" ? enabled.Skip(1).FirstOrDefault() : g == "no" ? enabled.LastOrDefault() : enabled.FirstOrDefault();
                if (pick != null) return Took("player:georgio_" + g, v, pick.Index);
            }
            if (v.ClientId == "loan_shark2" || v.ClientId == "loan_shark_collect")
            {
                int i = Mortgage(v);
                if (i >= 0) return i;
            }
            // A shady inspector's bribe: paid when it is under the expected loss of what is not concealed (each
            // stolen item is found at its heat %, contraband always) and under `conceal.bribe_max` (default 200).
            if (v.ClientId == "securityInspector")
            {
                var bribe = v.Choices.FirstOrDefault(x => x.Enabled && ((x.Label ?? "").IndexOf("bribe", StringComparison.OrdinalIgnoreCase) >= 0 || (x.Text ?? "").IndexOf("bribe", StringComparison.OrdinalIgnoreCase) >= 0 || (x.Text ?? "").IndexOf("credits", StringComparison.OrdinalIgnoreCase) >= 0));
                if (bribe != null)
                {
                    var m = System.Text.RegularExpressions.Regex.Match((bribe.Text ?? "") + " " + (bribe.Label ?? ""), @"(\d+)");
                    int amount = m.Success ? int.Parse(m.Groups[1].Value) : int.MaxValue;
                    double loss = Conceal.ExposedLoss();
                    int max = (D.Raw["conceal"] as Newtonsoft.Json.Linq.JObject)?.Value<int?>("bribe_max") ?? 200;
                    if (amount <= max && amount < loss && v.Cash >= amount) return Took("player:bribe_inspector", v, bribe.Index);
                    var other = v.Choices.FirstOrDefault(x => x.Enabled && x != bribe);
                    if (other != null) return Took("player:decline_inspector_bribe", v, other.Index);
                }
            }
            if (v.ClientId == "businessPermitOfficial")
            {
                string mode = PermitMode(v.Cash, v.RentDue);
                Permit = mode == "buy" ? "buy" : "refuse";
            }
            return base.Dialogue(v);
        }

        // The mortgage is the other way to win: after the down payment, weekly payments replace the rent and
        // the last one ends the run as a buyout (`buyout_mortgage`); a missed one repossesses goods worth it.
        // Take it when the cash after the down payment plus a cautious twelve weeks of the shop's
        // earnings (rent added back, since it stops) covers the payments; else ask him back next week.
        int Mortgage(DialogueView v)
        {
            var ps = PlayerStore.Instance;
            if (v.ClientId == "loan_shark_collect")
            {
                var pay = v.Choices.FirstOrDefault(c => c.Enabled && c.Label == "yes choice");
                return pay != null ? Took("player:mortgage_pay", v, pay.Index) : -1;
            }
            string mode = D.Raw.Value<string>("mortgage") ?? (D.Raw.Value<string>("loan") == "accept_if_rent" ? "accept_if_rent" : "auto");
            // Bridging a rent the cash cannot pay: the weekly payment replaces the rent from today.
            if (mode == "accept_if_rent") mode = v.Cash - ps.GetMortgageDownPayment() < ps.GetRentValue() + (P?.Floor ?? 0) || v.Cash < ps.GetRentValue() ? "take" : "auto";
            int down = ps.GetMortgageDownPayment(), weekly = ps.GetMortgageWeeklyPaymentPreview(), weeks = PlayerStore.GetMortgageWeeks();
            var yes = v.Choices.FirstOrDefault(c => c.Enabled && c.Label == "yes choice");
            var soon = v.Choices.FirstOrDefault(c => c.Enabled && c.Label == "soon choice");
            var no = v.Choices.FirstOrDefault(c => c.Enabled && c.Label == "no choice");
            // `buyout_by_day: X` (gym): while the cash buyout projects to be affordable by day X, ask him back next
            // week instead of signing; once it no longer does, `mortgage` decides as usual. The projection starts on
            // `buyout_check_from` (default 84): before it, Georgio's payback and the appraisal ramp make four weeks'
            // gain undershoot (docs/PROGRESS.md, cash buyout).
            if (mode != "decline" && soon != null && D.Raw.Value<int?>("buyout_by_day") is int by && CashBuyoutBy(by))
                return Took("player:mortgage_later_cash", v, soon.Index);
            bool take = mode == "take";
            if (mode == "auto" && P?.NetGainWeek is int gain)
            {
                // Goods the loan shark can repossess cover a missed payment; the rest of the stock may not.
                long stock = PlayerStock.RepoValue();
                long can = v.Cash - down + stock / 2 + (long)(weeks * 0.8 * (gain + ps.rentValue));
                // And a float: after the down payment, one and a half payments plus ~1 000 to keep buying
                // (Georgio's pending payback counts).
                int georgio = ps.loanOfferedToGeorgio > 0 ? ps.loanOfferedToGeorgio * 4 : 0;
                take = v.Cash - down + georgio >= weekly * 1.5 + (D.Raw.Value<int?>("mortgage_float") ?? 1000)
                    && can + georgio >= (long)(weekly * weeks * (D.Raw.Value<double?>("mortgage_margin") ?? 1.0));
            }
            // `mortgage_window: [from, to]`: game days on which auto/take may sign.
            if (D.Raw["mortgage_window"] is Newtonsoft.Json.Linq.JArray w && w.Count == 2 && (P?.Day < (int)w[0] || P?.Day > (int)w[1])) take = false;
            // `mortgage_cover: k`: sign only while the cash left after the down payment plus the repossessable
            // stock covers k payments.
            if (take && D.Raw.Value<double?>("mortgage_cover") is double cover && v.Cash - down + PlayerStock.RepoValue() < cover * weekly) take = false;
            if (take && yes != null) return Took("player:mortgage_take", v, yes.Index);
            if (mode == "decline" && no != null) return Took("player:mortgage_decline", v, no.Index);
            return soon != null ? Took("player:mortgage_later", v, soon.Index) : -1;
        }

        // Cash, 70 % of the sellable stock and the last four weeks' net-worth gain carried forward cover the
        // price on day `by` (base + the daily rise after day 50, capped: DifficultyRules).
        static bool CashBuyoutBy(int by)
        {
            var r = Difficulty.Rules;
            int? gain = P?.NetGainWeek4;
            if (r == null || gain == null) return false;
            int today = P.Day;
            if (today >= by) return false;
            if (today < (D.Raw.Value<int?>("buyout_check_from") ?? 84)) return true;
            long price = r.propertyBasePrice + Math.Min((long)r.propertyPriceIncreaseCap, (long)r.propertyPriceIncreasePerDay * Math.Max(0, by - 50));
            double projected = PlayerStore.Instance.playerCash + PlayerStock.Value() * 0.7 + Math.Max(0, gain.Value) * (by - today) / 7.0;
            return projected >= price;
        }

        // buy when the cash covers fee + rent + the floor (or the plan says buy and it is affordable);
        // otherwise bribe from day 45 (25-95 a week in goods, docs/re/MITIGATIONS.md), which is far cheaper
        // for a shop whose cash is its working capital.
        // Set when a visit found too few goods to donate: the bribe route is failing, buy as soon as affordable.
        public static bool BribeFailing;

        public static string PermitMode(int cash, int rentDue)
        {
            int fee = PlayerStore.GetBusinessPermitPrice();
            string want = D.Permit;
            if (want == "refuse") return "refuse";
            // Under a mortgage the next payments are what the cash must keep covering (rent no longer is).
            var ps = PlayerStore.Instance;
            if (ps.isPayingMortgage) rentDue = 2 * ps.mortgageWeeklyPayment;
            bool affordable = cash >= fee + rentDue + Math.Max(P?.Floor ?? 0, 0);
            if (want == "buy") return affordable ? "buy" : "bribe";
            if (want == "bribe") return "bribe";
            // Unset: buy when the fee leaves the rent, the floor and 500 covered (an unlicensed shop's record
            // ends in an arrest whenever the shelf is too bare to bribe), or once the bribe debt piles up;
            // else bribe. `permit: bribe` never buys.
            // Hoarding toward the mortgage threshold (`mortgage_save_from_day` reached, or `mortgage: take` before
            // signing): the 2 000 fee waits unless the evidence is already high.
            bool hoarding = !ps.isPayingMortgage && ((D.Raw.Value<int?>("mortgage_save_from_day") is int sf && (P?.Day ?? 0) >= sf) || D.Raw.Value<string>("mortgage") == "take");
            if (hoarding && (ps.secData?.evidenceLevel ?? 0) < 60) return "bribe";
            // Under the bribe route, OPERATING_WITHOUT_LICENSE still lands when a donation falls short; once the
            // evidence passes 30 the permit (fee + rent) is cheaper than the arrest.
            if ((ps.secData?.evidenceLevel ?? 0) >= (D.Raw.Value<int?>("permit_evidence") ?? 30) && cash >= fee + rentDue) return "buy";
            return affordable && (BribeFailing || cash >= fee + rentDue + Math.Max(P?.Floor ?? 0, 0) + 500 || PlayerStore.Instance.storeClientData.businessPermitBribeAmount > 300) ? "buy" : "bribe";
        }

        // `jackson`: the list of item ids to buy at Dr. Jackson's (default: the water sign); `jackson_every` nights
        // between visits (default 7; 0 = never). `jackson_equipment: true` (gym) also buys there what the engines
        // want (Production.Wants: the furnace and its modules, hydroponic bays, cells, the purifier, the
        // bioreactor), `staged` being how many of that id are already on his counter.
        static List<string> WindowSigns => (D.Raw["window_signs"] as Newtonsoft.Json.Linq.JArray)?.Select(x => (string)x).ToList();
        static bool JacksonEquipment => D.Raw.Value<bool?>("jackson_equipment") ?? false;

        public static bool JacksonWants(GameItem i, int staged)
        {
            if (JacksonEquipment && Production.IsInput(i.identifier) && (i.identifier != "energy_credit" || Production.Charged(i))
                && staged < Production.Wants(i.identifier)) return true;
            var ids = (D.Raw["jackson"] as Newtonsoft.Json.Linq.JArray)?.Select(x => (string)x).ToList()
                ?? new List<string> { "sign_water" };
            if (WindowSigns is List<string> signs) ids = ids.Concat(signs).ToList();
            int wineWants = Production.JacksonWineWants(i.identifier);
            if (wineWants > 0) return EmporiumEntry.Instance.GetAllOwnedItems(false, true).Count(x => x != null && x.identifier == i.identifier) + staged < wineWants;
            if (!ids.Contains(i.identifier)) return false;
            if (i.identifier == "wine_rack" && !Production.WineOn) return false;
            // A sign in the display case brings that trade's supplier three days in four
            // (ShowcaseHelper display predictions): the water sign, water for the cellar.
            if (i.identifier == "sign_water" && !Production.WineOn && !Production.WaterSellOn) return false;
            var owned = EmporiumEntry.Instance.GetAllOwnedItems(false, true);
            return owned.Count(x => x != null && x.identifier == i.identifier) < (i.identifier == "storage_bay" ? 2 : 1);
        }

        static readonly string[] JacksonStock = { "furnace", "furnace_module_junk", "furnace_module_blast", "hydroponic", "water_purifier", "wine_yeast_infinite" };
        int lastJackson = -1000;

        public override NightPlan Night(NightView v)
        {
            var plan = base.Night(v);
            int every = D.Raw.Value<int?>("jackson_every") ?? 7;
            // `jackson_sunday: true`: go on Sundays instead, when his prices are 10 % off (VanillaAfterhourEvents
            // jackson_discount); `jackson_every` then counts Sundays.
            bool sunday = D.Raw.Value<bool?>("jackson_sunday") ?? false;
            bool due = every > 0 && (sunday ? AfterhourEvents.IsActive("jackson_discount") && (v.Day / 7) % Math.Max(1, every / 7) == 0 : v.Day % every == 6);
            // `jackson_need: N` (gym, with `jackson_equipment`): also go when an engine wants equipment he always or often
            // stocks, at most once in N nights.
            int need = D.Raw.Value<int?>("jackson_need") ?? 0;
            bool needed = need > 0 && v.Day - lastJackson >= need && JacksonEquipment && JacksonStock.Any(id => Production.Wants(id) > 0);
            if ((due || needed) && v.Locations.Contains("dr_jackson") && v.Cash > v.RentDue + 800)
            {
                plan.Outing = "dr_jackson";
                plan.Scavenge = false;
                lastJackson = v.Day;
                defaults.Take("player:jackson_visit");
            }
            // `scavenge_max_wound: k` (gym): no dump outing while the wound is worse than k (HealthData.woundState, which
            // the night log tells a player: +2 a minor, +5 a major, -4 a night once stable). Above 5 the door is shut
            // until it heals; one more wound gets there from 1 (a major) or from 4 (a minor). Unset: no such rest.
            if (plan.Outing == "dumping_ground" && D.Raw.Value<int?>("scavenge_max_wound") is int maxWound && PlayerStore.Instance.healthData.woundState > maxWound)
            {
                plan.Outing = null;
                plan.Scavenge = false;
                defaults.Take("player:wound_rest", "wound " + PlayerStore.Instance.healthData.woundState);
            }
            // `scavenge_every: N`: a dump outing costs the next day a customer; search one night in N.
            int scavEvery = D.Raw.Value<int?>("scavenge_every") ?? 1;
            if (D.Scavenge == false || (plan.Outing == "dumping_ground" && scavEvery > 1 && v.Day % scavEvery != 0))
            {
                plan.Outing = null;
                plan.Scavenge = false;
            }
            plan.CallWild = true;
            return plan;
        }

        // The plan's services are on whatever the cash (the tablet still drops the dearest it cannot pay).
        // Unset: insurance and janitorial stay on unless tonight's bill itself is unaffordable (garbage
        // halves attractiveness and with it the queue; an uninsured shop is burgled), and the power service
        // is off while nothing owned draws store power.
        public override ISet<string> Services(NightView v)
        {
            var on = new HashSet<string>(StringComparer.Ordinal);
            if (D.Services != null)
            {
                foreach (var id in D.Services)
                    if (v.Services.Any(x => x.Id == id && x.Unlocked)) on.Add(id);
                defaults.Take("player:services_plan", string.Join(",", on));
                return on;
            }
            foreach (var x in v.Services.Where(x => x.Using && !x.Mandatory)) on.Add(x.Id);
            // `services_mode: ev` (experts' habit, docs/research/EXPERT-HYPOTHESES.md H3): no insurance in the
            // first three nights (the new-shop burglary grace) or while the stock is cheap next to the fee;
            // janitorial every second night.
            bool ev = (D.Raw.Value<string>("services_mode") ?? "ev") == "ev";
            int bill = 0;
            foreach (var id in new[] { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" })
            {
                var x = v.Services.FirstOrDefault(y => y.Id == id && y.Unlocked);
                if (x == null) continue;
                bool want = !ev || (id == "INSURANCE_SERVICE" ? v.Day > 3 && PlayerStock.Value() * 0.35 > x.Cost : v.Day % 2 == 0);
                // `alarm: perfect` (Modules.cs): no insurance on a night the alarm stops every burglary.
                if (want && id == "INSURANCE_SERVICE" && Modules.SkipInsurance(v.Day, x.Cost)) want = false;
                if (want && id == "INSURANCE_SERVICE" && (P?.Cuts("insurance") ?? false)) want = false;
                if (want && v.Cash >= bill + x.Cost) { on.Add(id); bill += x.Cost; }
                else on.Remove(id);
            }
            var power = v.Services.FirstOrDefault(y => y.Id == "POWER_SERVICE" && y.Unlocked && !y.Mandatory);
            if (power != null && !DrawsStorePower()) on.Remove(power.Id);
            defaults.Take("player:services", string.Join(",", on));
            return on;
        }

        static bool DrawsStorePower() => EmporiumEntry.Instance.GetAllOwnedItems(false, false).Any(i => i != null && (CurrentConverterHelper.IsCurrentConverter(i) || (i.identifier ?? "").Contains("recharger") || (i.identifier ?? "").Contains("powerblock")));

        // Grace first (a missed rent ends the run without it), then the plan's list, then the engine's
        // upgrades when the cash after them still covers the rent and the floor.
        static readonly HashSet<string> Essential = new HashSet<string>(StringComparer.Ordinal) { "LANDLORD_GRACE", "WINEMAKER", "RETIRED_FARMER", "WATER_TRADER", "GUTTERFLOW_TAP", "RUSTWATER_TAP" };

        public override IList<string> Upgrades(UpgradeView v)
        {
            var order = new List<string> { "LANDLORD_GRACE" };
            if (Furnace.WantsMiner) order.Add("MINER");
            bool vend = Vending.Buys && (P?.Day ?? 0) >= Vending.BuyDay;
            if (vend) order.Add("VENDING_MACHINE");
            bool fount = Fountain.Buys && (P?.Day ?? 0) >= Fountain.BuyDay;
            if (fount) order.Add("VENDING_FOUNTAIN");
            order.AddRange(Suppliers.Upgrades());
            if (D.UpgradesBuy != null) order.AddRange(D.UpgradesBuy);
            else order.AddRange(Engine.Upgrades(v));
            // Growth bought with Wild favor (a currency good for nothing else) or real surplus: more traffic
            // and window buyers, no running cost.
            var growth = (D.Raw["growth"] as Newtonsoft.Json.Linq.JArray)?.Select(x => (string)x).ToArray()
                ?? new[] { "RENOVATION1", "SHOWCASE_I", "RENOVATION2", "RENOVATION3", "SHOWCASE_II" };
            if (D.UpgradesBuy == null) order.AddRange(growth);
            // Favor left over buys anything else useful (favor is good for nothing but upgrades).
            // With wine running, the tap (filtered, tablet-treated tap water for the bays and bottles) on favor alone.
            var extras = new[] { "BUY_REFERAL", "SELL_REFERAL", "MARKETPLACE_REQUESTS", "SHOWCASE_II", "RENOVATION2", "RENOVATION3" }
                .Concat(Production.WineOn ? new[] { "WATER_TRADER", "GUTTERFLOW_TAP", "RUSTWATER_TAP" } : new string[0]).ToArray();
            if (D.Raw.Value<bool?>("spend_favor") ?? true) order.AddRange(extras);
            if ((PlayerStore.Instance.secData?.evidenceLevel ?? 0) >= 50) order.Add("EVIDENCE_REMOVAL");
            var picks = new List<string>();
            if (Environment.GetEnvironmentVariable("FENCE_WILD_DEBUG") != null)
                Console.Error.WriteLine($"wild d{P?.Day} cash={v.Cash} favor={v.Favor} order={string.Join(",", order)} | " + string.Join(" ", v.Upgrades.Where(u => order.Contains(u.Id)).Select(u => $"{u.Id}:{u.Cost}:s{u.State}:b{u.Buyable}:u{u.Unlocked}")));
            int funds = v.Cash + v.Favor;
            int keep = v.RentDue + Math.Max(P?.Floor ?? 0, 0) + (D.UpgradeReserve ?? 0);
            int favor = v.Favor;
            foreach (var id in order.Distinct())
            {
                var u = v.Upgrades.FirstOrDefault(x => x.Id == id);
                if (u == null || !u.Buyable || u.Cost > funds) continue;
                if (id != "LANDLORD_GRACE" && funds - u.Cost < keep && favor < u.Cost) continue;
                if ((id == "VENDING_MACHINE" && vend) || (id == "VENDING_FOUNTAIN" && fount) || Suppliers.Wants(id))
                {
                    if (funds - u.Cost < keep + (id == "VENDING_MACHINE" ? Vending.Reserve : id == "VENDING_FOUNTAIN" ? Fountain.Reserve : Suppliers.Reserve(id))) continue;
                    picks.Add(id);
                    funds -= u.Cost;
                    favor = Math.Max(0, favor - u.Cost);
                    continue;
                }
                if (growth.Contains(id) && D.UpgradesBuy == null && favor < u.Cost && funds - u.Cost < keep + 1500) continue;
                if (!growth.Contains(id) && extras.Contains(id) && favor < u.Cost && !(D.UpgradesBuy?.Contains(id) ?? false)) continue;
                // Cash beyond favor: the engine's essentials pay it; anything else only from a real surplus,
                // and never more than `upgrades_cash_max` (favor pays first, as the game does).
                int cash = Math.Max(0, u.Cost - favor);
                bool essential = Essential.Contains(id) || (id == "MINER" && Furnace.WantsMiner) || (id == "PLANT_DNA" && Production.DnaOn);
                // `upgrades_cash_max` applies within `upgrades_cash_max_days: [from, to]` when given.
                bool inWindow = !(D.Raw["upgrades_cash_max_days"] is Newtonsoft.Json.Linq.JArray cw && cw.Count == 2) || ((P?.Day ?? 0) >= (int)cw[0] && (P?.Day ?? 0) <= (int)cw[1]);
                int cashMax = inWindow ? D.Raw.Value<int?>("upgrades_cash_max") ?? int.MaxValue : int.MaxValue;
                // `wine_dna` sets its own cash test for PLANT_DNA (Production.PlanDna).
                if (cash > cashMax && !(id == "PLANT_DNA" && Production.DnaOn)) continue;
                if (!essential && cash > 0 && funds - u.Cost < keep + 1500) continue;
                if (id != "LANDLORD_GRACE" && cash > 0 && (P?.Cuts("upgrades") ?? false)) continue;
                picks.Add(id);
                funds -= u.Cost;
                favor = Math.Max(0, favor - u.Cost);
            }
            defaults.Take(picks.Count > 0 ? "player:wild_buy" : "player:wild_none", v.Via + ": " + string.Join(",", picks));
            return picks;
        }
    }

    // The engine path the executor builds when the plan does not name upgrades (production lives in
    // Production.cs); none until an engine is chosen.
    internal static partial class Engine
    {
        public static IEnumerable<string> Upgrades(UpgradeView v) => Plan(v);

        static partial void PlanImpl(UpgradeView v, List<string> into);

        static IEnumerable<string> Plan(UpgradeView v)
        {
            var l = new List<string>();
            PlanImpl(v, l);
            return l;
        }
    }

    // The counter half of the executor: the in-process answer to a barter decide (GymHeuristic routes here
    // when the human role is the player). Reads only the request, like a bridge policy, plus the day's plan.
    internal static class PlayerPolicy
    {
        public static bool Active;

        static readonly int[] Pcts = { 10, 25, 50, 75 };

        static bool Has(DecideRequest req, string id) => req.Options.Any(o => o.Id == id);

        static (double accept, double walk) Odds(DecideRequest req, string id)
        {
            string label = req.Options.FirstOrDefault(o => o.Id == id)?.Label ?? "";
            if (label.Contains("always accepted")) return (1, 0);
            var a = System.Text.RegularExpressions.Regex.Match(label, @"accept (\d+)%");
            var w = System.Text.RegularExpressions.Regex.Match(label, @"walk out (\d+)%");
            return (a.Success ? int.Parse(a.Groups[1].Value) / 100.0 : 0, w.Success ? int.Parse(w.Groups[1].Value) / 100.0 : 0);
        }

        static double MarkupBudgetCut(int pct) => pct <= 10 ? 0.10 : pct <= 25 ? 0.15 : pct <= 50 ? 0.20 : 0.25;

        public static string Pick(DecideRequest req, out string why)
        {
            var c = req.Context;
            string intent = c?["client"]?.Value<string>("intent");
            var theirs = c?["counter"]?["theirs"] as JArray ?? new JArray();
            var yours = c?["counter"]?["yours_staged"] as JArray ?? new JArray();
            bool sells = intent == "buy" || (intent == "sell_and_buy" && yours.Count > 0);
            // Furnace.cs: Dr. Jackson buys materials and sells modules; with both sides staged there is no
            // sell_all and buy_all settles the exchange.
            if (sells && intent == "sell_and_buy" && !Has(req, "sell_all") && Has(req, "buy_all") && (Furnace.BuyingInputs(theirs) || Modules.BuyingInputs(theirs))) sells = false;
            bool buys = intent == "sell" || (intent == "sell_and_buy" && (yours.Count == 0 || !sells) && theirs.Count > 0);
            if (sells) return Sell(req, c, yours, out why);
            if (buys) return Buy(req, c, theirs, out why);
            why = "nothing to weigh";
            return Has(req, "dismiss") ? "dismiss" : null;
        }

        static string Sell(DecideRequest req, JObject c, JArray yours, out string why)
        {
            var d = Directives.Current;
            var p = PlayerDay.Current;
            // Water refill buyers are swaps (`sell_and_buy`): they bring empties and take filled water; there
            // is no sell_all, and buy_all closes the deal (they pay and leave the empties).
            var theirsHere = c["counter"]?["theirs"] as JArray ?? new JArray();
            if (yours.Count > 0 && !Has(req, "sell_all") && Has(req, "buy_all") && c["client"]?.Value<string>("intent") == "sell_and_buy"
                && yours.All(i => PlayerStock.OnCounter(i.Value<int?>("uid") ?? -1) is GameItem g && g.GetGameItemType().Contains("WATER"))
                && theirsHere.All(i => PlayerStock.OnCounter(i.Value<int?>("uid") ?? -1) is GameItem t && t.IsTag("LIQUID_CONTAINER_TAG") && WaterHelper.IsEmpty(t)))
            {
                why = "swap: their empties for our water";
                return "buy_all";
            }
            if (yours.Count == 0 || !Has(req, "sell_all"))
            {
                if (yours.Count == 0)
                    foreach (var t in c["client"]?["interests"] as JArray ?? new JArray())
                        PlayerDemand.Missed((string)t, p?.Day ?? 0);
                why = "nothing sellable staged";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            long price = yours.Sum(i => i.Value<long?>("price") ?? 0);
            long est = yours.Sum(i => i.Value<long?>("estimated_value") ?? 0);
            long? budget = c["client"]?.Value<long?>("budget");
            double B = budget ?? double.MaxValue;
            bool liq = p?.Liquidating ?? false;
            // Prices on the counter carry the retail markup; the floor is a share of the estimate.
            // The plan's floor for ordinary sales and the sell-down floor are separate: liquidating takes the
            // lower of the two.
            int normalPct = d.MinPct ?? 75;
            double minPct = (liq ? Math.Min(normalPct, d.LiquidateMinPct ?? 65) : normalPct) / 100.0;
            double floor = est * minPct;

            string best = null;
            double bestValue = -1;
            void Consider(string id, double v)
            {
                if (v + 0.5 >= floor && v > bestValue + 0.5) { best = id; bestValue = v; }
            }
            Consider("sell_all", Math.Min(price, B));
            string mode = d.Bargain ?? "ev_positive";
            if (mode != "never" && budget.HasValue)
                foreach (int n in Pcts)
                    if (Has(req, "offer_discount_" + n))
                        Consider("offer_discount_" + n, Math.Min(price * (1 - n / 100.0), budget.Value * (1 + 2 * n / 100.0)));
            if ((mode == "ev_positive" && !liq) || mode == "aggressive")
            {
                double baseValue = Math.Max(bestValue, 0);
                foreach (int n in Pcts)
                {
                    string id = "add_markup_" + n;
                    if (!Has(req, id)) continue;
                    var (s, w) = Odds(req, id);
                    double up = Math.Min(price * (1 + n / 100.0), B * (1 - MarkupBudgetCut(n)));
                    double v = s * up + (1 - s - w) * baseValue;
                    if (mode == "aggressive" && n == 25) v = double.MaxValue / 4;
                    if (v > bestValue + 0.5 && up >= floor) { best = id; bestValue = v; }
                }
            }
            if (best == null)
            {
                why = $"best offer under {minPct:P0} of estimate {est}";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            why = $"{best} for about {bestValue:0} (estimate {est}{(liq ? ", liquidating" : "")})";
            return best;
        }

        static string Buy(DecideRequest req, JObject c, JArray theirs, out string why)
        {
            var d = Directives.Current;
            var p = PlayerDay.Current;
            int cap = d.HotStockCap ?? int.MaxValue;
            long cash = c["store"]?.Value<long?>("cash") ?? 0;
            // `theirs` lists everything on the counter; buy_all takes only what was staged on the back
            // counter (the player sees which), wholesale takes the whole lot.
            var back = PlayerStock.BackUids();
            var staged = new JArray(theirs.Where(i => back.Contains(i.Value<int?>("uid") ?? -1)).ToArray());
            if (staged.Count == 0) staged = theirs;
            // Contraband and heat are judged on what each option buys: buy_all takes the staged items,
            // wholesale the whole lot (a seller's other goods may be contraband the player never stages).
            bool buyContraband = d.Raw["buy"]?.Value<bool?>("contraband") ?? false;
            bool Bad(JToken i) => !buyContraband && ((i.Value<bool?>("serious_contraband") ?? false) || (i.Value<int?>("contraband_level") ?? 0) > 0);
            bool HotFits(IEnumerable<JToken> lot)
            {
                var hot = lot.Where(i => (i.Value<int?>("stolen_heat") ?? 0) > 0 || (i.Value<int?>("contraband_level") ?? 0) > 0).ToList();
                if (hot.Count == 0) return true;
                // With a smuggler's bay, hot goods are capped by its free room instead of a count.
                if (Conceal.Bays().Count > 0 && hot.Sum(i => PlayerStock.CellsOf(i.Value<int?>("uid") ?? -1)) <= Conceal.FreeCells()) return true;
                return PlayerStock.HotCount() + hot.Count <= cap;
            }
            bool stagedOk = !staged.Any(Bad) && HotFits(staged);
            bool lotOk = !theirs.Any(Bad) && HotFits(theirs);
            if (!stagedOk && !lotOk)
            {
                why = staged.Any(Bad) ? "contraband" : "hot stock cap";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            long est = staged.Sum(i => i.Value<long?>("estimated_value") ?? 0);
            long price = staged.Sum(i => i.Value<long?>("price") ?? 0);
            long estAll = theirs.Sum(i => i.Value<long?>("estimated_value") ?? 0);
            long? wholesale = c["client"]?["wholesale"]?.Type == JTokenType.Object ? c["client"]["wholesale"].Value<long?>("price") : null;
            long floor = p?.Floor ?? 0;
            long spendLeft = d.MaxSpend.HasValue ? d.MaxSpend.Value - (p?.Spent ?? 0) : long.MaxValue;
            // `mortgage_max_spend`: a daily cap on trading purchases that applies only while paying the mortgage.
            if (PlayerStore.Instance.isPayingMortgage && d.Raw.Value<long?>("mortgage_max_spend") is long mms) spendLeft = Math.Min(spendLeft, mms - (p?.Spent ?? 0));
            bool liq = p?.Liquidating ?? false;
            // Hard sells at the estimate (no retail markup), so the shop lives on volume bought just under it.
            bool hard = PlayerStore.Instance.difficultyId == "hard" || PlayerStore.Instance.difficultyId == "challenge";
            double wsMax = (d.WholesaleMaxPct ?? (liq ? 70 : hard ? 95 : 85)) / 100.0;
            // Lean stock starves the counter: below the restock level a lot at its estimate is worth taking
            // (it sells at the retail markup); above it only discounted lots are.
            // Under the mortgage, goods worth a payment turn a missed payment into a repossession instead of
            // the end of the run: keep 1.5 payments of repo-eligible stock.
            var pst = PlayerStore.Instance;
            bool repoShort = pst.isPayingMortgage && PlayerStock.RepoValue() < pst.mortgageWeeklyPayment * (d.Raw.Value<double?>("repo_reserve_payments") ?? 1.5);
            bool restock = (!liq && PlayerStock.Value() < (d.RestockBelow ?? 0)) || repoShort;
            double singleMax = (d.SingleMaxPct ?? (liq ? 70 : restock ? 100 : 85)) / 100.0;
            bool inputsOnly = staged.Count > 0 && staged.All(i => Production.IsInput(i.Value<string>("id")));
            // The plan's preferred types (an announced shortage, a persona's line) are bought up to their estimate.
            // Types buyers asked for this week and found nothing of (H16, stock breadth): restock them at
            // their estimate (they sell at the retail markup) while the shelf holds fewer than two.
            var prefer = d.PreferTypes.Concat(p?.RisingTypes ?? new HashSet<string>()).Concat(p?.ShowType != null && (d.Raw.Value<bool?>("restock_show_type") ?? true) ? new[] { p.ShowType } : new string[0])
                .Concat(d.Raw.Value<bool?>("restock_missed") == false ? Enumerable.Empty<string>() : PlayerDemand.Wanted(p?.Day ?? 0)).ToList();
            if (prefer.Count > 0 && staged.Count > 0 && staged.All(i => PlayerStock.OnCounter(i.Value<int?>("uid") ?? -1) is GameItem g && PlayerRole.TypeIs(g, prefer)))
            {
                // Restocking, not speculating: single lots at their estimate only while small.
                if (price <= (d.Raw.Value<int?>("restock_lot_max") ?? 300)) singleMax = Math.Max(singleMax, 1.0);
                wsMax = Math.Max(wsMax, 1.0);
            }
            // `breadth`: goods of the types the buyers ask for and the shop lacks, staged on their own.
            if (Breadth.On && staged.Count > 0 && Breadth.Covers(staged.Select(i => PlayerStock.OnCounter(i.Value<int?>("uid") ?? -1)).Where(g => g != null), p?.Day ?? 0)
                && price <= Breadth.LotMax) singleMax = Math.Max(singleMax, Breadth.MaxPct);
            if (!inputsOnly && d.StockCap.HasValue && PlayerStock.Value() > d.StockCap.Value)
            {
                why = "stock over the plan's cap";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            int free = PlayerStock.FreeCells();
            int cells = staged.Sum(i => PlayerStock.CellsOf(i.Value<int?>("uid") ?? -1));
            int cellsAll = theirs.Sum(i => PlayerStock.CellsOf(i.Value<int?>("uid") ?? -1));
            bool Afford(double cost) => cash - cost >= floor && cost <= spendLeft;
            if (p?.SpendDownTo is int sdt && cash > sdt)
            {
                // Spending down for Georgio: anything at or under its estimate.
                wsMax = Math.Max(wsMax, 1.0);
                singleMax = Math.Max(singleMax, 1.0);
            }
            // Engine inputs keep flowing under a mortgage while cash + 70 % of the sellable stock still covers
            // the next payment (the payment floor alone starved the shop of bottles and water), and are exempt
            // from the plan's spend cap and cash floor unless `inputs_max_spend` / `inputs_floor` say otherwise.
            long inputsFloor = d.Raw.Value<long?>("inputs_floor") ?? (pst.isPayingMortgage ? 0 : (p?.Rent2Floor ?? floor));
            long inputsSpendLeft = d.Raw.Value<long?>("inputs_max_spend") is long ims ? ims - (p?.Spent ?? 0) : long.MaxValue;
            bool AffordInput(double cost)
            {
                if (cost > inputsSpendLeft || cash - cost < inputsFloor) return false;
                if (p?.Cuts("inputs") ?? false) return false;
                if (!pst.isPayingMortgage) return true;
                return cash - cost + PlayerStock.Value() * 0.7 >= pst.mortgageWeeklyPayment;
            }
            // Engine inputs are bought near their estimate: a bottle turns 12 into 85-175 of wine.
            if (inputsOnly && cells <= free && stagedOk)
            {
                if (wholesale.HasValue && lotOk && Has(req, "accept_wholesale") && AffordInput(wholesale.Value) && staged.Count == theirs.Count)
                {
                    why = $"engine inputs, wholesale {wholesale}";
                    return "accept_wholesale";
                }
                if (Has(req, "buy_all") && price <= est * 1.3 && AffordInput(price))
                {
                    why = $"engine inputs for {price} (estimate {est})";
                    return "buy_all";
                }
            }
            bool wsOk = lotOk && wholesale.HasValue && Has(req, "accept_wholesale") && wholesale.Value <= estAll * wsMax && Afford(wholesale.Value) && cellsAll <= free;
            bool buyOk = stagedOk && Has(req, "buy_all") && price <= est * singleMax && Afford(price) && cells <= free;
            // `short_rent_buy_pct: P` (gym): while the cash is under the rent floor with rent day a few days away,
            // holding it cannot pay the rent and an empty shelf turns buyers away; buy lots at P % of their
            // estimate or less, down to `short_rent_reserve` (default 0) cash, to sell before rent day
            // (`short_rent_days`, default 3, is the nearest rent day it still buys for).
            if (!wsOk && !buyOk && d.Raw.Value<int?>("short_rent_buy_pct") is int srPct && p != null && cash < floor && p.RentDaysLeft >= (d.Raw.Value<int?>("short_rent_days") ?? 3) && !pst.isPayingMortgage)
            {
                long reserve = d.Raw.Value<long?>("short_rent_reserve") ?? 0;
                bool Short(long cost, long value) => cost <= value * srPct / 100 && cash - cost >= reserve && cost <= spendLeft;
                wsOk = lotOk && wholesale.HasValue && Has(req, "accept_wholesale") && Short(wholesale.Value, estAll) && cellsAll <= free;
                buyOk = stagedOk && Has(req, "buy_all") && Short(price, est) && cells <= free;
            }

            // Demanding a discount from a seller risks a walk-out (-3 rep): only under ev_positive/aggressive, or the
            // plan's own `bargain_buy` (never | ev_positive | aggressive).
            string mode = d.Raw.Value<string>("bargain_buy") ?? d.Bargain ?? "ev_positive";
            // A demanded discount is a bad bargain for the rep; near -20 Lower Levels (Poor Reviews: a daily
            // customer lost) stop demanding from their sellers.
            double llRep = c["reputation"]?["lower_levels"]?.Value<double?>("value") ?? 0;
            bool hardRules = PlayerStore.Instance.difficultyId == "hard" || PlayerStore.Instance.difficultyId == "challenge";
            if (c["client"]?.Value<string>("faction") == "lower_levels" && llRep <= (d.Raw.Value<double?>("ll_rep_guard") ?? (hardRules ? 0 : -12))) mode = "never";
            if (mode == "ev_positive" || mode == "aggressive")
            {
                double g0 = Math.Max(0, Math.Max(wsOk ? estAll - wholesale.Value : 0, buyOk ? est - price : 0));
                string best = null;
                double bestV = g0;
                foreach (int n in Pcts)
                {
                    string id = "demand_discount_" + n;
                    if (!Has(req, id)) continue;
                    var (s, w) = Odds(req, id);
                    double f = 1 - n / 100.0;
                    double gs = Math.Max(0, Math.Max(stagedOk && Has(req, "buy_all") && cells <= free && price * f <= est * singleMax && Afford(price * f) ? est - price * f : 0,
                        lotOk && wholesale.HasValue && cellsAll <= free && wholesale.Value * f <= estAll * wsMax && Afford(wholesale.Value * f) ? estAll - wholesale.Value * f : 0));
                    // A walk-out costs 3 rep; count it as a few credits of future trade.
                    double v = s * gs + (1 - s - w) * g0 - w * 15;
                    if (mode == "aggressive" && n == 25) v = double.MaxValue / 4;
                    if (v > bestV + 0.5) { best = id; bestV = v; }
                }
                if (best != null)
                {
                    why = $"{best} (EV {bestV:0} vs {g0:0})";
                    return best;
                }
            }
            if (wsOk)
            {
                why = $"wholesale {wholesale} for an estimate of {est}";
                return "accept_wholesale";
            }
            if (buyOk)
            {
                why = $"buy {price} for an estimate of {est}";
                return "buy_all";
            }
            why = $"asking {price} (wholesale {wholesale?.ToString() ?? "-"}) against {est}: over the plan's limits or the floor {floor}";
            return Has(req, "dismiss") ? "dismiss" : null;
        }
    }

    // Buyers turned away for want of a type, by day (the player remembers who asked for what).
    internal static class PlayerDemand
    {
        static readonly List<(int day, string type)> missed = new List<(int, string)>();

        public static void Reset() => missed.Clear();

        public static void Missed(string type, int day)
        {
            if (!string.IsNullOrEmpty(type)) missed.Add((day, type));
        }

        public static IEnumerable<string> Wanted(int day)
        {
            missed.RemoveAll(m => m.day < day - 7);
            var owned = EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null && !Production.Reserved(i)).ToList();
            return missed.GroupBy(m => m.type).Where(g => g.Count() >= 3)
                .Select(g => g.Key).Where(t => owned.Count(i => PlayerRole.TypeIs(i, new[] { t })) < 2).ToList();
        }
    }

    // Stock reads the counter policy needs (the player can see all of this on screen).
    internal static class PlayerStock
    {
        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        public static int HotCount() => Owned().Count(StolenHelper.IsStolenItem);

        // Trading stock at estimate: engine inputs and the plan's held items are not stock to cap.
        public static long Value() => Owned().Where(i => !Production.Reserved(i) && !Directives.Current.HoldUids.Contains(i.uniqueId) && (i.parentInventory?.GetParentItem() == null || (Storage.On && Storage.InBay(i)))).Sum(i => (long)PlayerRole.Est(i));

        public static GameItem OnCounter(int uid)
        {
            var e = EmporiumEntry.Instance;
            foreach (var inv in new GameGridInventory[] { e.backInvinvElement, e.frontInvinvElement })
                foreach (var i in inv.childItems)
                    if (i != null && i.uniqueId == uid) return i;
            return null;
        }

        public static long RepoValue()
        {
            var ps = PlayerStore.Instance;
            return ps.GetAllRepoEligibleItems().Sum(i => (long)i.GetCurrentValue(false, true, true, false, 0L));
        }

        public static int FreeCells()
        {
            var inv = EmporiumEntry.Instance.invElement;
            int used = inv.childItems.Where(i => i != null).Sum(PlayerRole.Cells);
            return inv.inventoryShape.width * inv.inventoryShape.height - used;
        }

        public static HashSet<int> BackUids() => new HashSet<int>(EmporiumEntry.Instance.backInvinvElement.childItems.Where(i => i != null).Select(i => i.uniqueId));

        public static int CellsOf(int uid)
        {
            var e = EmporiumEntry.Instance;
            foreach (var inv in new GameGridInventory[] { e.backInvinvElement, e.frontInvinvElement })
                foreach (var i in inv.childItems)
                    if (i != null && i.uniqueId == uid) return PlayerRole.Cells(i);
            return 1;
        }
    }
}
