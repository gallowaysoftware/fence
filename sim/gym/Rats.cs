using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `engine: rats` (HusbandryHelper, AnimalCage, MachineFeedDispenser): rats breed, grow and eat in a cage overnight
    // (OnAnimalItemDayEnd); everything else is the player's. Each morning, at the cage window: rats found elsewhere go
    // into the cage; droppings go to the bin; dead rats and those at `rats_butcher_g` (default 400) are cut up with
    // the cleaver's two strokes (kill, then cut: HusbandryHelper.OnButcher, by weight into raw meat per 400 g, small
    // raw meat per 200 g, morsels); owned food is blended into the cage's dispenser (rats draw from it up to 85 %
    // hunger); an owned ingot grows the cage a row (AnimalCage.UpgradeSize, up to 8 rows of 10) and scrap mends it.
    // Breeders are kept (`rats_breeders: [females, males]`, default [1, 1]), and the youngest grower of a sex is
    // spared while its breeder is within four nights of old age. Growers of 150 g and up are cut, heaviest first,
    // while the rats fill more than `rats_occupancy` of the cage (default 0.7: above 0.55 fertility falls a fifth,
    // above 0.7 each rat kills a smaller one 5 % of nights, above 0.9 15 %). The meat is ordinary FOOD stock.
    internal static class Rats
    {
        public static bool On => (Directives.Current.Engine ?? "").Split('+').Contains("rats");

        // `rats_food_cut_g` (default 300): the lightest grower cut when the food would not last two nights.
        static int FoodCutG => Directives.Current.Raw.Value<int?>("rats_food_cut_g") ?? 300;
        static int ButcherG => Directives.Current.Raw.Value<int?>("rats_butcher_g") ?? 400;
        static double Occupancy => Directives.Current.Raw.Value<double?>("rats_occupancy") ?? 0.7;
        static int Females => (Directives.Current.Raw["rats_breeders"] as JArray)?.Count == 2 ? (int)Directives.Current.Raw["rats_breeders"][0] : 1;
        static int Males => (Directives.Current.Raw["rats_breeders"] as JArray)?.Count == 2 ? (int)Directives.Current.Raw["rats_breeders"][1] : 1;
        // Kcal kept in the dispenser and in held food (a rat burns ~65 a night, a newborn needs ~300).
        static int FeedKcal => Directives.Current.Raw.Value<int?>("rats_feed_kcal") ?? 6000;

        // Food the executor buys for the dispenser: the foods with the most calories per credit (10 or more). Rats never
        // get rat meat (cannibalism, prions); other meat is fine.
        static readonly HashSet<string> Feeds = new HashSet<string>(StringComparer.Ordinal)
        {
            "beis_icecream", "processed_milk", "processed_juice", "fat_meat", "meat_scrap", "fish_scrap", "cup_noodle", "nutrifruit", "processed_cheese",
            "galaxy_blend", "li_eat_snackbar", "cat_bar", "fillerweed", "meal_cap",
        };

        static int Kcal(GameItem i) => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt();

        // Any food the dispenser may take in a pinch: calories, not rat, not canned or a pellet, legal and clean.
        static bool Edible(GameItem i) => i.IsTag("CALORIE_VALUE_TAG") && Kcal(i) > 0 && !i.IsTag("CANNED_FOOD_TAG") && !GutterflowPelletHelper.IsPellet(i) && !IndustrialFishFeedHelper.IsFeed(i)
            && i.GetTagReadonly("MEAT_TYPE_TAG").GetString() != "ANIMAL_RAT" && !Rat(i) && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0 && !Production.Reserved(i) && !Directives.Current.HoldUids.Contains(i.uniqueId);

        static bool Live() => Cages().Any(c => AnimalCage.GetCageInventory(c)?.childItems.Any(i => i != null && Rat(i) && !Dead(i)) ?? false) || Owned().Any(i => Rat(i) && !Dead(i));

        const string Ingot = "metal_ingot", Scrap = "scrap_metal";

        public static int Butchered, Died, Born, Upgrades, Repairs, Cleaned, Blended, FirstDay, Pinched, HungryDays, FoodCuts;
        public static long CutsValue, KcalBlended, MeatSold, MeatSales, FeedSpend;
        static readonly HashSet<int> seen = new HashSet<int>(), breeders = new HashSet<int>();

        public static void Reset()
        {
            Butchered = Died = Born = Upgrades = Repairs = Cleaned = Blended = FirstDay = Pinched = HungryDays = FoodCuts = 0;
            CutsValue = KcalBlended = MeatSold = MeatSales = FeedSpend = 0;
            seen.Clear();
            breeders.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static bool Rat(GameItem i) => i.IsTag("ANIMAL_ITEM_TAG");
        static bool Dead(GameItem i) => i.IsTag("ANIMAL_IS_DEAD_TAG");
        static float Weight(GameItem i) => i.GetTagReadonly("ANIMAL_WEIGHT_TAG").GetFloat();
        static int Age(GameItem i) => i.GetTagReadonly("ANIMAL_AGE_TAG").GetInt();
        static bool Female(GameItem i) => i.GetTagReadonly("ANIMAL_SEXE_TAG").GetString() == "FEMALE";
        static bool Sick(GameItem i) => i.IsTag("ANIMAL_DISEASED_TAG") || i.IsTag("ANIMAL_PRION_TAG");

        public static List<GameItem> Cages() => EmporiumEntry.Instance == null ? new List<GameItem>() : Owned().Where(i => i.IsTag("CAGE_TAG") && Parent(i) == null).ToList();

        static GameItem Dispenser(GameItem cage) => AnimalCage.GetDispenserSlot(cage)?.childItem is GameItem d && d.IsTag("FEED_DISPENSER_TAG") ? d : null;

        static int StoredKcal() => Cages().Select(Dispenser).Where(d => d != null).Sum(MachineFeedDispenser.GetAnimalKcal);

        static int HeldFeedKcal() => Owned().Where(i => Feeds.Contains(i.identifier ?? "") && Parent(i) == null).Sum(Kcal);

        public static bool Reserved(GameItem i)
        {
            if (!On) return false;
            if (i.IsTag("CAGE_TAG") || i.IsTag("FEED_DISPENSER_TAG") || Rat(i) || i.identifier == "kitchen_cleaver") return true;
            if (Feeds.Contains(i.identifier ?? "")) return StoredKcal() < FeedKcal;
            return false;
        }

        public static bool IsInput(string id) => On && id != null && (Feeds.Contains(id) || id == Ingot || id == Scrap);

        public static int Wants(string id)
        {
            if (!On || id == null || Cages().Count == 0) return 0;
            if (Feeds.Contains(id))
            {
                if (!Live()) return 0;
                int gap = FeedKcal - StoredKcal() - HeldFeedKcal();
                return gap <= 0 ? 0 : (gap + 399) / 400;
            }
            if (id == Ingot) return Cages().Any(AnimalCage.CanUpgradeSize) && !Owned().Any(i => i.identifier == Ingot && Parent(i) == null) ? 1 : 0;
            if (id == Scrap) return Cages().Any(c => Health(c) < 60) && !Owned().Any(i => i.identifier == Scrap && Parent(i) == null) ? 1 : 0;
            return 0;
        }

        // A night's food: ~120 a rat, a litter's first meal for a pregnant female.
        static int Need(GameGridInventory grid) => grid == null ? 0 : grid.childItems.Where(i => i != null && Rat(i) && !Dead(i)).Sum(r => r.IsTag("ANIMAL_PREGNANT_TAG") ? 1000 : 120);

        static int Health(GameItem cage) => cage.GetTagReadonly("CAGE_HEALTH_TAG").GetInt() * 100 / Math.Max(1, cage.GetTagReadonly("CAGE_MAX_HEALTH_TAG").GetInt());

        static int Cells(GameGridInventory g) => g.inventoryShape.width * g.inventoryShape.height;

        static int RatCells(GameGridInventory g) => g.childItems.Where(i => i != null && Rat(i)).Sum(PlayerRole.Cells);

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            var cages = Cages();
            if (cages.Count == 0) return;
            if (FirstDay == 0) FirstDay = ep.Day;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var cage in cages)
            {
                // The cage belongs in the stock grid (a kit lands on the back counter, where a buyer could take it).
                if (!ReferenceEquals(cage.parentInventory, stock)) Chore.Accept(stock, cage, -1);
                try { Tend(ep, cage, stock); }
                catch (Exception e) { ep.ProductionError(e); }
            }
            // Rats outside a cage (the kit's, a stray) go into the emptiest cage.
            foreach (var r in Owned().Where(i => Rat(i) && !(i.parentInventory?.IsTag("CAGE_TAG") ?? false)).ToList())
            {
                var into = cages.Select(AnimalCage.GetCageInventory).Where(g => g != null).OrderBy(g => RatCells(g) * 100 / Math.Max(1, Cells(g))).FirstOrDefault();
                if (into != null) Chore.Accept(into, r, -1);
            }
        }

        static void Tend(IExecutorHost ep, GameItem cage, GameGridInventory stock)
        {
            var grid = AnimalCage.GetCageInventory(cage);
            if (grid == null) return;
            var bin = EmporiumEntry.Instance.trashcan?.contentWindow?.child as GameInventory;
            foreach (var f in grid.childItems.Where(i => i != null && i.IsTag("ANIMAL_FECES_TAG")).ToList())
            {
                if (bin != null && Chore.Accept(bin, f, -1) == 0) Cleaned++;
                else { Chore.Act("discard", f, () => f.Destroy()); Cleaned++; }
            }
            var rats = grid.childItems.Where(i => i != null && Rat(i)).ToList();
            foreach (var r in rats.Where(r => seen.Add(r.uniqueId) && Age(r) == 0)) Born++;
            foreach (var r in rats.Where(Dead).ToList())
            {
                Died++;
                Cut(ep, r, stock);
            }
            var alive = grid.childItems.Where(i => i != null && Rat(i) && !Dead(i)).ToList();
            if (Environment.GetEnvironmentVariable("FENCE_RATS_DEBUG") != null)
                foreach (var r in alive)
                    Console.Error.WriteLine($"rats d{ep.Day} {r.uniqueId} {(Female(r) ? "F" : "M")} age={Age(r)} g={Weight(r):0} fert={r.GetTagReadonly("ANIMAL_CURRENT_FERTILITY_TAG").GetFloat():0} lon={GeneticHelper.GetCurrentStat(r, "ANIMAL_LONGEVITY_TAG"):0.00} hunger={r.GetTagReadonly("ANIMAL_HUNGER_TAG").GetFloat():0} preg={r.IsTag("ANIMAL_PREGNANT_TAG")} sick={Sick(r)} breeder={breeders.Contains(r.uniqueId)} old={Old(r)} mates={r.GetTagReadonly("ANIMAL_MAX_MATING_COUNT_TAG").GetFloat()} cells={PlayerRole.Cells(r)}");
            // A breeder breeds until it dies (then it is cut up): old age takes it within a few nights of growthStages[5].
            breeders.RemoveWhere(u => !alive.Any(r => r.uniqueId == u && !Sick(r)));
            KeepBreeders(alive, true, Females);
            KeepBreeders(alive, false, Males);
            // Successors: the youngest healthy grower of each sex is spared the butcher while a breeder is old.
            var spare = new HashSet<int>();
            foreach (bool female in new[] { true, false })
                if (alive.Any(r => breeders.Contains(r.uniqueId) && Female(r) == female && Age(r) + 4 >= LifeDays(r)) || alive.Count(r => breeders.Contains(r.uniqueId) && Female(r) == female) < (female ? Females : Males))
                    foreach (var r in alive.Where(r => Female(r) == female && !breeders.Contains(r.uniqueId) && !Sick(r) && !Old(r)).OrderBy(Age).ThenBy(r => r.uniqueId).Take(1))
                        spare.Add(r.uniqueId);
            foreach (var r in alive.Where(r => !breeders.Contains(r.uniqueId) && !spare.Contains(r.uniqueId) && (Weight(r) >= ButcherG || Old(r) || (Sick(r) && Weight(r) >= 100))).ToList())
                Cut(ep, r, stock);
            // Food for fewer than two nights: the heaviest growers go first rather than starve.
            int food = StoredKcal() + HeldFeedKcal();
            foreach (var r in grid.childItems.Where(i => i != null && Rat(i) && !Dead(i) && !breeders.Contains(i.uniqueId) && !spare.Contains(i.uniqueId) && Weight(i) >= FoodCutG).OrderByDescending(Weight).ToList())
            {
                if (food >= 2 * Need(grid)) break;
                Cut(ep, r, stock);
                FoodCuts++;
            }
            // Growers under 150 g are worth a morsel: they are cut for room only when the cage is near the 90 % at
            // which rats start killing each other nightly at 15 %.
            int cap = (int)(Cells(grid) * Occupancy), hard = (int)(Cells(grid) * 0.85);
            foreach (var r in grid.childItems.Where(i => i != null && Rat(i) && !Dead(i) && !breeders.Contains(i.uniqueId) && !spare.Contains(i.uniqueId)).OrderByDescending(Weight).ToList())
            {
                int used = RatCells(grid);
                if (used <= cap || (Weight(r) < 150 && used <= hard)) break;
                Cut(ep, r, stock);
            }
            Feed(ep, cage);
            var ingot = Owned().FirstOrDefault(i => i.identifier == Ingot && Parent(i) == null && !StolenHelper.IsStolenItem(i));
            if (ingot != null && AnimalCage.CanUpgradeSize(cage))
            {
                Chore.Act("cage_upgrade", cage, () => AnimalCage.UpgradeSize(cage, ingot));
                Upgrades++;
            }
            var scrap = Owned().FirstOrDefault(i => i.identifier == Scrap && Parent(i) == null);
            if (scrap != null && Health(cage) < 60 && AnimalCage.CanRepair(cage))
            {
                Chore.Act("cage_repair", cage, () => AnimalCage.Repair(cage, scrap));
                Repairs++;
            }
        }

        // A rat past its prime: old age kills 75 % a night from growthStages[5] × longevity.
        static bool Old(GameItem r) => Age(r) + 1 >= LifeDays(r);

        static double LifeDays(GameItem r)
        {
            var sp = HusbandryHelper.GetSpecies(r);
            return sp == null ? double.MaxValue : sp.growthStages[5] * GeneticHelper.GetCurrentStat(r, "ANIMAL_LONGEVITY_TAG");
        }

        static void KeepBreeders(List<GameItem> alive, bool female, int want)
        {
            int have = alive.Count(r => breeders.Contains(r.uniqueId) && Female(r) == female);
            foreach (var r in alive.Where(r => Female(r) == female && !breeders.Contains(r.uniqueId) && HusbandryHelper.IsMature(r) && !Sick(r) && !Old(r)).OrderBy(Age).ThenBy(r => r.uniqueId))
            {
                if (have >= want) break;
                breeders.Add(r.uniqueId);
                have++;
            }
        }

        // The cleaver's two strokes: a live rat is killed, a dead one cut up; the cuts are ours and go to stock.
        static void Cut(IExecutorHost ep, GameItem rat, GameGridInventory stock)
        {
            var cuts = new List<GameItem>();
            Chore.Act("butcher", rat, () =>
            {
                if (!Dead(rat)) HusbandryHelper.OnAnimalDead(rat);
                HusbandryHelper.OnButcher(rat, c => cuts.Add(c));
            });
            if (cuts.Count == 0) return;
            Butchered++;
            breeders.Remove(rat.uniqueId);
            foreach (var c in cuts)
            {
                Chore.Act("own", c, () => GeneralHelper.SetItemOwned(c, true));
                CutsValue += PlayerRole.Est(c);
                if (Chore.Accept(stock, c, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, c, -1);
            }
            ep.Human("production", null, null, new JObject { ["action"] = "butcher", ["cuts"] = cuts.Count }, "player:rats");
        }

        static void Feed(IExecutorHost ep, GameItem cage)
        {
            var d = Dispenser(cage);
            if (d == null) return;
            var grid = MachineFeedDispenser.GetStorageGrid(d);
            if (grid == null || MachineFeedDispenser.GetAnimalKcal(d) >= FeedKcal) return;
            int added = 0;
            foreach (var f in Owned().Where(i => Feeds.Contains(i.identifier ?? "") && Parent(i) == null && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).ToList())
            {
                if (MachineFeedDispenser.GetAnimalKcal(d) + added >= FeedKcal) break;
                if (Chore.Accept(grid, f, -1) == 0) added += Kcal(f);
            }
            // Short of a night's food: the shop's own food stock, most kcal per credit first.
            int need = Need(AnimalCage.GetCageInventory(cage));
            if (MachineFeedDispenser.GetAnimalKcal(d) + added < need)
                foreach (var f in Owned().Where(i => Parent(i) == null && Edible(i) && !Feeds.Contains(i.identifier ?? "")).OrderByDescending(i => Kcal(i) / (double)Math.Max(1, PlayerRole.Est(i))).ThenBy(i => i.uniqueId).ToList())
                {
                    if (MachineFeedDispenser.GetAnimalKcal(d) + added >= need) break;
                    if (Chore.Accept(grid, f, -1) == 0) { added += Kcal(f); Pinched++; }
                }
            if (added == 0) { if (MachineFeedDispenser.GetAnimalKcal(d) < need) HungryDays++; return; }
            var press = ep.MachineButton(d, LocHelper.GetLocalizedMechanic("mech_blender_btn_start"));
            if (press == null) { if (MachineFeedDispenser.GetAnimalKcal(d) < need) HungryDays++; return; }
            int before = MachineFeedDispenser.GetAnimalKcal(d);
            press();
            int gained = MachineFeedDispenser.GetAnimalKcal(d) - before;
            if (gained > 0) { KcalBlended += gained; Blended++; }
            if (MachineFeedDispenser.GetAnimalKcal(d) < need) HungryDays++;
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || item == null) return;
            if (sold && item.GetTagReadonly("MEAT_TYPE_TAG").GetString() == "ANIMAL_RAT") { MeatSold++; MeatSales += amount; }
            if (!sold && Feeds.Contains(item.identifier ?? "")) FeedSpend += amount;
        }

        public static JObject Status()
        {
            if (!On) return null;
            var cages = Cages();
            var grids = cages.Select(AnimalCage.GetCageInventory).Where(g => g != null).ToList();
            return new JObject
            {
                ["first_day"] = FirstDay, ["cages"] = cages.Count, ["cells"] = grids.Sum(Cells), ["rats"] = grids.Sum(g => g.childItems.Count(i => i != null && Rat(i) && !Dead(i))),
                ["rat_cells"] = grids.Sum(RatCells), ["born"] = Born, ["died"] = Died, ["butchered"] = Butchered, ["cuts_value"] = CutsValue,
                ["meat_sold"] = MeatSold, ["meat_sales"] = MeatSales, ["feed_spend"] = FeedSpend, ["kcal_blended"] = KcalBlended, ["blends"] = Blended,
                ["stored_kcal"] = StoredKcal(), ["pinched"] = Pinched, ["hungry_days"] = HungryDays, ["food_cuts"] = FoodCuts, ["upgrades"] = Upgrades, ["repairs"] = Repairs, ["cleaned"] = Cleaned,
                ["cage_health"] = new JArray(cages.Select(Health)),
            };
        }
    }
}
