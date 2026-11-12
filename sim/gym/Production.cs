using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The executor's production engines, driven through the game's own item APIs (the ones the machine
    // windows call): wine for now (docs/re/PRODUCTION.md §2, docs/sim/VERIFIED.md). Each morning every
    // empty bottle is filled (a water ration, or a clean water container poured in), given its best one or
    // two berries and a yeast, and started; the night ferments and ages it. Inputs are bought from any
    // seller and never sold; finished wine is ordinary stock.
    internal static partial class Production
    {
        // Wine by default on every difficulty (hard: same median survival, twice the net worth at day 100).
        static string EngineName => Directives.Current.Engine ?? "wine";
        public static bool WineOn => EngineName.Contains("wine");
        // Selling tap water made clean (Water.cs); on by `engine: water` (or `wine+water`).
        public static bool WaterSellOn => EngineName.Contains("water");

        static readonly HashSet<string> Yeasts = new HashSet<string>(StringComparer.Ordinal) { "wine_yeast", "wine_superyeast", "wine_yeast_infinite" };

        // Clean enough to ferment in: base (98.00 %) or better keeps the water term >= 0 (WATER.md).
        const int MinPurity = 9600;

        public static int Started, Finished, Emptied;
        public static long FinishedValue;
        static readonly HashSet<int> seenFinished = new HashSet<int>();

        public static void Reset()
        {
            Started = Finished = Emptied = Harvested = WaterTreatedMl = WaterRelabelled = PurifiedMl = Printed = UntestedWaits = 0;
            idleBays.Clear();
            unseededSince.Clear();
            keepBerries.Clear();
            keepRations.Clear();
            reservedValue = 0;
            PlayerRole.BribeFailing = false;
            PlayerDemand.Reset();
            Breadth.Reset();
            Storage.Reset();
            Conceal.Reset();
            Vending.Reset();
            Fountain.Reset();
            Appraisal.Reset();
            Labels.Reset();
            Modules.Reset();
            Suppliers.Reset();
            treatedOn.Clear();
            purifiedOn.Clear();
            keepWater.Clear();
            faucet = null;
            FinishedValue = 0;
            seenFinished.Clear();
            Furnace.Reset();
            Rats.Reset();
            Fish.Reset();
            Printer.Reset();
            Arms.Reset();
            StolenTrade.Reset();
            Crops.Reset();
            Lab.Reset();
            Quality.Reset();
            ResetQuality();
            ResetSales();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        // A berry worth fermenting: anything but moldy.
        static bool Usable(GameItem i) => i.identifier == "wine_berry" && i.GetTagReadonly("BERRY_CONDITION_INT").GetInt() >= -1;

        static bool Bottle(GameItem i) => i.identifier == "wine_bottle";
        static int State(GameItem i) => i.GetTagReadonly("WINE_CONTAINER_STATE").GetInt();
        static bool EmptyBottle(GameItem i) => Bottle(i) && State(i) == 0;

        static bool CleanWater(GameItem i)
        {
            if (Bottle(i) || Parent(i) != null || treatedOn.ContainsKey(i.uniqueId) || i.identifier == "water_pitcher" || !i.IsTag("LIQUID_CONTAINER_TAG") || (i.identifier ?? "").IndexOf("water", StringComparison.Ordinal) < 0) return false;
            try
            {
                return !WaterHelper.IsEmpty(i) && Label(i) >= MinPurity;
            }
            catch (Exception)
            {
                return false;
            }
        }

        // Clean water the cellar keeps for its bottles and bays (the fountain leaves it).
        public static bool HeldForWine(GameItem i) => WineOn && keepWater.Contains(i.uniqueId);

        // Inputs the engine holds: unfinished bottles, yeast, berries, rations and clean water.
        public static bool Reserved(GameItem i)
        {
            if (Directives.Current.ReleaseUids.Contains(i.uniqueId)) return false;
            if (Furnace.Reserved(i) || Modules.Reserved(i) || Rats.Reserved(i) || Fish.Reserved(i) || Printer.Reserved(i) || Arms.Reserved(i) || Crops.Reserved(i) || Lab.Reserved(i)) return true;
            if (PlantSafe(i) || reedNights.ContainsKey(i.uniqueId) || (ReedOn && i.identifier == "water_tank") || (ReedOn && i.identifier == "water_jug" && Parent(i) == null && WaterHelper.IsEmpty(i))) return true;
            if (WaterSellOn && (WaterReserved(i) || treatedOn.ContainsKey(i.uniqueId) || keepWater.Contains(i.uniqueId))) return true;
            if (!WineOn) return false;
            if (WinePurifierOn && i.identifier == "water_purifier") return true;
            if (DnaReserved(i)) return true;
            // The bays and their seeds are held from day one (a farmer start's bays came before WINEMAKER).
            if (i.IsTag("HYDROPONIC_MACHINE_TAG") && idleBays.Contains(i.uniqueId)) return false;
            if (i.IsTag("HYDROPONIC_MACHINE_TAG") || i.identifier == "bloomberry_seed" || i.identifier == "wine_rack" || (ReedOn && i.identifier == ReedSeedId)) return true;
            if (!NetworkUpgrade.IsWinemakerUnlocked()) return false;
            if (Bottle(i)) return State(i) != 2;
            if ( i.identifier == "nutrient_tablet" || i.identifier == "nutrient_tablet_container" || (Charged(i) && (Bays().Any() || DnaUsesCells))) return true;
            // Berries, rations and clean water only as far as the bottles and bays can use them (the rest is
            // stock: a full grid turns away the winemaker's bottles).
            if (i.identifier == "wine_berry") return keepBerries.Contains(i.uniqueId);
            if (WineHelper.IsWaterRation(i)) return keepRations.Contains(i.uniqueId);
            return Yeasts.Contains(i.identifier) || (CleanWater(i) && keepWater.Contains(i.uniqueId)) || treatedOn.ContainsKey(i.uniqueId) || WaterReserved(i);
        }

        // How many more of an item the engine wants, given what is owned.
        // Bays left unseeded two weeks with no bloomberry seed in hand are stock again (a farmer start's
        // bays come with other seeds); the plan's `engine_reserve_max` caps what inputs may tie up.
        static readonly HashSet<int> idleBays = new HashSet<int>();
        static readonly Dictionary<int, int> unseededSince = new Dictionary<int, int>();
        static long reservedValue;
        static readonly HashSet<int> keepBerries = new HashSet<int>(), keepRations = new HashSet<int>();

        // What the engine holds back from sale: the best berries (two per empty bottle, plus four), rations
        // and clean water for the empty bottles, water for the bays.
        public static void Earmark()
        {
            keepBerries.Clear();
            keepRations.Clear();
            keepWater.Clear();
            var owned = Owned().ToList();
            int bottles = owned.Count(EmptyBottle);
            if (TargetScore != null) EarmarkForScore(owned, bottles);
            else
                foreach (var b in owned.Where(i => Usable(i) && Parent(i) == null).OrderByDescending(i => i.GetTagReadonly("BERRY_CONDITION_INT").GetInt()).Take(2 * bottles + 4))
                    keepBerries.Add(b.uniqueId);
            int needMl = bottles * 750 + 750;
            foreach (var r in owned.Where(WineHelper.IsWaterRation))
            {
                if (needMl <= 0) break;
                keepRations.Add(r.uniqueId);
                needMl -= r.identifier == "water_ration" ? 750 : 375;
            }
            needMl += Bays().Count * 1500;
            foreach (var c in owned.Where(CleanWater).OrderByDescending(Label))
            {
                if (needMl <= 0) break;
                keepWater.Add(c.uniqueId);
                needMl -= WaterHelper.GetCurrentCapacityML(c);
            }
            EarmarkBays(owned);
            foreach (var id in bayReserve) keepWater.Add(id);
            if (Crops.On) Crops.Earmark(Bays());
        }

        static void Audit(IExecutorHost ep)
        {
            Earmark();
            idleBays.Clear();
            bool seedInHand = Owned().Any(i => i.identifier == "bloomberry_seed" && Parent(i) == null);
            foreach (var b in Bays())
            {
                if (MachineHydroponic.GetInputSlot(b).childItem != null || seedInHand) { unseededSince.Remove(b.uniqueId); continue; }
                if (!unseededSince.TryGetValue(b.uniqueId, out int since)) unseededSince[b.uniqueId] = since = ep.Day;
                if (ep.Day - since >= 14) idleBays.Add(b.uniqueId);
            }
            reservedValue = Owned().Where(i => Reserved(i) && Parent(i) == null).Sum(i => (long)PlayerRole.Est(i));
        }

        public static int Wants(string id)
        {
            if (DnaWants(id) > 0) return DnaWants(id);
            int w = WantsEngine(id), f = Furnace.Wants(id) + Modules.Wants(id) + Rats.Wants(id) + Fish.Wants(id) + Printer.Wants(id) + Arms.Wants(id) + (Crops.On ? Crops.Wants(id, Bays()) : 0) + Lab.Wants(id);
            return f > 0 ? Math.Max(w, 0) + f : w;
        }

        static int WantsEngine(string id)
        {
            if (id != "hydroponic" && Directives.Current.Raw.Value<int?>("engine_reserve_max") is int cap && reservedValue > cap) return 0;
            if (id != null && (IsWaterInput(id) || id == "water_jug") && (WaterSellOn || WineOn)) return WaterWants(id);
            if (id == "energy_credit" && WaterSellOn && Math.Max(WaterWants(id), 0) > 0) return 1;
            if (!WineOn || !NetworkUpgrade.IsWinemakerUnlocked() || id == null) return 0;
            var owned = Owned().ToList();
            int bottles = owned.Count(EmptyBottle);
            // Bottles that the water at hand can fill: berries and yeast beyond those only tie up cash.
            int waterMl = owned.Where(WineHelper.IsWaterRation).Sum(r => r.identifier == "water_ration" ? 750 : 375)
                + owned.Where(CleanWater).Sum(WaterHelper.GetCurrentCapacityML);
            int fillable = Math.Min(bottles, waterMl / 750);
            bool harvesting = Bays().Any(b => MachineHydroponic.GetInputSlot(b).childItem != null);
            switch (id)
            {
                case "wine_bottle": return Math.Max(0, 8 - bottles);
                case "wine_yeast":
                case "wine_superyeast":
                    return Math.Max(0, fillable + 2 - owned.Count(i => Yeasts.Contains(i.identifier)));
                // The bioreactor is reusable: one is enough.
                case "wine_yeast_infinite": return owned.Any(i => i.identifier == "wine_yeast_infinite") ? 0 : 1;
                case "wine_berry": return Math.Max(0, (harvesting ? 2 * fillable : Math.Max(4, 2 * fillable + 2)) - owned.Count(Usable));
                case "water_ration": return Math.Max(0, bottles + 8 - owned.Count(WineHelper.IsWaterRation));
                case "water_ration_small": return Math.Max(0, 2 * (bottles + 8) - owned.Count(WineHelper.IsWaterRation));
                // The retired farmer brings a bay every visit: more bays, more berries (they bind next to water).
                case "hydroponic": return Bays().Count >= 1 && Bays().All(b => MachineHydroponic.GetInputSlot(b).childItem != null) && Bays().Count < (Directives.Current.Raw.Value<int?>("bays") ?? 3) + DnaExtraBays() + Crops.ExtraBays + (ReedOn ? 1 : 0) && PlayerStore.Instance.playerCash > 1500 ? 1 : 0;
                case "bloomberry_seed" when Crops.On: return Math.Max(0, Math.Min(Bays().Count, (Directives.Current.Raw.Value<int?>("bays") ?? 3) + DnaExtraBays()) - Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == "bloomberry_seed") - owned.Count(i => i.identifier == "bloomberry_seed" && Parent(i) == null)) + DnaWantsSeed();
                case "bloomberry_seed": return Math.Max(0, Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem == null) - (ReedBayDue() ? 1 : 0) - owned.Count(i => i.identifier == "bloomberry_seed" && Parent(i) == null)) + DnaWantsSeed();
                case ReedSeedId: return ReedOn ? Math.Max(0, 2 - owned.Count(i => i.identifier == ReedSeedId)) : 0;
                // A bay at its mineral level drinks about a tablet a week, more on a refill: every one offered while short.
                case "nutrient_tablet": return QualityBays ? Math.Max(0, 6 * Bays().Count - owned.Count(i => i.identifier == "nutrient_tablet")) : Bays().Any() && owned.Count(i => i.identifier == "nutrient_tablet") < 8 ? 1 : 0;
                case "nutrient_tablet_container": return Bays().Any() && owned.Count(i => i.identifier == "nutrient_tablet") < (QualityBays ? 3 * Bays().Count : 8) ? 1 : 0;
                case "energy_credit": return Bays().Any() || DnaUsesCells ? Math.Max(0, Bays().Count + 1 + (DnaUsesCells ? 1 : 0) + (WinePurifierOn ? Purifiers().Count : 0) + (WineRacks > 0 ? owned.Count(i => i.identifier == "wine_rack") : 0) - owned.Count(i => Charged(i) && Parent(i) == null)) : 0;
            }
            return 0;
        }

        static readonly HashSet<string> WaterIds = new HashSet<string>(StringComparer.Ordinal) { "bottled_water", "large_bottled_water", "small_bottled_water", "bottled_water_premium", "water_jug" };

        // A seller's water worth buying for the bottles: clean enough and not already covered.
        public static bool WantsWater(GameItem i)
        {
            if (!WineOn || !NetworkUpgrade.IsWinemakerUnlocked() || !WaterIds.Contains(i.identifier ?? "")) return false;
            bool clean;
            try { clean = !WaterHelper.IsEmpty(i) && Label(i) >= MinPurity; }
            catch (Exception) { return false; }
            if (!clean) return false;
            var owned = Owned().ToList();
            int needMl = owned.Count(EmptyBottle) * 750 + 1500 * (1 + Bays().Count) - owned.Where(WineHelper.IsWaterRation).Sum(r => r.identifier == "water_ration" ? 750 : 375) - owned.Where(CleanWater).Sum(WaterHelper.GetCurrentCapacityML);
            return needMl > 0;
        }

        // The wine rack (Dr. Jackson, 350): a powered rack ages every bottle in it a second step a night. Bottles
        // younger than `wine_min_age` go in (while it has room and power), older ones come out to be sold.
        static void TendRacks()
        {
            int minAge = Directives.Current.Raw.Value<int?>("wine_min_age") ?? 0;
            if (!PlayerRole.WineHoldCashOk()) minAge = 0;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var rack in Owned().Where(i => i.identifier == "wine_rack" && Parent(i) == null).ToList())
            {
                var bat = MachineHelper.GetBatterySlot(rack);
                if (bat != null && (bat.childItem == null || PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) < 24))
                {
                    var cell = Owned().Where(x => Charged(x) && Parent(x) == null).OrderByDescending(x => PowerHelper.GetAvailableEnergyFromItem(x, false)).FirstOrDefault();
                    if (cell != null)
                    {
                        if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                        Chore.Accept(bat, cell, -1);
                    }
                }
                foreach (var b in GraphUtils.FindAllChildrenType<GameItem>(rack).Where(x => Bottle(x) && x.GetTagReadonly("AGE_INT").GetInt() >= minAge).ToList())
                    Chore.Accept(stock, b, -1);
                foreach (var b in Owned().Where(x => Bottle(x) && State(x) == 2 && Parent(x) == null && x.GetTagReadonly("AGE_INT").GetInt() < minAge && WineHelper.GetWineQualityTier(x) > 0).ToList())
                {
                    bool placed = false;
                    foreach (var inv in GraphUtils.FindChildrenType<GameGridInventory>(rack))
                        if (Chore.Accept(inv, b, -1) == 0 && Parent(b) == rack) { placed = true; break; }
                    if (!placed) break;
                }
            }
        }

        // A finished bottle's rank among the held ones, youngest first (0 = youngest).
        public static int AgingRank(GameItem bottle)
        {
            int age = bottle.GetTagReadonly("AGE_INT").GetInt();
            return Owned().Count(i => Bottle(i) && State(i) == 2 && i != bottle && (i.GetTagReadonly("AGE_INT").GetInt() < age || (i.GetTagReadonly("AGE_INT").GetInt() == age && i.uniqueId < bottle.uniqueId)));
        }

        // With `engine: none` nothing is an engine input (a winemaker's bottles are plain stock then).
        public static bool IsInput(string id) => Furnace.IsInput(id) || Modules.IsInput(id) || Rats.IsInput(id) || Fish.IsInput(id) || Printer.IsInput(id) || Arms.IsInput(id) || Crops.IsInput(id) || Lab.IsInput(id) || ((WineOn || WaterSellOn) && IsInputAny(id));

        static bool IsInputAny(string id) => (DnaOn && Donors.Contains(id ?? "")) || IsWaterInput(id) || id == "hydroponic" || id == "bloomberry_seed" || (ReedOn && id == ReedSeedId) || id == "nutrient_tablet" || id == "nutrient_tablet_container" || (id == "energy_credit" && (Bays().Any() || DnaUsesCells || (WaterSellOn && (Owned().Any(i => i.identifier == "water_purifier") || Fountain.Machine() != null)))) || WaterIds.Contains(id ?? "") || id == "wine_bottle" || id == "wine_berry" || id == "water_ration" || id == "water_ration_small" || (id != null && Yeasts.Contains(id));

        static bool Put(GameItem into, GameItem it)
        {
            Chore.Act("own", it, () => GeneralHelper.SetItemOwned(it, true));
            if (Chore.Accept(into, it, -1) == 0 && ReferenceEquals(Parent(it), into)) return true;
            foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(into))
                if (Chore.Accept(inv, it, -1) == 0 && it.parentInventory != null && ReferenceEquals(Parent(it), into)) return true;
            return false;
        }

        public static void Morning(IExecutorHost ep)
        {
            // The rats and the printer first: the cage's mending scrap and growing ingot and the printer's scrap are taken
            // before the furnace loads every scrap.
            Rats.Morning(ep);
            Fish.Morning(ep);
            Printer.Morning(ep);
            Arms.Morning(ep);
            Lab.Morning(ep);
            Furnace.Morning(ep);
            Modules.Morning(ep);
            try { Audit(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            // The plan's `print` entries press the bottle printer: {"action":"print","machine_uid":N,"size":"small|normal|large","count":k}.
            foreach (var e in Directives.Current.Production ?? new JArray())
                if ((string)e?["action"] == "print" && Owned().FirstOrDefault(x => x.uniqueId == ((int?)e["machine_uid"] ?? 0) && x.identifier == "bottle_printer") is GameItem printer)
                    try { Print(ep, printer, (string)e["size"] ?? "small", (int?)e["count"] ?? 1); }
                    catch (Exception ex) { ep.ProductionError(ex); }
            if (WaterSellOn && !WineOn)
            {
                try { TreatWater(ep); }
                catch (Exception e) { ep.ProductionError(e); }
                return;
            }
            if (!WineOn || !NetworkUpgrade.IsWinemakerUnlocked()) return;
            var owned = Owned().ToList();
            foreach (var b in owned.Where(Bottle).Where(b => State(b) == 2))
            {
                if (seenFinished.Add(b.uniqueId))
                {
                    Finished++;
                    FinishedValue += PlayerRole.Est(b);
                }
                // Undrinkable wine is worth nothing; emptied, it is a bottle again.
                try
                {
                    if (WineHelper.GetWineQualityTier(b) == 0 && WineHelper.CanEmptyWineBottle(b)) { Chore.Act("empty_wine", b, () => WineHelper.EmptyWineBottle(b)); Emptied++; }
                }
                catch (Exception e) { ep.ProductionError(e); }
            }
            foreach (var bay in Bays())
            {
                try { TendBay(ep, bay); Quality.Bay(bay); }
                catch (Exception e) { ep.ProductionError(e); }
            }
            try { TendDna(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            try { TreatWater(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            try { TendWinePurifier(ep); }
            catch (Exception e) { ep.ProductionError(e); }
            try { TendRacks(); }
            catch (Exception e) { ep.ProductionError(e); }
            Earmark();
            owned = Owned().ToList();
            if (Environment.GetEnvironmentVariable("FENCE_WINE_DEBUG") != null)
                foreach (var b in owned.Where(EmptyBottle))
                    Console.Error.WriteLine($"wine d{ep.Day} bottle {b.uniqueId} parent={Parent(b)?.identifier} inv={b.parentInventory?.GetType().Name} ml={WaterHelper.GetCurrentCapacityML(b)} inside=[{string.Join(",", GraphUtils.FindAllChildrenType<GameItem>(b).Select(x => x.identifier))}] rations={owned.Count(WineHelper.IsWaterRation)}");
            // The plan's `production` entries: `start_wine` on a bottle (machine_uid) goes first; `empty` pours an
            // undrinkable wine out. Others are recorded, not executed.
            var first = new HashSet<int>();
            foreach (var e in Directives.Current.Production ?? new JArray())
            {
                string action = (string)e?["action"];
                int uid = (int?)e?["machine_uid"] ?? 0;
                var it = owned.FirstOrDefault(x => x.uniqueId == uid);
                if (action == "start_wine" && it != null) first.Add(uid);
                else if (action == "empty" && it != null && Bottle(it) && WineHelper.CanEmptyWineBottle(it)) Chore.Act("empty_wine", it, () => WineHelper.EmptyWineBottle(it));
            }
            foreach (var bottle in owned.Where(i => EmptyBottle(i) && Parent(i) == null).OrderByDescending(i => first.Contains(i.uniqueId)).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId))
            {
                try
                {
                    PourHq(bottle);
                    if (!Fill(bottle)) continue;
                    var inside = GraphUtils.FindAllChildrenType<GameItem>(bottle);
                    if (TargetScore is int target && inside.All(i => i.identifier != "wine_berry") && !BerriesToScore(ep, bottle, target)) continue;
                    inside = GraphUtils.FindAllChildrenType<GameItem>(bottle);
                    int berries = inside.Count(i => i.identifier == "wine_berry");
                    var free = Owned().Where(i => Usable(i) && Parent(i) == null)
                        .OrderByDescending(i => i.GetTagReadonly("BERRY_CONDITION_INT").GetInt()).ThenBy(i => i.uniqueId).ToList();
                    // Two berries when there are berries for every empty bottle, else one (bottles bind).
                    int emptyLeft = Owned().Count(i => EmptyBottle(i) && i != bottle && GraphUtils.FindAllChildrenType<GameItem>(i).All(x => x.identifier != "wine_berry"));
                    int want = free.Count(x => x.GetTagReadonly("BERRY_CONDITION_INT").GetInt() >= -1) >= 2 * (emptyLeft + 1) ? 2 : 1;
                    // An under/over-ripe berry alone scores 1 (Bucket Brew, 85); with a second one, 2 or better.
                    if (want == 1 && free.FirstOrDefault()?.GetTagReadonly("BERRY_CONDITION_INT").GetInt() == -1 && free.Count(x => x.GetTagReadonly("BERRY_CONDITION_INT").GetInt() >= -1) >= 2) want = 2;
                    foreach (var berry in free)
                    {
                        if (berries >= want) break;
                        // A moldy berry scores -1: alone with a yeast it makes undrinkable wine, beside a good one it
                        // costs a grade. Never used.
                        if (berry.GetTagReadonly("BERRY_CONDITION_INT").GetInt() < -1) break;
                        if (Put(bottle, berry)) berries++;
                    }
                    if (berries == 0) break;
                    if (!inside.Any(i => Yeasts.Contains(i.identifier)))
                    {
                        var yeast = Owned().Where(i => Yeasts.Contains(i.identifier) && Parent(i) == null)
                            .OrderBy(i => i.identifier == "wine_superyeast" ? 0 : i.identifier == "wine_yeast" ? 1 : 2).ThenBy(i => i.uniqueId).FirstOrDefault();
                        if (yeast != null) Put(bottle, yeast);
                    }
                    // The score the bottle will get (PRODUCTION.md §2: berries + pair + yeast + water term, the water
                    // tested as an AquaScan or strip would): a score <= 0 is undrinkable, so add a berry or wait.
                    int Score()
                    {
                        var all = GraphUtils.FindAllChildrenType<GameItem>(bottle);
                        var bs = all.Where(x => x.identifier == "wine_berry").Select(x => x.GetTagReadonly("BERRY_CONDITION_INT").GetInt()).ToList();
                        int sc = bs.Sum(c => c == 2 ? 2 : c == 1 ? 1 : c == -1 ? 0 : -1) + (bs.Count >= 2 ? 1 : 0) + (all.Any(x => Yeasts.Contains(x.identifier)) ? 1 : 0);
                        return sc + KnownWaterTerm(bottle);
                    }
                    if (Score() <= 0)
                    {
                        var extra = Owned().Where(i => Usable(i) && Parent(i) == null).OrderByDescending(i => i.GetTagReadonly("BERRY_CONDITION_INT").GetInt()).FirstOrDefault();
                        if (extra != null) Put(bottle, extra);
                        // Still undrinkable: the water is too poor; pour it out (the bottle is refilled tomorrow).
                        if (Score() <= 0) { Chore.Act("empty", bottle, () => WaterHelper.EmptyContainer(bottle)); continue; }
                    }
                    Quality.Bottle(bottle);
                    if (Chore.Act("start_wine", bottle, () => WineHelper.TryStartFermentation(bottle)))
                    {
                        Started++;
                        StartSeen(bottle);
                        ep.Human("production", null, ep.Uid(bottle), new JObject { ["action"] = "start_wine", ["berries"] = berries }, "player:wine");
                    }
                }
                catch (Exception e)
                {
                    ep.ProductionError(e);
                }
            }
        }

        // ---- the hydroponic bay (bloomberries for the bottles) -----------------------------------

        public static int Harvested;

        static List<GameItem> Bays()
        {
            var e = EmporiumEntry.Instance;
            if (e == null) return new List<GameItem>();
            return new[] { e.invElement, e.showcaseElement, e.backInvinvElement, e.frontInvinvElement }.SelectMany(inv => inv.childItems)
                .Where(i => i != null && i.IsTag("HYDROPONIC_MACHINE_TAG") && GeneralHelper.IsItemOwned(i)).ToList();
        }

        static bool BayThirsty(GameItem bay)
        {
            var w = MachineHydroponic.GetWaterSlotItem(bay);
            return w == null || WaterHelper.GetCurrentCapacityML(w) < 600;
        }

        public static bool Charged(GameItem cell) => cell.identifier == "energy_credit" && PowerHelper.GetAvailableEnergyFromItem(cell, false) >= 30;

        static int Charge(GameItem bay)
        {
            var b = MachineHelper.GetBatterySlot(bay)?.childItem;
            return b == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(b, false);
        }

        // Each morning, as a player at the bay's window: take the berries out, plant a bloomberry seed,
        // keep a battery with charge, and keep at least a night's water of the cleanest water owned (the
        // farmer's kit ships Ghostwater, which rots every berry; docs/sim/VERIFIED.md), one nutrient
        // tablet per litre so the plant keeps its minerals.
        // Kits land on the back counter, where the next buyer's sale would take them: stow every bay.
        public static void Stow()
        {
            Lab.Stow();
            Fish.Stow();
            if (!WineOn && !WaterSellOn) return;
            Earmark();
            if (!WineOn) return;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var bay in Bays().Where(b => !ReferenceEquals(b.parentInventory, stock) && !ReferenceEquals(b.parentInventory, EmporiumEntry.Instance.showcaseElement)))
                if (Chore.Accept(stock, bay, -1) != 0) Chore.Accept(EmporiumEntry.Instance.showcaseElement, bay, -1);
        }

        static void TendBay(IExecutorHost ep, GameItem bay)
        {
            var stock = EmporiumEntry.Instance.invElement;
            // A kit lands on the back counter; the bay belongs in the stock grid (where a buyer never takes it).
            // With a full stock grid it waits in the display case (no window buyer wants a machine).
            if (!ReferenceEquals(bay.parentInventory, stock) && Chore.Accept(stock, bay, -1) != 0 && !ReferenceEquals(bay.parentInventory, EmporiumEntry.Instance.showcaseElement))
                Chore.Accept(EmporiumEntry.Instance.showcaseElement, bay, -1);
            foreach (var berry in MachineHydroponic.GetOutputSlot(bay).childItems.Where(i => i != null).ToList())
                if (Chore.Accept(stock, berry, -1) == 0)
                {
                    if (Crops.On && berry.identifier != "wine_berry") { Crops.Harvest(berry); continue; }
                    if (berry.identifier != "wine_berry") continue;
                    Harvested++;
                    HarvestSeen(berry);
                }
            var input = MachineHydroponic.GetInputSlot(bay);
            if (input.childItem == null)
            {
                var crop = Crops.SeedFor(Bays(), (Directives.Current.Raw.Value<int?>("bays") ?? 3) + DnaExtraBays());
                var seed = crop ?? (ReedBayDue() ? ReedSeed() : SeedToPlant());
                if (seed != null && Modules.ReadyToPlant(ep, bay) && Chore.Accept(input, seed, -1) == 0 && crop != null) Crops.Planted++;
            }
            if (Charge(bay) < 16)
            {
                var cell = Owned().Where(i => i.identifier == "energy_credit" && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).FirstOrDefault();
                var slot = MachineHelper.GetBatterySlot(bay);
                if (cell != null && PowerHelper.GetAvailableEnergyFromItem(cell, false) > Charge(bay))
                {
                    var old = slot.childItem;
                    if (old != null) Chore.Accept(stock, old, -1);
                    Chore.Accept(slot, cell, -1);
                }
            }
            if (Crops.Water(ep, bay)) return;
            if (IsReed(input.childItem))
            {
                TendReedWater(ep, bay, stock);
                return;
            }
            if (QualityBays)
            {
                TendBayWater(ep, bay, input.childItem, stock);
                return;
            }
            var ws = MachineHydroponic.GetWaterSlot(bay);
            var water = ws.childItem;
            int purity = Label(water);
            int ml = water == null ? 0 : WaterHelper.GetCurrentCapacityML(water);
            int bayMin = WinePurifierOn ? HqPurity : 9800;
            var better = Owned().Where(i => CleanWater(i) && i != water && Label(i) >= bayMin)
                .OrderByDescending(Label).ThenByDescending(WaterHelper.GetCurrentCapacityML).FirstOrDefault();
            int before = ml;
            // A dirty container out, a clean one in (only when the stock grid has room for the old one).
            if (better != null && (water == null || (purity < bayMin && Chore.Accept(stock, water, -1) == 0)))
            {
                if (Chore.Accept(ws, better, -1) == 0) { water = better; before = 0; }
                else if (water != null) Chore.Accept(ws, water, -1);
            }
            // Topped up from the cleanest containers (pouring one into the other), whatever is in the slot.
            if (water != null && WaterHelper.GetCurrentCapacityML(water) < 600)
                foreach (var src in Owned().Where(i => CleanWater(i) && i != water).OrderByDescending(Label).ToList())
                {
                    if (WaterHelper.GetCurrentCapacityML(water) >= 600 || WaterHelper.IsFull(water)) break;
                    Chore.Act("pour", water, () => WaterHelper.TransferLiquid(src, water));
                }
            int added = water == null ? 0 : WaterHelper.GetCurrentCapacityML(water) - before;
            if (added > 0)
            {
                // A nutrient tablet per litre added keeps the plant's minerals (tablets may sit in their box).
                foreach (var tab in Owned().Where(i => i.identifier == "nutrient_tablet" && (Parent(i) == null || (Parent(i).identifier ?? "").EndsWith("_container", StringComparison.Ordinal))).Take(Math.Max(1, (added + 500) / 1000)).ToList())
                    Chore.Act("dissolve", water, () => ChemicalHelper.DissolveInto(tab, water));
                ep.Human("production", null, ep.Uid(bay), new JObject { ["action"] = "bay_water", ["purity"] = Label(water), ["ml"] = WaterHelper.GetCurrentCapacityML(water), ["added_ml"] = added }, "player:bay");
            }
        }

        // >= 749 ml of clean water in the bottle: a ration, two small ones, or a pour.
        static bool Fill(GameItem bottle)
        {
            if (WaterHelper.GetCurrentCapacityML(bottle) >= 749) return true;
            if (QualityBottles && FillBest(bottle)) return true;
            if (WaterHelper.GetCurrentCapacityML(bottle) == 0)
            {
                var ration = Owned().FirstOrDefault(i => i.identifier == "water_ration");
                if (ration != null)
                {
                    Chore.Act("pour", bottle, () => WineHelper.PourRationIntoBottle(bottle, ration));
                    Poured(bottle, 2);
                    return true;
                }
            }
            var smalls = Owned().Where(i => i.identifier == "water_ration_small").ToList();
            int have = WaterHelper.GetCurrentCapacityML(bottle);
            // Half a bottle is worth starting only when the rest is at hand (never strand 375 ml).
            if (have + 375 * smalls.Count + Owned().Where(i => CleanWater(i) && !ForBays(i)).Sum(WaterHelper.GetCurrentCapacityML) < 749) return false;
            foreach (var small in smalls.Take(2))
            {
                if (WaterHelper.GetCurrentCapacityML(bottle) >= 749) break;
                Chore.Act("pour", bottle, () => WineHelper.PourRationIntoBottle(bottle, small));
                Poured(bottle, 2);
            }
            foreach (var src in Owned().Where(i => CleanWater(i) && !ForBays(i)).OrderByDescending(Label).ToList())
            {
                if (WaterHelper.GetCurrentCapacityML(bottle) >= 749) break;
                Poured(bottle, LabelGrade(src));
                Chore.Act("pour", bottle, () => WaterHelper.TransferLiquid(src, bottle));
            }
            return WaterHelper.GetCurrentCapacityML(bottle) >= 749;
        }

        public static JObject Status()
        {
            if (!NetworkUpgrade.IsWinemakerUnlocked()) return TapOn ? new JObject { ["water"] = WaterStatus() } : new JObject();
            var owned = Owned().ToList();
            return new JObject
            {
                ["wine"] = new JObject
                {
                    ["empty_bottles"] = owned.Count(EmptyBottle),
                    ["fermenting"] = owned.Count(i => Bottle(i) && State(i) == 1),
                    ["finished_in_stock"] = owned.Count(i => Bottle(i) && State(i) == 2),
                    ["berries"] = owned.Count(i => i.identifier == "wine_berry"),
                    ["yeast"] = owned.Count(i => Yeasts.Contains(i.identifier)),
                    ["water_rations"] = owned.Count(WineHelper.IsWaterRation),
                    ["bays"] = Bays().Count,
                    ["bay_seeded"] = Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem != null),
                    ["bay_detail"] = new JArray(Bays().Select(b =>
                    {
                        var w = MachineHydroponic.GetWaterSlotItem(b);
                        var sd = MachineHydroponic.GetInputSlot(b).childItem;
                        return (JToken)$"charge {Charge(b)} water {(w == null ? 0 : WaterHelper.GetCurrentCapacityML(w))}ml@{Label(w)} seed {sd?.identifier ?? "-"} health {(sd == null ? -1 : HydroponicHelper.GetHealthPercent(sd))} in {b.parentInventory?.GetType().Name}";
                    })),
                    ["tap"] = TapOn,
                    ["water_treated_ml"] = WaterTreatedMl,
                    ["clean_water_ml"] = owned.Where(CleanWater).Sum(WaterHelper.GetCurrentCapacityML),
                    ["energy_credits"] = owned.Count(i => i.identifier == "energy_credit" && Parent(i) == null),
                    ["harvested_total"] = Harvested,
                    ["started_total"] = Started,
                    ["finished_total"] = Finished,
                },
            };
        }

        public static JObject Totals()
        {
            var t = new JObject { ["wine_started"] = Started, ["wine_finished"] = Finished, ["wine_value_at_finish"] = FinishedValue, ["wine_emptied_undrinkable"] = Emptied, ["berries_harvested"] = Harvested, ["water_treated_ml"] = WaterTreatedMl, ["water_relabelled"] = WaterRelabelled, ["wine_sold"] = SalesTotals(), ["quality"] = Quality.Totals(QualityStatus()) };
            if (DnaOn) t["dna"] = DnaTotals();
            if (Crops.Totals(Bays()) is JObject crops) t["crops"] = crops;
            if (Lab.Status() is JObject lab) t["lab"] = lab;
            return t;
        }
    }

    internal static partial class Engine
    {
        static partial void PlanImpl(UpgradeView v, List<string> into)
        {
            // Selling water needs the tap first of all.
            if (Production.WaterSellOn)
                foreach (var id in new[] { "WATER_TRADER", "GUTTERFLOW_TAP", "RUSTWATER_TAP" })
                {
                    var u = v.Upgrades.FirstOrDefault(x => x.Id == id);
                    if (u != null && !u.Unlocked && v.Cash + v.Favor >= u.Cost + v.RentDue + 300) into.Add(id);
                }
            if (!Production.WineOn) return;
            into.Add("WINEMAKER");
            Production.PlanDna(v, into);
            // The water trader's weekly visitor sells four wine bottles (bottles bind the cellar) plus filters,
            // tablets and a pitcher for tap water.
            if (NetworkUpgrade.IsWinemakerUnlocked() && v.Cash + v.Favor >= 300 * (PlayerStore.Instance.difficultyId == "hard" ? 3 : 2) / 2 + v.RentDue + 400) into.Add("WATER_TRADER");
            // Berries bind once bottles and water flow: the farmer's bay (6 a harvest) and his seeds.
            if (NetworkUpgrade.IsWinemakerUnlocked() && v.Cash + v.Favor >= 350 * (PlayerStore.Instance.difficultyId == "hard" ? 3 : 2) / 2 + v.RentDue + 400) into.Add("RETIRED_FARMER");
            // Water binds once berries flow: the tap (and a filter pitcher and tablets) makes Basewater.
            if (NetworkUpgrade.IsRetiredFarmerUnlocked() && (Directives.Current.Raw.Value<bool?>("tap") ?? false))
                foreach (var id in new[] { "WATER_TRADER", "GUTTERFLOW_TAP", "RUSTWATER_TAP" })
                {
                    var u = v.Upgrades.FirstOrDefault(x => x.Id == id);
                    if (u != null && !u.Unlocked && v.Cash + v.Favor >= u.Cost + v.RentDue + 600) into.Add(id);
                }
        }
    }
}
