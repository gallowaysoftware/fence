using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `engine: fish` (MachineRAS, AquacultureHelper; docs/re/AQUACULTURE.md): tilapia breed, grow and eat in a RAS tank
    // overnight (AquacultureHelper.TickTank); everything else is the player's. Each morning, at each tank's window:
    // stray fish and roe go into a tank (a fish outside one dies at night); owned water tops the tank up to its need;
    // a charged cell goes into the battery; the slotted dispenser is kept full (or, without one, a bag of industrial
    // feed lies loose in the tank); filters go in once the waste nears the 3 % at which fish start to suffer; scrap,
    // ingots and electronics upgrade the tank (width, height, power). Breeders are kept (`fish_breeders: [females,
    // males]`, default by tank size) and replaced from the young stock before old age. A grower is filleted with an
    // owned blade once its tooltip's meat reaches `fish_harvest_meat` (default 400, one fish fillet), or earlier when the
    // tank lacks the room or water a clutch of fry needs. Everything read here is on the fish's or the tank's tooltip.
    internal static class Fish
    {
        public static bool On => (Directives.Current.Engine ?? "").Split('+').Contains("fish");

        static int HarvestMeat => Directives.Current.Raw.Value<int?>("fish_harvest_meat") ?? 400;
        // Kcal kept in the dispenser and in held feed.
        static int FeedKcal => Directives.Current.Raw.Value<int?>("fish_feed_kcal") ?? 10000;
        static bool Upgrade => Directives.Current.Raw.Value<bool?>("fish_upgrade") ?? true;
        // `fish_big: true`: Wild's AQUACULTURE_II (1 200), then eels and sturgeon in a second tank, fed the tilapia's own
        // fillets (an eel turns a kcal of meat into ~0.245 of eel fillet, a tilapia fillet sells for ~0.14 a kcal).
        public static bool BigOn => On && (Directives.Current.Raw.Value<bool?>("fish_big") ?? false);
        // Cash kept after AQUACULTURE_II beyond the rent and floor.
        public static int UpgradeReserve => Directives.Current.Raw.Value<int?>("fish_big_reserve") ?? 600;
        public static bool WantsUpgrade(string id) => BigOn && id == "AQUACULTURE_II" && Licensed && !NetworkUpgrade.IsAquacultureIIUnlocked() && Tanks().Count >= 2;

        const string Feed = "industrial_fish_feed", Pellet = "gutterflow_pellet", Dispenser = "livestock_feed_dispenser", Filter = "water_filter",
            Scrap = "scrap_metal", Ingot = "metal_ingot", Electronic = "common_electronic", Cell = "energy_credit", Jug = "water_jug", Converter = "current_converter",
            Eel = "eel", ElectricEel = "electric_eel", Sturgeon = "sturgeon", Surgery = "surgery_tool";
        // The aquafarmer's fertilised roe (four a visit): each hatches six juveniles, ~70 of fillets apiece once fed.
        static readonly HashSet<string> RoeIds = new HashSet<string>(StringComparer.Ordinal) { "tilapia_roe", "gutter_tilapia_roe", "graft_tilapia_roe" };
        static bool BuyRoe => Directives.Current.Raw.Value<bool?>("fish_roe") ?? false;
        // Waste % at which a filter goes in (fish suffer above 3 %, die at 8 %; graft tilapia at half that).
        const double FilterAt = 1.0;
        // Room a hatch needs: a fry clutch is 2×2, and a hatch with no room is lost (AquacultureHelper.HatchRoe).
        const int ClutchCells = 4;

        public static int Filleted, Died, Clutches, Hatched, Upgrades, Filters, FirstDay, HungryNights, Unpowered, BigBought, BigFilleted, Strips;
        public static long FilletValue, FilletSold, FilletSales, FeedSpend, KcalAdded, MeatFed, BigValue, CaviarValue;
        static readonly HashSet<int> seen = new HashSet<int>(), breeders = new HashSet<int>();

        public static void Reset()
        {
            Filleted = Died = Clutches = Hatched = Upgrades = Filters = FirstDay = HungryNights = Unpowered = BigBought = BigFilleted = Strips = 0;
            FilletValue = FilletSold = FilletSales = FeedSpend = KcalAdded = MeatFed = BigValue = CaviarValue = 0;
            seen.Clear();
            breeders.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static IEnumerable<GameItem> Loose(string id) => Owned().Where(i => i.identifier == id && Parent(i) == null && !StolenHelper.IsStolenItem(i));
        static int Energy(GameItem cell) => cell == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(cell, false);

        public static List<GameItem> Tanks() => EmporiumEntry.Instance == null ? new List<GameItem>() : Owned().Where(i => i.IsTag("RAS_TAG") && Parent(i) == null).ToList();

        static bool Stock(GameItem i) => AquacultureHelper.IsFish(i) || AquacultureHelper.IsGroup(i);
        static bool Live(GameItem i) => AquacultureHelper.IsLiveStock(i);
        static bool Female(GameItem i) => i.GetTagReadonly("FISH_SEX_TAG").GetString() == "FEMALE";
        static int Age(GameItem i) => i.GetTagReadonly("FISH_AGE_TAG").GetInt();
        static FishSpecies Species(GameItem i) => AquacultureHelper.GetSpecies(i);
        static bool Adult(GameItem i) => AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i) && AquacultureHelper.GetStage(i) >= FishStage.Adult;
        static bool Breeds(GameItem i) => Species(i)?.CanBreed ?? false;

        // Raise-only fish (eels, sturgeon): they never breed and eat only meat.
        static bool Big(GameItem i) => AquacultureHelper.IsFish(i) && !(Species(i)?.CanBreed ?? true);
        static bool HasBig(GameItem ras) => MachineRAS.GetTankGrid(ras)?.childItems.Any(i => i != null && Big(i) && !AquacultureHelper.IsDead(i)) ?? false;

        // The big-fish tank: the wet tank already holding eels or sturgeon, else (once AQUACULTURE_II is owned and a
        // second tank is wet) the wet tank with the fewest tilapia.
        static GameItem BigTank(List<GameItem> tanks)
        {
            if (!BigOn) return null;
            var wet = tanks.Where(t => WaterMl(t) > 0).ToList();
            var has = wet.FirstOrDefault(HasBig);
            if (has != null) return has;
            if (!NetworkUpgrade.IsAquacultureIIUnlocked() || wet.Count < 2) return null;
            return wet.OrderBy(t => MachineRAS.GetTankGrid(t)?.childItems.Count(i => i != null && Live(i)) ?? 0).ThenByDescending(t => t.uniqueId).First();
        }

        // Eel and sturgeon pieces' kcal a big tank holds for tonight: its dispenser's meat plus loose fish pieces.
        static readonly HashSet<string> Pieces = new HashSet<string>(StringComparer.Ordinal) { "fish_scrap", "tiny_fish_fillet", "small_fish_fillet", "fish_fillet", "large_fish_fillet", "fat_meat", "meat_scrap" };

        static readonly HashSet<string> KitchenBlades = new HashSet<string>(StringComparer.Ordinal) { "kitchen_knife", "kitchen_cleaver" };

        // The same blade every day (the lowest id), so the one held back from sale is the one used.
        static GameItem Blade() => Owned().Where(i => i.IsTag("BLADE_ITEM_TAG") && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).FirstOrDefault();

        static GameItem SlottedDispenser(GameItem ras) => MachineRAS.GetDispenserSlot(ras)?.childItem is GameItem d && d.IsTag("FEED_DISPENSER_TAG") ? d : null;

        static int Cells(GameGridInventory g) => g.inventoryShape.width * g.inventoryShape.height;
        static int Used(GameGridInventory g) => g.childItems.Where(i => i != null).Sum(PlayerRole.Cells);

        // A tank's breeders by size: each female and the male take an adult's 3×2, and a female's clutch (six
        // juveniles of 2 cells for the night they fill up) needs about six cells more on average.
        static int Females(GameGridInventory g) => (Directives.Current.Raw["fish_breeders"] as JArray)?.Count == 2 ? (int)Directives.Current.Raw["fish_breeders"][0] : Math.Max(2, (Cells(g) - 6) / 12);
        static int Males => (Directives.Current.Raw["fish_breeders"] as JArray)?.Count == 2 ? (int)Directives.Current.Raw["fish_breeders"][1] : 1;

        // Fish feed, and the cheap foods the rats eat (10+ kcal a credit; a tilapia turns a kcal into ~0.13 of fillet).
        static bool Food(GameItem i) => i.identifier == Feed || i.identifier == Pellet || Rats.Feeds.Contains(i.identifier ?? "");
        static int FeedHeld() => Owned().Where(i => Food(i) && Parent(i) == null).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt());
        static int FeedStored() => Tanks().Select(SlottedDispenser).Where(d => d != null).Sum(d => d.GetTagReadonly("STORED_KCAL_TAG").GetInt())
            + Tanks().Select(MachineRAS.GetTankGrid).Where(g => g != null).SelectMany(g => g.childItems).Where(i => i != null && IndustrialFishFeedHelper.IsFeed(i)).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt());

        static bool AnyLive() => Tanks().Any(t => MachineRAS.GetTankGrid(t)?.childItems.Any(i => i != null && Live(i)) ?? false) || Owned().Any(i => Live(i));

        public static bool Reserved(GameItem i)
        {
            if (!On || Tanks().Count == 0) return false;
            if (CurrentConverterHelper.IsCurrentConverter(i) && Parent(i) == null && Tanks().Any(t => MachineRAS.IsWorthRunning(t) && !Converts(t))) return true;
            if (i.IsTag("RAS_TAG") || Stock(i) || i.IsTag("FEED_DISPENSER_TAG") || i.identifier == Feed || i.identifier == Pellet || i.identifier == Filter) return true;
            if (Rats.Feeds.Contains(i.identifier ?? "")) return FeedStored() < FeedKcal;
            if (BigOn && i.identifier == Surgery && Parent(i) == null) return true;
            // Fish pieces are held for the big tank while it has less than three nights of meat (an eel turns them into
            // nearly twice their sale value).
            if (BigOn && Pieces.Contains(i.identifier ?? "") && Parent(i) == null && !i.IsTag("GUTTERFLOW_TAINTED_TAG"))
            {
                var big = Tanks().FirstOrDefault(HasBig);
                if (big != null && BigMeat(big) + Owned().Where(x => Pieces.Contains(x.identifier ?? "") && Parent(x) == null).Sum(x => x.GetTagReadonly("CALORIE_VALUE_TAG").GetInt()) <= 3 * NeedTonight(MachineRAS.GetTankGrid(big)) + i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt()) return true;
            }
            if (i.IsTag("BLADE_ITEM_TAG") && (ReferenceEquals(i, Blade()) || KitchenBlades.Contains(i.identifier ?? ""))) return true;
            return false;
        }

        public static bool IsInput(string id) => On && id != null && (id == Feed || id == Pellet || id == Dispenser || id == Filter || id == Scrap || id == Ingot || id == Electronic || id == Cell || id == Jug || id == Converter || KitchenBlades.Contains(id) || Rats.Feeds.Contains(id)
            || (BigOn && (id == Eel || id == ElectricEel || id == Sturgeon || id == Surgery)) || (BuyRoe && RoeIds.Contains(id)));

        // Capital waits for the business permit: without the Board, cash spent on tanks before the permit ends runs in an
        // arrest for operating without a licence.
        static readonly HashSet<string> Capital = new HashSet<string>(StringComparer.Ordinal) { Converter, Dispenser, Scrap, Ingot, Eel, ElectricEel, Sturgeon, Surgery, "tilapia_roe", "gutter_tilapia_roe", "graft_tilapia_roe" };

        static bool Licensed => PlayerStore.Instance?.hasBusinessPermit ?? true;

        public static int Wants(string id)
        {
            if (!On || id == null) return 0;
            var tanks = Tanks();
            if (tanks.Count == 0) return 0;
            if (!Licensed && Capital.Contains(id)) return 0;
            if (RoeIds.Contains(id))
            {
                if (!BuyRoe) return 0;
                // Room for the clutch to hatch and grow (a fry group, then six 2-cell juveniles) and a week of feed for it.
                var big = BigTank(tanks);
                int room = tanks.Where(t => t != big && WaterMl(t) > 0).Select(MachineRAS.GetTankGrid).Where(g => g != null).Sum(g => Math.Max(0, Cells(g) - Used(g) - 12) / 12);
                int pending = Owned().Count(i => i.IsTag("FISH_ROE_TAG") && i.IsTag("FISH_FERTILIZED_TAG"));
                return FeedStored() + FeedHeld() >= 4000 ? Math.Max(0, Math.Min(room, 4) - pending) : 0;
            }
            if (Rats.Feeds.Contains(id))
            {
                if (!AnyLive()) return 0;
                int short_ = FeedKcal / 2 - FeedStored() - FeedHeld();
                return short_ <= 0 ? 0 : (short_ + 399) / 400;
            }
            switch (id)
            {
                case Feed:
                    if (!AnyLive()) return 0;
                    int gap = FeedKcal - FeedStored() - FeedHeld();
                    return gap <= 0 ? 0 : (gap + 4999) / 5000;
                // Pellets only for gutter tilapia (pellet-proof); any other fish is trash grade past a quarter of pellets.
                case Pellet:
                    return tanks.Any(t => MachineRAS.GetTankGrid(t)?.childItems.Any(i => i != null && Live(i) && Species(i) == FishSpecies.GutterTilapia) ?? false) && FeedHeld() < FeedKcal ? 1 : 0;
                case Dispenser:
                    return tanks.Count(t => SlottedDispenser(t) == null) - Owned().Count(i => i.IsTag("FEED_DISPENSER_TAG") && Parent(i) == null) > 0 ? 1 : 0;
                // An eel fouls 2 ml per 100 g a night: a big tank uses up a 400-durability filter in three or four nights.
                case Filter:
                    return Math.Max(0, (tanks.Any(HasBig) ? 4 : 1) - GoodFilters().Count());
                // Two spare charged cells per running tank: a cell seller comes rarely, and three unpowered nights kill the stock.
                case Cell:
                    return Math.Max(0, 2 * tanks.Count(MachineRAS.IsWorthRunning) - Loose(Cell).Count(c => Energy(c) >= 30));
                case Scrap:
                    return Upgrade && tanks.Any(t => MachineRAS.CanUpgradeWidth(t) && WaterMl(t) > 0) && !Loose(Scrap).Any() ? 1 : 0;
                case Ingot:
                    return Upgrade && tanks.Any(t => MachineRAS.CanUpgradeHeight(t) && WaterMl(t) > 0) && !Loose(Ingot).Any() ? 1 : 0;
                case Electronic:
                    return Upgrade && tanks.Any(t => MachineRAS.CanUpgradePowerEfficiency(t) && WaterMl(t) > 0 && (MachineryHelper.GetMachinePowerUsage(t) > 1 || MachineRAS.CanUpgradeWidth(t) || MachineRAS.CanUpgradeHeight(t))) && !Loose(Electronic).Any() ? 1 : 0;
                // A big tank wants all 12 L: the waste share is what hurts, and more water dilutes it.
                case Jug:
                    return tanks.Any(t => WaterMl(t) < (HasBig(t) ? 11000 : Math.Max(2000, Need(t)))) && !Owned().Any(Pourable) ? 1 : 0;
                // Dr. Jackson's converter runs a tank on the shop's cash (1.5 a unit) once the kit's cells are spent.
                // One per running tank that neither converts nor holds an electric eel to charge it.
                case Converter:
                    return Math.Max(0, tanks.Count(t => MachineRAS.IsWorthRunning(t) && !Converts(t) && !SelfPowered(t)) - Loose(Converter).Count());
                // A spare: a knife can be lost (a fight, a burglary), and without one the tank's carcasses pile up.
                case "kitchen_knife":
                case "kitchen_cleaver":
                    return Owned().Count(i => i.IsTag("BLADE_ITEM_TAG") && !StolenHelper.IsStolenItem(i)) < 2 ? 1 : 0;
                case Eel:
                case ElectricEel:
                case Sturgeon:
                    return BigWants(id, tanks);
                case Surgery:
                    return BigOn && Owned().Any(i => i.identifier == Sturgeon) && !Owned().Any(i => i.identifier == Surgery) ? 1 : 0;
            }
            return 0;
        }

        // The aquafarmer's big fish (AquacultureII: three a week, eel or electric eel juveniles at 100, sturgeon adolescents
        // at 500): one electric eel per big tank (its 1–3 power a night runs the tank), then eels while the tank has the
        // room and water of an adult eel, one sturgeon while cash allows.
        static int BigWants(string id, List<GameItem> tanks)
        {
            var big = BigTank(tanks);
            if (big == null) return 0;
            var g = MachineRAS.GetTankGrid(big);
            var owned = Owned().Where(i => Big(i) && !AquacultureHelper.IsDead(i)).ToList();
            int free = Cells(g) - Used(g), water = WaterMl(big) - owned.Sum(i => Species(i).stageWaterNeedMl[4]);
            if (id == ElectricEel) return owned.Any(i => i.identifier == ElectricEel) || free < 10 || water < 1200 ? 0 : 1;
            // Eels eat what the tilapia tanks fillet (~500 kcal a fish); `fish_eels` (default 2) is what that meat feeds.
            if (id == Eel) return free >= 10 && water >= 1200 && owned.Count(i => i.identifier == Eel) < (Directives.Current.Raw.Value<int?>("fish_eels") ?? 2) ? 1 : 0;
            if (id == Sturgeon) return owned.Count(i => i.identifier == Sturgeon) < 1 && free >= 14 && water >= 3000 && PlayerStore.Instance.playerCash > (PlayerDay.Current?.RentDue ?? 0) + 1500 ? 1 : 0;
            return 0;
        }

        static int WaterMl(GameItem ras) => AquacultureHelper.GetWaterMl(MachineRAS.GetWaterSlot(ras)?.childItem);
        static int Need(GameItem ras) => AquacultureHelper.GetTotalWaterNeedMl(MachineRAS.GetTankGrid(ras));

        // Water a player would pour into a fish tank: any owned, unreserved container of water that is not for sale as premium.
        static bool Pourable(GameItem i) => i.IsTag("LIQUID_CONTAINER_TAG") && !i.IsTag("RAS_WATER_TANK_TAG") && Parent(i) == null && !i.IsTag("CANTEEN_TAG")
            && (i.identifier == Jug || i.identifier == "water_pitcher" || (i.identifier ?? "").Contains("bottled_water")) && i.identifier != "bottled_water_premium"
            && WaterHelper.GetCurrentCapacityML(i) > 0 && !Production.Reserved(i) && !Directives.Current.HoldUids.Contains(i.uniqueId);

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            var tanks = Tanks();
            if (tanks.Count == 0) return;
            if (FirstDay == 0) FirstDay = ep.Day;
            var stock = EmporiumEntry.Instance.invElement;
            // Water first: a tank is worth stocking only once it holds water. Half a tank (6 L, one jug) runs a tank's
            // stock, so every tank gets half before any is topped up.
            foreach (var ras in tanks)
                if (!ReferenceEquals(ras.parentInventory, stock)) Chore.Accept(stock, ras, -1);
            foreach (int target in new[] { 6000, 12000 })
                foreach (var ras in tanks.OrderByDescending(HasBig))
                    try { Fill(ras, target); }
                    catch (Exception e) { ep.ProductionError(e); }
            foreach (var ras in tanks)
                try { Power(ras, stock); }
                catch (Exception e) { ep.ProductionError(e); }
            // Stray stock (the kit's, a purchase) goes into the wet tank with the most free room.
            var bigTank = BigTank(tanks);
            Place(tanks, bigTank);
            // Tilapia found in the big tank move to a tilapia tank (or are filleted when none has room).
            if (bigTank != null)
                foreach (var f in MachineRAS.GetTankGrid(bigTank).childItems.Where(i => i != null && Live(i) && !Big(i)).ToList())
                {
                    var to = Ready(tanks).Where(t => t != bigTank).Select(MachineRAS.GetTankGrid).FirstOrDefault(g => g != null && Cells(g) - Used(g) >= PlayerRole.Cells(f) + 4);
                    if (to == null || Chore.Accept(to, f, -1) != 0) { if (AquacultureHelper.IsFish(f)) Fillet(ep, f, stock); else Chore.Act("discard", f, () => f.Destroy()); }
                }
            Pair(Ready(tanks).Where(t => t != bigTank).ToList());
            foreach (var ras in tanks.Where(t => t != bigTank))
                try { Tend(ep, ras, stock); }
                catch (Exception e) { ep.ProductionError(e); }
            // The big tank last: it eats what the tilapia tanks filleted this morning.
            if (bigTank != null)
                try { TendBig(ep, bigTank, stock); }
                catch (Exception e) { ep.ProductionError(e); }
        }

        // Stock outside a tank dies at night: big fish into the big tank, tilapia, roe and fry into the tilapia tank with the
        // most fish that still has a clutch's room. Also run after every purchase (Stow): a fish bought by day or delivered
        // with an upgrade at night must be in water before the night.
        static void Place(List<GameItem> tanks, GameItem bigTank)
        {
            foreach (var f in Owned().Where(i => Big(i) && !AquacultureHelper.IsDead(i) && !AquacultureHelper.IsInTank(i.parentInventory)).ToList())
            {
                if (bigTank != null) { if (Chore.Accept(MachineRAS.GetTankGrid(bigTank), f, -1) == 0) BigBought++; }
            }
            foreach (var f in Owned().Where(i => Live(i) && !Big(i) && !AquacultureHelper.IsInTank(i.parentInventory)).OrderByDescending(Adult).ThenBy(i => i.uniqueId).ToList())
            {
                // The tank with the most fish that still has a clutch's room: a pair split across tanks never breeds.
                var into = Ready(tanks).Where(t => t != bigTank).Select(MachineRAS.GetTankGrid).Where(g => g != null && Cells(g) - Used(g) >= PlayerRole.Cells(f) + 12)
                    .OrderByDescending(g => g.childItems.Count(i => i != null && Live(i))).ThenByDescending(g => Cells(g) - Used(g)).FirstOrDefault()
                    ?? Ready(tanks).Where(t => t != bigTank).Select(MachineRAS.GetTankGrid).Where(g => g != null).OrderByDescending(g => Cells(g) - Used(g)).FirstOrDefault();
                if (into != null) Chore.Accept(into, f, -1);
            }
        }

        public static void Stow()
        {
            if (!On) return;
            var tanks = Tanks();
            if (tanks.Count == 0 || !Owned().Any(i => Live(i) && !AquacultureHelper.IsInTank(i.parentInventory))) return;
            Place(tanks, BigTank(tanks));
        }

        // A tank worth stocking: water, and power for two weeks (a converter, or a cell beside the one in use).
        static IEnumerable<GameItem> Ready(List<GameItem> tanks) => tanks.Where(t => WaterMl(t) > 0 && (Converts(t) || MachineRAS.HasLiveStock(t)
            || Energy(MachineHelper.GetBatterySlot(t)?.childItem) >= 14 * MachineryHelper.GetMachinePowerUsage(t)));

        // A wet tank without a breeder of a sex gets the youngest spare one from a tank that has two.
        static void Pair(List<GameItem> wet)
        {
            foreach (var ras in wet)
            {
                var grid = MachineRAS.GetTankGrid(ras);
                if (grid == null) continue;
                foreach (bool female in new[] { true, false })
                {
                    if (grid.childItems.Any(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i) && Breeds(i) && Female(i) == female)) continue;
                    var donor = wet.Where(t => t != ras).Select(MachineRAS.GetTankGrid).Where(g => g != null)
                        .SelectMany(g => g.childItems.Where(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i) && Breeds(i) && Female(i) == female && !breeders.Contains(i.uniqueId)))
                        .Where(i => i.parentInventory.childItems.Count(o => o != null && AquacultureHelper.IsFish(o) && !AquacultureHelper.IsDead(o) && Breeds(o) && Female(o) == female) > 1)
                        .OrderBy(Age).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (donor != null) Chore.Accept(grid, donor, -1);
                }
            }
        }

        static void Fill(GameItem ras, int target)
        {
            var tank = MachineRAS.GetWaterSlot(ras)?.childItem;
            if (tank == null) return;
            foreach (var src in Owned().Where(Pourable).OrderBy(i => PlayerRole.Est(i) / (double)Math.Max(1, WaterHelper.GetCurrentCapacityML(i))).ThenBy(i => i.uniqueId).ToList())
            {
                if (WaterMl(ras) >= target || WaterHelper.GetFreeCapacity(tank) <= 0) break;
                Chore.Act("pour", tank, () => WaterHelper.TransferLiquid(src, tank));
            }
        }

        // Electric eels charge their tank's battery before it draws (AquacultureHelper.GeneratePower, early pass).
        static bool SelfPowered(GameItem ras) => MachineRAS.GetTankGrid(ras) is GameGridInventory g && AquacultureHelper.GetTankPowerOutput(g) >= MachineryHelper.GetMachinePowerUsage(ras);

        static bool Converts(GameItem ras) => MachineHelper.GetBatterySlot(ras)?.childItem is GameItem b && CurrentConverterHelper.IsCurrentConverter(b);

        static void Power(GameItem ras, GameGridInventory stock)
        {
            int usage = MachineryHelper.GetMachinePowerUsage(ras);
            var bat = MachineHelper.GetBatterySlot(ras);
            if (bat == null || Converts(ras)) return;
            var conv = Loose(Converter).FirstOrDefault();
            if (conv != null && MachineRAS.IsWorthRunning(ras) && (bat.childItem == null || Chore.Accept(stock, bat.childItem, -1) == 0))
            {
                Chore.Accept(bat, conv, -1);
                return;
            }
            if (bat == null || Energy(bat.childItem) >= 2 * usage) return;
            // A tank whose electric eels make its draw recharges any cell put in it: trade this tank's low cell for its full one.
            var charger = Tanks().FirstOrDefault(t => t != ras && SelfPowered(t) && Energy(MachineHelper.GetBatterySlot(t)?.childItem) > Energy(bat.childItem) + usage);
            if (charger != null)
            {
                var cbat = MachineHelper.GetBatterySlot(charger);
                var full = cbat.childItem;
                var low = bat.childItem;
                if (low == null || Chore.Accept(stock, low, -1) == 0)
                    if (Chore.Accept(stock, full, -1) == 0)
                    {
                        Chore.Accept(bat, full, -1);
                        if (low != null) Chore.Accept(cbat, low, -1);
                        return;
                    }
            }
            // A loose cell, or the cell of a tank with nothing alive to run.
            var cell = Loose(Cell).Concat(Tanks().Where(t => t != ras && !MachineRAS.IsWorthRunning(t)).Select(t => MachineHelper.GetBatterySlot(t)?.childItem).Where(c => c != null))
                .OrderByDescending(Energy).FirstOrDefault();
            if (cell == null || Energy(cell) <= Energy(bat.childItem)) return;
            var old = bat.childItem;
            if (old == null || Chore.Accept(stock, old, -1) == 0) Chore.Accept(bat, cell, -1);
        }

        static void Tend(IExecutorHost ep, GameItem ras, GameGridInventory stock)
        {
            var grid = MachineRAS.GetTankGrid(ras);
            if (grid == null) return;
            if (!MachineHelper.CanPower(ras) && MachineRAS.HasLiveStock(ras)) Unpowered++;
            foreach (var r in grid.childItems.Where(i => i != null && AquacultureHelper.IsGroup(i) && seen.Add(i.uniqueId)))
            {
                if (IsRoe(r)) Clutches++;
                else Hatched++;
            }
            // Carcasses foul the water and only feed the others: fillet them.
            foreach (var c in grid.childItems.Where(i => i != null && AquacultureHelper.IsCarcass(i)).ToList())
            {
                Died++;
                if (Environment.GetEnvironmentVariable("FENCE_FISH_DEBUG") != null)
                    Console.Error.WriteLine($"fish-dead d{ep.Day} {c.identifier} {(Female(c) ? "F" : "M")} age={Age(c)} hunger={AquacultureHelper.GetHunger(c):0} starving={c.IsTag("FISH_IS_STARVING_TAG")} bat={Energy(MachineHelper.GetBatterySlot(ras)?.childItem)} water={WaterMl(ras)}/{Need(ras)} breeder={breeders.Contains(c.uniqueId)}");
                Fillet(ep, c, stock);
            }
            var live = grid.childItems.Where(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i)).ToList();
            if (Environment.GetEnvironmentVariable("FENCE_FISH_DEBUG") != null)
            {
                Console.Error.WriteLine($"fish d{ep.Day} ras={ras.uniqueId} cells={Used(grid)}/{Cells(grid)} water={WaterMl(ras)}/{Need(ras)} waste={AquacultureHelper.GetWastePercent(MachineRAS.GetWaterSlot(ras)?.childItem):0.00}% bat={Energy(MachineHelper.GetBatterySlot(ras)?.childItem)} use={MachineryHelper.GetMachinePowerUsage(ras)} disp={SlottedDispenser(ras)?.GetTagReadonly("STORED_KCAL_TAG").GetInt()} feedheld={FeedHeld()}");
                foreach (var f in grid.childItems.Where(i => i != null && Stock(i)))
                    Console.Error.WriteLine($"  {f.identifier} {(AquacultureHelper.IsFish(f) ? (Female(f) ? "F" : "M") : "grp")} age={Age(f)} g={AquacultureHelper.GetWeightGrams(f)} meat={AquacultureHelper.GetMeatKcal(f)} hunger={AquacultureHelper.GetHunger(f):0} hp={AquacultureHelper.GetHealth(f):0} speed={AquacultureHelper.GetGrowthSpeed(f):0.00} breeder={breeders.Contains(f.uniqueId)} cells={PlayerRole.Cells(f)}");
            }
            breeders.RemoveWhere(u => !live.Any(f => f.uniqueId == u));
            int females = Females(grid), males = Males;
            KeepBreeders(live, true, females);
            KeepBreeders(live, false, males);
            // Heirs: the youngest healthy growers of a sex are spared while that sex is short of breeders (one per missing
            // breeder), plus one while a breeder is within ten nights of old age (a juvenile needs five to come of age).
            var spare = new HashSet<int>();
            foreach (bool female in new[] { true, false })
            {
                int want = female ? females : males, have = live.Count(f => breeders.Contains(f.uniqueId) && Female(f) == female);
                int heirs = Math.Max(0, want - have) + (live.Any(f => breeders.Contains(f.uniqueId) && Female(f) == female && Age(f) + 10 >= Species(f).oldAgeDays) ? 1 : 0);
                foreach (var f in live.Where(f => Female(f) == female && Breeds(f) && !breeders.Contains(f.uniqueId) && Age(f) + 10 < Species(f).oldAgeDays).OrderBy(Age).ThenBy(f => f.uniqueId).Take(heirs))
                    spare.Add(f.uniqueId);
            }
            bool Grower(GameItem f) => !breeders.Contains(f.uniqueId) && !spare.Contains(f.uniqueId);
            // Old growers: past old age half die each night. Breeders are left to it (the morning fillets the carcass).
            foreach (var f in live.Where(f => Age(f) + 1 >= Species(f).oldAgeDays && Grower(f)).ToList())
                Fillet(ep, f, stock);
            live = grid.childItems.Where(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i)).ToList();
            foreach (var f in live.Where(f => Grower(f) && AquacultureHelper.GetMeatKcal(f) >= HarvestMeat).ToList())
                Fillet(ep, f, stock);
            // Room and water for the next clutch: the growers with the most meat go first.
            int clutches = live.Count(f => breeders.Contains(f.uniqueId) && Female(f) && Breeds(f));
            foreach (var f in grid.childItems.Where(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i) && Grower(i)).OrderByDescending(AquacultureHelper.GetMeatKcal).ThenBy(i => i.uniqueId).ToList())
            {
                bool cramped = clutches > 0 && Cells(grid) - Used(grid) < ClutchCells;
                bool dry = WaterMl(ras) > 0 && Need(ras) > WaterMl(ras);
                if (!cramped && !dry) break;
                Fillet(ep, f, stock);
            }
            Feeding(ep, ras, grid, stock);
            // Short of tonight's food: hungry fish eat their tank mates and starve in two nights, so growers go first.
            foreach (var f in grid.childItems.Where(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i) && Grower(i)).OrderByDescending(AquacultureHelper.GetMeatKcal).ThenBy(i => i.uniqueId).ToList())
            {
                if (FoodIn(ras, grid) >= NeedTonight(grid)) break;
                Fillet(ep, f, stock);
            }
            Filtering(ras, stock);
            if (Upgrade && WaterMl(ras) > 0) Improve(ras);
        }

        static bool IsRoe(GameItem g) => g.IsTag("FISH_ROE_TAG");

        // Eels are filleted at elder (day 18, ~3 000 meat, six eel fillets); electric eels are kept for their power until
        // near old age; a sturgeon's eggs are stripped with the surgery tool whenever her load is full (five jars:
        // caviar, the most valuable livestock output), and she is kept to old age.
        static void TendBig(IExecutorHost ep, GameItem ras, GameGridInventory stock)
        {
            var grid = MachineRAS.GetTankGrid(ras);
            if (grid == null) return;
            if (!MachineHelper.CanPower(ras) && MachineRAS.HasLiveStock(ras)) Unpowered++;
            foreach (var c in grid.childItems.Where(i => i != null && AquacultureHelper.IsCarcass(i)).ToList()) { Died++; Fillet(ep, c, stock); }
            if (Environment.GetEnvironmentVariable("FENCE_FISH_DEBUG") != null)
            {
                Console.Error.WriteLine($"fish-big d{ep.Day} ras={ras.uniqueId} cells={Used(grid)}/{Cells(grid)} water={WaterMl(ras)}/{Need(ras)} waste={AquacultureHelper.GetWastePercent(MachineRAS.GetWaterSlot(ras)?.childItem):0.00}% filters={Loose(Filter).Count()} batid={MachineHelper.GetBatterySlot(ras)?.childItem?.identifier} slotted={string.Join(",", Enumerable.Range(0, 4).Select(k => (MachineRAS.GetFilterGrid(ras)?.GetElement(k % 4, k / 4) as GameSlotInventory)?.childItem is GameItem fi ? DurabilityHelper.GetRealDurability(fi).ToString() : "-"))} bat={Energy(MachineHelper.GetBatterySlot(ras)?.childItem)} use={MachineryHelper.GetMachinePowerUsage(ras)} disp={SlottedDispenser(ras)?.GetTagReadonly("STORED_KCAL_TAG").GetInt()} meat={(SlottedDispenser(ras) is GameItem dd ? MachineFeedDispenser.GetMeatKcal(dd) : -1)} grid={(SlottedDispenser(ras) is GameItem d2 ? MachineFeedDispenser.GetStorageGrid(d2)?.childItems.Count(x => x != null) : -1)} loose={grid.childItems.Count(x => x != null && Pieces.Contains(x.identifier ?? ""))}");
                foreach (var f in grid.childItems.Where(i => i != null && Stock(i)))
                    Console.Error.WriteLine($"  {f.identifier} age={Age(f)} g={AquacultureHelper.GetWeightGrams(f)} meat={AquacultureHelper.GetMeatKcal(f)} hunger={AquacultureHelper.GetHunger(f):0} hp={AquacultureHelper.GetHealth(f):0} eggs={AquacultureHelper.GetEggGrams(f):0}");
            }
            foreach (var f in grid.childItems.Where(i => i != null && Big(i) && !AquacultureHelper.IsDead(i)).ToList())
            {
                var sp = Species(f);
                if (sp.LaysEggs && AquacultureHelper.IsGravid(f) && AquacultureHelper.CanStripEggs(f) && Owned().FirstOrDefault(i => i.identifier == Surgery) is GameItem tool)
                {
                    var before = new HashSet<int>(grid.childItems.Where(i => i != null).Select(i => i.uniqueId));
                    Chore.Act("strip_eggs", f, () => AquacultureHelper.StripEggs(f));
                    Strips++;
                    foreach (var jar in grid.childItems.Where(i => i != null && !before.Contains(i.uniqueId) && !Stock(i)).ToList())
                    {
                        Chore.Act("own", jar, () => GeneralHelper.SetItemOwned(jar, true));
                        CaviarValue += PlayerRole.Est(jar);
                        if (Chore.Accept(stock, jar, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, jar, -1);
                    }
                }
                bool done = sp == FishSpecies.Eel ? AquacultureHelper.GetStage(f) >= FishStage.Elder
                    : Age(f) + 2 >= sp.oldAgeDays;
                if (done) { BigFilleted++; Fillet(ep, f, stock, big: true); }
            }
            FeedBig(ep, ras, grid, stock);
            Filtering(ras, stock);
            if (Upgrade && WaterMl(ras) > 0) Improve(ras);
        }

        static int BigMeat(GameItem ras) => (SlottedDispenser(ras) is GameItem d ? MachineFeedDispenser.GetMeatKcal(d) : 0)
            + (MachineRAS.GetTankGrid(ras)?.childItems.Where(i => i != null && Pieces.Contains(i.identifier ?? "")).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt()) ?? 0);

        // Tonight's meat for the big fish: the cheapest fish pieces per kcal first (scrap, then the small fillets), into the
        // dispenser when one is slotted (fish draw meat to 85 %), else loose in the tank (a hungry fish swallows one whole).
        static void FeedBig(IExecutorHost ep, GameItem ras, GameGridInventory grid, GameGridInventory stock)
        {
            int need = NeedTonight(grid) + 200;
            var disp = SlottedDispenser(ras);
            if (disp == null)
            {
                var spare = Owned().FirstOrDefault(i => i.IsTag("FEED_DISPENSER_TAG") && Parent(i) == null);
                if (spare != null && Chore.Accept(MachineRAS.GetDispenserSlot(ras), spare, -1) == 0) disp = spare;
            }
            int have = (disp == null ? 0 : MachineFeedDispenser.GetMeatKcal(disp)) + grid.childItems.Where(i => i != null && Pieces.Contains(i.identifier ?? "")).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt());
            var into = disp != null ? MachineFeedDispenser.GetStorageGrid(disp) : grid;
            int added = 0;
            foreach (var p in Owned().Where(i => Pieces.Contains(i.identifier ?? "") && Parent(i) == null && !StolenHelper.IsStolenItem(i) && !Directives.Current.HoldUids.Contains(i.uniqueId) && !i.IsTag("GUTTERFLOW_TAINTED_TAG"))
                .OrderBy(i => PlayerRole.Est(i) / (double)Math.Max(1, i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt())).ThenBy(i => i.uniqueId).ToList())
            {
                if (have + added >= need * (disp != null ? 3 : 1)) break;
                int k = p.GetTagReadonly("CALORIE_VALUE_TAG").GetInt();
                if (Chore.Accept(into, p, -1) == 0) { added += k; MeatFed += k; }
            }
            if (disp != null && added > 0)
                ep.MachineButton(ras, LocHelper.GetLocalizedMechanic("mech_ras_btn_blend"))?.Invoke();
            if (have + added < need) HungryNights++;
        }

        static void KeepBreeders(List<GameItem> live, bool female, int want)
        {
            int have = live.Count(f => breeders.Contains(f.uniqueId) && Female(f) == female);
            foreach (var f in live.Where(f => Female(f) == female && Breeds(f) && !breeders.Contains(f.uniqueId) && Adult(f) && Age(f) + 7 < Species(f).oldAgeDays && AquacultureHelper.GetHealth(f) > 0).OrderBy(Age).ThenBy(f => f.uniqueId))
            {
                if (have >= want) break;
                breeders.Add(f.uniqueId);
                have++;
            }
        }

        // A blade's two strokes (AquacultureHelper.OnBladeUsed): a live fish is killed, a carcass filleted; the pieces go to stock.
        static void Fillet(IExecutorHost ep, GameItem fish, GameGridInventory stock, bool big = false)
        {
            if (Blade() == null) { if (Environment.GetEnvironmentVariable("FENCE_FISH_DEBUG") != null) Console.Error.WriteLine($"fish-noblade d{ep.Day}"); return; }
            var tankGrid = fish.parentInventory as GameGridInventory;
            var pieces = new List<GameItem>();
            Chore.Act("fillet", fish, () =>
            {
                if (!AquacultureHelper.IsDead(fish)) AquacultureHelper.OnFishDead(fish);
                // OnFillet's own body: its guard reads a headless item (no UI handler) as already destroyed.
                var made = AquacultureHelper.CreateFilletPieces(fish);
                if (made == null) return;
                fish.Destroy();
                pieces.AddRange(made.Where(p => p != null));
            });
            if (Environment.GetEnvironmentVariable("FENCE_FISH_DEBUG") != null)
                Console.Error.WriteLine($"fish-fillet d{ep.Day} {fish.identifier} age={Age(fish)} meat={AquacultureHelper.GetMeatKcal(fish)} pieces={string.Join(",", pieces.Select(p => p.identifier))}");
            if (pieces.Count == 0) return;
            Filleted++;
            breeders.Remove(fish.uniqueId);
            var ras = tankGrid?.GetParentItem();
            var disp = ras == null ? null : SlottedDispenser(ras);
            foreach (var p in pieces)
            {
                Chore.Act("own", p, () => GeneralHelper.SetItemOwned(p, true));
                // Fish scrap feeds the tank (meat speeds tilapia growth a quarter); the fillets are stock.
                if (p.identifier == "fish_scrap" && disp != null && Chore.Accept(MachineFeedDispenser.GetStorageGrid(disp), p, -1) == 0) continue;
                FilletValue += PlayerRole.Est(p);
                if (big) BigValue += PlayerRole.Est(p);
                if (Chore.Accept(stock, p, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, p, -1);
            }
            ep.Human("production", null, null, new JObject { ["action"] = "fillet", ["pieces"] = pieces.Count }, "player:fish");
        }

        // Tonight's drain (the tooltip's figure) plus the refill to 85 % of a fish below 75 %.
        static int NeedTonight(GameGridInventory grid) => grid.childItems.Where(i => i != null && Live(i) && !i.IsTag("FISH_ROE_TAG"))
            .Sum(i => (int)Math.Ceiling(AquacultureHelper.GetExpectedDrainTonight(i, Species(i))) + 20);

        static int FoodIn(GameItem ras, GameGridInventory grid) => (SlottedDispenser(ras)?.GetTagReadonly("STORED_KCAL_TAG").GetInt() ?? 0)
            + grid.childItems.Where(i => i != null && (IndustrialFishFeedHelper.IsFeed(i) || GutterflowPelletHelper.IsPellet(i))).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt());

        static void Feeding(IExecutorHost ep, GameItem ras, GameGridInventory grid, GameGridInventory stock)
        {
            bool gutterOnly = grid.childItems.Where(i => i != null && Live(i)).All(i => Species(i) == FishSpecies.GutterTilapia);
            int need = NeedTonight(grid);
            var disp = SlottedDispenser(ras);
            if (disp == null)
            {
                var spare = Owned().FirstOrDefault(i => i.IsTag("FEED_DISPENSER_TAG") && Parent(i) == null);
                if (spare != null && Chore.Accept(MachineRAS.GetDispenserSlot(ras), spare, -1) == 0) disp = spare;
            }
            if (disp != null)
            {
                // Bags lying in the tank from before the dispenser go into it.
                foreach (var bag in grid.childItems.Where(i => i != null && (IndustrialFishFeedHelper.IsFeed(i) || GutterflowPelletHelper.IsPellet(i))).ToList())
                    Chore.Accept(stock, bag, -1);
                var store = MachineFeedDispenser.GetStorageGrid(disp);
                int stored = disp.GetTagReadonly("STORED_KCAL_TAG").GetInt(), added = 0;
                if (store == null || stored >= FeedKcal) { if (stored < need) HungryNights++; return; }
                foreach (var f in Owned().Where(i => Food(i) && (i.identifier != Pellet || gutterOnly) && Parent(i) == null && !StolenHelper.IsStolenItem(i) && !Directives.Current.HoldUids.Contains(i.uniqueId)).OrderBy(i => i.identifier == Feed ? 0 : 1).ThenBy(i => i.uniqueId).ToList())
                {
                    if (stored + added >= FeedKcal) break;
                    int k = f.GetTagReadonly("CALORIE_VALUE_TAG").GetInt();
                    if (Chore.Accept(store, f, -1) == 0) added += k;
                }
                if (added > 0 || store.childItems.Any(i => i != null))
                {
                    var press = ep.MachineButton(ras, LocHelper.GetLocalizedMechanic("mech_ras_btn_blend"));
                    if (press != null)
                    {
                        press();
                        KcalAdded += Math.Max(0, disp.GetTagReadonly("STORED_KCAL_TAG").GetInt() - stored);
                    }
                }
                if (disp.GetTagReadonly("STORED_KCAL_TAG").GetInt() < need) HungryNights++;
                return;
            }
            // No dispenser: fish draw straight from a feed bag in the tank (one is enough; it splits at 1 000 kcal).
            int loose = grid.childItems.Where(i => i != null && (IndustrialFishFeedHelper.IsFeed(i) || GutterflowPelletHelper.IsPellet(i))).Sum(i => i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt());
            if (loose >= need) return;
            var next = Owned().Where(i => (i.identifier == Feed || (i.identifier == Pellet && gutterOnly)) && Parent(i) == null && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).FirstOrDefault();
            if (next != null && Chore.Accept(grid, next, -1) == 0) { KcalAdded += next.GetTagReadonly("CALORIE_VALUE_TAG").GetInt(); return; }
            // No bag: other food loose in the tank; a hungry fish swallows one item whole a night (AquacultureHelper.TryFeed).
            int hungry = grid.childItems.Count(i => i != null && Live(i) && !i.IsTag("FISH_ROE_TAG")), put = grid.childItems.Count(i => i != null && Rats.Feeds.Contains(i.identifier ?? ""));
            foreach (var f in Owned().Where(i => Rats.Feeds.Contains(i.identifier ?? "") && Parent(i) == null && !StolenHelper.IsStolenItem(i) && !Directives.Current.HoldUids.Contains(i.uniqueId)).OrderBy(i => i.uniqueId).ToList())
            {
                if (put >= hungry) break;
                if (Chore.Accept(grid, f, -1) == 0) { put++; KcalAdded += f.GetTagReadonly("CALORIE_VALUE_TAG").GetInt(); }
            }
            if (put == 0) HungryNights++;
        }

        // Spent filters (durability 0) come back to stock and are worth nothing to a tank.
        static IEnumerable<GameItem> GoodFilters() => Loose(Filter).Where(i => DurabilityHelper.GetRealDurability(i) > 0);

        static void Filtering(GameItem ras, GameGridInventory stock)
        {
            var water = MachineRAS.GetWaterSlot(ras)?.childItem;
            var filters = MachineRAS.GetFilterGrid(ras);
            if (water == null || filters == null) return;
            var slots = Enumerable.Range(0, MachineRAS.FILTER_SLOT_COUNT).Select(i => filters.GetElement(i % 4, i / 4) as GameSlotInventory).Where(s => s != null).ToList();
            foreach (var s in slots.Where(s => s.childItem != null && DurabilityHelper.GetRealDurability(s.childItem) <= 0).ToList())
            {
                var spent = s.childItem;
                if (Chore.Accept(stock, spent, -1) != 0) Chore.Act("discard", spent, () => spent.Destroy());
            }
            // A big tank keeps two live filters (eels foul ~2 % of 6 L a night, more than one filter's last night can hold).
            int want = HasBig(ras) ? 2 : AquacultureHelper.GetWastePercent(water) >= FilterAt ? 1 : 0;
            while (slots.Count(s => s.childItem != null && DurabilityHelper.GetRealDurability(s.childItem) > 0) < want)
            {
                var f = GoodFilters().FirstOrDefault();
                var free = slots.FirstOrDefault(s => s.childItem == null);
                if (f == null || free == null || Chore.Accept(free, f, -1) != 0) break;
                Filters++;
            }
        }

        // MachineRAS.UpdatePowerUsage before its floor of 1: electronics bought ahead of a size upgrade still count.
        static int RawDraw(GameItem ras) => MachineRAS.POWER_USE + 2 * ras.GetTagReadonly("RAS_WIDTH_UPGRADE_COUNT_TAG").GetInt() + 3 * ras.GetTagReadonly("RAS_HEIGHT_UPGRADE_COUNT_TAG").GetInt() - 2 * ras.GetTagReadonly("RAS_POWER_UPGRADE_COUNT_TAG").GetInt();

        // Electronics first (each takes 2 off the nightly draw, down to 1), then width (scrap, +2 draw) and height (ingot, +3).
        static void Improve(GameItem ras)
        {
            var e = Loose(Electronic).FirstOrDefault();
            if (e != null && MachineRAS.CanUpgradePowerEfficiency(ras) && (MachineryHelper.GetMachinePowerUsage(ras) > 1 || MachineRAS.CanUpgradeWidth(ras) || MachineRAS.CanUpgradeHeight(ras))) { Chore.Act("ras_upgrade", ras, () => MachineRAS.UpgradePowerEfficiency(ras, e)); Upgrades++; }
            // A size upgrade waits until the draw stays within 3 a night after it.
            var s = Loose(Scrap).FirstOrDefault();
            if (s != null && MachineRAS.CanUpgradeWidth(ras) && RawDraw(ras) + 2 <= 3) { Chore.Act("ras_upgrade", ras, () => MachineRAS.UpgradeWidth(ras, s)); Upgrades++; }
            var ingot = Loose(Ingot).FirstOrDefault();
            if (ingot != null && MachineRAS.CanUpgradeHeight(ras) && RawDraw(ras) + 3 <= 3) { Chore.Act("ras_upgrade", ras, () => MachineRAS.UpgradeHeight(ras, ingot)); Upgrades++; }
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || item == null) return;
            if (sold && (item.GetTagReadonly("MEAT_TYPE_TAG").GetString() ?? "").StartsWith("FISH", StringComparison.Ordinal)) { FilletSold++; FilletSales += amount; }
            if (!sold && (item.identifier == Feed || item.identifier == Pellet)) FeedSpend += amount;
        }

        public static JObject Status()
        {
            if (!On) return null;
            var tanks = Tanks();
            var grids = tanks.Select(MachineRAS.GetTankGrid).Where(g => g != null).ToList();
            return new JObject
            {
                ["first_day"] = FirstDay, ["tanks"] = tanks.Count, ["cells"] = grids.Sum(Cells), ["used_cells"] = grids.Sum(Used),
                ["fish"] = grids.Sum(g => g.childItems.Count(i => i != null && AquacultureHelper.IsFish(i) && !AquacultureHelper.IsDead(i))),
                ["groups"] = grids.Sum(g => g.childItems.Count(i => i != null && AquacultureHelper.IsGroup(i))),
                ["clutches"] = Clutches, ["hatched"] = Hatched, ["filleted"] = Filleted, ["died"] = Died, ["fillet_value"] = FilletValue,
                ["fillets_sold"] = FilletSold, ["fillet_sales"] = FilletSales, ["feed_spend"] = FeedSpend, ["kcal_added"] = KcalAdded,
                ["hungry_nights"] = HungryNights, ["unpowered"] = Unpowered, ["upgrades"] = Upgrades, ["filters"] = Filters,
                ["water_ml"] = new JArray(tanks.Select(WaterMl)),
                ["big_bought"] = BigBought, ["big_filleted"] = BigFilleted, ["big_value"] = BigValue, ["strips"] = Strips, ["caviar_value"] = CaviarValue, ["meat_fed"] = MeatFed,
                ["big_live"] = grids.Sum(g => g.childItems.Count(i => i != null && Big(i) && !AquacultureHelper.IsDead(i))),
            };
        }
    }
}
