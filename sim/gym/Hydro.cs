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
        // `reeds: N` (default 0): N more bays grow hydroreed, whose purifying IV strips 28 % of every contaminant but
        // minerals from its water each night (docs/re/PLANTS.md §4). Each holds a jug filled raw at the faucet (Wild's
        // WATER_TRADER and GUTTERFLOW_TAP are bought for it) until every contaminant is under 10 bp; the jug comes out
        // plant-safe with the tap's own minerals, which sit in a bloomberry's or coffee plant's band, and the bays
        // drink it before anything else.
        static int Reeds => WineOn ? Math.Max(0, QualityPlan?.Value<int?>("reeds") ?? 0) : 0;
        static bool ReedOn => Reeds > 0;

        // Only once the cellar's own upgrades are in and its bloomberries planted: the tap is for the reed.
        public static IEnumerable<string> ReedUpgrades()
        {
            if (!ReedNeedsTap()) yield break;
            yield return "WATER_TRADER";
            yield return "GUTTERFLOW_TAP";
        }

        public static bool WantsReedUpgrade(string id) => ReedNeedsTap() && (id == "WATER_TRADER" || id == "GUTTERFLOW_TAP");

        static bool TapForReedsOnly => ReedOn && !WaterSellOn && !(Directives.Current.Raw.Value<bool?>("tap") ?? false);

        // After the business permit when the plan buys one: a tap before it piles OPERATING_WITHOUT_LICENSE up to an
        // arrest (docs/research/PRODUCTION-VERDICTS.md §8).
        static bool ReedNeedsTap() => ReedOn && NetworkUpgrade.IsWinemakerUnlocked() && NetworkUpgrade.IsRetiredFarmerUnlocked()
            && (PlayerStore.Instance.hasBusinessPermit || Directives.Current.Raw.Value<string>("permit") != "buy")
            && Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == "bloomberry_seed") >= Math.Min(2, BloomTarget);

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
            foreach (var c in owned.Where(i => KnownClean(i, 3)).OrderBy(LabelGrade).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId))
            {
                if (need <= 0) break;
                bayReserve.Add(c.uniqueId);
                need -= WaterHelper.GetCurrentCapacityML(c);
            }
        }

        static bool ForBays(GameItem c) => bayReserve.Contains(c.uniqueId);

        // Reed-cleaned water ranks above every label (its label is stale, its contaminants are gone).
        static int BayGrade(GameItem c) => PlantSafe(c) ? -1 : LabelGrade(c);

        // ---- the hydroreed bay -------------------------------------------------------------------------------

        const string ReedSeedId = "hydroreed_seed";
        static readonly Dictionary<int, int> reedNights = new Dictionary<int, int>();
        static readonly HashSet<int> reedClean = new HashSet<int>();
        public static int ReedCured;

        static bool IsReed(GameItem seed) => seed != null && seed.identifier == ReedSeedId;

        static int BloomTarget => (Directives.Current.Raw.Value<int?>("bays") ?? 3) + DnaExtraBays();

        // An empty bay is a reed's once the bloomberries have their bays and fewer than `reeds` reeds grow.
        static bool ReedBayDue()
        {
            if (!ReedOn) return false;
            var bays = Bays();
            if (bays.Count(b => IsReed(MachineHydroponic.GetInputSlot(b).childItem)) >= Reeds) return false;
            return bays.Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == "bloomberry_seed") >= BloomTarget;
        }

        static GameItem ReedSeed() => Owned().Where(i => i.identifier == ReedSeedId && Parent(i) == null).OrderBy(i => i.uniqueId).FirstOrDefault();

        // The purity the container went into the reed at (its label then was the truth: a faucet fill or a tested one).
        static readonly Dictionary<int, int> reedFrom = new Dictionary<int, int>();

        // Nights until the worst contaminant is under 10 bp of water at 28 % a night. Tap and premade water carry at
        // most ~35 % of their impurity as chemical, the slowest to go (WATER.md §1.4): Gutterflow at 85 % ~13 nights,
        // Rustwater ~11, Basewater ~6.
        static int ReedNightsFor(int purityBp)
        {
            double chem = 0.35 * (10000 - purityBp) * 10000.0 / Math.Max(1, purityBp);
            return chem <= 10 ? 1 : (int)Math.Ceiling(Math.Log(10 / chem) / Math.Log(0.72)) + 1;
        }

        // The tap's minerals (15 % of its impurity, WATER.md §1.4) survive the reed: the bays' ledger starts from them.
        static int TapMineralBp(int purityBp) => (int)(0.15 * (10000 - purityBp) * 10000.0 / Math.Max(1, purityBp));

        static bool PlantSafe(GameItem c) => reedClean.Contains(c.uniqueId);

        // The reed's vessel is a tank: filled raw at the faucet, cured in place, then drawn on by the bays until it runs
        // low, when it is drained and refilled. Without the tap it takes the cleanest water owned instead.
        static void TendReedWater(IExecutorHost ep, GameItem bay, GameGridInventory stock)
        {
            var ws = MachineHydroponic.GetWaterSlot(bay);
            var c = ws.childItem;
            if (c == null || WaterHelper.IsEmpty(c))
            {
                // The biggest empty vessel owned: a reed cures any volume in the same nights.
                var empty = Owned().Where(i => ReedVessel(i) && WaterHelper.IsEmpty(i) && !treatedOn.ContainsKey(i.uniqueId) && i != c)
                    .OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (empty != null && (c == null || Capacity(empty) > Capacity(c)) && (c == null || Chore.Accept(EmporiumEntry.Instance.invElement, c, -1) == 0))
                {
                    if (c != null) reedClean.Remove(c.uniqueId);
                    if (Chore.Accept(ws, empty, -1) == 0) c = empty;
                    else c = ws.childItem;
                }
                if (c == null) return;
            }
            if (!WaterHelper.IsEmpty(c) && !PlantSafe(c))
            {
                int n = (reedNights.TryGetValue(c.uniqueId, out int k) ? k : 0) + 1;
                int from = reedFrom.TryGetValue(c.uniqueId, out int f) ? f : Label(c);
                bool done = Scanner()
                    ? new[] { "heavy_metal", "chemical_contaminant", "microbe", "organic_waste" }.All(id => WaterHelper.GetContaminantPercentage(c, id) <= 10)
                    : n > ReedNightsFor(from);
                reedNights[c.uniqueId] = n;
                TestReedTank(c, n);
                if (done)
                {
                    reedClean.Add(c.uniqueId);
                    if (reedTap.Contains(c.uniqueId)) mineralBp[c.uniqueId] = TapMineralBp(from);
                    ReedCured++;
                    if (ReedFirstDay == 0) ReedFirstDay = ep.Day;
                    ReedCuredMl += WaterHelper.GetCurrentCapacityML(c);
                    ep.Human("production", null, ep.Uid(bay), new JObject { ["action"] = "reed_cured", ["ml"] = WaterHelper.GetCurrentCapacityML(c), ["nights"] = n }, "player:bay");
                }
            }
            if (Environment.GetEnvironmentVariable("FENCE_BAY_DEBUG") != null)
                Console.Error.WriteLine($"reed d{ep.Day} {bay.uniqueId} {c.identifier}#{c.uniqueId} ml={WaterHelper.GetCurrentCapacityML(c)} n={(reedNights.TryGetValue(c.uniqueId, out int nn) ? nn : -1)} safe={PlantSafe(c)} tap={TapOn}");
            if (!WaterHelper.IsEmpty(c) && (!(PlantSafe(c) || reedHq.Contains(c.uniqueId)) || WaterHelper.GetCurrentCapacityML(c) >= 200)) return;
            if (!WaterHelper.IsEmpty(c)) Chore.Act("empty", c, () => WaterHelper.EmptyContainer(c));
            reedClean.Remove(c.uniqueId);
            reedTap.Remove(c.uniqueId);
            mineralBp.Remove(c.uniqueId);
            faucet ??= Chore.Faucet();
            reedHq.Remove(c.uniqueId);
            var pitcher = Pitcher();
            if (TapOn && pitcher != null && FreshFilter(pitcher))
            {
                // Filtered at the faucet (the pitcher filters what runs into it): minerals, organics and particles out,
                // so the reed has only chemical and heavy metal to take, and the water reaches HQ in ~4 nights.
                if (!WaterHelper.IsEmpty(pitcher)) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
                int guard = 0;
                while (!WaterHelper.IsFull(c) && FilterLeft(pitcher) > 0 && guard++ < 20)
                {
                    Chore.Act("tap", pitcher, () => pitcher.onThisTargetItemFunc?.Invoke(pitcher, faucet));
                    if (WaterHelper.IsEmpty(pitcher)) break;
                    Chore.Act("pour", c, () => WaterHelper.TransferLiquid(pitcher, c));
                }
                if (!WaterHelper.IsEmpty(pitcher)) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
                if (!WaterHelper.IsEmpty(c)) reedFiltered.Add(c.uniqueId);
            }
            else if (TapOn)
            {
                Chore.Act("tap", c, () => c.onThisTargetItemFunc?.Invoke(c, faucet));
                if (!WaterHelper.IsEmpty(c)) reedTap.Add(c.uniqueId);
            }
            else
                foreach (var src in Owned().Where(i => KnownClean(i, 3) && !PlantSafe(i) && !ForBays(i)).OrderBy(LabelGrade).ThenByDescending(WaterHelper.GetCurrentCapacityML).ToList())
                {
                    if (WaterHelper.IsFull(c)) break;
                    Chore.Act("pour", c, () => WaterHelper.TransferLiquid(src, c));
                }
            if (WaterHelper.IsEmpty(c)) return;
            // The worst contaminant goes slowest: a mix is as slow as its label (the receiver's) or worse; a fresh fill's
            // label is the truth.
            reedNights[c.uniqueId] = 0;
            reedFrom[c.uniqueId] = TapOn ? Label(c) : Math.Min(Label(c), 9600);
        }

        // Filtered tank water tested HQ or better: the bottles' and bays' water (lab verdict.hydroreed: 20 L of filtered
        // rust water 96.95 % -> 99.08 % in four nights).
        static readonly HashSet<int> reedFiltered = new HashSet<int>(), reedHq = new HashSet<int>();

        static void TestReedTank(GameItem c, int nights)
        {
            if (!reedFiltered.Contains(c.uniqueId) || reedHq.Contains(c.uniqueId) || nights < 4 || WaterHelper.IsEmpty(c)) return;
            if (TestWater(c) && LabelGrade(c) <= 1) reedHq.Add(c.uniqueId);
        }

        // Reed tanks tested HQ, wherever they sit.
        static List<GameItem> HqTanks(GameItem except) => Owned().Where(i => reedHq.Contains(i.uniqueId) && i != except && !WaterHelper.IsEmpty(i) && LabelGrade(i) <= 1)
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

        static int Capacity(GameItem i) => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt();

        static bool ReedVessel(GameItem i) => (WaterVessel(i) || i.identifier == "water_tank") && Parent(i) == null && !Lab.Holds(i);

        // Cured water the bays draw on: reed tanks and loose cured containers, fullest first.
        static List<GameItem> Tanks(GameItem except) => Owned().Where(i => PlantSafe(i) && i != except && !WaterHelper.IsEmpty(i)
                && (Parent(i) == null || IsReed(MachineHydroponic.GetInputSlot(Parent(i))?.childItem)))
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

        public static int ReedCuredMl, ReedFirstDay;

        static readonly HashSet<int> reedTap = new HashSet<int>();

        // Plant-safe water for any engine's bays (docs/research/QUALITY.md, "Clean water supply"): loose owned
        // containers a reed has cured, largest first. Its label is stale (the reed never relabels); its minerals are
        // the tap's.
        public static List<GameItem> PlantSafeWater() => Owned().Where(i => PlantSafe(i) && Parent(i) == null && !WaterHelper.IsEmpty(i))
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

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
            reedNights.Clear();
            reedClean.Clear();
            reedFrom.Clear();
            reedTap.Clear();
            reedFiltered.Clear();
            reedHq.Clear();
            ReedCured = ReedCuredMl = ReedFirstDay = 0;
            StripsSpent = BayTablets = BaySwaps = 0;
        }

        static JObject QualityStatus() => new JObject { ["strips_spent"] = StripsSpent, ["bay_tablets"] = BayTablets, ["bay_swaps"] = BaySwaps, ["reed_cured"] = ReedCured, ["reed_cured_ml"] = ReedCuredMl, ["reed_first_day"] = ReedFirstDay, ["untested_waits"] = UntestedWaits };
    }
}
