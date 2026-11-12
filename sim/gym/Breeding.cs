using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `breeding: {lines: [...]}` (docs/research/BREEDING.md; rules in docs/re/PLANTS.md §3–6): gene lines bred on the DNA
    // manipulator. A line is a crop and an edit queue (one trait rank per entry, in order). For a crop whose harvest
    // clones the seed (fillerweed, coffee, kotton, hydroreed, dream cap) every edit lands on a cold child of the line's
    // best genome and is inherited by every later harvest, so the line compounds; a bloomberry (no seeds back) is edited
    // on fresh bought seeds, heat accumulating, until the next operation's odds fall below `give_up`. Before an apply
    // whose odds are under `min_success`, a `bank` trait of negative complexity (nutrient-poor, unappetizing on
    // fillerweed: they act only on produce the press destroys) is applied first: the genome's complexity drops for
    // good. Disks: the first one of a trait by random extraction from the donor most likely to give it; refilled from the
    // donor giving the most uses (a seed's innate trait gives its whole rank, so a bred line's own clones become rich
    // donors) before its last use is spent. `press` blends caffeine produce (its only liquid is caffeine) in the lab's
    // blender and presses it into caffeine pills the same morning: no alcohol, reagents, filter or evaporator.
    //
    // Reads: trait ranks, heat, complexity (gene scanner), disk trait and uses (disk label), the operation's odds (the
    // manipulator's tooltip), innate ranks (the scanner marks them), produce composition (spectrometer). All VISIBLE.
    internal static partial class Production
    {
        sealed class Line
        {
            public string Name, Crop;
            public string[] Edits, Bank;
            public int MinSuccess, GiveUp;
            public bool Clones;
        }

        static readonly Dictionary<string, JObject> Presets = new Dictionary<string, JObject>(StringComparer.Ordinal)
        {
            // Pure-caffeine produce (docs/research/BREEDING.md §2): caffeine first (potent needs an active liquid),
            // then each rank by its gain per point of complexity.
            ["caffeine_weed"] = JObject.Parse("{\"crop\":\"fillerweed_seed\",\"edits\":[\"production_caffeine\",\"production_caffeine\",\"production_caffeine\",\"potent\",\"production_caffeine\",\"production_caffeine\",\"potent\",\"production_caffeine\",\"production_caffeine\",\"potent\"],\"bank\":[\"nutrient_poor\",\"unappetizing\"],\"min_success\":85}"),
            // Bloomberry on the cellar's tap water: purifying cleans its own bay (lab: wine points a berry 1.16 → 1.58),
            // bountiful +1 berry; the second op is 55 %.
            ["bloom"] = JObject.Parse("{\"crop\":\"bloomberry_seed\",\"edits\":[\"purifying\",\"bountiful\"],\"give_up\":50}"),
            // Coffee: bountiful II (11 beans) and fast grower II (2 nights) double beans a bay-night.
            ["coffee"] = JObject.Parse("{\"crop\":\"coffee_bean\",\"edits\":[\"fast_grower\",\"bountiful\",\"fast_grower\",\"bountiful\",\"potent\"],\"min_success\":0}"),
        };

        static JToken linesFor;
        static List<Line> lines = new List<Line>();

        static JObject BreedCfg => Directives.Current.Raw["breeding"] as JObject;

        static List<Line> Lines()
        {
            var raw = BreedCfg?["lines"];
            if (ReferenceEquals(raw, linesFor)) return lines;
            linesFor = raw;
            lines = new List<Line>();
            foreach (var t in (raw as JArray) ?? new JArray())
            {
                var o = t.Type == JTokenType.String ? Presets.TryGetValue((string)t, out var p) ? p : null : t as JObject;
                if (o == null) continue;
                if (o["preset"] != null && Presets.TryGetValue((string)o["preset"], out var basis)) { var m = (JObject)basis.DeepClone(); m.Merge(o); o = m; }
                var plant = PlantRegistry.Get((string)o["crop"] ?? "");
                if (plant == null) continue;
                lines.Add(new Line
                {
                    Name = t.Type == JTokenType.String ? (string)t : (string)o["preset"] ?? plant.id,
                    Crop = plant.id,
                    Edits = (o["edits"] as JArray)?.Select(x => (string)x).Where(x => PlantTraitRegistry.Get(x) != null).ToArray() ?? new string[0],
                    Bank = (o["bank"] as JArray)?.Select(x => (string)x).Where(x => PlantTraitRegistry.Get(x) != null).ToArray() ?? new string[0],
                    MinSuccess = o.Value<int?>("min_success") ?? 85,
                    GiveUp = o.Value<int?>("give_up") ?? 40,
                    Clones = plant.returnsSeeds || plant.HarvestIsOwnSeed,
                });
            }
            return lines;
        }

        public static bool BreedOn => WineOn && BreedCfg != null && Lines().Count > 0;
        static int Manipulators => Math.Max(1, BreedCfg?.Value<int?>("manipulators") ?? 1);
        static bool PressOn => BreedOn && (BreedCfg.Value<bool?>("press") ?? true) && Lab.On;
        static int PressHold => BreedCfg?.Value<int?>("hold") ?? 120;

        static Line BloomLine => BreedOn ? Lines().FirstOrDefault(l => l.Crop == "bloomberry_seed") : null;

        static int Innate(string crop, string trait) => PlantRegistry.Get(crop)?.GetInnateRank(trait) ?? 0;

        // How many entries of the queue this genome has, in order.
        static int Progress(GameItem seed, Line l)
        {
            if (seed == null || seed.identifier != l.Crop) return 0;
            var need = new Dictionary<string, int>(StringComparer.Ordinal);
            for (int k = 0; k < l.Edits.Length; k++)
            {
                string t = l.Edits[k];
                need[t] = (need.TryGetValue(t, out int n) ? n : Innate(l.Crop, t)) + 1;
                if (PlantGeneticsHelper.GetTraitRank(seed, t) < need[t]) return k;
            }
            return l.Edits.Length;
        }

        static string NextTrait(GameItem seed, Line l)
        {
            int k = Progress(seed, l);
            return k < l.Edits.Length ? l.Edits[k] : null;
        }

        // The ranking every planter and earmark of a line's crop uses: the most-edited genome first.
        public static int BreedRank(GameItem seed)
        {
            if (!BreedOn || seed == null) return 0;
            var l = Lines().FirstOrDefault(x => x.Crop == seed.identifier);
            return l == null ? 0 : Progress(seed, l) * 1000 + Bank(seed, l);
        }

        static int Bank(GameItem seed, Line l) => l.Bank.Sum(t => Math.Max(0, PlantGeneticsHelper.GetTraitRank(seed, t) - Innate(l.Crop, t)));

        static bool Growing(GameItem s) => s.IsTag("GROWING_TAG");

        static IEnumerable<GameItem> LineSeeds(Line l) => Owned().Where(i => i.identifier == l.Crop && i.IsTag("SEED_ITEM_TAG") && !Growing(i) && Parent(i) == null && !StolenHelper.IsStolenItem(i));

        static int Elite(Line l) => Owned().Where(i => i.identifier == l.Crop && i.IsTag("SEED_ITEM_TAG")).Select(s => Progress(s, l)).DefaultIfEmpty(0).Max();

        // A bloomberry line is done while no cellar bay grows a plain plant and no seed is half-edited.
        static bool LineDone(Line l) => l.Clones ? Elite(l) >= l.Edits.Length
            : Bays().Any(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == "bloomberry_seed") && BloomPlainBays() == 0 && !Owned().Any(i => i.identifier == "bloomberry_seed" && Parent(i) == null && BloomUnfinished(i));

        // A bloomberry is finished when its queue is done or its next op falls under `give_up`.
        static bool BloomFinished(GameItem s)
        {
            var l = BloomLine;
            if (l == null || s?.identifier != "bloomberry_seed") return false;
            int p = Progress(s, l);
            if (p == 0) return false;
            if (p >= l.Edits.Length) return true;
            return Chance(s, null) < l.GiveUp;
        }

        static bool BloomUnfinished(GameItem s) => Progress(s, BloomLine) > 0 && !BloomFinished(s);

        // The manipulator tooltip's odds for the next op on this seed.
        static int Chance(GameItem seed, GameItem m) =>
            PlantGeneticsHelper.GetSuccessChance(PlantGeneticsHelper.GetEffectiveComplexity(seed), m == null ? 0 : ModuleHelper.GetAjustedQuality(m));

        static IEnumerable<GameItem> Disks(string trait) => Owned().Where(i => IsDisk(i) && Parent(i) == null && PlantGeneticsHelper.GetDiskTrait(i)?.id == trait);

        static IEnumerable<GameItem> Blanks() => Owned().Where(i => IsDisk(i) && Parent(i) == null && PlantGeneticsHelper.IsBlankDisk(i));

        // Every trait some unfinished line will still apply (queue entries left, bank traits).
        static HashSet<string> TraitsWanted()
        {
            var w = new HashSet<string>(StringComparer.Ordinal);
            foreach (var l in Lines())
            {
                if (LineDone(l)) continue;
                int from = l.Clones ? Elite(l) : 0;
                foreach (var t in l.Edits.Skip(from)) w.Add(t);
                foreach (var t in l.Bank) w.Add(t);
            }
            return w;
        }

        // Seeds a line keeps: its best (bays + 2) of the crop.
        static HashSet<int> KeptLineSeeds()
        {
            var keep = new HashSet<int>();
            foreach (var l in Lines().Where(x => x.Clones))
            {
                int bays = Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == l.Crop);
                foreach (var s in LineSeeds(l).OrderByDescending(s => BreedRank(s)).ThenBy(s => PlantGeneticsHelper.GetHeat(s)).ThenBy(s => s.uniqueId).Take(bays + 2))
                    keep.Add(s.uniqueId);
            }
            return keep;
        }

        // The donor for a trait: refills prefer the most uses a success (a bred clone's innate rank), random
        // extractions the best chance to pick it. Never one of the seeds a line keeps.
        static GameItem Donor(string trait, bool refill, HashSet<int> taken, GameItem m)
        {
            var t = PlantTraitRegistry.Get(trait);
            if (t == null) return null;
            var kept = KeptLineSeeds();
            var cands = Owned().Where(i => i.IsTag("SEED_ITEM_TAG") && Parent(i) == null && !Growing(i) && HydroponicHelper.GetPlant(i) != null && !StolenHelper.IsStolenItem(i)
                && PlantGeneticsHelper.GetTraitRank(i, trait) > 0 && !kept.Contains(i.uniqueId) && !taken.Contains(i.uniqueId) && !Crops.Reserved(i)).ToList();
            var precision = m == null ? null : MachineDnaManipulator.GetPrecisionTrait(m);
            double Score(GameItem s)
            {
                double p = Chance(s, m) / 100.0;
                if (refill) return p * PlantGeneticsHelper.ExtractionUses(s, t);
                int n = precision != null ? (precision.id == trait ? 1 : 0) : PlantGeneticsHelper.GetExtractableTraits(s).Count;
                return n == 0 ? 0 : p / n * PlantGeneticsHelper.ExtractionUses(s, t);
            }
            return cands.Select(s => (s, v: Score(s))).Where(x => x.v > 0).OrderByDescending(x => x.v).ThenBy(x => x.s.unitValue).ThenBy(x => x.s.uniqueId).Select(x => x.s).FirstOrDefault();
        }

        // The bought seed most likely to give a trait on a random extraction (fewest extractable innate traits).
        static string DonorSpecies(string trait) => PlantRegistry.All.Where(p => p.GetInnateRank(trait) > 0 && p.id != "nutrifruit_seed")
            .OrderBy(p => p.innateTraits.Count(kv => !(PlantTraitRegistry.Get(kv.Key)?.locked ?? true))).ThenBy(p => p.id, StringComparer.Ordinal).Select(p => p.id).FirstOrDefault();

        static readonly HashSet<string> needDonor = new HashSet<string>(StringComparer.Ordinal);
        static bool needBlank;
        public static int BreedOps, BreedApplied, BreedExtracts, BreedRefills, BreedBankOps, BreedFails;
        static readonly Dictionary<string, int> breedStalls = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly Dictionary<int, (string op, string trait, int rank, int uses)> pending = new Dictionary<int, (string, string, int, int)>();
        static readonly JArray breedLog = new JArray();

        static void BreedStall(string why) => breedStalls[why] = (breedStalls.TryGetValue(why, out int n) ? n : 0) + 1;

        static void ResetBreeding()
        {
            linesFor = null;
            lines = new List<Line>();
            needDonor.Clear();
            needBlank = needVessel = false;
            BreedOps = BreedApplied = BreedExtracts = BreedRefills = BreedBankOps = BreedFails = 0;
            breedStalls.Clear();
            pending.Clear();
            breedLog.Clear();
            pressMade.Clear();
            WeedItems = WeedPills = 0;
            WeedPillValue = WeedPillSales = WeedPillsSold = 0;
        }

        static List<GameItem> ManipulatorsOwned() => Owned().Where(i => i.identifier == "plant_dna_manipulator" && Parent(i) == null).OrderBy(i => i.uniqueId).ToList();

        static void TendBreeding(IExecutorHost ep)
        {
            needDonor.Clear();
            needBlank = false;
            ReplantBred(ep);
            StowDna();
            var stock = EmporiumEntry.Instance.invElement;
            var taken = new HashSet<int>();
            // Last night's clones out of the bays' output before the bays replant: the cold elite goes to the manipulator.
            foreach (var bay in Bays())
                foreach (var x in MachineHydroponic.GetOutputSlot(bay).childItems.Where(x => x != null && x.IsTag("SEED_ITEM_TAG") && Lines().Any(l => l.Clones && l.Crop == x.identifier)).ToList())
                    Shelve(x);
            foreach (var m in ManipulatorsOwned())
            {
                PowerManipulator(m, stock);
                if (MachineProgressHelper.GetMachineState(m) == "STATE_WORKING") continue;
                var seedSlot = MachineDnaManipulator.GetSeedSlot(m);
                var inSlot = MachineDnaManipulator.GetInputDiskSlot(m);
                var outSlot = MachineDnaManipulator.GetOutputDiskSlot(m);
                // An op loaded yesterday that did not start (no charge): its seed and disk are still in their slots
                // (a finished op ejects the disk), so it stays loaded.
                var disk0 = inSlot.childItem ?? outSlot.childItem;
                if (pending.ContainsKey(m.uniqueId) && seedSlot.childItem != null && disk0 != null && MachineDnaManipulator.PlanOperation(m) is MachineDnaManipulator.OperationPlan plan0 && plan0.blockedReason.Length == 0)
                {
                    taken.Add(seedSlot.childItem.uniqueId);
                    taken.Add(disk0.uniqueId);
                    BreedStall("unstarted");
                    continue;
                }
                Settle(ep, m);
                foreach (var d in (MachineDnaManipulator.GetEjectGrid(m)?.childItems ?? new List<GameItem>()).Where(x => x != null).ToList())
                    Shelve(d);
                Unload(m);
                foreach (var l in Lines())
                    if (PlanLine(ep, m, l, taken)) break;
            }
        }

        // PLANT_DNA's kit and Dr. Jackson's land on the back counter, where a buyer's "sell all" takes them: into stock
        // (the display case when stock is full; a full stock first puts goods for sale out on the back counter to make
        // room for the machine). Applies to `wine_dna` too.
        public static void StowDna()
        {
            if (!DnaOn) return;
            var e = EmporiumEntry.Instance;
            foreach (var i in Owned().Where(i => Parent(i) == null && ReferenceEquals(i.parentInventory, e.backInvinvElement)
                && (i.identifier == "plant_dna_manipulator" || i.identifier == "gene_scanner" || IsDisk(i) || (i.identifier ?? "").StartsWith("dna_manipulator_module", StringComparison.Ordinal))).ToList())
            {
                if (Chore.Accept(e.invElement, i, -1) == 0 || Chore.Accept(e.showcaseElement, i, -1) == 0) continue;
                if (i.identifier != "plant_dna_manipulator") continue;
                foreach (var g in e.invElement.childItems.Where(x => x != null && !Reserved(x) && !x.IsTag("HYDROPONIC_MACHINE_TAG")).OrderBy(x => PlayerRole.Est(x)).ThenBy(x => x.uniqueId).Take(16).ToList())
                {
                    if (Chore.Accept(e.backInvinvElement, g, -1) != 0) break;
                    if (Chore.Accept(e.invElement, i, -1) == 0) break;
                }
            }
        }

        static void PowerManipulator(GameItem m, GameInventory stock)
        {
            var bat = MachineHelper.GetBatterySlot(m);
            if (bat == null || (bat.childItem != null && PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) >= 12)) return;
            var cell = Owned().Where(i => Charged(i) && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (cell == null) { BreedStall("power"); return; }
            if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
            Chore.Accept(bat, cell, -1);
        }

        // Yesterday's finished op: read its outcome off the seed and disk (the scanner and the label).
        static void Settle(IExecutorHost ep, GameItem m)
        {
            if (!pending.TryGetValue(m.uniqueId, out var p)) return;
            pending.Remove(m.uniqueId);
            var seed = MachineDnaManipulator.GetSeedSlot(m).childItem;
            bool ok;
            if (p.op == "apply") ok = seed != null && PlantGeneticsHelper.GetTraitRank(seed, p.trait) > p.rank;
            else
            {
                var disk = (MachineDnaManipulator.GetOutputDiskSlot(m).childItem ?? MachineDnaManipulator.GetEjectGrid(m)?.childItems.FirstOrDefault(x => x != null && IsDisk(x)));
                ok = disk != null && PlantGeneticsHelper.IsWrittenDisk(disk) && (p.op == "extract" || PlantGeneticsHelper.GetDiskUses(disk) > p.uses);
            }
            if (!ok) BreedFails++;
            if (breedLog.Count < 120) breedLog.Add(new JObject { ["day"] = ep.Day, ["done"] = p.op, ["trait"] = p.trait, ["ok"] = ok, ["uid"] = seed?.uniqueId });
        }

        static bool PlanLine(IExecutorHost ep, GameItem m, Line l, HashSet<int> taken)
        {
            if (LineDone(l)) return false;
            GameItem seed;
            if (l.Clones)
            {
                int elite = Elite(l);
                // A cold clone of the best genome; one stays for each of the line's bays first.
                var pool = LineSeeds(l).Where(s => !taken.Contains(s.uniqueId) && Progress(s, l) == elite).OrderBy(s => PlantGeneticsHelper.GetHeat(s)).ThenByDescending(s => Bank(s, l)).ThenBy(s => s.uniqueId).ToList();
                if (pool.Count == 0) { BreedStall("no_line_seed:" + l.Name); return false; }
                seed = pool[0];
            }
            else
            {
                // A bloomberry being edited first, then a fresh one, while bred seeds are short of the plain bays.
                if (BloomBredInHand() >= Math.Max(1, BloomPlainBays()) && !Owned().Any(i => i.identifier == l.Crop && Parent(i) == null && BloomUnfinished(i))) return false;
                seed = Owned().Where(i => i.identifier == l.Crop && Parent(i) == null && !taken.Contains(i.uniqueId) && !BloomFinished(i))
                    .OrderByDescending(i => Progress(i, l)).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (seed == null) { BreedStall("no_bloom_seed"); return false; }
            }
            string trait = NextTrait(seed, l);
            if (trait == null) return false;
            bool bank = false;
            if (l.Bank.Length > 0 && Chance(seed, m) < l.MinSuccess)
            {
                // A bank trait with a disk at hand, or one an extraction can write now; otherwise the edit goes ahead
                // at the lower odds (and the missing donor or blank is bought).
                var usable = l.Bank.Where(t => PlantGeneticsHelper.CanApplyTrait(seed, PlantTraitRegistry.Get(t), out _)).ToList();
                var b = usable.FirstOrDefault(t => Disks(t).Any(d => !taken.Contains(d.uniqueId)))
                    ?? usable.FirstOrDefault(t => Blanks().Any(d => !taken.Contains(d.uniqueId)) && Donor(t, false, taken, m) != null);
                if (b != null) { trait = b; bank = true; }
                else foreach (var t in usable) { needDonor.Add(t); if (!Blanks().Any()) needBlank = true; }
            }
            if (!PlantGeneticsHelper.CanApplyTrait(seed, PlantTraitRegistry.Get(trait), out string why)) { BreedStall("refused:" + trait); return false; }
            var disk = Disks(trait).Where(d => !taken.Contains(d.uniqueId)).OrderByDescending(PlantGeneticsHelper.GetDiskUses).ThenBy(d => d.uniqueId).FirstOrDefault();
            if (disk != null)
            {
                // Its last use: refill first while the trait is still wanted after this one.
                if (PlantGeneticsHelper.GetDiskUses(disk) <= 1 && StillWanted(trait, seed, l))
                {
                    var donor = Donor(trait, true, taken, m);
                    if (donor != null && Load(ep, m, "refill", donor, disk, trait, taken)) return true;
                    if (donor == null) needDonor.Add(trait);
                }
                return Load(ep, m, bank ? "bank" : "apply", seed, disk, trait, taken);
            }
            // No disk of it: a random extraction onto a blank from the likeliest donor.
            var blank = Blanks().FirstOrDefault(d => !taken.Contains(d.uniqueId));
            var src = Donor(trait, false, taken, m);
            if (src == null) needDonor.Add(trait);
            if (blank == null) needBlank = true;
            if (blank == null || src == null) { BreedStall((blank == null ? "blank:" : "donor:") + trait); return false; }
            return Load(ep, m, "extract", src, blank, trait, taken);
        }

        // The trait is applied again later (more queue entries for it, or it is a bank trait).
        static bool StillWanted(string trait, GameItem seed, Line l)
        {
            if (l.Bank.Contains(trait)) return true;
            int at = Progress(seed, l);
            return l.Edits.Skip(at + 1).Contains(trait) || Lines().Any(o => o != l && !LineDone(o) && o.Edits.Contains(trait));
        }

        static bool Load(IExecutorHost ep, GameItem m, string op, GameItem seed, GameItem disk, string trait, HashSet<int> taken)
        {
            var seedSlot = MachineDnaManipulator.GetSeedSlot(m);
            bool apply = op == "apply" || op == "bank";
            int a, b;
            if (apply) { a = Chore.Accept(seedSlot, seed, -1); b = a == 0 ? Chore.Accept(MachineDnaManipulator.GetInputDiskSlot(m), disk, -1) : -9; }
            else { a = Chore.Accept(MachineDnaManipulator.GetOutputDiskSlot(m), disk, -1); b = a == 0 ? Chore.Accept(seedSlot, seed, -1) : -9; }
            if (a != 0 || b != 0)
            {
                Unload(m);
                BreedStall("load:" + op);
                if (breedLog.Count < 80) breedLog.Add(new JObject { ["day"] = ep.Day, ["op"] = op, ["trait"] = trait, ["load_failed"] = $"{a}/{b}", ["seed"] = seed.identifier, ["seed_parent"] = Parent(seed)?.identifier ?? seed.parentInventory?.GetType().Name, ["growing"] = Growing(seed), ["owned"] = GeneralHelper.IsItemOwned(seed) });
                return false;
            }
            var plan = MachineDnaManipulator.PlanOperation(m);
            if (plan == null || plan.blockedReason.Length > 0) { Unload(m); BreedStall("blocked:" + op); return false; }
            taken.Add(seed.uniqueId);
            taken.Add(disk.uniqueId);
            pending[m.uniqueId] = (apply ? "apply" : op, trait, PlantGeneticsHelper.GetTraitRank(seed, trait), PlantGeneticsHelper.IsWrittenDisk(disk) ? PlantGeneticsHelper.GetDiskUses(disk) : 0);
            BreedOps++;
            DnaOps++;
            if (op == "apply") BreedApplied++;
            else if (op == "bank") BreedBankOps++;
            else if (op == "refill") BreedRefills++;
            else BreedExtracts++;
            if (breedLog.Count < 120) breedLog.Add(new JObject { ["day"] = ep.Day, ["load"] = op, ["trait"] = trait, ["seed"] = seed.identifier, ["uid"] = seed.uniqueId, ["rank"] = PlantGeneticsHelper.GetTraitRank(seed, trait), ["chance"] = plan.successChance, ["elite"] = Lines().Where(x => x.Clones).Select(x => Elite(x)).DefaultIfEmpty(0).Max() });
            ep.Human("production", null, ep.Uid(m), new JObject { ["op"] = "dna_" + op, ["trait"] = trait, ["seed"] = seed.identifier, ["seed_rank"] = PlantGeneticsHelper.GetTraitRank(seed, trait), ["chance"] = plan.successChance, ["work"] = plan.progress }, "player:dna");
            return true;
        }

        static void Unload(GameItem m)
        {
            foreach (var slot in new[] { MachineDnaManipulator.GetInputDiskSlot(m), MachineDnaManipulator.GetOutputDiskSlot(m), MachineDnaManipulator.GetSeedSlot(m) })
                if (slot.childItem != null) Shelve(slot.childItem);
        }

        // Into stock, or the display case when stock is full (a manipulator's slot must be empty to load it).
        static void Shelve(GameItem i)
        {
            var e = EmporiumEntry.Instance;
            if (Chore.Accept(e.invElement, i, -1) != 0) Chore.Accept(e.showcaseElement, i, -1);
        }

        // ---- the bloomberry line through the cellar's bays (Dna.cs's replant and extra-bay rules) ----

        static int BloomBredInHand() => Owned().Count(i => i.identifier == "bloomberry_seed" && Parent(i) == null && BloomFinished(i));

        static int BloomPlainBays() => Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem s && s.identifier == "bloomberry_seed" && Progress(s, BloomLine) == 0);

        // ---- what the program buys ----

        static int BreedJacksonWants(string id)
        {
            if (!NetworkUpgrade.IsPlantDnaUnlocked() || Lines().All(LineDone)) return 0;
            switch (id)
            {
                case "plant_dna_manipulator": return Manipulators;
                case "plant_dna_disk": return needBlank ? Owned().Count(IsDisk) + 2 - Blanks().Count() : 0;
            }
            return 0;
        }

        static int BreedDonorWants(string id)
        {
            if (id == "water_pitcher" && needVessel) return Owned().Count(i => i.identifier == id) < 3 ? 1 : 0;
            if (!NetworkUpgrade.IsPlantDnaUnlocked() || needDonor.Count == 0) return 0;
            if (!needDonor.Any(t => DonorSpecies(t) == id)) return 0;
            var kept = KeptLineSeeds();
            return Owned().Count(i => i.identifier == id && Parent(i) == null && !Crops.Reserved(i) && !kept.Contains(i.uniqueId)) >= 2 ? 0 : 1;
        }

        static bool needVessel;

        static bool BreedDonorId(string id) => needDonor.Any(t => DonorSpecies(t) == id) || (needVessel && id == "water_pitcher");

        static bool BreedReserved(GameItem i)
        {
            if (i.identifier == "plant_dna_manipulator" || IsDisk(i) || (i.identifier ?? "").StartsWith("dna_manipulator_module", StringComparison.Ordinal) || i.identifier == "gene_scanner" || i.identifier == "hatchet") return true;
            if (Lines().Any(l => l.Crop == i.identifier) && i.IsTag("SEED_ITEM_TAG") && (KeptLineSeeds().Contains(i.uniqueId) || BreedRank(i) > 0)) return true;
            if (PressOn && IsDryCaffeine(i)) return Owned().Count(IsDryCaffeine) <= PressHold;
            return false;
        }

        // ---- the press: caffeine produce → blender → finisher, the same morning ----

        static readonly HashSet<int> pressMade = new HashSet<int>();
        public static int WeedItems, WeedPills;
        public static long WeedPillValue, WeedPillSales, WeedPillsSold;

        // Produce whose only liquid is caffeine (the trait's infusion on a crop with no chemistry of its own).
        static bool IsDryCaffeine(GameItem i)
        {
            if (i == null || i.IsTag("SEED_ITEM_TAG") || !i.IsTag("CHEM_ITEM_TAG") || !i.IsTag("BLENDABLE_TAG") || i.IsTag("CHEMICAL_PRODUCT_TAG")) return false;
            int caf = i.GetTagReadonly(Liquid.GetCurrentPartFromId("caffeine")).GetInt();
            if (caf <= 0) return false;
            foreach (var l in Liquid.Liquids)
                if (l.identifier != "caffeine" && i.GetTagReadonly(l.currentPart).GetInt() > 0) return false;
            return true;
        }

        static readonly string[] PressVessels = { "water_pitcher", "water_jug", "large_bottled_water", "bottled_water" };

        // Any empty vessel no other engine holds; the press is done within the morning, so none is kept.
        static GameItem PressVessel()
        {
            return Owned().Where(i => i.IsTag("LIQUID_CONTAINER_TAG") && (PressVessels.Contains(i.identifier) || i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt() >= 1000) && i.identifier != "wine_bottle"
                    && Parent(i) == null && WaterHelper.IsEmpty(i) && !Lab.Holds(i) && !HeldForWine(i) && !i.IsTag("HYDROPONIC_MACHINE_TAG"))
                .OrderByDescending(i => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt()).ThenBy(i => i.uniqueId).FirstOrDefault();
        }

        static GraphNodeStorage Body(GameItem m, int i) => (GraphNodeStorage)MachineHelper.GetBodyStorage(m, i);

        public static void PressWeed(IExecutorHost ep)
        {
            if (!PressOn) return;
            try { Press(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static void Press(IExecutorHost ep)
        {
            var items = Owned().Where(i => Parent(i) == null && IsDryCaffeine(i)).OrderBy(i => i.uniqueId).ToList();
            var blender = Owned().FirstOrDefault(i => i.identifier == "blender");
            var finisher = Owned().FirstOrDefault(i => i.identifier == "chem_finisher");
            if (blender == null || finisher == null) { if (items.Count > 0) BreedStall("press_machines"); return; }
            var bslot = Body(blender, 2) as GameSlotInventory;
            var fslot = Body(finisher, 1) as GameSlotInventory;
            // The finisher busy with the lab's batch: another morning. The lab's vessel waiting in the blender is set
            // aside and put back (its next step takes it from wherever it is).
            if (bslot == null || fslot?.childItem != null) { if (items.Count > 0) BreedStall("press_busy"); return; }
            var aside = bslot.childItem;
            var v = PressVessel();
            if (v == null) { if (items.Count > 0) { BreedStall("press_vessel"); needVessel = true; } return; }
            needVessel = false;
            var stock = EmporiumEntry.Instance.invElement;
            // A pill draws 5 ml and what is left under 5 ml is poured out: a batch waits for `press_min_ml` (25) of caffeine.
            int keep = 100 - MachineBlender.GetLoss(blender);
            long add = items.Sum(i => (long)i.GetTagReadonly(Liquid.GetCurrentPartFromId("caffeine")).GetInt()) * keep / 100;
            if (add < Math.Max(5000, (BreedCfg.Value<int?>("press_min_ml") ?? 25) * 1000)) return;
            string start = LocHelper.GetLocalizedMechanic("mech_blender_btn_start");
            {
                if (aside != null) Shelve(aside);
                if (Chore.Accept(bslot, v, -1) != 0) { BreedStall("press_blender"); if (aside != null) Chore.Accept(bslot, aside, -1); return; }
                var input = Body(blender, 1);
                for (int round = 0; round < 40 && items.Count > 0; round++)
                {
                    int loaded = 0;
                    var ids = new List<string>();
                    foreach (var it in items.ToList())
                        if (Chore.Accept(input, it, -1) == 0) { loaded++; items.Remove(it); ids.Add(it.identifier); }
                    if (loaded == 0) break;
                    var press = ep.MachineButton(blender, start);
                    if (press == null) { BreedStall("press_button"); break; }
                    press();
                    WeedItems += loaded;
                    if (breedLog.Count < 80) breedLog.Add(new JObject { ["day"] = ep.Day, ["op"] = "blend", ["items"] = loaded, ["ids"] = string.Join(",", ids.Distinct()), ["vessel_ml"] = WaterHelper.GetTotalVolume(v) / 1000 });
                }
                foreach (var left in (input as GameInventory)?.childItems.Where(x => x != null).ToList() ?? new List<GameItem>())
                    Chore.Accept(stock, left, -1);
                Chore.Accept(stock, v, -1);
                if (aside != null) Chore.Accept(bslot, aside, -1);
            }
            if (WaterHelper.GetTotalVolume(v) < 5000 || WaterHelper.IsWet(v)) return;
            if (Chore.Accept(fslot, v, -1) != 0) { BreedStall("press_finisher"); return; }
            Chore.Act("configure", finisher, () => finisher.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString("caffeine_pill")));
            var output = (GameInventory)Body(finisher, 2);
            string label = LocHelper.GetLocalizedMechanic("mech_chem_finisher_btn_output");
            int pressed = 0;
            for (int k = 0; k < 400 && MachineChemFinisher.CanOutput(finisher, fslot); k++)
            {
                var btn = ep.MachineButton(finisher, label);
                if (btn == null) break;
                btn();
                pressed++;
                foreach (var p in output.childItems.Where(x => x != null).ToList())
                {
                    if (!pressMade.Add(p.uniqueId)) continue;
                    Chore.Act("own", p, () => GeneralHelper.SetItemOwned(p, true));
                    WeedPills++;
                    WeedPillValue += PlayerRole.Est(p);
                    if (Chore.Accept(stock, p, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, p, -1);
                }
            }
            if (WaterHelper.GetTotalVolume(v) > 0) Chore.Act("empty", v, () => WaterHelper.EmptyContainer(v));
            Chore.Accept(stock, v, -1);
            if (pressed > 0) ep.Human("production", null, ep.Uid(finisher), new JObject { ["action"] = "weed_press", ["units"] = pressed, ["vessel"] = v.identifier }, "player:breeding");
        }

        public static void BreedTrade(bool sold, GameItem item, int amount)
        {
            if (!sold || item == null || !pressMade.Contains(item.uniqueId)) return;
            WeedPillsSold++;
            WeedPillSales += amount;
        }

        static JObject BreedTotals()
        {
            var o = new JObject
            {
                ["ops"] = BreedOps, ["applied"] = BreedApplied, ["bank_ops"] = BreedBankOps, ["extracts"] = BreedExtracts, ["refills"] = BreedRefills, ["failed"] = BreedFails,
                ["manipulators"] = ManipulatorsOwned().Count, ["stalls"] = JObject.FromObject(breedStalls), ["log"] = breedLog.DeepClone(),
                ["press"] = new JObject { ["items"] = WeedItems, ["pills"] = WeedPills, ["value"] = WeedPillValue, ["sold"] = WeedPillsSold, ["sales"] = WeedPillSales },
            };
            var ls = new JObject();
            foreach (var l in Lines())
            {
                var best = Owned().Where(i => i.identifier == l.Crop && i.IsTag("SEED_ITEM_TAG")).OrderByDescending(i => BreedRank(i)).FirstOrDefault();
                ls[l.Name] = new JObject
                {
                    ["elite"] = l.Clones ? Elite(l) : (int?)null, ["of"] = l.Edits.Length,
                    ["genome"] = best == null ? null : new JObject(PlantGeneticsHelper.GetTraits(best).Where(t => t.rank != Innate(l.Crop, t.trait.id)).Select(t => new JProperty(t.trait.id, t.rank))),
                    ["complexity"] = best == null ? (int?)null : PlantGeneticsHelper.GetComplexity(best),
                    ["bays"] = Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == l.Crop),
                    ["bred_planted"] = Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem s && s.identifier == l.Crop && Progress(s, l) > 0),
                };
            }
            o["lines"] = ls;
            return o;
        }
    }
}
