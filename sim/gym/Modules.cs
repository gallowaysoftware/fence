using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Machine modules (docs/re/MODULES.md §5), every lever off by default. Machines and modules are bought from
    // Dr. Jackson at the counter through the engine-inputs path (Production.IsInput/Wants, buy_all) and fitted by
    // dropping them into the machine's module grid (Chore.Accept, which runs the bay's own callbacks).
    // - `alarm: "perfect"`: the security alarm, modules reaching performance 50 (an overclock and a performance
    //   module, or two overclocks: stop rate 50 + P, every burglary stopped), a charged cell in its slot, and no
    //   insurance on nights it is powered and perfect (PlayerStore.HandleInsurance).
    // - `transmitters: ["sec", "bm", "ul", "rev"]`: one alarm transmitter per faction, in that order, fitted before
    //   the squares and set by its click (MachineAlarm.ToggleTransmitter): +1 rep (×0.75 on hard) each powered
    //   night. Implies the alarm.
    // - `jackson_module_intro: "buy"`: the module introduction's three modules at the first-time discount.
    // - `bay_modules: "nightly"`: an unplanted hydroponic bay waits up to `bay_modules_wait` days (default 14) for
    //   performance 100 (a bloomberry harvest every night instead of every second); quality modules go in beside.
    // - `projector_modules: "tier1"`: a mirage projector, kept powered, with fineness and quality modules (boost
    //   100 + Q/2 + P/4, MachineProjector.GetAttractivenessBoost); up to four fineness are bought for it.
    // - `furnace_eco: true`: two eco modules beside each furnace's blast module (16 -> 10 power a smelt).
    internal static class Modules
    {
        static JObject R => Directives.Current.Raw;

        const string Perf = "system_module_performance", Eff = "system_module_efficiancy", Qual = "system_module_quality";
        const string Overclock = "system_module_overclock", Eco = "system_module_eco", Fine = "system_module_fineness";
        const string Transmitter = "alarm_module_transmitter", Alarm = "security_alarm", Projector = "mirage_projector";
        const string IntroClient = "inventorModuleIntro";

        static bool AlarmPerfect => R.Value<string>("alarm") == "perfect";
        static bool AlarmOn => AlarmPerfect || Factions().Count > 0;
        static bool IntroOn => R.Value<string>("jackson_module_intro") == "buy";
        static bool BayOn => R.Value<string>("bay_modules") == "nightly";
        static bool ProjectorOn => R.Value<string>("projector_modules") == "tier1";
        static bool FurnaceEcoOn => R.Value<bool?>("furnace_eco") ?? false;
        public static bool On => AlarmOn || IntroOn || BayOn || ProjectorOn || FurnaceEcoOn;

        static int BayWait => R.Value<int?>("bay_modules_wait") ?? 14;

        public static int AlarmDay, PerfectDay, PerfectNights, InsuranceSkipped, InsuranceSaved, IntroBought, Fitted, BayFits, BayWaitDays, CellsSwapped;
        public static long Spend;
        static readonly Dictionary<int, int> baySeen = new Dictionary<int, int>();
        static readonly Dictionary<string, int> bought = new Dictionary<string, int>(StringComparer.Ordinal);

        public static void Reset()
        {
            AlarmDay = PerfectDay = PerfectNights = InsuranceSkipped = InsuranceSaved = IntroBought = Fitted = BayFits = BayWaitDays = CellsSwapped = 0;
            Spend = 0;
            baySeen.Clear();
            bought.Clear();
        }

        static List<string> Factions()
        {
            var list = new List<string>();
            foreach (var x in R["transmitters"] as JArray ?? new JArray())
            {
                string f = ((string)x ?? "").ToLowerInvariant() switch
                {
                    "sec" or "security" or "faction_security" => "FACTION_SECURITY",
                    "bm" or "black_market" or "faction_black_market" => "FACTION_BLACK_MARKET",
                    "ul" or "upper_levels" or "faction_upper_level" => "FACTION_UPPER_LEVEL",
                    "rev" or "revolution" or "faction_revolution" => "FACTION_REVOLUTION",
                    _ => null,
                };
                if (f != null && !list.Contains(f)) list.Add(f);
            }
            return list;
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static bool Loose(GameItem i) => Parent(i) == null;
        static int Cash => PlayerStore.Instance.playerCash;
        static int RentDue => PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();

        static int P(GameItem m) => m.GetTagReadonly("BONUS_PERCENTAGE_PERFORMANCE_INT").GetInt();
        static int Q(GameItem m) => m.GetTagReadonly("BONUS_PERCENTAGE_QUALITY_INT").GetInt();
        static bool Universal(GameItem m) => m.IsTag("MODULE_TAG") && m.IsTag("MODULE_TYPE_UNIVERSAL");

        static GameGridInventory Grid(GameItem machine) => MachineHelper.GetModuleInv(machine);
        static IEnumerable<GameItem> In(GameItem machine) => Grid(machine)?.childItems.Where(m => m != null) ?? Enumerable.Empty<GameItem>();
        static int Charge(GameItem machine) => MachineHelper.GetBatterySlot(machine)?.childItem is GameItem c ? PowerHelper.GetAvailableEnergyFromItem(c, false) : 0;
        static int Draw(GameItem machine) => MachineryHelper.GetMachinePowerUsage(machine);

        static GameItem Machine(string id) => Owned().Where(i => i.identifier == id && Loose(i) && !i.IsTag("BROKEN_MACHINE_TAG")).OrderBy(i => i.uniqueId).FirstOrDefault();
        static GameItem AlarmItem() => Machine(Alarm);
        static GameItem ProjectorItem() => Machine(Projector);

        static int AlarmP() => AlarmItem() is GameItem a ? MachineryHelper.GetCurrentPerformanceBonus(a) : 0;

        // Loose positive-performance squares the alarm can still use, best first.
        static List<GameItem> LooseP() => Owned().Where(i => Loose(i) && Universal(i) && P(i) > 0).OrderByDescending(P).ThenBy(i => i.uniqueId).ToList();

        // Squares fitted in the alarm, and the performance the best of them and the loose ones could reach in the room the
        // bay has: three squares beside a transmitter (by auto-placement), four without. Degrading and learning modules
        // carry their current stats in their tags, so a worn module ranks below a fresh one.
        static List<GameItem> AlarmSquares() => AlarmItem() is GameItem a ? In(a).Where(m => m.identifier != Transmitter && m.IsTag("MODULE_TAG")).ToList() : new List<GameItem>();
        static int SquareSlots() => Math.Max(Factions().Count > 0 ? 3 : 4, AlarmSquares().Count);
        static List<GameItem> AlarmPool() => AlarmSquares().Concat(LooseP()).Where(m => P(m) > 0).OrderByDescending(P).ThenBy(m => m.uniqueId).ToList();
        static int AlarmReach() => AlarmPool().Take(SquareSlots()).Sum(P);

        static int NeedAlarmP() => AlarmPerfect ? Math.Max(0, 50 - AlarmReach()) : 0;

        static int TransmittersHeld() => Owned().Count(i => i.identifier == Transmitter);

        static List<GameItem> PendingBays() => BayOn
            ? Owned().Where(i => i.IsTag("HYDROPONIC_MACHINE_TAG") && Loose(i) && MachineHydroponic.GetInputSlot(i)?.childItem == null && Waiting(i)).ToList()
            : new List<GameItem>();

        static bool Waiting(GameItem bay)
        {
            int day = PlayerDay.Current?.Day ?? 0;
            return !baySeen.TryGetValue(bay.uniqueId, out int since) || day - since < BayWait;
        }

        static int NeedBayP() => BayOn ? Math.Max(0, PendingBays().Sum(b => Math.Max(0, 100 - MachineryHelper.GetCurrentPerformanceBonus(b))) - (AlarmPerfect && AlarmP() < 50 ? 0 : LooseP().Sum(P))) : 0;

        static bool Afford(int reserve) => Cash >= RentDue + reserve;

        static bool AtIntro() => PlayerStore.Instance?.currentClientInstance?.GetClientBlueprint()?.identifier == IntroClient;

        // How many more of an item the levers want bought; 0 for anything else and whenever they are off.
        public static int Wants(string id)
        {
            if (!On || id == null || EmporiumEntry.Instance == null) return 0;
            if (IntroOn && AtIntro() && (id == Perf || id == Eff || id == Qual)) return 1;
            switch (id)
            {
                case Alarm: return AlarmOn && AlarmItem() == null && Afford(300) ? 1 : 0;
                case Transmitter: return Factions().Count > TransmittersHeld() && Afford(300) ? 1 : 0;
                case Overclock: return (NeedAlarmP() > 0 || NeedBayP() > 0) && Afford(300) ? 1 : 0;
                // Without a transmitter four performance modules (64) make the alarm perfect; beside one, only an
                // overclock and a performance module fit (auto-placement: one long and two squares).
                // Beside a transmitter the room is short, so a performance module (16) only when it closes the gap.
                case Perf: return ((NeedAlarmP() > 0 && (Factions().Count == 0 || NeedAlarmP() <= 16)) || NeedBayP() > 0) && Afford(200) ? 1 : 0;
                case Projector: return ProjectorOn && ProjectorItem() == null && Afford(1000) ? 1 : 0;
                case Fine: return ProjectorOn && ProjectorItem() != null && In(ProjectorItem()).Count() + Owned().Count(i => Loose(i) && i.identifier == Fine) < 4 && Afford(600) ? 1 : 0;
                case Eco: return FurnaceEcoOn && Furnace.Furnaces().Any(f => In(f).Any(m => m.identifier == "furnace_module_blast")) && Owned().Count(i => i.identifier == Eco) < 2 * Furnace.Furnaces().Count && Afford(400) ? 1 : 0;
                case "energy_credit": return NeedsCell() && !Owned().Any(i => Loose(i) && Production.Charged(i)) ? 1 : 0;
            }
            return 0;
        }

        public static bool IsInput(string id) => On && id != null && (Wants(id) > 0 || (id == "energy_credit" && NeedsCell()));

        // The seller's goods staged on the back counter are all inputs, some of them the levers' (Dr. Jackson buys
        // and sells: buy_all settles the exchange, as for the furnace).
        public static bool BuyingInputs(JArray theirs)
        {
            if (!On) return false;
            var back = PlayerStock.BackUids();
            var staged = theirs.Where(i => back.Contains(i.Value<int?>("uid") ?? -1)).Select(i => i.Value<string>("id")).ToList();
            return staged.Count > 0 && staged.All(Production.IsInput) && staged.Any(IsInput);
        }

        static bool NeedsCell() => Powered().Any(m => Charge(m) < 3 * Math.Max(1, Draw(m)));

        static IEnumerable<GameItem> Powered()
        {
            if (AlarmOn && AlarmItem() is GameItem a) yield return a;
            if (ProjectorOn && ProjectorItem() is GameItem p) yield return p;
        }

        // What the levers hold back from sale: their machines and the loose modules one of them can still use.
        public static bool Reserved(GameItem i)
        {
            if (!On || EmporiumEntry.Instance == null) return false;
            string id = i.identifier;
            if (id == Alarm) return AlarmOn && AlarmItem()?.uniqueId == i.uniqueId;
            if (id == Projector) return ProjectorOn && ProjectorItem()?.uniqueId == i.uniqueId;
            if (!i.IsTag("MODULE_TAG") || !Loose(i)) return false;
            if (id == Transmitter) return Factions().Count > 0;
            if (id == Eco && FurnaceEcoOn) return true;
            if (Universal(i) && P(i) > 0 && ((AlarmPerfect && AlarmP() < 50) || BayOn)) return true;
            if (Universal(i) && Q(i) > 0 && (ProjectorOn || BayOn)) return true;
            return false;
        }

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            foreach (var b in Owned().Where(i => i.IsTag("HYDROPONIC_MACHINE_TAG") && Loose(i)))
                if (!baySeen.ContainsKey(b.uniqueId)) baySeen[b.uniqueId] = ep.Day;
            try { TendAlarm(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            try { TendProjector(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            try { TendFurnaces(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static bool Fit(IExecutorHost ep, GameItem machine, GameItem module)
        {
            var grid = Grid(machine);
            if (grid == null || Chore.Accept(grid, module, -1) != 0 || !ReferenceEquals(Parent(module), machine))
            {
                if (!Loose(module)) Chore.Accept(EmporiumEntry.Instance.invElement, module, -1);
                return false;
            }
            Fitted++;
            ep.Human("fit_module", null, ep.Uid(module), new JObject { ["machine"] = machine.identifier, ["machine_uid"] = ep.Uid(machine), ["module"] = module.identifier }, "player:modules");
            return true;
        }

        // A charged cell in, when the one in the slot would not see the machine through the next two nights.
        static void Power(GameItem machine)
        {
            var slot = MachineHelper.GetBatterySlot(machine);
            if (slot == null || Charge(machine) >= 2 * Math.Max(1, Draw(machine))) return;
            var cell = Owned().Where(i => Loose(i) && i.identifier == "energy_credit").OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (cell == null || PowerHelper.GetAvailableEnergyFromItem(cell, false) <= Charge(machine)) return;
            if (slot.childItem != null) Chore.Accept(EmporiumEntry.Instance.invElement, slot.childItem, -1);
            if (Chore.Accept(slot, cell, -1) == 0) CellsSwapped++;
        }

        static void TendAlarm(IExecutorHost ep)
        {
            if (!AlarmOn) return;
            var alarm = AlarmItem();
            if (alarm == null) return;
            if (AlarmDay == 0) AlarmDay = ep.Day;
            // Long transmitters first: the grid's auto-placement fits one long beside two squares, not after them.
            var factions = Factions();
            foreach (var t in Owned().Where(i => i.identifier == Transmitter && Loose(i)).OrderBy(i => i.uniqueId).ToList())
                if (In(alarm).Count(m => m.identifier == Transmitter) < factions.Count && !Fit(ep, alarm, t)) break;
            var fitted = In(alarm).Where(m => m.identifier == Transmitter).OrderBy(m => m.uniqueId).ToList();
            for (int k = 0; k < fitted.Count && k < factions.Count; k++)
            {
                var tx = fitted[k];
                for (int n = 0; n < 4 && tx.GetTagReadonly(MachineAlarm.TRANSMITTER_TARGET_TAG).GetString() != factions[k]; n++)
                    Chore.Act("transmitter", tx, () => MachineAlarm.ToggleTransmitter(tx));
            }
            ClearDead(alarm);
            // Below 50 (a module wore down, burnt out or never came) refit from scratch, best first, but only once the
            // fitted and loose squares together reach 50, so a lone module waits for its partner.
            if (AlarmPerfect && MachineryHelper.GetCurrentPerformanceBonus(alarm) < 50 && AlarmReach() >= 50)
            {
                var pool = AlarmPool();
                foreach (var m in AlarmSquares()) Chore.Accept(EmporiumEntry.Instance.invElement, m, -1);
                foreach (var m in pool)
                {
                    if (MachineryHelper.GetCurrentPerformanceBonus(alarm) >= 50) break;
                    Fit(ep, alarm, m);
                }
            }
            Power(alarm);
        }

        // Burnt-out (an overvolted module's end) and corrupt modules come out to stock to make room.
        static void ClearDead(GameItem machine)
        {
            foreach (var m in In(machine).Where(m => m.identifier == "system_module_ruined" || m.identifier == "system_module_corrupt").ToList())
                Chore.Accept(EmporiumEntry.Instance.invElement, m, -1);
        }

        // Tonight's alarm stops every burglary: performance 50 or more and the cell covers its draw.
        public static bool AlarmPerfectTonight()
        {
            if (!AlarmPerfect || EmporiumEntry.Instance == null) return false;
            var alarm = AlarmItem();
            if (alarm == null || MachineryHelper.GetCurrentPerformanceBonus(alarm) < 50) return false;
            var cell = MachineHelper.GetBatterySlot(alarm)?.childItem;
            return cell != null && PowerHelper.CanDrawPowerSource(cell, Draw(alarm));
        }

        // The night's insurance decision: skipped (and counted) while the alarm is perfect.
        public static bool SkipInsurance(int day, int cost)
        {
            if (!AlarmPerfectTonight()) return false;
            if (PerfectDay == 0) PerfectDay = day;
            PerfectNights++;
            InsuranceSkipped++;
            InsuranceSaved += cost;
            return true;
        }

        static void TendProjector(IExecutorHost ep)
        {
            if (!ProjectorOn) return;
            var p = ProjectorItem();
            if (p == null) return;
            ClearDead(p);
            // Fineness and quality (and any neural core) by boost Q/2 + P/4, while the grid takes them.
            foreach (var m in Owned().Where(i => Loose(i) && Universal(i) && Q(i) > 0 && Q(i) / 2 + P(i) / 4 > 0).OrderByDescending(i => Q(i) / 2 + P(i) / 4).ThenBy(i => i.uniqueId).ToList())
                if (!Fit(ep, p, m)) break;
            Power(p);
        }

        static void TendFurnaces(IExecutorHost ep)
        {
            if (!FurnaceEcoOn) return;
            foreach (var f in Furnace.Furnaces().Where(f => In(f).Any(m => m.identifier == "furnace_module_blast")))
            {
                var eco = Owned().Where(i => Loose(i) && i.identifier == Eco).OrderBy(i => i.uniqueId).Take(2 - In(f).Count(x => x.identifier == Eco)).ToList();
                if (eco.Count == 0) continue;
                var stock = EmporiumEntry.Instance.invElement;
                // Auto-placement does not repack: two squares beside the blast and junk modules (both long) fit
                // only in some insertion orders. Take everything out and keep the first order that seats the most,
                // blast first in priority, then eco, then junk.
                var all = In(f).Concat(eco).ToList();
                int Score(List<GameItem> placed) => placed.Count(m => m.identifier == "furnace_module_blast") * 100 + placed.Count(m => m.identifier == Eco) * 10 + placed.Count(m => m.identifier != Eco && m.identifier != "furnace_module_blast");
                List<GameItem> Place(IEnumerable<GameItem> order)
                {
                    foreach (var m in In(f).ToList()) Chore.Accept(stock, m, -1);
                    var placed = new List<GameItem>();
                    foreach (var m in order)
                        if (Chore.Accept(Grid(f), m, -1) == 0 && ReferenceEquals(Parent(m), f)) placed.Add(m);
                    return placed;
                }
                var longs = all.Where(m => m.identifier != Eco).OrderByDescending(m => m.identifier == "furnace_module_blast").ToList();
                var squares = all.Where(m => m.identifier == Eco).ToList();
                var orders = new List<List<GameItem>>
                {
                    longs.Concat(squares).ToList(),
                    squares.Concat(longs).ToList(),
                    longs.Take(1).Concat(squares).Concat(longs.Skip(1)).ToList(),
                };
                int best = -1, bestScore = -1;
                for (int k = 0; k < orders.Count; k++)
                {
                    int sc = Score(Place(orders[k]));
                    if (sc > bestScore) { best = k; bestScore = sc; }
                }
                var seated = Place(orders[best]);
                foreach (var m in seated.Where(m => m.identifier == Eco && eco.Contains(m)))
                {
                    Fitted++;
                    ep.Human("fit_module", null, ep.Uid(m), new JObject { ["machine"] = f.identifier, ["machine_uid"] = ep.Uid(f), ["module"] = m.identifier }, "player:modules");
                }
            }
        }

        // Before a seed goes into an empty hydroponic bay: with `bay_modules`, fit what reaches performance 100 (plus
        // quality while it fits) and plant; without it, wait while the bay is young.
        public static bool ReadyToPlant(IExecutorHost ep, GameItem bay)
        {
            if (!BayOn) return true;
            if (!baySeen.ContainsKey(bay.uniqueId)) baySeen[bay.uniqueId] = ep.Day;
            int have = MachineryHelper.GetCurrentPerformanceBonus(bay);
            // The alarm comes first: while it is short of 50 the bay takes no performance module.
            var squares = AlarmPerfect && AlarmP() < 50 ? new List<GameItem>() : LooseP();
            if (have < 100 && have + squares.Sum(P) >= 100)
                foreach (var m in squares)
                {
                    if (MachineryHelper.GetCurrentPerformanceBonus(bay) >= 100 || !Fit(ep, bay, m)) break;
                    BayFits++;
                }
            if (MachineryHelper.GetCurrentPerformanceBonus(bay) >= 100)
            {
                foreach (var m in Owned().Where(i => Loose(i) && Universal(i) && Q(i) > 0 && P(i) >= 0).OrderByDescending(Q).ThenBy(i => i.uniqueId).ToList())
                    if (!Fit(ep, bay, m)) break;
                return true;
            }
            if (Waiting(bay)) { BayWaitDays++; return false; }
            return true;
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || sold || item == null) return;
            string id = item.identifier;
            if (id == Alarm || id == Projector || id == Transmitter || id == Perf || id == Eff || id == Qual || id == Overclock || id == Eco || id == Fine || id == "energy_credit")
            {
                if (id != "energy_credit") Spend += amount;
                if (AtIntro()) IntroBought++;
                bought[id] = (bought.TryGetValue(id, out int n) ? n : 0) + 1;
            }
        }

        public static JObject Status()
        {
            if (!On) return null;
            var alarm = AlarmItem();
            var proj = ProjectorItem();
            var o = new JObject
            {
                ["alarm_day"] = AlarmDay, ["perfect_day"] = PerfectDay, ["perfect_nights"] = PerfectNights,
                ["insurance_skipped"] = InsuranceSkipped, ["insurance_saved"] = InsuranceSaved,
                ["intro_bought"] = IntroBought, ["fitted"] = Fitted, ["bay_fits"] = BayFits, ["bay_wait_days"] = BayWaitDays,
                ["cells_swapped"] = CellsSwapped, ["capital_spend"] = Spend,
                ["bought"] = JObject.FromObject(bought),
            };
            if (alarm != null)
                o["alarm"] = new JObject
                {
                    ["performance"] = MachineryHelper.GetCurrentPerformanceBonus(alarm), ["draw"] = Draw(alarm), ["charge"] = Charge(alarm),
                    ["modules"] = new JArray(In(alarm).Select(m => m.identifier == Transmitter ? Transmitter + ":" + m.GetTagReadonly(MachineAlarm.TRANSMITTER_TARGET_TAG).GetString() : m.identifier).ToArray()),
                };
            if (proj != null)
                o["projector"] = new JObject
                {
                    ["boost"] = MachineProjector.GetAttractivenessBoost(proj), ["draw"] = Draw(proj), ["charge"] = Charge(proj),
                    ["modules"] = new JArray(In(proj).Select(m => m.identifier).ToArray()),
                };
            return o;
        }
    }
}
