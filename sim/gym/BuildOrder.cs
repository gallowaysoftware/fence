using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Where a kit part is bought: Wild's network (an upgrade), a seller at the counter, or Dr. Jackson's after-hours shop.
    internal enum BuyAt { Wild, Seller, Jackson }

    internal sealed class BuildPart
    {
        public string Id;
        public int Count;
        public int Price;
        public BuyAt At;
    }

    // One engine's next purchase as a unit: what is still missing, what the engine earns a day once it is all owned,
    // and what must be complete first. Engines describe themselves in BuildKits.cs; the planner never buys a fragment.
    internal sealed class BuildKit
    {
        public string Name;
        public List<BuildPart> Missing = new List<BuildPart>();
        // Net of the kit's running inputs, at the paper rate (`build.income` overrides).
        public double IncomePerDay;
        public int RampDays;
        public string[] Needs = new string[0];
        // Charge a night once running; with a power kit planned, power comes first.
        public int DrawPerNight;
        // Stock-grid cells its loose parts take (storage comes first when the grid cannot take them).
        public int Cells;
        // A prerequisite the shop cannot buy yet (a Wild upgrade not ready): skipped today.
        public string Blocked;
        // Bought first, without the payback test (a storage bay when the grid is short).
        public bool Urgent;

        public int Cost => Missing.Sum(p => p.Price * p.Count);

        public BuildKit Part(string id, int count, int price, BuyAt at)
        {
            if (count > 0) Missing.Add(new BuildPart { Id = id, Count = count, Price = price, At = at });
            return this;
        }
    }

    // `build: {...}` (gym, off when absent): one capital plan for every engine (docs/research/BUILD-ORDER.md). Each
    // morning it holds the rent, the permit (or the official's next donation), the counter's stock money and, before
    // Georgio's loan, everything; then commits to whole kits in order of what they return per credit over the days the
    // run has left. Engines keep their own buying code: the planner answers their gates (Cap, AllowsUpgrade,
    // JacksonWants) for the ids it manages, and nothing else changes.
    internal static class BuildOrder
    {
        static JObject Cfg => Directives.Current.Raw["build"] as JObject;
        public static bool On => Cfg != null;

        static int Int(string k, int def) => Cfg?.Value<int?>(k) ?? def;
        static double Num(string k, double def) => Cfg?.Value<double?>(k) ?? def;
        static double Paper(string table, string kit, double def) => (Cfg?[table] as JObject)?.Value<double?>(kit) ?? def;
        public static double Income(string kit, double def) => Paper("income", kit, def);
        public static int Ramp(string kit, int def) => (int)Paper("ramp", kit, def);

        static HashSet<string> Listed(string key) => Cfg?[key] is JArray a ? new HashSet<string>(a.Select(x => (string)x), StringComparer.Ordinal) : null;
        public static bool KitOn(string kit) => On && (Listed("kits")?.Contains(kit) ?? true) && !(Listed("skip")?.Contains(kit) ?? false);
        static bool Must(string kit) => Listed("must")?.Contains(kit) ?? false;

        // Base prices (hard, as the gym has paid them); a price paid in this run replaces the table's.
        static readonly Dictionary<string, int> Table = new Dictionary<string, int>(StringComparer.Ordinal)
        {
            ["hydroponic"] = 390, ["blender"] = 260, ["heating_plate"] = 325, ["evaporator"] = 390, ["chem_finisher"] = 390,
            ["chem_scanner"] = 520, ["plant_dna_manipulator"] = 1300, ["plant_dna_disk"] = 65, ["gene_scanner"] = 260,
            ["recharger"] = 225, ["water_tank"] = 455, ["water_pitcher"] = 60, ["storage_bay"] = 390, ["storage_bay_chem"] = 400,
            ["energy_credit"] = 150, ["furnace"] = 375, ["printer"] = 455, ["printer_module_metal"] = 325, ["printer_module_cnc"] = 325,
            [Arms.HandmadeChip] = 200, [Arms.HeavyChip] = 260, [Arms.PartChip] = 150,
        };
        static readonly Dictionary<string, int> paid = new Dictionary<string, int>(StringComparer.Ordinal);

        public static int Price(string id)
        {
            if (paid.TryGetValue(id, out int p)) return p;
            if ((Cfg?["prices"] as JObject)?.Value<int?>(id) is int c) return c;
            if (Table.TryGetValue(id, out int t)) return t;
            return PlayerStore.Instance.networkUpgrade.TryGetValue(id, out var u) ? u.GetCost() : 200;
        }

        // ---- state ----------------------------------------------------------------------------------------------

        static readonly List<string> committed = new List<string>();
        static readonly Dictionary<string, BuildKit> today = new Dictionary<string, BuildKit>(StringComparer.Ordinal);
        static readonly HashSet<string> managed = new HashSet<string>(StringComparer.Ordinal);
        static readonly Dictionary<string, JObject> kitLog = new Dictionary<string, JObject>(StringComparer.Ordinal);
        static readonly Dictionary<string, int> declined = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly Dictionary<string, int> waits = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly JArray daysLog = new JArray();
        static int dayNow, held, lastJacksonAsk = -1000, budgetNow;
        static bool frozen;
        public static int Stowed, Displaced;

        public static void Reset()
        {
            committed.Clear();
            today.Clear();
            managed.Clear();
            kitLog.Clear();
            declined.Clear();
            waits.Clear();
            daysLog.Clear();
            paid.Clear();
            tried.Clear();
            committedOwned.Clear();
            dayNow = held = budgetNow = 0;
            lastJacksonAsk = -1000;
            frozen = false;
            Stowed = Displaced = 0;
        }

        // ---- the capital plan ------------------------------------------------------------------------------------

        static int GeorgioDay()
        {
            if (Cfg?.Value<int?>("georgio_day") is int d) return d;
            try { return StorylineSchedule.StartDay("georgio") + 37; }
            catch (Exception) { return 43; }
        }

        static bool LoanPending => PlayerStore.Instance.loanOfferedToGeorgio == 0 && !PlayerStore.Instance.isPayingMortgage && dayNow <= GeorgioDay();

        // Cash that is not the planner's to spend: the rent and services until it is paid, the permit fee (or the
        // official's next donation), the counter's stock money.
        static int Holds(PlayerDay p, IList<JObject> days)
        {
            var ps = PlayerStore.Instance;
            int svc = ps.storeService.Where(s => s.unlocked && s.isUsing).Sum(s => s.cost);
            int rent = p?.RentDue ?? ps.GetRentValue();
            int left = p?.RentDaysLeft ?? ps.dayUntilRent;
            int hold = rent + svc * (Math.Max(0, left) + 1);
            string permit = Directives.Current.Permit ?? "buy";
            if (!ps.hasBusinessPermit)
            {
                if (permit == "buy") hold += PlayerStore.GetBusinessPermitPrice();
                else if (dayNow >= 38)
                {
                    int next = dayNow + (10 - dayNow % 7) % 7;
                    hold += Math.Max(0, ps.storeClientData.businessPermitBribeAmount) + 25 + 10 * (next / 7);
                }
            }
            return hold + TradeMoney(days) + Int("reserve", 0);
        }

        // The counter's stock money: `trade_days` (4) days of what it spent on goods to resell over the last week.
        static int TradeMoney(IList<JObject> days)
        {
            var recent = days.Skip(Math.Max(0, days.Count - 7)).ToList();
            if (recent.Count == 0) return Int("trade_floor", 300);
            double perDay = recent.Average(d => (double)((d["cash_flow"] as JObject)?.Value<int?>("trade_purchases") ?? 0));
            return Math.Max(Int("trade_floor", 300), (int)(perDay * Num("trade_days", 4)));
        }

        // What a credit of stock money earns the counter a day: its margin over the last two weeks on its money.
        static double CounterReturn(IList<JObject> days)
        {
            var recent = days.Skip(Math.Max(0, days.Count - 14)).ToList();
            if (recent.Count < 7) return 0;
            double sold = recent.Sum(d => (double)((d["cash_flow"] as JObject)?.Value<int?>("trade_sales") ?? 0));
            double bought = recent.Sum(d => (double)((d["cash_flow"] as JObject)?.Value<int?>("trade_purchases") ?? 0));
            double money = Math.Max(500, PlayerStock.Value() * 0.6 + TradeMoney(days));
            return Math.Max(0, sold - bought) / recent.Count / money;
        }

        // The last day a purchase can pay for: `horizon`, else the projected cash buyout (the Board routes end there),
        // else `horizon_default` (200).
        static int Horizon(PlayerDay p)
        {
            if (Cfg?.Value<int?>("horizon") is int h) return h;
            var ps = PlayerStore.Instance;
            int fallback = Int("horizon_default", 200);
            if (!(Directives.Current.Raw.Value<int?>("buyout_by_day") is int by) || ps.IsPropertyPaid) return fallback;
            if (!(p?.NetGainWeek4 is int gain) || gain <= 0) return by;
            double liquid = ps.playerCash + PlayerStock.Value() * 0.7;
            double need = ps.GetStoreBuyPrice(false) + 100 - liquid;
            int projected = dayNow + (int)Math.Ceiling(Math.Max(0, need) / (gain / 7.0));
            return Math.Max(dayNow + 14, Math.Min(Math.Max(by, dayNow + 14), projected));
        }

        // Every morning after the day's plan (ExecutorMorning.Plan).
        public static void Morning(PlayerDay p, IList<JObject> days)
        {
            if (!On || p == null) return;
            dayNow = p.Day;
            today.Clear();
            managed.Clear();
            foreach (var k in BuildKits.All())
            {
                foreach (var id in BuildKits.PartIds(k.Name)) managed.Add(id);
                today[k.Name] = k;
            }
            var ps = PlayerStore.Instance;
            held = Holds(p, days);
            int budget = ps.playerCash - held;
            int georgio = GeorgioDay();
            frozen = LoanPending && dayNow > georgio - Int("georgio_hold_days", 7);
            int horizon = Horizon(p);
            double rho = CounterReturn(days);
            var log = new JObject { ["day"] = dayNow, ["cash"] = ps.playerCash, ["held"] = held, ["horizon"] = horizon, ["counter_return"] = Math.Round(rho, 4), ["free_cells"] = PlayerStock.FreeCells() };
            if (frozen) log["frozen"] = "georgio";

            // Committed kits keep their claim on the budget until complete; nothing jumps them.
            foreach (var name in committed.ToList())
            {
                if (!today.TryGetValue(name, out var k) || k.Missing.Count == 0)
                {
                    committed.Remove(name);
                    if (kitLog.TryGetValue(name, out var kl) && kl["complete_day"] == null) kl["complete_day"] = dayNow;
                    continue;
                }
                budget -= k.Cost;
                Count(waits, name + (frozen ? ":georgio" : ps.playerCash - held < k.Missing.Min(x => x.Price) ? ":cash" : ":seller"));
            }
            var approved = new JArray();
            if (!frozen)
                foreach (var k in Ranked(horizon, rho, georgio))
                {
                    if (committed.Contains(k.Name)) continue;
                    var unit = Unit(k);
                    int cost = unit.Sum(x => x.Cost);
                    if (cost > budget) { Count(declined, k.Name + ":budget"); continue; }
                    foreach (var x in unit)
                    {
                        if (committed.Contains(x.Name)) continue;
                        committed.Add(x.Name);
                        kitLog[x.Name] = new JObject { ["approved_day"] = dayNow, ["cost"] = x.Cost, ["income"] = Math.Round(x.IncomePerDay, 1), ["parts"] = new JArray(x.Missing.Select(m => m.Id + "x" + m.Count)), ["spent"] = 0 };
                        approved.Add(x.Name);
                    }
                    budget -= cost;
                }
            budgetNow = budget;
            // `insure_kit: N`: the night's insurance stays on (past `buyout_save`'s cut) while N or more of planned kit
            // is owned loose, since burglars take loose modules and chips.
            if (Cfg.Value<int?>("insure_kit") is int ik && p.BuyoutCuts != null && p.BuyoutCuts.Contains("insurance") && KitValue() >= ik)
            {
                p.BuyoutCuts = new HashSet<string>(p.BuyoutCuts.Where(c => c != "insurance"), StringComparer.Ordinal);
                log["insured"] = true;
            }
            if (approved.Count > 0) log["approved"] = approved;
            if (committed.Count > 0) log["committed"] = new JArray(committed);
            log["budget"] = budget;
            if (approved.Count > 0 || dayNow % 7 == 0) daysLog.Add(log);
        }

        // A kit and the incomplete kits it needs, needs first: bought as one unit or not at all.
        static List<BuildKit> Unit(BuildKit k)
        {
            var unit = new List<BuildKit>();
            void Add(BuildKit x)
            {
                if (unit.Contains(x)) return;
                foreach (var n in x.Needs)
                    if (today.TryGetValue(n, out var dep) && dep.Missing.Count > 0) Add(dep);
                unit.Add(x);
            }
            Add(k);
            return unit;
        }

        // Kits worth buying today, best first: income a credit over the days left after their ramp. A kit must pay
        // back before the horizon and beat the counter's own return on the same money; a credit spent before Georgio's
        // loan costs `georgio_mult` (2.5: he lends half the morning's cash and repays four times it). `must` kits skip
        // the tests. Power and storage are scored by the kits they serve (BuildKits).
        static IEnumerable<BuildKit> Ranked(int horizon, double rho, int georgio)
        {
            var scored = new List<(BuildKit k, double score)>();
            foreach (var k in today.Values.Where(x => x.Missing.Count > 0 && x.Blocked == null).OrderBy(x => x.Name, StringComparer.Ordinal))
            {
                var unit = Unit(k);
                if (unit.Any(x => x.Blocked != null)) { Count(declined, k.Name + ":blocked"); continue; }
                double cost = unit.Sum(x => x.Cost) * (LoanPending ? Num("georgio_mult", 2.5) : 1);
                double income = unit.Sum(x => x.IncomePerDay);
                int ramp = unit.Max(x => x.RampDays);
                int left = horizon - dayNow - ramp;
                if (Must(k.Name) || k.Urgent) { scored.Add((k, double.MaxValue)); continue; }
                if (income <= 0 || cost <= 0) { if (cost > 0) Count(declined, k.Name + ":no_income"); continue; }
                if (income * left < cost * Num("payback_margin", 1.0)) { Count(declined, k.Name + ":payback"); continue; }
                if (PushesToMortgage(unit.Sum(x => x.Cost), income, ramp)) { Count(declined, k.Name + ":buyout"); continue; }
                // `counter_test`: also beat the counter's average return on its money. Off by default: the holds already
                // keep the counter's stock money, and cash beyond them is the buyout's savings, which earn nothing.
                if ((Cfg.Value<bool?>("counter_test") ?? false) && income / cost < rho) { Count(declined, k.Name + ":counter"); continue; }
                scored.Add((k, income * left / cost));
            }
            return scored.OrderByDescending(s => s.score).ThenBy(s => s.k.Name, StringComparer.Ordinal).Select(s => s.k).ToList();
        }

        // A run on course for the cash buyout by `buyout_by_day` must still reach it after the kit: cash, 70 % of the
        // stock and the last four weeks' gain carried forward, less the kit, plus its income after the ramp.
        static bool PushesToMortgage(int cost, double income, int ramp)
        {
            var ps = PlayerStore.Instance;
            if (!(Directives.Current.Raw.Value<int?>("buyout_by_day") is int by) || ps.IsPropertyPaid || ps.isPayingMortgage) return false;
            if (!(PlayerDay.Current?.NetGainWeek4 is int gain)) return false;
            int days = Math.Max(0, by - dayNow);
            double reach = ps.playerCash + PlayerStock.Value() * 0.7 + gain / 7.0 * days;
            double price = ps.GetStoreBuyPrice(false) + 100;
            return reach >= price && reach - cost + income * Math.Max(0, days - ramp) < price;
        }

        static void Count(Dictionary<string, int> d, string k) => d[k] = (d.TryGetValue(k, out int n) ? n : 0) + 1;

        // ---- the gates engines ask ---------------------------------------------------------------------------------

        public static bool Manages(string id) => On && id != null && managed.Contains(id);

        // Parts buyable now: a committed kit's, once the kits it needs are complete (power before the machines that draw,
        // a storage bay before machines the grid cannot take).
        static IEnumerable<BuildPart> Open() => frozen ? Enumerable.Empty<BuildPart>()
            : committed.Where(today.ContainsKey).Where(n => today[n].Needs.All(x => !today.TryGetValue(x, out var dep) || dep.Missing.Count == 0))
                .SelectMany(n => today[n].Missing);

        static int Free() => PlayerStore.Instance.playerCash - held;

        // How many of a managed id the engines may buy now: the committed kits' missing count while the cash over the
        // holds covers one, on top of what the route's own core still buys of a shared id (the cellar's bays).
        // Unmanaged ids (and the planner off) keep the engine's own answer.
        public static int Cap(string id, int engineWants)
        {
            if (!Manages(id)) return engineWants;
            int core = BuildKits.Core(id, engineWants);
            var parts = Open().Where(x => x.Id == id && x.At != BuyAt.Wild).ToList();
            if (parts.Count == 0) return core;
            return core + (Free() >= parts.Min(x => x.Price) * 9 / 10 ? parts.Sum(x => x.Count) : 0);
        }

        // Wild's upgrades the plan buys tonight, first in the night's order.
        public static IEnumerable<string> WildIds() => Open().Where(x => x.At == BuyAt.Wild).Select(x => x.Id).Distinct().ToList();

        // For a managed upgrade: true buys it now as a committed part (the cash over the holds; `funds` is what the
        // night's earlier picks left) or on favor alone (favor buys nothing else); null leaves a shared id the route's
        // core buys (WINEMAKER's water trader and farmer) to the old rules; false skips it.
        public static bool? AllowsUpgrade(string id, int cost, int funds, int favor)
        {
            if (favor >= cost) return true;
            if (WildIds().Contains(id) && funds - cost >= held) return true;
            return BuildKits.Core(id, 1) > 0 ? (bool?)null : false;
        }

        public static bool JacksonWants(string id, int staged) => On && Manages(id) && Open().Any(x => x.Id == id && x.At == BuyAt.Jackson) && staged < Cap(id, 0);

        // A trip to Dr. Jackson when a committed kit waits on his stock, at most every `jackson_every` (5) nights.
        public static bool WantsJackson(int day)
        {
            if (!On || day - lastJacksonAsk < Int("jackson_every", 5)) return false;
            if (!Open().Any(x => x.At == BuyAt.Jackson && Free() >= x.Price)) return false;
            lastJacksonAsk = day;
            return true;
        }

        // Parts the planner bought are held from sale until their engine takes them.
        static int KitValue() => EmporiumEntry.Instance.GetAllOwnedItems(false, true)
            .Where(i => i != null && committedOwned.Contains(i.uniqueId)).Sum(i => PlayerRole.Est(i));

        public static bool Holds(GameItem i) => On && i.parentInventory?.GetParentItem() == null && committedOwned.Contains(i.uniqueId);
        static readonly HashSet<int> committedOwned = new HashSet<int>();

        public static void Bought(GameItem item, int amount)
        {
            if (!On || item == null || !Manages(item.identifier)) return;
            paid[item.identifier] = Math.Max(1, amount);
            committedOwned.Add(item.uniqueId);
            Spend(item.identifier, amount);
        }

        public static void BoughtUpgrade(string id, int cashSpent)
        {
            if (!On || !Manages(id)) return;
            Spend(id, cashSpent);
        }

        static void Spend(string id, int amount)
        {
            string kit = committed.FirstOrDefault(n => today.TryGetValue(n, out var k) && k.Missing.Any(x => x.Id == id))
                ?? today.Values.FirstOrDefault(k => BuildKits.PartIds(k.Name).Contains(id))?.Name ?? "other";
            if (!kitLog.TryGetValue(kit, out var o)) kitLog[kit] = o = new JObject { ["unplanned"] = true, ["spent"] = 0 };
            o["spent"] = o.Value<int>("spent") + amount;
            var buys = o["buys"] as JArray ?? new JArray();
            buys.Add(id + "@" + dayNow + ":" + amount);
            o["buys"] = buys;
        }

        // ---- one safe stow path ----------------------------------------------------------------------------------

        // A kit part or engine input left on a counter goes with the next buyer's "sell all": into stock, the display
        // case when stock is full, and when both are full the cheapest unreserved stock goes out to make room for it.
        public static bool Put(GameItem i, bool displace = true)
        {
            var e = EmporiumEntry.Instance;
            if (Chore.Accept(e.invElement, i, -1) == 0 || Chore.Accept(e.showcaseElement, i, -1) == 0) { Stowed++; return true; }
            // Once a day an item: the goods pushed out come back when it still does not fit (no churn between customers).
            if (!displace || (tried.TryGetValue(i.uniqueId, out int d) && d == dayNow)) return false;
            tried[i.uniqueId] = dayNow;
            // The cheapest unreserved goods out one at a time until the part fits (free cells are scattered: a count is
            // not a shape); they go back if it never does.
            var out_ = new List<GameItem>();
            foreach (var g in e.invElement.childItems.Where(x => x != null && GeneralHelper.IsItemOwned(x) && !Production.Reserved(x) && x.contentWindow == null)
                         .OrderBy(x => PlayerRole.Est(x)).ThenBy(x => x.uniqueId).Take(24).ToList())
            {
                if (Chore.Accept(e.backInvinvElement, g, -1) != 0) break;
                out_.Add(g);
                if (Chore.Accept(e.invElement, i, -1) == 0) { Stowed++; Displaced += out_.Count; return true; }
            }
            foreach (var g in out_) Chore.Accept(e.invElement, g, -1);
            return false;
        }
        static readonly Dictionary<int, int> tried = new Dictionary<int, int>();

        // Owned kit parts and engine-held items on either counter; only a kit part pushes stock out for room (a lab bundle
        // delivered into a full grid sat on the back counter and went with the next sale).
        public static void Stow()
        {
            if (!On) return;
            var e = EmporiumEntry.Instance;
            foreach (var i in new[] { e.backInvinvElement, e.frontInvinvElement }.SelectMany(inv => inv.childItems)
                         .Where(i => i != null && GeneralHelper.IsItemOwned(i) && (managed.Contains(i.identifier ?? "") || Production.Reserved(i))).ToList())
                Put(i, managed.Contains(i.identifier ?? ""));
        }

        // Storage bays are part of the plan (`storage`, default on): bought when the grid cannot take a kit, and used
        // (Breadth.cs Storage): the stock grid, not cash, was what kept the lab's machines out.
        public static bool StorageOn => On && (Cfg.Value<bool?>("storage") ?? true);

        public static JObject Status()
        {
            if (!On) return null;
            return new JObject
            {
                ["kits"] = JObject.FromObject(kitLog.ToDictionary(kv => kv.Key, kv => kv.Value)),
                ["committed"] = new JArray(committed),
                ["declined"] = JObject.FromObject(declined),
                ["waiting"] = JObject.FromObject(waits),
                ["stowed"] = Stowed, ["displaced"] = Displaced,
                ["days"] = daysLog.DeepClone(),
            };
        }

        public static JObject Today() => !On ? null : new JObject { ["free_cells"] = PlayerStock.FreeCells(), ["held"] = held, ["budget"] = budgetNow, ["frozen"] = frozen, ["committed"] = new JArray(committed) };
    }
}
