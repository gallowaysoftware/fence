using System;
using System.Collections.Generic;
using System.Linq;
using static Fence.Lab.ProductionExps;

namespace Fence.Lab
{
    // docs/research/BREEDING.md, docs/re/PLANTS.md §3–6: the DNA manipulator's odds and timing, disk uses, what an edited
    // genome harvests, and the caffeine-weed chain (fillerweed carrying caffeine → blender → finisher), all on the game's
    // own manipulator, bays (EndNight) and chem machines.
    public static class BreedExps
    {
        static GameItem Manipulator(params string[] modules)
        {
            var m = W.Own("plant_dna_manipulator");
            Put(MachineHelper.GetBatterySlot(m), Battery(100000));
            foreach (var id in modules)
                if (!Put(ChemExps.Storage(m, 1), Owned(id))) throw new InvalidOperationException("manipulator: module rejected " + id);
            return m;
        }

        static GameItem Disk(string trait, int uses)
        {
            var d = Owned("plant_dna_disk");
            if (trait != null) PlantGeneticsHelper.WriteDisk(d, PlantTraitRegistry.Get(trait), 1, uses);
            return d;
        }

        static GameItem Seed(string id, params (string trait, int rank)[] edits)
        {
            var s = Owned(id);
            foreach (var (t, r) in edits) PlantGeneticsHelper.SetTraitRank(s, t, r);
            return s;
        }

        static int Rank(GameItem s, string t) => PlantGeneticsHelper.GetTraitRank(s, t);

        static bool Working(GameItem m) => MachineProgressHelper.GetMachineState(m) == "STATE_WORKING";

        // Nights until every manipulator is idle again (an op starts and works its first night on the night it is loaded).
        static int RunOps(List<GameItem> ms, int max = 20)
        {
            int n = 0;
            do { W.QuietDay(); n++; } while (ms.Any(Working) && n < max);
            return n;
        }

        // The manipulator's odds, work and heat (PlantGeneticsHelper): success = clamp(95 − 5·max(0, complexity + heat) + ⌊Q/2⌋, 5, 100),
        // work = 20 + 4·max(0, ·) at 10 a night; a failed apply spends no use; heat += 2 + |trait complexity| either way.
        public static void Odds(int seed)
        {
            W.New(seed);
            W.Cash = 1000000;
            int n = Math.Max(60, Program.N / 20);
            foreach (var (crop, heat) in new[] { ("coffee_bean", 0), ("coffee_bean", 4), ("coffee_bean", 8), ("coffee_bean", 14), ("fillerweed_seed", 0), ("fillerweed_seed", 10) })
            {
                var ms = new List<GameItem>();
                var seeds = new List<GameItem>();
                var disks = new List<GameItem>();
                int plannedChance = -1, plannedWork = -1;
                for (int i = 0; i < n; i++)
                {
                    var m = Manipulator();
                    var s = Seed(crop);
                    if (heat > 0) s.ModifyTag("GENETIC_HEAT_INT", st => st.Enable().SetInt(heat));
                    var d = Disk("bountiful", 3);
                    if (!Put(MachineDnaManipulator.GetSeedSlot(m), s) || !Put(MachineDnaManipulator.GetInputDiskSlot(m), d)) throw new InvalidOperationException("load");
                    var plan = MachineDnaManipulator.PlanOperation(m);
                    plannedChance = plan.successChance;
                    plannedWork = plan.progress;
                    ms.Add(m); seeds.Add(s); disks.Add(d);
                }
                int c = PlantGeneticsHelper.GetComplexity(seeds[0]);
                int eff = c + heat;
                int nights = RunOps(ms);
                int ok = seeds.Count(s => Rank(s, "bountiful") > PlantList_Innate(crop, "bountiful"));
                int usesSpent = disks.Count(d => PlantGeneticsHelper.GetDiskUses(d) == 2);
                int heatAfter = PlantGeneticsHelper.GetHeat(seeds[0]);
                double want = Math.Max(5, Math.Min(100, 95 - 5 * Math.Max(0, eff))) / 100.0;
                R.Exact($"manipulator [{crop}, heat {heat}]: tooltip success % / work", $"{(int)(want * 100)}/{20 + 4 * Math.Max(0, eff)}", $"{plannedChance}/{plannedWork}");
                R.Rate($"manipulator [{crop}, complexity {c} + heat {heat}]: apply succeeds", want, ok, n, $"nights {nights} (work {plannedWork} at 10 a night); uses spent on success only: {usesSpent}/{ok}; heat after {heatAfter} (= {heat} + 2 + 4)");
            }
        }

        static int PlantList_Innate(string crop, string trait) => PlantRegistry.Get(crop)?.GetInnateRank(trait) ?? 0;

        // Extraction: a random pick among the seed's unlocked traits, uses = the seed's rank of an innate trait (else 1);
        // a refill adds the same uses with no pick; without the stabilizer the seed is destroyed; with it the seed keeps
        // its genome less one rank and gains heat.
        public static void Extract(int seed)
        {
            W.New(seed);
            W.Cash = 1000000;
            int n = Math.Max(90, Program.N / 5);
            var ms = new List<GameItem>();
            var disks = new List<GameItem>();
            for (int i = 0; i < n; i++)
            {
                var m = Manipulator();
                var d = Disk(null, 0);
                Put(MachineDnaManipulator.GetOutputDiskSlot(m), d);
                Put(MachineDnaManipulator.GetSeedSlot(m), Seed("coffee_bean"));
                ms.Add(m); disks.Add(d);
            }
            RunOps(ms);
            var picks = disks.Where(d => PlantGeneticsHelper.IsWrittenDisk(d)).GroupBy(d => PlantGeneticsHelper.GetDiskTrait(d).id).ToDictionary(g => g.Key, g => g.Count());
            int written = picks.Values.Sum();
            foreach (var t in new[] { "production_caffeine", "production_coffee_mush", "potent" })
                R.Rate($"random extraction from a coffee bean: picks {t}", 1 / 3.0, picks.GetValueOrDefault(t), Math.Max(1, written), "of the successful extractions");
            R.Rate("random extraction from a coffee bean (complexity 0): succeeds", 0.95, written, n);
            R.Exact("extraction without the stabilizer destroys the bean", 0, ms.Count(m => MachineDnaManipulator.GetSeedSlot(m).childItem != null), n);

            // Refills from bred donors: an innate trait gives the donor's whole rank, a carried one gives 1.
            var cases = new[] { ("coffee_bean", "production_caffeine", 3, "innate, bred to III"), ("coffee_bean", "potent", 4, "innate, bred to IV"), ("fillerweed_seed", "production_caffeine", 3, "carried III"), ("fillerweed_seed", "hardy", 3, "innate III") };
            foreach (var (crop, trait, rank, label) in cases)
            {
                var rm = new List<GameItem>();
                var rd = new List<GameItem>();
                for (int i = 0; i < 40; i++)
                {
                    var m = Manipulator();
                    var d = Disk(trait, 1);
                    Put(MachineDnaManipulator.GetOutputDiskSlot(m), d);
                    if (!Put(MachineDnaManipulator.GetSeedSlot(m), Seed(crop, (trait, rank)))) throw new InvalidOperationException("refill seed rejected");
                    rm.Add(m); rd.Add(d);
                }
                RunOps(rm);
                var gains = rd.Select(d => PlantGeneticsHelper.GetDiskUses(d) - 1).Where(g => g > 0).Distinct().OrderBy(g => g).ToList();
                R.Exact($"refill a {trait} disk from a {crop} with {label}: uses gained", trait == "production_caffeine" && crop == "fillerweed_seed" ? "1" : rank.ToString(), string.Join(",", gains), 40,
                    $"succeeded {rd.Count(d => PlantGeneticsHelper.GetDiskUses(d) > 1)}/40; still the same trait: {rd.All(d => PlantGeneticsHelper.GetDiskTrait(d)?.id == trait)}");
            }

            // Stabilizer: the donor survives, one rank lighter, with heat.
            var sm = new List<GameItem>();
            var ss = new List<GameItem>();
            for (int i = 0; i < 40; i++)
            {
                var m = Manipulator("dna_manipulator_module_stabilizer");
                var d = Disk("production_caffeine", 1);
                Put(MachineDnaManipulator.GetOutputDiskSlot(m), d);
                var s = Seed("coffee_bean", ("production_caffeine", 3));
                Put(MachineDnaManipulator.GetSeedSlot(m), s);
                sm.Add(m); ss.Add(s);
            }
            int nights = RunOps(sm);
            var kept = ss.Where(s => s.parentInventory != null).ToList();
            R.Info("stabilized refill from a caffeine-III bean: bean kept, its caffeine rank after, heat after", $"kept {kept.Count}/40; ranks {string.Join(",", kept.Select(s => Rank(s, "production_caffeine")).Distinct().OrderBy(x => x))}; heat {string.Join(",", kept.Select(PlantGeneticsHelper.GetHeat).Distinct())}", 40, $"nights {nights} (stabilizer: speed 10 → 7)");
        }

        static GameItem Bay(Action<GameItem> fill, out GameItem jug)
        {
            var bay = W.Own("hydroponic");
            Put(MachineHelper.GetBatterySlot(bay), Battery(100000));
            jug = Owned("water_jug");
            fill(jug);
            Put(MachineHydroponic.GetWaterSlot(bay), jug);
            return bay;
        }

        static List<GameItem> TakeOutput(GameItem bay)
        {
            var outp = MachineHydroponic.GetOutputSlot(bay);
            var fresh = outp.childItems.Where(x => x != null).ToList();
            foreach (var x in fresh) outp.Expel(x);
            return fresh;
        }

        // The caffeine weed: fillerweed (1 night, immune to contaminants, tap water, no tablets) carrying the caffeine trait.
        // Every produce item is infused with 2 ml × r^0.75 of caffeine, × (1 + 0.25·p^0.75) with potent; nothing else is in
        // it, so the blender gives a dry, pure caffeine residue that the finisher presses with no alcohol, no reagents,
        // no filter and no evaporator.
        public static void CaffeineWeed(int seed)
        {
            W.New(seed);
            W.Cash = 1000000;
            // The manipulator refuses potent on a seed with no active liquid, accepts it after caffeine.
            var plain = Seed("fillerweed_seed");
            var withCaf = Seed("fillerweed_seed", ("production_caffeine", 1));
            var potent = Disk("potent", 1);
            R.Exact("potent disk on a plain fillerweed: refused / on a caffeine fillerweed: accepted", "False/True",
                $"{PlantGeneticsHelper.IsDiskCompatibleWithSeed(potent, plain)}/{PlantGeneticsHelper.IsDiskCompatibleWithSeed(potent, withCaf)}");
            var caf = Disk("production_caffeine", 1);
            R.Exact("caffeine disk on a fillerweed (harvest is not its own seed; production traits act on any produce): accepted", true, PlantGeneticsHelper.IsDiskCompatibleWithSeed(caf, plain));

            // A real apply on the manipulator, then grow it.
            var m = Manipulator();
            var s0 = Seed("fillerweed_seed");
            Put(MachineDnaManipulator.GetSeedSlot(m), s0);
            Put(MachineDnaManipulator.GetInputDiskSlot(m), Disk("production_caffeine", 1));
            int c0 = MachineDnaManipulator.PlanOperation(m).successChance;
            RunOps(new List<GameItem> { m });
            R.Info("apply caffeine to a fresh fillerweed (simple genome II: complexity −8)", $"chance {c0}%; rank after {Rank(s0, "production_caffeine")}; complexity {PlantGeneticsHelper.GetComplexity(s0)}; heat {PlantGeneticsHelper.GetHeat(s0)}");

            var genomes = new[] { (0, 0), (1, 0), (2, 0), (3, 1), (4, 2), (6, 2) };
            var tap = new Action<GameItem>(j => WaterHelper.FillWithRustWater(j));
            var produce = new Dictionary<(int, int), List<GameItem>>();
            foreach (var (r, p) in genomes)
            {
                var edits = new List<(string, int)>();
                if (r > 0) edits.Add(("production_caffeine", r));
                if (p > 0) edits.Add(("potent", p));
                var bay = Bay(tap, out var jug);
                var s = Seed("fillerweed_seed", edits.ToArray());
                s.ModifyTag("GENETIC_HEAT_INT", st => st.Enable().SetInt(9));
                Put(MachineHydroponic.GetInputSlot(bay), s);
                var all = new List<GameItem>();
                var harvestNights = new List<int>();
                var seedsBack = new List<GameItem>();
                int ml0 = WaterHelper.GetCurrentCapacityML(jug);
                for (int night = 1; night <= 4; night++)
                {
                    W.QuietDay();
                    var got = TakeOutput(bay);
                    if (got.Count == 0) continue;
                    harvestNights.Add(night);
                    seedsBack.AddRange(got.Where(x => x.identifier == "fillerweed_seed"));
                    all.AddRange(got.Where(x => x.identifier != "fillerweed_seed"));
                    var next = got.FirstOrDefault(x => x.identifier == "fillerweed_seed");
                    if (next != null) Put(MachineHydroponic.GetInputSlot(bay), next);
                }
                produce[(r, p)] = all;
                double cafMl = all.Sum(x => ChemExps.P(x, "caffeine")) / 1000.0;
                long otherUl = all.Sum(x => Liquid.Liquids.Where(l => l.identifier != "caffeine").Sum(l => x.IsTag("CHEM_ITEM_TAG") ? ChemExps.P(x, l.identifier) : 0));
                double want = 2 * PlantGeneticsHelper.RankScale(r) * (1 + 0.25 * PlantGeneticsHelper.RankScale(p));
                var perItem = all.Select(x => ChemExps.P(x, "caffeine")).Distinct().OrderBy(x => x).ToList();
                R.Info($"caffeine weed [caffeine {r}, potent {p}] on raw rust (tap) water: harvests, items/harvest, caffeine per item (µL), other liquids", $"nights {string.Join(",", harvestNights)}; {all.Count / (double)Math.Max(1, harvestNights.Count):0.0} items; {string.Join("/", perItem)} µL (paper {want * 1000:0}); other {otherUl} µL; {cafMl / Math.Max(1, harvestNights.Count):0.0} ml a harvest",
                    harvestNights.Count, $"seeds back {seedsBack.Count} over {harvestNights.Count} harvests; child traits caf {string.Join(",", seedsBack.Select(x => Rank(x, "production_caffeine")).Distinct())} potent {string.Join(",", seedsBack.Select(x => Rank(x, "potent")).Distinct())} heat {string.Join(",", seedsBack.Select(PlantGeneticsHelper.GetHeat).Distinct())}; water {ml0 - WaterHelper.GetCurrentCapacityML(jug)} ml over 4 nights; produce contraband {all.Count(x => x.IsTag("CONTRABAND_ITEM_TAG"))}; food {all.Count(x => x.IsGameItemType("FOOD"))}/{all.Count}");
                if (r > 0) R.Near($"caffeine weed [caffeine {r}, potent {p}]: caffeine per item, ml", want, perItem.DefaultIfEmpty(0).Max() / 1000.0, 0.002);
            }

            // Blend and press: 0 and 4 blender modules, 0 / 1 / 4 finisher modules.
            foreach (var (bm, fm) in new[] { (0, 0), (0, 1), (4, 4) })
            {
                var items = produce[(4, 2)].Concat(produce[(6, 2)]).Concat(produce[(3, 1)]).ToList();
                var fresh = new List<GameItem>();
                foreach (var x in items)
                {
                    var copy = Owned("fillerweed");
                    ChemicalHelper.InitChemItem(copy, isBlendable: true);
                    ChemicalHelper.AddChemContentUL(copy, "caffeine", ChemExps.P(x, "caffeine"));
                    fresh.Add(copy);
                }
                var v = ChemExps.Container("water_pitcher");
                var bl = ChemExps.ChemMachine("blender", bm);
                Put(ChemExps.Storage(bl, 2), v);
                int loaded = fresh.Count(x => Put(ChemExps.Storage(bl, 1), x));
                ChemExps.Press(ChemExps.Buttons(bl)[0]);
                ChemExps.Lift(v);
                W.Place(W.E.invElement, v);
                string residue = ChemExps.Parts(v);
                bool wet = WaterHelper.IsWet(v);
                var made = ChemExps.Finish(v, "caffeine_pill", fm);
                double caffeineIn = items.Sum(x => ChemExps.P(x, "caffeine")) / 1000.0;
                R.Info($"caffeine weed → blender ({bm} modules) → finisher ({fm} modules), no alcohol or reagents, same morning", ChemExps.Summary(made), made.Count,
                    $"{loaded} items, {caffeineIn:0.0} ml caffeine in; residue {residue}; wet {wet}; pills per ml {made.Count / Math.Max(0.1, caffeineIn):0.000} (paper (1 − loss)/5 = {(1 - new[] { 0.35, 0.2, 0.1, 0.05, 0 }[bm]) / 5:0.000})");
            }
        }

        // Coffee: the harvest is its own seed, so every bean is a clone of the parent with heat 0 and the seed-produce hooks
        // (caffeine, coffee mush, potent) applied. Bountiful scales the count, fast grower the nights.
        public static void Coffee(int seed)
        {
            W.New(seed);
            W.Cash = 1000000;
            var pure = new Action<GameItem>(j => { WaterHelper.FillWithPureWater(j); for (int i = 0; i < 9; i++) ChemicalHelper.DissolveInto(Owned("nutrient_tablet"), j); });
            var genomes = new (string label, (string, int)[] edits)[]
            {
                ("plain", new (string, int)[0]),
                ("bountiful II", new[] { ("bountiful", 2) }),
                ("bountiful IV", new[] { ("bountiful", 4) }),
                ("fast grower II", new[] { ("fast_grower", 2) }),
                ("bountiful II + fast grower II", new[] { ("bountiful", 2), ("fast_grower", 2) }),
                ("caffeine III", new[] { ("production_caffeine", 3) }),
                ("coffee mush III", new[] { ("production_coffee_mush", 3) }),
                ("potent III", new[] { ("potent", 3) }),
            };
            foreach (var (label, edits) in genomes)
            {
                var bays = Enumerable.Range(0, 3).Select(k => Bay(pure, out GameItem _)).ToList();
                foreach (var b in bays) Put(MachineHydroponic.GetInputSlot(b), Seed("coffee_bean", edits));
                var log = new List<(int night, int beans)>();
                var beans = new List<GameItem>();
                for (int night = 1; night <= 9; night++)
                {
                    W.QuietDay();
                    foreach (var b in bays)
                    {
                        var got = TakeOutput(b);
                        if (got.Count == 0) continue;
                        log.Add((night, got.Count));
                        beans.AddRange(got);
                        Put(MachineHydroponic.GetInputSlot(b), got[0]);
                    }
                }
                var one = beans.FirstOrDefault();
                double caf = beans.Count == 0 ? 0 : beans.Average(x => ChemExps.P(x, "caffeine")) / 1000.0;
                double mush = beans.Count == 0 ? 0 : beans.Average(x => ChemExps.P(x, "coffee_mush")) / 1000.0;
                R.Info($"coffee [{label}] on pure water + tablets: harvest nights (bay 1); beans a harvest; per bean caffeine / mush ml", $"nights {string.Join(",", log.Where((x, i) => i % 3 == 0).Select(x => x.night))}; {log.Average(x => x.beans):0.0} beans; {caf:0.00} / {mush:0.00} ml",
                    log.Count, $"bean traits {string.Join(" ", PlantGeneticsHelper.GetTraits(one).Select(t => t.trait.id + "=" + t.rank))}; heat {string.Join(",", beans.Select(PlantGeneticsHelper.GetHeat).Distinct())}; caffeine after a 1× alcohol extraction ≈ {caf + 0.75 * mush:0.0} ml a bean");
            }
        }

        // Bloomberry on Basewater-grade water (the executor's tap water): plain vs hardy I vs purifying I vs purifying I + bountiful I.
        public static void Bloom(int seed)
        {
            W.New(seed);
            W.Cash = 1000000;
            // The executor's tap water: Rustwater through a water filter, a nutrient tablet a litre.
            var tap = new Action<GameItem>(j =>
            {
                for (int k = 0; k < 3; k++)
                {
                    var raw = Owned("water_pitcher");
                    WaterHelper.FillWithRustWater(raw);
                    var pitcher = Owned("water_pitcher");
                    Put(pitcher, Owned("water_filter"));
                    WaterHelper.TransferLiquid(raw, pitcher);
                    WaterHelper.TransferLiquid(pitcher, j);
                }
                for (int i = 0; i < WaterHelper.GetCurrentCapacityML(j) / 1000; i++) ChemicalHelper.DissolveInto(Owned("nutrient_tablet"), j);
            });
            var basew = new Action<GameItem>(j => { WaterHelper.FillWithBaseWater(j); for (int i = 0; i < 6; i++) ChemicalHelper.DissolveInto(Owned("nutrient_tablet"), j); });
            var setups = new (string label, (string, int)[] edits)[]
            {
                ("plain", new (string, int)[0]),
                ("hardy I", new[] { ("hardy", 1) }),
                ("purifying I", new[] { ("purifying", 1) }),
                ("purifying I + bountiful I", new[] { ("purifying", 1), ("bountiful", 1) }),
                ("bountiful I", new[] { ("bountiful", 1) }),
            };
            int nights = 21;
            foreach (var (water, fill) in new[] { ("base + tablets", basew), ("tap-treated", tap) })
            foreach (var (label, edits) in setups)
            {
                var bays = Enumerable.Range(0, 6).Select(k => Bay(fill, out GameItem _)).ToList();
                if (water == "tap-treated" && label == "plain")
                {
                    var j0 = MachineHydroponic.GetWaterSlot(bays[0]).childItem;
                    R.Info("tap-treated water in the bay", $"{WaterHelper.GetCurrentCapacityML(j0)} ml; chem {WaterHelper.GetContaminantPercentage(j0, "chemical_contaminant")} heavy {WaterHelper.GetContaminantPercentage(j0, "heavy_metal")} mineral {WaterHelper.GetContaminantPercentage(j0, "mineral")} bp");
                }
                foreach (var b in bays) Put(MachineHydroponic.GetInputSlot(b), Seed("bloomberry_seed", edits));
                var early = new List<int>();
                var late = new List<int>();
                int harvests = 0;
                for (int night = 1; night <= nights; night++)
                {
                    W.QuietDay();
                    foreach (var b in bays)
                    {
                        var got = TakeOutput(b);
                        if (got.Count == 0) continue;
                        harvests++;
                        (night <= 9 ? early : late).AddRange(got.Select(x => Tag(x, "BERRY_CONDITION_INT")));
                    }
                }
                double Pts(List<int> g) => g.Count == 0 ? 0 : g.Average(c => c switch { -2 => -1, -1 => 0, 1 => 1, 2 => 2, _ => 0 });
                R.Info($"bloomberry [{label}] on {water}, 6 bays × {nights} nights: berries a harvest; wine points a berry nights 1–9 / 10–21", $"{(early.Count + late.Count) / (double)Math.Max(1, harvests):0.00}; {Pts(early):0.00} / {Pts(late):0.00}", harvests,
                    $"late grades {string.Join(" ", new[] { -2, -1, 1, 2 }.Select(c => $"{c}:{late.Count(x => x == c)}"))}");
            }
        }
    }
}
