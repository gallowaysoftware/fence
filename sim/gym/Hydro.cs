using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `quality` (docs/research/QUALITY.md): the best grade the shop's equipment allows, read only from what a player
    // sees. A container's water is known by its label, which is the truth after a faucet or premade fill, a
    // portable-purifier pass or a strip/AquaScan test; tablets, filters, the purifier and mixing leave it stale, so
    // treated or bought water is tested before the bays or bottles rely on it.
    // - `bays` (default true): the bay takes the best-graded water owned, never a worse grade on top of a better
    //   one, and nutrient tablets keep the plant's minerals inside its band (a ledger of what went in and what
    //   the plant eats, or the AquaScan's reading when one is owned).
    // - `bottles` (default true): High Quality or Pure water goes into the bottles before rations (+1 score), and
    //   the score the cellar plans with is the poured water's known grade, not the hidden truth.
    internal static partial class Production
    {
        static JObject QualityPlan => Directives.Current.Raw["quality"] as JObject;
        static bool QualityBays => QualityPlan?.Value<bool?>("bays") ?? true;
        static bool QualityBottles => QualityPlan?.Value<bool?>("bottles") ?? true;
        public static int StripsSpent, BayTablets, BaySwaps;

        // The label's purity in basis points (0 when empty).
        public static int Label(GameItem c)
        {
            if (c == null || WaterHelper.IsEmpty(c)) return 0;
            return WaterFeatureHelper.GetWaterFeature(c)?.customIntValue1 ?? 0;
        }

        static int LabelGrade(GameItem c) => WaterHelper.IsEmpty(c) ? 5 : WaterFeatureHelper.GetPurityArrayIndex(Label(c));

        // Containers whose label the executor has confirmed (tested, or filled at the faucet) since it last changed them.
        static readonly HashSet<int> vouched = new HashSet<int>();

        public static void Vouch(GameItem c) => vouched.Add(c.uniqueId);
        public static bool Vouched(GameItem c) => vouched.Contains(c.uniqueId);
        public static void Unvouch(GameItem c) => vouched.Remove(c.uniqueId);

        static bool Scanner() => Owned().Any(i => i.identifier == "aquascan");

        // A strip (or the AquaScan, free) on an owned container, and Match when the label's band is wrong.
        public static bool TestWater(GameItem c)
        {
            if (WaterHelper.IsEmpty(c)) return false;
            bool scanner = Scanner();
            var strip = scanner ? null : Strips().FirstOrDefault();
            if (!scanner && strip == null) return false;
            if (strip != null)
            {
                Chore.Act("test_strip", c, () => strip.onItemTargetItemFunc?.Invoke(strip, c));
                StripsSpent++;
            }
            var f = WaterFeatureHelper.GetWaterFeature(c);
            if (f != null && GeneralHelper.IsItemOwned(c) && !f.IsExposed() && !f.IsFeatureMatch())
            {
                Chore.Act("test_strip", c, () => f.ExposeFeature(lowkey: true));
                Chore.Act("relabel", c, () => WaterFeatureHelper.OnWaterLabelUpdated(c));
                WaterRelabelled++;
            }
            vouched.Add(c.uniqueId);
            return true;
        }

        // Owned loose water the bays and bottles may use, by label: base or better, not waiting for its test.
        static bool KnownClean(GameItem i, int worstGrade = 2) => !Bottle(i) && Parent(i) == null && !treatedOn.ContainsKey(i.uniqueId) && i.identifier != "water_pitcher"
            && i.IsTag("LIQUID_CONTAINER_TAG") && (i.identifier ?? "").IndexOf("water", StringComparison.Ordinal) >= 0 && !WineHelper.IsWaterRation(i)
            && !WaterHelper.IsEmpty(i) && LabelGrade(i) <= worstGrade && !Lab.Holds(i);

        // Bought water carries the seller's word: tested before a bay or bottle relies on it, while strips last.
        static void Vet(IEnumerable<GameItem> cands)
        {
            foreach (var c in cands.Where(c => !vouched.Contains(c.uniqueId)).ToList())
                if (!TestWater(c)) break;
        }

        // ---- the bay's water and minerals -------------------------------------------------------------------

        // The best-graded water, up to five nights of the bays' drinking, is theirs: the bottles take the rest (water binds
        // the cellar's bottles; a longer hold cost more bottles than it saved berries).
        static readonly HashSet<int> bayReserve = new HashSet<int>();

        static void EarmarkBays(List<GameItem> owned)
        {
            bayReserve.Clear();
            if (!QualityBays) return;
            int need = Bays().Sum(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem sd ? HydroponicHelper.GetWaterUsePerCycle(sd) * 5 : 0);
            // Cured stores are the bays' anyway (they draw on them first) and hold far more than five nights: the bottles share them.
            foreach (var c in owned.Where(i => KnownClean(i, 3) && !cleanStore.Contains(i.uniqueId)).OrderBy(LabelGrade).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId))
            {
                if (need <= 0) break;
                bayReserve.Add(c.uniqueId);
                need -= WaterHelper.GetCurrentCapacityML(c);
            }
        }

        static bool ForBays(GameItem c) => bayReserve.Contains(c.uniqueId);

        // Reed-cleaned water ranks above every label (its label is stale, its contaminants are gone).
        static int BayGrade(GameItem c) => PlantSafe(c) ? -1 : LabelGrade(c);

        // Estimated mineral content of each bay's water, in bp of the water (HydroponicHelper.HandleNutriment's unit).
        static readonly Dictionary<int, int> mineralBp = new Dictionary<int, int>();
        const int TabletUl = 15000;

        static int MineralBp(GameItem w) => Scanner() ? WaterHelper.GetContaminantPercentage(w, "mineral") : mineralBp.TryGetValue(w.uniqueId, out int bp) ? bp : 0;

        static void TendBayWater(IExecutorHost ep, GameItem bay, GameItem seed, GameGridInventory stock)
        {
            var ws = MachineHydroponic.GetWaterSlot(bay);
            var water = ws.childItem;
            // Last night the plant ate its minerals: the use rate per progress point times the bay's speed.
            if (water != null && seed != null && mineralBp.TryGetValue(water.uniqueId, out int had) && WaterHelper.GetCurrentCapacityML(water) > 0)
            {
                int eaten = HydroponicHelper.GetMineralUseRate(seed) * Math.Max(1, bay.GetTagReadonly("CURRENT_PROCESSING_SPEED_TAG").GetInt()) * 10 / WaterHelper.GetCurrentCapacityML(water);
                mineralBp[water.uniqueId] = Math.Max(0, had - eaten);
            }
            int need = seed == null ? 100 : HydroponicHelper.GetWaterUsePerCycle(seed);
            var tanks = Tanks(water);
            int inTanks = tanks.Sum(WaterHelper.GetCurrentCapacityML);
            int ml0 = water == null ? 0 : WaterHelper.GetCurrentCapacityML(water);
            // Cured water replaces the slot's only when the tanks hold ten nights of it; a cured bay tops up from any.
            if (water != null && PlantSafe(water) ? inTanks > 0 : inTanks >= 10 * need)
            {
                // Cured water is poured, not swapped: no stock room needed, and the bay keeps its vessel.
                if (water == null)
                {
                    var v = Owned().Where(i => WaterVessel(i) && WaterHelper.IsEmpty(i) && i.identifier != "water_jug")
                        .OrderByDescending(i => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt()).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (v != null && Chore.Accept(ws, v, -1) == 0) water = v;
                }
                if (water != null && !PlantSafe(water))
                {
                    if (!WaterHelper.IsEmpty(water)) Chore.Act("empty", water, () => WaterHelper.EmptyContainer(water));
                    mineralBp.Remove(water.uniqueId);
                    reedClean.Add(water.uniqueId);
                    BaySwaps++;
                }
                if (water != null && WaterHelper.GetCurrentCapacityML(water) < 6 * need)
                    foreach (var t in tanks)
                    {
                        if (WaterHelper.IsFull(water)) break;
                        int before = WaterHelper.GetCurrentCapacityML(water), tbp = MineralBp(t);
                        Chore.Act("pour", water, () => WaterHelper.TransferLiquid(t, water));
                        int added = WaterHelper.GetCurrentCapacityML(water) - before;
                        int bp = mineralBp.TryGetValue(water.uniqueId, out int wb) ? wb : 0;
                        if (added > 0) mineralBp[water.uniqueId] = (bp * before + tbp * added) / (before + added);
                    }
            }
            else
            {
                // Cured water running out: the bay goes back to the best label.
                if (water != null && PlantSafe(water) && ml0 < 3 * need) reedClean.Remove(water.uniqueId);
                int grade = water == null || WaterHelper.IsEmpty(water) ? 6 : BayGrade(water);
                // Ghostwater only when nothing better is owned: a paused bay grows nothing, a ghost one bad berries.
                var pool = Owned().Where(i => KnownClean(i, 3) && i != water).OrderBy(LabelGrade).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();
                var tanksHq = HqTanks(water);
                // A reed's HQ tank is poured, not moved (it stays in the reed's slot).
                if (tanksHq.Count > 0 && water != null && LabelGrade(water) > 1)
                {
                    if (!WaterHelper.IsEmpty(water)) Chore.Act("empty", water, () => WaterHelper.EmptyContainer(water));
                    mineralBp[water.uniqueId] = 0;
                }
                if (water != null && (WaterHelper.IsEmpty(water) || LabelGrade(water) <= 1))
                    foreach (var t in tanksHq)
                    {
                        if (WaterHelper.GetCurrentCapacityML(water) >= 6 * need || WaterHelper.IsFull(water)) break;
                        int b0 = WaterHelper.GetCurrentCapacityML(water), wb = mineralBp.TryGetValue(water.uniqueId, out int m0) ? m0 : 0;
                        Chore.Act("pour", water, () => WaterHelper.TransferLiquid(t, water));
                        int add = WaterHelper.GetCurrentCapacityML(water) - b0;
                        if (add > 0) mineralBp[water.uniqueId] = wb * b0 / (b0 + add);
                    }
                var best = pool.FirstOrDefault();
                // A better grade replaces the slot's water outright (mixing would leave the worse water's contaminants).
                if (best != null && (water == null || LabelGrade(best) < grade || WaterHelper.GetCurrentCapacityML(water) < need) && (water == null || Chore.Accept(stock, water, -1) == 0))
                {
                    if (water != null) mineralBp.Remove(water.uniqueId);
                    if (Chore.Accept(ws, best, -1) == 0)
                    {
                        water = best;
                        grade = LabelGrade(best);
                        pool.Remove(best);
                        mineralBp[water.uniqueId] = Scanner() ? WaterHelper.GetContaminantPercentage(water, "mineral") : 0;
                        BaySwaps++;
                    }
                    else water = ws.childItem;
                }
                if (water == null) return;
                // Topped up only from the same grade or better.
                int before = WaterHelper.GetCurrentCapacityML(water);
                if (before < 6 * need && !PlantSafe(water))
                    foreach (var src in pool.Where(x => LabelGrade(x) <= grade).ToList())
                    {
                        if (WaterHelper.GetCurrentCapacityML(water) >= 6 * need || WaterHelper.IsFull(water)) break;
                        Chore.Act("pour", water, () => WaterHelper.TransferLiquid(src, water));
                    }
                int ml = WaterHelper.GetCurrentCapacityML(water);
                if (ml > before && mineralBp.TryGetValue(water.uniqueId, out int bp0)) mineralBp[water.uniqueId] = before == 0 ? 0 : bp0 * before / ml;
            }
            if (water == null) return;
            if (seed != null) DoseMinerals(ep, bay, seed, water);
            if (Environment.GetEnvironmentVariable("FENCE_BAY_DEBUG") != null)
                Console.Error.WriteLine($"bay d{ep.Day} {bay.uniqueId} {water.identifier}#{water.uniqueId} ml={WaterHelper.GetCurrentCapacityML(water)} label={Label(water)} safe={PlantSafe(water)} ledger={MineralBp(water)} true={WaterHelper.GetContaminantPercentage(water, "mineral")} tanks={tanks.Count}");
        }

        // Nutrient tablets (15 ml of mineral each) to bring the water to the plant's level, never past its band's top
        // when one fewer stays inside it; below the band costs twice what above it does (1 % vs 0.5 % a point).
        static void DoseMinerals(IExecutorHost ep, GameItem bay, GameItem seed, GameItem water)
        {
            int ml = WaterHelper.GetCurrentCapacityML(water);
            // A tablet is 15 ml: in less than ~0.6 L it alone overshoots any band.
            if (ml < 600) return;
            int level = HydroponicHelper.GetNutrimentLevel(seed), range = HydroponicHelper.GetNutrimentRange(seed);
            int bp = MineralBp(water);
            if (bp >= level - range / 2) return;
            int per = TabletUl * 10 / ml;
            int k = Math.Max(0, (int)Math.Round((level - bp) / (double)Math.Max(1, per)));
            while (k > 0 && bp + k * per > level + range) k--;
            if (k == 0 && bp < level - range) k = 1;
            var tabs = Owned().Where(i => i.identifier == "nutrient_tablet" && (Parent(i) == null || (Parent(i).identifier ?? "").EndsWith("_container", StringComparison.Ordinal))).Take(k).ToList();
            foreach (var tab in tabs)
                Chore.Act("dissolve", water, () => ChemicalHelper.DissolveInto(tab, water));
            if (tabs.Count == 0) return;
            BayTablets += tabs.Count;
            mineralBp[water.uniqueId] = bp + tabs.Count * per;
            ep.Human("production", null, ep.Uid(bay), new JObject { ["action"] = "bay_minerals", ["tablets"] = tabs.Count, ["ml"] = ml, ["label"] = Label(water) }, "player:bay");
        }

        // ---- the bottle's water --------------------------------------------------------------------------

        // The worst grade poured into each unstarted bottle (a ration is Basewater), which is what its wine score reads.
        static readonly Dictionary<int, int> bottleGrade = new Dictionary<int, int>();

        static void Poured(GameItem bottle, int grade) => bottleGrade[bottle.uniqueId] = Math.Max(grade, bottleGrade.TryGetValue(bottle.uniqueId, out int g) ? g : 0);

        // WineHelper.GetWaterQualityScore by grade: Pure/HQ +1, Base 0, Ghost -1, Rust -2, Gutter -3.
        static int WaterTerm(int grade) => grade <= 1 ? 1 : 2 - grade;

        // The water term the executor expects for a bottle, from what it poured in; a bottle it did not fill counts as
        // Basewater (the winemaker's rations).
        static int KnownWaterTerm(GameItem bottle) => WaterTerm(bottleGrade.TryGetValue(bottle.uniqueId, out int g) ? g : 2);

        // High Quality or better first: a whole bottle's worth from tested containers, so the term is +1.
        static bool FillBest(GameItem bottle)
        {
            if (WaterHelper.GetCurrentCapacityML(bottle) != 0) return false;
            Vet(Owned().Where(i => KnownClean(i, 1)).OrderByDescending(WaterHelper.GetCurrentCapacityML).Take(2));
            // HQ before Pure (the term is the same; Pure is worth more to a plant).
            var hq = Owned().Where(i => KnownClean(i, 1) && vouched.Contains(i.uniqueId) && !ForBays(i)).OrderByDescending(LabelGrade).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId)
                .Concat(HqTanks(null).Where(t => Parent(t) != null)).ToList();
            if (hq.Sum(WaterHelper.GetCurrentCapacityML) < 749) return false;
            int g = 0;
            foreach (var src in hq)
            {
                if (WaterHelper.GetCurrentCapacityML(bottle) >= 749) break;
                g = Math.Max(g, LabelGrade(src));
                Chore.Act("pour", bottle, () => WaterHelper.TransferLiquid(src, bottle));
            }
            Poured(bottle, g);
            return WaterHelper.GetCurrentCapacityML(bottle) >= 749;
        }

        static void ResetQuality()
        {
            vouched.Clear();
            mineralBp.Clear();
            bayReserve.Clear();
            bottleGrade.Clear();
            ResetCellar();
            StripsSpent = BayTablets = BaySwaps = 0;
        }

        static JObject QualityStatus() => new JObject { ["strips_spent"] = StripsSpent, ["bay_tablets"] = BayTablets, ["bay_swaps"] = BaySwaps, ["reed_cured"] = ReedCured, ["reed_cured_ml"] = ReedCuredMl, ["reed_first_day"] = ReedFirstDay, ["reed_fills"] = ReedFills, ["reed_filtered_fills"] = ReedFiltered, ["tank_growths"] = TankGrowths, ["pure_batches"] = PureBatches, ["untested_waits"] = UntestedWaits };
    }
}
