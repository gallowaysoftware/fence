using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `wine_dna: true` (docs/re/WINE.md §3): breed contaminant-tolerant bloomberries. Wild's PLANT_DNA, then Dr. Jackson's
    // DNA manipulator and blank disks; each morning, as a player at the machine: extract a trait from a donor seed
    // (fillerweed, nutrifruit, hydroreed: all carry `hardy`) onto a blank disk, keep the disk when it reads Hardy, and
    // apply it to a fresh bloomberry seed, which then goes into the next empty bay (an extra bay is bought for it, or
    // an owned hatchet clears an ordinary plant). `wine_dna_precision: true` also buys the precision module and tunes
    // it on the first Hardy disk, so later extractions take only that trait. Everything read here is on the disk's
    // label or the gene scanner's readout.
    internal static partial class Production
    {
        public static bool DnaOn => WineOn && (Directives.Current.Raw.Value<bool?>("wine_dna") ?? false);
        static bool DnaPrecision => DnaOn && (Directives.Current.Raw.Value<bool?>("wine_dna_precision") ?? false);
        // Cash kept after Wild's PLANT_DNA beyond the rent and floor.
        static int DnaReserve => Directives.Current.Raw.Value<int?>("wine_dna_reserve") ?? 1500;
        // Hardy ranks wanted on each bloomberry seed (a second rank costs a second operation at ~55 %).
        static int DnaRanks => Math.Max(1, Directives.Current.Raw.Value<int?>("wine_dna_ranks") ?? 1);

        // `wine_dna_trait` (default "hardy"): "purifying" cleans the bloomberry's own bay water to rank I's floors (refruit
        // health ~90 on any Basewater, docs/re/PLANTS.md §2), and only the hydroreed carries it (rank IV: four uses a disk).
        static string Trait => Directives.Current.Raw.Value<string>("wine_dna_trait") == "purifying" ? "purifying" : "hardy";
        static readonly HashSet<string> HardyDonors = new HashSet<string>(StringComparer.Ordinal) { "fillerweed_seed", "nutrifruit_seed", "hydroreed_seed" };
        static readonly HashSet<string> PurifyingDonors = new HashSet<string>(StringComparer.Ordinal) { "hydroreed_seed" };
        static HashSet<string> Donors => Trait == "purifying" ? PurifyingDonors : HardyDonors;

        public static int DnaOps, DnaReplanted;

        static int Hardy(GameItem seed) => PlantGeneticsHelper.GetTraitRank(seed, Trait);

        static GameItem Manipulator() => Owned().FirstOrDefault(i => i.identifier == "plant_dna_manipulator" && Parent(i) == null);

        static bool IsDisk(GameItem i) => i.identifier == "plant_dna_disk";

        static bool HardyDisk(GameItem i) => IsDisk(i) && PlantGeneticsHelper.GetDiskTrait(i)?.id == Trait;

        static bool Bloomseed(GameItem i) => i.identifier == "bloomberry_seed" && Parent(i) == null;

        // Bred seeds waiting for a bay, and plain seeds a Hardy disk can still improve.
        static int BredInHand() => Owned().Count(i => Bloomseed(i) && Hardy(i) >= DnaRanks);

        static bool DiskReady() => Owned().Any(i => HardyDisk(i) && Parent(i) == null);

        // Plain bays left to replace: every bay ends up with a bred plant.
        static int PlainBays() => Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem s && Hardy(s) < DnaRanks);

        static bool DnaDone() => PlainBays() == 0 && Bays().Any();

        public static bool DnaReserved(GameItem i)
        {
            if (!DnaOn) return false;
            if (i.identifier == "plant_dna_manipulator" || (IsDisk(i) && (PlantGeneticsHelper.IsBlankDisk(i) || HardyDisk(i))) || (i.identifier ?? "").StartsWith("dna_manipulator_module", StringComparison.Ordinal) || i.identifier == "gene_scanner" || i.identifier == "hatchet") return true;
            return Donors.Contains(i.identifier ?? "") && !DnaDone();
        }

        // Dr. Jackson's stock the lever wants, as a total owned.
        static int JacksonDnaWants(string id)
        {
            if (!DnaOn || !NetworkUpgrade.IsPlantDnaUnlocked() || DnaDone()) return 0;
            var owned = Owned().ToList();
            switch (id)
            {
                case "plant_dna_manipulator": return 1;
                case "plant_dna_disk":
                    int blanks = owned.Count(i => IsDisk(i) && PlantGeneticsHelper.IsBlankDisk(i));
                    return DiskReady() ? 0 : owned.Count(IsDisk) + Math.Max(0, 2 - blanks);
                case "dna_manipulator_module_precision": return DnaPrecision ? 1 : 0;
            }
            return 0;
        }

        // A plain bloomberry seed for the disk, and one bay more than the plan's while a bred seed waits and no hatchet
        // can clear a plain plant.
        static int DnaWantsSeed() => DnaOn && DiskReady() && !Owned().Any(i => Bloomseed(i) && Hardy(i) < DnaRanks) ? 1 : 0;

        static int DnaExtraBays() => DnaOn && BredInHand() > 0 && !Owned().Any(i => i.identifier == "hatchet") ? 1 : 0;

        public static void PlanDna(UpgradeView v, List<string> into)
        {
            if (!DnaOn || !NetworkUpgrade.IsRetiredFarmerUnlocked()) return;
            var u = v.Upgrades.FirstOrDefault(x => x.Id == "PLANT_DNA");
            if (u != null && !u.Unlocked && v.Cash + v.Favor >= u.Cost + v.RentDue + DnaReserve) into.Add("PLANT_DNA");
        }

        static void TendDna(IExecutorHost ep)
        {
            if (!DnaOn) return;
            ReplantBred(ep);
            var m = Manipulator();
            if (m == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            var bat = MachineHelper.GetBatterySlot(m);
            if (bat != null && (bat.childItem == null || PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) < 12))
            {
                var cell = Owned().Where(i => Charged(i) && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (cell != null)
                {
                    if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                    Chore.Accept(bat, cell, -1);
                }
            }
            if (MachineProgressHelper.GetMachineState(m) == "STATE_WORKING") return;
            var seedSlot = MachineDnaManipulator.GetSeedSlot(m);
            var inSlot = MachineDnaManipulator.GetInputDiskSlot(m);
            var outSlot = MachineDnaManipulator.GetOutputDiskSlot(m);
            foreach (var d in (MachineDnaManipulator.GetEjectGrid(m)?.childItems ?? new List<GameItem>()).Where(x => x != null).ToList())
                Chore.Accept(stock, d, -1);
            // An operation loaded but not started (no charge last night) stays loaded.
            var held = seedSlot.childItem;
            if (held != null && ((Donors.Contains(held.identifier ?? "") && PlantGeneticsHelper.IsBlankDisk(outSlot.childItem))
                || (Bloomseed2(held) && Hardy(held) < DnaRanks && inSlot.childItem != null && HardyDisk(inSlot.childItem)))) return;
            // Yesterday's work back to the shelf: a disk left in a slot, a seed that is done.
            foreach (var slot in new[] { inSlot, outSlot })
                if (slot.childItem != null && (slot != outSlot || !PlantGeneticsHelper.IsBlankDisk(slot.childItem))) Chore.Accept(stock, slot.childItem, -1);
            if (seedSlot.childItem is GameItem inMachine && !(Bloomseed2(inMachine) && Hardy(inMachine) < DnaRanks && DiskReady()))
                Chore.Accept(stock, inMachine, -1);
            TunePrecision(m);
            if (DnaDone()) return;
            var disk = Owned().Where(i => HardyDisk(i) && Parent(i) == null).OrderBy(i => i.uniqueId).FirstOrDefault();
            if (disk != null)
            {
                var plain = seedSlot.childItem ?? Owned().Where(i => Bloomseed(i) && Hardy(i) < DnaRanks).OrderByDescending(Hardy).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (plain == null || BredInHand() >= Math.Max(1, PlainBays())) return;
                if (outSlot.childItem != null) Chore.Accept(stock, outSlot.childItem, -1);
                if (seedSlot.childItem == null) Chore.Accept(seedSlot, plain, -1);
                if (seedSlot.childItem == plain && Chore.Accept(inSlot, disk, -1) == 0)
                {
                    DnaOps++;
                    ep.Human("production", null, ep.Uid(m), new JObject { ["action"] = "dna_apply", ["trait"] = Trait, ["seed_rank"] = Hardy(plain) }, "player:dna");
                }
                return;
            }
            // No Hardy disk yet: extract from the donor with the most Hardy ranks (more uses on the disk).
            var blank = outSlot.childItem ?? Owned().Where(i => IsDisk(i) && PlantGeneticsHelper.IsBlankDisk(i) && Parent(i) == null).OrderBy(i => i.uniqueId).FirstOrDefault();
            var donor = Owned().Where(i => Donors.Contains(i.identifier ?? "") && Parent(i) == null && Hardy(i) > 0).OrderByDescending(Hardy).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (blank == null || donor == null) return;
            if (outSlot.childItem == null) Chore.Accept(outSlot, blank, -1);
            if (outSlot.childItem == blank && Chore.Accept(seedSlot, donor, -1) == 0 && seedSlot.childItem == donor)
            {
                DnaOps++;
                ep.Human("production", null, ep.Uid(m), new JObject { ["action"] = "dna_extract", ["donor"] = donor.identifier, ["precision"] = MachineDnaManipulator.GetPrecisionTrait(m)?.id }, "player:dna");
            }
        }

        static bool Bloomseed2(GameItem i) => i.identifier == "bloomberry_seed";

        // Donor seeds bought from sellers while no Hardy disk is in hand; cells for the manipulator.
        static int DnaWants(string id)
        {
            if (!DnaOn || !NetworkUpgrade.IsPlantDnaUnlocked() || DnaDone() || DiskReady() || !Donors.Contains(id)) return 0;
            return Owned().Any(i => Donors.Contains(i.identifier ?? "") && Parent(i) == null) ? 0 : 1;
        }

        static bool DnaUsesCells => DnaOn && Manipulator() != null;

        // The precision module goes in once it is tuned on a Hardy disk (using the module on the disk).
        static void TunePrecision(GameItem m)
        {
            if (!DnaPrecision || MachineDnaManipulator.GetPrecisionTrait(m) != null) return;
            var module = Owned().FirstOrDefault(i => i.identifier == "dna_manipulator_module_precision" && Parent(i) == null);
            var disk = Owned().FirstOrDefault(i => HardyDisk(i) && Parent(i) == null);
            if (module == null || disk == null) return;
            if (module.canThisTargetItemFunc != null && module.canThisTargetItemFunc(module, disk)) Chore.Act("dna_disk", module, () => module.onThisTargetItemFunc?.Invoke(module, disk));
            if (PlantTraitRegistry.Get(module.GetTagReadonly("DNA_PRECISION_TRAIT_STRING").GetString()) == null) return;
            foreach (var grid in GraphUtils.FindChildrenType<GameGridInventory>(m))
                if (Chore.Accept(grid, module, -1) == 0 && Parent(module) == m) break;
        }

        // A bred seed waiting: into an empty bay (TendBay plants the best seed first), or a hatchet clears the plain
        // plant of a bay that was just harvested (its berries are already out).
        static void ReplantBred(IExecutorHost ep)
        {
            if (BredInHand() == 0 || Bays().Any(b => MachineHydroponic.GetInputSlot(b).childItem == null)) return;
            var hatchet = Owned().FirstOrDefault(i => i.identifier == "hatchet");
            if (hatchet == null) return;
            var bay = Bays().Where(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem s && Hardy(s) < DnaRanks).OrderBy(b => b.uniqueId).FirstOrDefault();
            if (bay == null) return;
            if (hatchet.canThisTargetItemFunc != null && !hatchet.canThisTargetItemFunc(hatchet, bay)) return;
            Chore.Act("uproot", bay, () => HydroponicHelper.RemovePlantFromHydroponic(bay));
            DnaReplanted++;
            ep.Human("production", null, ep.Uid(bay), new JObject { ["action"] = "dna_replant" }, "player:dna");
        }

        // The seed TendBay plants: a bred one first when the lever is on.
        static GameItem SeedToPlant() => DnaOn
            ? Owned().Where(i => Bloomseed(i)).OrderByDescending(Hardy).ThenBy(i => i.uniqueId).FirstOrDefault()
            : Owned().FirstOrDefault(i => i.identifier == "bloomberry_seed" && Parent(i) == null);

        static JObject DnaTotals() => new JObject { ["ops"] = DnaOps, ["bred_planted"] = Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem is GameItem s && Hardy(s) >= DnaRanks), ["replanted"] = DnaReplanted, ["manipulator"] = Manipulator() != null, ["hardy_disk"] = DiskReady() };
    }
}
