using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/PRODUCTION.md: wine, aging, garden, furnace, scavenging. All nights are the game's own EndNight.
    public static class ProductionExps
    {
        internal static GameItem Owned(string id) { var it = W.Make(id); GeneralHelper.SetItemOwned(it, true); return it; }

        internal static bool Put(GraphNodeStorage into, GameItem it)
        {
            GeneralHelper.SetItemOwned(it, true);
            if (GraphUtils.TryAcceptAll(into, it, -1) == 0 && it.parentInventory != null) return true;
            // A machine or bottle item: try each of its own inventories (content window panels).
            if (into is GameItem host)
                foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(host))
                    if (GraphUtils.TryAcceptAll(inv, it, -1) == 0 && it.parentInventory != null) return true;
            return false;
        }

        internal static int Tag(GameItem it, string tag) => it.GetTagReadonly(tag).GetInt();

        internal static GameItem Battery(int charge = 5000) { var b = PreBuiltItemHelper.EnergyCredit(charge); GeneralHelper.SetItemOwned(b, true); return b; }

        // grade: 0 pure .. 5 gutter; -1 = ration (base)
        static GameItem Bottle(int grade, int[] berries, string yeast)
        {
            var b = W.Own("wine_bottle");
            if (grade < 0) WineHelper.PourRationIntoBottle(b, Owned("water_ration"));
            else WaterHelper.AddWater(b, grade);
            foreach (var c in berries)
            {
                var berry = Owned("wine_berry");
                WineHelper.SetBerryCondition(berry, c);
                if (!Put(b, berry)) return null;
            }
            if (yeast != null && !Put(b, Owned(yeast))) return null;
            return b;
        }

        static int BerryScore(int c) => c switch { -2 => -1, -1 => 0, 1 => 1, 2 => 2, _ => 0 };
        static int WaterTerm(int grade) => grade switch { 0 => 1, 1 => 1, 2 => 0, 3 => -1, 4 => -2, 5 => -3, _ => 0 };
        static int Tier(int score) => score >= 6 ? 5 : score == 5 ? 4 : score >= 3 ? 3 : score == 2 ? 2 : score == 1 ? 1 : 0;
        static int TierValue(int tier) => tier switch { 5 => 175, 4 => 150, 3 => 125, 2 => 100, 1 => 85, _ => 0 };

        public static void Wine(int seed)
        {
            W.New(seed);
            var cases = new List<(int[] berries, string yeast, int grade)>
            {
                (new[] { -2 }, "wine_yeast", -1), (new[] { -1 }, "wine_yeast", -1), (new[] { 1 }, "wine_yeast", -1), (new[] { 2 }, "wine_yeast", -1),
                (new[] { 1, 1 }, "wine_yeast", -1), (new[] { 2, 1 }, "wine_yeast", -1), (new[] { 2, 2 }, "wine_yeast", -1), (new[] { 2, -2 }, "wine_yeast", -1),
                (new[] { 1 }, null, -1), (new[] { 2, 2, 2 }, "wine_yeast", -1), (new[] { 2, 1 }, "wine_yeast", 0), (new[] { 2, 1 }, "wine_yeast", 1),
                (new[] { 2, 1 }, "wine_yeast", 2), (new[] { 2, 1 }, "wine_yeast", 3), (new[] { 2, 1 }, "wine_yeast", 4), (new[] { 2, 1 }, "wine_yeast", 5),
                (new[] { 2 }, "wine_superyeast", -1), (new[] { 2 }, "wine_yeast_infinite", -1), (new[] { 2 }, "wine_yeast_red", -1),
            };
            var bottles = new List<(GameItem b, int[] berries, string yeast, int grade, int startedNight, bool started)>();
            foreach (var (berries, yeast, grade) in cases)
            {
                var b = Bottle(grade, berries, yeast);
                if (b == null)
                {
                    R.Info($"wine: {berries.Length} berries + {yeast ?? "no yeast"} do not fit in one bottle", "rejected");
                    continue;
                }
                int waterTrue = WineHelper.GetWaterQualityScore(b);
                bool ok = WineHelper.TryStartFermentation(b);
                bottles.Add((b, berries, yeast, grade, 0, ok));
            }
            var finishedAt = new Dictionary<GameItem, int>();
            var valueAtFinish = new Dictionary<GameItem, long>();
            var ageTrace = new Dictionary<GameItem, List<(int age, long value)>>();
            for (int night = 1; night <= 8; night++)
            {
                W.QuietDay();
                foreach (var x in bottles)
                {
                    if (!finishedAt.ContainsKey(x.b) && Tag(x.b, "WINE_CONTAINER_STATE") == 2)
                    {
                        finishedAt[x.b] = night;
                        valueAtFinish[x.b] = x.b.GetCurrentValue(false, true, true, false, 0);
                    }
                    if (finishedAt.ContainsKey(x.b))
                    {
                        if (!ageTrace.ContainsKey(x.b)) ageTrace[x.b] = new List<(int, long)>();
                        ageTrace[x.b].Add((Tag(x.b, "AGE_INT"), x.b.GetCurrentValue(false, true, true, false, 0)));
                    }
                }
            }
            foreach (var x in bottles)
            {
                string label = $"[{string.Join(",", x.berries)}] {x.yeast ?? "no yeast"} water {(x.grade < 0 ? "ration" : "grade " + x.grade)}";
                if (!x.started) { R.Info("wine " + label + ": TryStartFermentation", false); continue; }
                bool red = x.yeast == "wine_yeast_red";
                int score = Tag(x.b, "WINE_SCORE_INT");
                int water = x.grade < 0 ? 0 : WaterTerm(x.grade);
                int want = x.berries.Sum(BerryScore) + (x.berries.Length >= 2 ? 1 : 0) + (x.yeast != null ? 1 : 0) + water;
                int nights = finishedAt.TryGetValue(x.b, out var fn) ? fn : -1;
                int wantNights = x.yeast switch { null => 4, "wine_superyeast" => 1, _ => 2 };
                R.Exact($"wine {label}: fermentation nights", wantNights, nights);
                if (red)
                {
                    R.Info($"wine {label}: counterfeit result", $"score {score}, value {valueAtFinish.GetValueOrDefault(x.b)}, contraband {ContrabandHelper.GetContrabandLevelInInt(x.b)}, ages: {(ageTrace.TryGetValue(x.b, out var rt) ? string.Join(",", rt.Select(a => a.age)) : "-")}");
                    continue;
                }
                R.Exact($"wine {label}: score = berries + pair + yeast + water term", want, score);
                int tier = WineHelper.GetWineQualityTier(x.b);
                R.Exact($"wine {label}: tier for score {score}", Tier(score), tier);
                if (ageTrace.TryGetValue(x.b, out var tr) && tr.Count > 0)
                {
                    R.Exact($"wine {label}: value on the finishing morning (tier {tier}, age {tr[0].age})", TierValue(tier) + (int)Math.Round(3 * tr[0].age * (TierValue(tier) / 100.0)), tr[0].value, 1,
                        "doc table value at finish " + TierValue(tier) + "; trace " + string.Join(",", tr.Select(a => $"a{a.age}:{a.value}")));
                }
            }
            // Aging: per-night value step by tier (doc: +3 x grade multiplier).
            foreach (var x in bottles.Where(b => b.yeast == "wine_yeast" && b.grade < 0 && ageTrace.ContainsKey(b.b) && ageTrace[b.b].Count >= 4))
            {
                var tr = ageTrace[x.b];
                var ages = tr.Select(a => a.age).ToList();
                bool onePerNight = Enumerable.Range(1, ages.Count - 1).All(i => ages[i] - ages[i - 1] == 1);
                int tier = WineHelper.GetWineQualityTier(x.b);
                double step = (double)(tr.Last().value - tr.First().value) / (tr.Count - 1);
                R.Near($"aging, tier {tier} [{string.Join(",", x.berries)}]: value +3 x (1+innate) per night", 3 * TierValue(tier) / 100.0, step, 0.51, tr.Count - 1,
                    $"age +1/night: {onePerNight}; values {string.Join(",", tr.Select(a => a.value))}");
            }
            // Refusals.
            {
                var dry = W.Own("wine_bottle");
                Put(dry, Owned("wine_berry"));
                R.Exact("TryStartFermentation without water", false, WineHelper.TryStartFermentation(dry));
                var noBerry = W.Own("wine_bottle");
                WineHelper.PourRationIntoBottle(noBerry, Owned("water_ration"));
                Put(noBerry, Owned("wine_yeast"));
                R.Exact("TryStartFermentation without a berry (legal yeast)", false, WineHelper.TryStartFermentation(noBerry));
                var half = W.Own("wine_bottle");
                WineHelper.PourRationIntoBottle(half, Owned("water_ration_small"));
                Put(half, Owned("wine_berry"));
                R.Exact("TryStartFermentation with 375 ml (needs >= 749)", false, WineHelper.TryStartFermentation(half));
                var cap = W.Own("wine_bottle");
                int fit = 0;
                for (int i = 0; i < 14; i++) if (Put(cap, Owned("wine_berry"))) fit++;
                R.Info("berries that fit in a bottle's ingredient grid (headless 1x1 footprints; real sprites are larger)", fit);
            }
        }

        // Rack: a bottle inside a powered wine rack vs one in stock.
        public static void Rack(int seed)
        {
            W.New(seed);
            var rack = W.Own("wine_rack");
            Put(MachineHelper.GetBatterySlot(rack), Battery());
            var inRack = WineHelper.CreateFinishedWineForDebug(5);
            var inStock = WineHelper.CreateFinishedWineForDebug(5);
            GeneralHelper.SetItemOwned(inRack, true);
            bool placed = Put(rack, inRack);
            W.Place(W.E.invElement, inStock);
            var a = new List<int>(); var s = new List<int>(); var va = new List<long>(); var vs = new List<long>();
            for (int night = 0; night < 6; night++)
            {
                a.Add(Tag(inRack, "AGE_INT")); s.Add(Tag(inStock, "AGE_INT"));
                va.Add(inRack.GetCurrentValue(false, true, true, false, 0)); vs.Add(inStock.GetCurrentValue(false, true, true, false, 0));
                W.QuietDay();
            }
            a.Add(Tag(inRack, "AGE_INT")); s.Add(Tag(inStock, "AGE_INT"));
            R.Info("rack: bottle placed inside the rack", placed, 1, "parent " + inRack.parentInventory?.GetType().Name);
            R.Exact("aging in stock: +1 AGE per night", "0,1,2,3,4,5,6", string.Join(",", s));
            R.Exact("aging in a powered rack, no modules: 'rack ages twice' = +2 per night (PRODUCTION.md inferred)", "0,2,4,6,8,10,12", string.Join(",", a), 6,
                $"rack values {string.Join(",", va)}; stock values {string.Join(",", vs)}; battery left {PowerHelper.GetAvailableEnergyFromItem(MachineHelper.GetBatterySlot(rack).childItem, false)}");
        }

        // Garden: bloomberry health, berry grades, yields and timing under different waters.
        public static void Garden(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            var setups = new List<(string label, Action<GameItem> fill)>
            {
                ("pure", j => WaterHelper.FillWithPureWater(j)),
                ("pure + 2 nutrient tablets/6L", j => { WaterHelper.FillWithPureWater(j); Dissolve(j, 2); }),
                ("pure + 6 nutrient tablets/6L", j => { WaterHelper.FillWithPureWater(j); Dissolve(j, 6); }),
                ("HQ", j => WaterHelper.FillWithHighQualityWater(j)),
                ("HQ + 6 nutrient tablets/6L", j => { WaterHelper.FillWithHighQualityWater(j); Dissolve(j, 6); }),
                ("base", j => WaterHelper.FillWithBaseWater(j)),
                ("ghost (farmer-start bay water)", j => WaterHelper.FillWithGhostwater(j)),
                ("rust", j => WaterHelper.FillWithRustWater(j)),
                ("pure + nutrient solution 100% (mini bottle poured in)", j => { WaterHelper.FillWithPureWater(j); WaterHelper.RemoveMl(j, 250); WaterHelper.FillWithNutrientSolution(j); }),
            };
            int reps = 6;
            var bays = new List<(string label, GameItem bay, GameItem seedItem, GameItem jug)>();
            foreach (var (label, fill) in setups)
                for (int r = 0; r < reps; r++)
                {
                    var bay = W.Own("hydroponic");
                    Put(MachineHelper.GetBatterySlot(bay), Battery());
                    var jug = Owned("water_jug");
                    fill(jug);
                    if (!Put(MachineHydroponic.GetWaterSlot(bay), jug)) throw new InvalidOperationException("jug rejected");
                    var sd = Owned("bloomberry_seed");
                    if (!Put(MachineHydroponic.GetInputSlot(bay), sd)) throw new InvalidOperationException("seed rejected");
                    bays.Add((label, bay, sd, jug));
                }
            var mineral0 = bays.Where((b, i) => i % reps == 0).Select(b => $"{b.label}: purity {WaterHelper.GetWaterPurity(b.jug)} mineral {WaterHelper.GetContaminantPercentage(b.jug, "mineral")}bp chem {WaterHelper.GetContaminantPercentage(b.jug, "chemical_contaminant")} org {WaterHelper.GetContaminantPercentage(b.jug, "organic_waste")} mic {WaterHelper.GetContaminantPercentage(b.jug, "microbe")} hm {WaterHelper.GetContaminantPercentage(b.jug, "heavy_metal")}").ToList();
            foreach (var m in mineral0) R.Info("garden water at planting", m);

            int nights = 13;
            var healthMorning = bays.ToDictionary(b => b.bay, b => new List<int>());
            var harvests = bays.ToDictionary(b => b.bay, b => new List<(int night, int count, int[] grades, int healthBefore)>());
            for (int night = 1; night <= nights; night++)
            {
                var before = bays.ToDictionary(b => b.bay, b => HydroponicHelper.GetHealthPercent(b.seedItem));
                var outBefore = bays.ToDictionary(b => b.bay, b => MachineHydroponic.GetOutputSlot(b.bay).childItems.Count);
                W.QuietDay();
                foreach (var b in bays)
                {
                    healthMorning[b.bay].Add(HydroponicHelper.GetHealthPercent(b.seedItem));
                    var outp = MachineHydroponic.GetOutputSlot(b.bay);
                    var fresh = outp.childItems.ToList();
                    if (fresh.Count > 0)
                    {
                        harvests[b.bay].Add((night, fresh.Count, fresh.Select(x => Tag(x, "BERRY_CONDITION_INT")).ToArray(), before[b.bay]));
                        foreach (var x in fresh) outp.Expel(x);
                    }
                }
            }
            foreach (var g in bays.GroupBy(b => b.label))
            {
                var all = g.SelectMany(b => harvests[b.bay]).ToList();
                var grades = all.SelectMany(h => h.grades).ToList();
                string nightsStr = string.Join("|", g.Take(1).Select(b => string.Join(",", harvests[b.bay].Select(h => $"n{h.night}x{h.count}"))));
                string healthStr = string.Join(",", healthMorning[g.First().bay]);
                string mix = string.Join(" ", new[] { -2, -1, 1, 2 }.Select(c => $"{c}:{grades.Count(x => x == c)}"));
                R.Info($"garden [{g.Key}]: harvest nights x berries (one bay)", nightsStr, all.Count, $"health each morning: {healthStr}");
                // Health at the harvest roll ~ morning-before minus the bay's per-night loss (measured on night 1).
                double expBad = 0;
                var est = new List<int>();
                foreach (var b in g)
                {
                    int drop = 100 - healthMorning[b.bay][0];
                    foreach (var h in harvests[b.bay])
                    {
                        int e = h.healthBefore - drop;
                        est.Add(e);
                        expBad += h.count * (e <= 25 ? 1.0 : e <= 55 ? 0.70 : e <= 85 ? 0.25 : 0.0);
                    }
                }
                int bad = grades.Count(x => x < 0);
                R.Rate($"garden [{g.Key}]: bad-berry share (moldy + under/over) vs roll table at estimated harvest health", expBad / Math.Max(1, grades.Count), bad, grades.Count,
                    $"grades {mix}; est. harvest health {est.Min()}..{est.Max()} (first harvests {string.Join(",", g.Select(b => harvests[b.bay].FirstOrDefault().healthBefore - (100 - healthMorning[b.bay][0])))}, later {string.Join(",", g.Take(2).Select(b => harvests[b.bay].Skip(1).FirstOrDefault().healthBefore - (100 - healthMorning[b.bay][0])))})");
            }
            // Timing and yield claims (pure water bays).
            var pureBay = bays.First(b => b.label == "pure");
            var hn = harvests[pureBay.bay].Select(h => h.night).ToList();
            R.Exact("bloomberry: first harvest after 3 nights, then every 2", "3,5,7,9,11,13", string.Join(",", hn));
            R.Exact("bloomberry: 8 berries per harvest (no modules)", "8", string.Join(",", harvests[pureBay.bay].Select(h => h.count).Distinct()));
            R.Exact("bloomberry seed survives harvests (perennial)", true, MachineHydroponic.GetInputSlot(pureBay.bay).childItem != null);
            // Health reported in the morning right after a harvest night.
            var hm = healthMorning[pureBay.bay];
            R.Info("pure water, no nutrients: plant health each morning (analyser 'growth score')", string.Join(",", hm), hm.Count,
                "health resets to 100 at refruit, in the same night as the harvest roll");
            // The per-band roll: test RollBloomberryCondition directly at fixed healths.
            foreach (var h in new[] { 10, 40, 70, 95 })
            {
                var counts = new Dictionary<int, int>();
                int n = 20000;
                for (int i = 0; i < n; i++)
                {
                    int c = (int)W.Call(typeof(PlantList), "RollBloomberryCondition", h);
                    counts[c] = counts.GetValueOrDefault(c) + 1;
                }
                var want = h <= 25 ? new Dictionary<int, double> { [-2] = .55, [-1] = .45 }
                    : h <= 55 ? new Dictionary<int, double> { [-2] = .25, [-1] = .45, [1] = .30 }
                    : h <= 85 ? new Dictionary<int, double> { [-1] = .25, [1] = .45, [2] = .30 }
                    : new Dictionary<int, double> { [1] = .40, [2] = .60 };
                foreach (var c in new[] { -2, -1, 1, 2 })
                    R.Rate($"berry roll at health {h}: P(condition {c})", want.GetValueOrDefault(c), counts.GetValueOrDefault(c), n);
            }
        }

        // Every crop a bay can grow, replanted from its own harvest, on pure water with the gym's nutrient dose (a tablet
        // per litre) and, for the cash crops, on each lower grade with no tablets: nights per harvest, produce per harvest,
        // value a night (docs/research/SECOND-ENGINE.md).
        public static void Crops(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            var pure = new Action<GameItem>(j => { WaterHelper.FillWithPureWater(j); Dissolve(j, 6); });
            var setups = new List<(string crop, string water, Action<GameItem> fill)>();
            foreach (var crop in new[] { "coffee_bean", "kotton_seed", "dream_cap_seed", "nutrifruit_seed", "fillerweed_seed", "bloomberry_seed" })
                setups.Add((crop, "pure+tablets", pure));
            foreach (var crop in new[] { "coffee_bean", "kotton_seed" })
            {
                setups.Add((crop, "base", j => WaterHelper.FillWithBaseWater(j)));
                setups.Add((crop, "ghost", j => WaterHelper.FillWithGhostwater(j)));
                setups.Add((crop, "rust", j => WaterHelper.FillWithRustWater(j)));
                setups.Add((crop, "gutter", j => WaterHelper.FillWithGutterflow(j)));
                setups.Add((crop, "base+tablets", j => { WaterHelper.FillWithBaseWater(j); Dissolve(j, 6); }));
                setups.Add((crop, "rust+tablets", j => { WaterHelper.FillWithRustWater(j); Dissolve(j, 6); }));
            }
            int reps = 4, nights = 24;
            var bays = new List<(string crop, string water, Action<GameItem> fill, GameItem bay, GameItem jug)>();
            foreach (var (crop, water, fill) in setups)
                for (int r = 0; r < reps; r++)
                {
                    var bay = W.Own("hydroponic");
                    Put(MachineHelper.GetBatterySlot(bay), Battery());
                    var jug = Owned("water_jug");
                    fill(jug);
                    Put(MachineHydroponic.GetWaterSlot(bay), jug);
                    if (!Put(MachineHydroponic.GetInputSlot(bay), Owned(crop))) throw new InvalidOperationException(crop + " rejected");
                    bays.Add((crop, water, fill, bay, jug));
                }
            var log = bays.ToDictionary(b => b.bay, b => new List<(int night, Dictionary<string, int> got, long value)>());
            for (int night = 1; night <= nights; night++)
            {
                W.QuietDay();
                foreach (var b in bays)
                {
                    var outp = MachineHydroponic.GetOutputSlot(b.bay);
                    var fresh = outp.childItems.Where(x => x != null).ToList();
                    if (fresh.Count > 0)
                    {
                        var got = fresh.GroupBy(x => x.identifier).ToDictionary(g => g.Key, g => g.Count());
                        // A seed crop's own seeds are its replanting stock, not produce; coffee replants one of its beans.
                        long value = fresh.Where(x => x.identifier != b.crop).Sum(x => x.unitValue) + (b.crop == "coffee_bean" ? fresh.Skip(1).Sum(x => x.unitValue) : 0);
                        log[b.bay].Add((night, got, value));
                        foreach (var x in fresh) outp.Expel(x);
                    }
                    var input = MachineHydroponic.GetInputSlot(b.bay);
                    if (input.childItem == null && !Put(input, Owned(b.crop))) throw new InvalidOperationException("replant " + b.crop);
                    if (WaterHelper.GetCurrentCapacityML(b.jug) < 2000) { WaterHelper.RemoveMl(b.jug, WaterHelper.GetCurrentCapacityML(b.jug)); b.fill(b.jug); }
                }
            }
            foreach (var g in bays.GroupBy(b => (b.crop, b.water)))
            {
                var all = g.SelectMany(b => log[b.bay]).ToList();
                string nightsStr = string.Join(",", log[g.First().bay].Select(h => h.night));
                string produce = string.Join(" ", all.SelectMany(h => h.got).GroupBy(kv => kv.Key).Select(kv => $"{kv.Key}:{kv.Sum(x => x.Value) / (double)Math.Max(1, all.Count):0.0}"));
                double perNight = all.Sum(h => h.value) / (double)(reps * nights);
                R.Info($"crops [{g.Key.crop}, {g.Key.water}]: harvest nights (one bay); items per harvest; produce value a night per bay", nightsStr, all.Count, $"{produce}; {perNight:0.0}/night");
            }
        }

        static void Dissolve(GameItem jug, int tablets)
        {
            for (int i = 0; i < tablets; i++) ChemicalHelper.DissolveInto(Owned("nutrient_tablet"), jug);
        }

        // The farmer start as shipped: its bays come with water; plant a bloomberry and watch.
        public static void FarmerStart(int seed)
        {
            W.New(seed, start: "farmer");
            var bays = W.E.invElement.childItems.Where(i => i.IsTag("HYDROPONIC_MACHINE_TAG")).ToList();
            R.Info("farmer start: hydroponic bays in stock", bays.Count, 1, string.Join("; ", bays.Select(b =>
            {
                var w = MachineHydroponic.GetWaterSlotItem(b);
                return $"{b.name}: water {(w == null ? "none" : w.identifier + " " + WaterHelper.GetWaterPurity(w) + "bp label " + WaterFeatureHelper.GetCurrentWaterDisplay(w))}, seed {MachineHydroponic.GetInputSlot(b).childItem?.identifier ?? "none"}, modules {string.Join("/", MachineHelper.GetModules(b).Select(m => m.identifier))}";
            })));
            var seeds = W.E.GetAllOwnedItems(false, true).Where(i => i.IsTag("SEED_ITEM_TAG")).Select(i => i.identifier).ToList();
            R.Info("farmer start: owned seeds", string.Join(",", seeds));
            foreach (var b in bays)
                if (MachineHydroponic.GetInputSlot(b).childItem == null) Put(MachineHydroponic.GetInputSlot(b), Owned("bloomberry_seed"));
            var harvest = new List<int>();
            var health = bays.ToDictionary(b => b, b => new List<int>());
            for (int night = 1; night <= 9; night++)
            {
                W.Cash = Math.Max(W.Cash, 2000);
                W.QuietDay();
                foreach (var b in bays)
                {
                    var sd = MachineHydroponic.GetInputSlot(b).childItem;
                    health[b].Add(sd == null ? -1 : HydroponicHelper.GetHealthPercent(sd));
                    var outp = MachineHydroponic.GetOutputSlot(b);
                    foreach (var x in outp.childItems.ToList())
                    {
                        if (x.identifier == "wine_berry") harvest.Add(Tag(x, "BERRY_CONDITION_INT"));
                        outp.Expel(x);
                    }
                }
            }
            R.Info("farmer start, bloomberry in each shipped bay with its shipped water, 9 nights: berry grades", string.Join(" ", new[] { -2, -1, 1, 2 }.Select(c => $"{c}:{harvest.Count(x => x == c)}")), harvest.Count,
                "health by morning per bay: " + string.Join(" | ", health.Values.Select(h => string.Join(",", h))));
        }

        public static void Furnace(int seed)
        {
            W.New(seed);
            var recipes = new (string label, string[] inputs, string module, string wantPurity, int wantIngots)[]
            {
                ("2 scrap", new[] { "scrap_metal", "scrap_metal" }, null, "LOW", 1),
                ("2 scrap + flux", new[] { "scrap_metal", "scrap_metal", "flux_agent" }, null, "FAIR", 1),
                ("1 scrap + 1 ore + flux", new[] { "scrap_metal", "common_ore", "flux_agent" }, null, "HIGH", 1),
                ("2 ore + flux", new[] { "common_ore", "common_ore", "flux_agent" }, null, "VERY_HIGH", 1),
                ("2 ore", new[] { "common_ore", "common_ore" }, null, "HIGH", 1),
                ("blast: 3 scrap + flux", new[] { "scrap_metal", "scrap_metal", "scrap_metal", "flux_agent" }, "furnace_module_blast", "FAIR", 2),
                ("blast: 3 ore + flux", new[] { "common_ore", "common_ore", "common_ore", "flux_agent" }, "furnace_module_blast", "VERY_HIGH", 2),
                ("2 ore + advanced flux", new[] { "common_ore", "common_ore", "advanced_flux_agent" }, null, "VERY_HIGH", 1),
            };
            var furnaces = new List<(string label, GameItem f, string want, int wantN, int startCharge)>();
            foreach (var r in recipes)
            {
                var f = W.Own("furnace");
                var bat = Battery(1000);
                Put(MachineHelper.GetBatterySlot(f), bat);
                if (r.module != null) Put(f, Owned(r.module));
                foreach (var id in r.inputs) if (!Put(f, Owned(id))) R.Info("furnace input rejected", id);
                furnaces.Add((r.label, f, r.wantPurity, r.wantIngots, PowerHelper.GetAvailableEnergyFromItem(bat, false)));
            }
            W.QuietDay();
            foreach (var (label, f, want, wantN, c0) in furnaces)
            {
                var all = GraphUtils.FindChildrenType<GameItem>(f).ToList();
                var ingots = all.Where(i => i.identifier == "metal_ingot").ToList();
                var left = all.Where(i => i.identifier != "metal_ingot" && i.identifier != "energy_credit" && !i.identifier.StartsWith("furnace_module") && !i.IsTag("MANUAL_TAG")).Select(i => i.identifier);
                string purity = string.Join(",", ingots.Select(IngotPurityHelper.GetPurity).Distinct());
                long value = ingots.Sum(i => i.GetCurrentValue(false, true, true, false, 0));
                int c1 = PowerHelper.GetAvailableEnergyFromItem(MachineHelper.GetBatterySlot(f).childItem, false);
                R.Exact($"furnace {label}: one night -> {wantN} ingot(s), purity {want}", $"{wantN} INGOT_PURITY_{want}", $"{ingots.Count} {purity}", 1,
                    $"value {value}; inputs left [{string.Join(",", left)}]; power used {c0 - c1}");
            }
        }

        // First 10 / 20 searches of a run (intro on) are immune to minor / major wounds.
        public static void ScavengeImmunity(int seed)
        {
            W.New(seed, skipIntro: false);
            var ps = W.PS;
            ps.healthData.luck = 0;
            R.Info("new game with intro: minor / major immunity searches left", $"{ps.dumpingGroundMinorWoundImmunitySearchesLeft} / {ps.dumpingGroundMajorWoundImmunitySearchesLeft}");
            var wounds = new List<string>();
            for (int i = 0; i < 40; i++)
            {
                ps.scavengingAttempts = 4; // the riskiest attempt: 9% minor, 2% major
                ps.healthData.isWoundedFresh = false;
                ps.healthData.woundState = 0;
                ScavHelper.ScavengeDumpingGrounds();
                wounds.Add(ps.healthData.woundState == 2 ? "m" : ps.healthData.woundState == 5 ? "M" : ".");
                W.ClearInventory(W.E.afterhourInventory as GameGridInventory);
            }
            string s = string.Concat(wounds);
            R.Exact("searches 1-10: no minor wound; 1-20: no major wound", true, !s.Substring(0, 10).Contains('m') && !s.Substring(0, 20).Contains('M'), 40, "sequence " + s);
        }

        // Scavenging: per-attempt wound odds, loot.
        public static void Scavenge(int seed)
        {
            W.New(seed);
            var ps = W.PS;
            ps.healthData.luck = 0;
            R.Info("HealthData.GetLuck() with luck=0", HealthData.GetLuck(), 1, $"EventModifier injury_chance {W.Call(typeof(ScavHelper), "EventModifier", "injury_chance")}, day {W.Day}, minor/major chance at 0 attempts {ScavHelper.GetMinorWoundChance()}/{ScavHelper.GetMajorWoundChance()}");
            int n = Program.N * 5;
            foreach (bool flashlight in new[] { false, true })
            {
                W.ClearInventory(W.E.afterhourInventory as GameGridInventory);
                if (flashlight) Put(W.E.afterhourInventory, Owned("flashlight"));
                for (int k = 0; k < 5; k++)
                {
                    int minor = 0, major = 0, loot = 0, stolen = 0;
                    var ids = new Dictionary<string, int>();
                    long val = 0;
                    for (int i = 0; i < n; i++)
                    {
                        ps.scavengingAttempts = k;
                        ps.healthData.isWoundedFresh = false;
                        ps.healthData.woundState = 0;
                        ps.dumpingGroundMinorWoundImmunitySearchesLeft = 0;
                        ps.dumpingGroundMajorWoundImmunitySearchesLeft = 0;
                        var before = W.E.afterhourInventory.childItems.ToList();
                        ScavHelper.ScavengeDumpingGrounds();
                        if (ps.healthData.woundState == 2) minor++;
                        else if (ps.healthData.woundState == 5) major++;
                        foreach (var it in W.E.afterhourInventory.childItems.Except(before).ToList())
                        {
                            loot++;
                            if (StolenHelper.IsStolenItem(it)) stolen++;
                            ids[it.identifier ?? "?"] = ids.GetValueOrDefault(it.identifier ?? "?") + 1;
                            val += it.GetCurrentValue(false, true, true, false, 0);
                            W.E.afterhourInventory.Expel(it);
                        }
                    }
                    double pm = (5 + k) / (flashlight ? 2.0 : 1.0) / 100, pM = (1 + 0.25 * k) / (flashlight ? 2.0 : 1.0) / 100;
                    string fl = flashlight ? " (flashlight)" : "";
                    R.Rate($"scavenge attempt #{k + 1}{fl}: minor wound = (5 + {k})%{(flashlight ? "/2" : "")}", pm, minor, n);
                    R.Rate($"scavenge attempt #{k + 1}{fl}: major wound = (1 - minor) x (1 + 0.25x{k})%{(flashlight ? "/2" : "")}", (1 - pm) * pM, major, n);
                    if (k == 0 && !flashlight)
                    {
                        R.Info("scavenge loot per successful search: items, mean value", $"{(double)loot / Math.Max(1, n - minor - major):0.###} items, {(double)val / Math.Max(1, loot):0.#} cr", loot,
                            "top: " + string.Join(", ", ids.OrderByDescending(kv => kv.Value).Take(25).Select(kv => kv.Key + " " + (100.0 * kv.Value / loot).ToString("0.0") + "%")));
                        R.Exact("scavenged items are never stolen", 0, stolen, loot);
                    }
                }
            }
            // Immunity: first 10 / 20 searches of a run.
            R.Info("new game: minor / major immunity searches left", $"{ps.dumpingGroundMinorWoundImmunitySearchesLeft} / {ps.dumpingGroundMajorWoundImmunitySearchesLeft}");
            R.Info("max scavenge attempts per visit (no perks)", ScavHelper.GetMaxScavAttempts());
        }
    }
}
