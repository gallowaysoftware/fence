using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/AQUACULTURE.md: the RAS tank on the game's own nights (AquacultureHelper.TickTank).
    public static class FishExps
    {
        static GameItem Tank(int waterMl = 12000, bool powered = true)
        {
            var ras = W.Own("ras");
            var tank = MachineRAS.GetWaterSlot(ras)?.childItem;
            if (tank == null) { tank = W.Make("ras_water_tank"); ProductionExps.Put(MachineRAS.GetWaterSlot(ras), tank); }
            while (AquacultureHelper.GetWaterMl(tank) < waterMl)
            {
                var jug = ProductionExps.Owned("water_jug");
                WaterHelper.AddWater(jug, 1);
                WaterHelper.TransferLiquid(jug, tank);
            }
            if (powered) ProductionExps.Put(MachineHelper.GetBatterySlot(ras), ProductionExps.Battery(5000));
            return ras;
        }

        static GameItem Fish(GameItem ras, FishSpecies sp, string sex, int age)
        {
            var f = AquacultureItems.SpawnFish(sp, sex, age);
            GeneralHelper.SetItemOwned(f, true);
            GraphUtils.TryAcceptAll(MachineRAS.GetTankGrid(ras), f, -1);
            return f;
        }

        static void Feed(GameItem ras)
        {
            var g = MachineRAS.GetTankGrid(ras);
            if (g.childItems.Any(i => i != null && IndustrialFishFeedHelper.IsFeed(i) && i.GetTagReadonly("CALORIE_VALUE_TAG").GetInt() > 2000)) return;
            GraphUtils.TryAcceptAll(g, ProductionExps.Owned("industrial_fish_feed"), -1);
        }

        // A fresh filter in an empty slot whenever the waste passes 1 % (over weeks it would reach the 3 % that hurts).
        static void Filter(GameItem ras)
        {
            var water = MachineRAS.GetWaterSlot(ras).childItem;
            if (AquacultureHelper.GetWastePercent(water) < 1) return;
            var grid = MachineRAS.GetFilterGrid(ras);
            for (int i = 0; i < MachineRAS.FILTER_SLOT_COUNT; i++)
                if (grid.GetElement(i % 4, i / 4) is GameSlotInventory slot)
                {
                    if (slot.childItem != null && DurabilityHelper.GetRealDurability(slot.childItem) <= 0) slot.childItem.Destroy();
                    if (slot.childItem == null) { GraphUtils.TryAcceptAll(slot, ProductionExps.Owned("water_filter"), -1); return; }
                }
        }

        static float Eaten(GameItem f) => f.GetTagReadonly("FISH_KCAL_EATEN_TAG").GetFloat();

        // The meat a fish shows is 0.9 x everything it has swallowed, the food still in its stomach included: a hungry
        // juvenile that fills up overnight shows that meal as meat the next morning.
        public static void Stomach(int seed)
        {
            W.New(seed);
            var ras = Tank();
            Feed(ras);
            var f = Fish(ras, FishSpecies.Tilapia, "MALE", 3);
            f.ModifyTag("FISH_HUNGER_TAG", s => s.Enable().SetFloat(100f));
            int meat0 = AquacultureHelper.GetMeatKcal(f);
            float eaten0 = Eaten(f);
            W.QuietDay();
            float hunger = AquacultureHelper.GetHunger(f);
            R.Near("fish stomach: a hungry tilapia refills to 85 % of 700 from a loose feed bag", 595, hunger, 6);
            R.Exact("fish stomach: meat = floor(0.9 x kcal swallowed)", (int)Math.Floor(Eaten(f) * 0.9f), AquacultureHelper.GetMeatKcal(f));
            R.Info("fish stomach: meat before / after one night (age 3, hunger 100 -> refill)", $"{meat0} -> {AquacultureHelper.GetMeatKcal(f)} (swallowed {Eaten(f) - eaten0:0})");
            R.Near("fish stomach: speed shown after the first meal is still the old diet's", 1.0, AquacultureHelper.GetGrowthSpeed(f), 0.01);
            W.QuietDay();
            R.Near("fish stomach: industrial-only diet grows at 0.75 (from the night after the meal)", 0.75, AquacultureHelper.GetGrowthSpeed(f), 0.01);
        }

        // A fed adult female spawns roe 15 % of nights; a fed adult male fertilises it the next night.
        public static void Breed(int seed)
        {
            W.New(seed);
            var ras = Tank();
            var male = Fish(ras, FishSpecies.Tilapia, "MALE", 8);
            var female = Fish(ras, FishSpecies.Tilapia, "FEMALE", 8);
            int nights = 0, roe = 0, fertile = 0, fedNights = 0;
            var seen = new HashSet<int>();
            for (int n = 0; n < 120; n++)
            {
                Feed(ras);
                Filter(ras);
                if (AquacultureHelper.GetHunger(female) >= 350 && AquacultureHelper.GetHunger(male) >= 350) fedNights++;
                W.QuietDay();
                nights++;
                var g = MachineRAS.GetTankGrid(ras);
                foreach (var r in g.childItems.Where(i => i != null && i.IsTag("FISH_ROE_TAG")).ToList())
                {
                    if (seen.Add(r.uniqueId)) roe++;
                    if (r.IsTag("FISH_FERTILIZED_TAG")) { fertile++; r.Destroy(); }
                }
                foreach (var x in g.childItems.Where(i => i != null && i.IsTag("FISH_FRY_TAG")).ToList()) x.Destroy();
                // Keep the pair adult: past elder they still breed, but old age kills them from day 27.
                male.ModifyTag("FISH_AGE_TAG", s => s.Enable().SetInt(10));
                female.ModifyTag("FISH_AGE_TAG", s => s.Enable().SetInt(10));
            }
            R.Info("fish breed: mornings both adults were fed (hunger >= 350) / alive", $"{fedNights}/{nights}, female hp {AquacultureHelper.GetHealth(female):0}, dead {AquacultureHelper.IsDead(female)}");
            R.Rate("fish breed: roe spawned per fed-female night", 0.15, roe, nights);
            R.Exact("fish breed: every roe is fertilised by a fed male", roe, fertile + MachineRAS.GetTankGrid(ras).childItems.Count(i => i != null && i.IsTag("FISH_ROE_TAG")));
        }

        // No power: every fish loses 34 % of its health a night, so the third dark night kills it.
        public static void Unpowered(int seed)
        {
            W.New(seed);
            var ras = Tank(powered: false);
            Feed(ras);
            var f = Fish(ras, FishSpecies.Tilapia, "FEMALE", 8);
            var hp = new List<float>();
            for (int n = 0; n < 3; n++) { W.QuietDay(); hp.Add(AquacultureHelper.GetHealth(f)); }
            R.Near("fish unpowered: health after one dark night (fed, no regen)", 66, hp[0], 1);
            R.Exact("fish unpowered: dead after the third dark night", true, AquacultureHelper.IsDead(f));
        }

        // A juvenile grows into its next sprite only where the tank has room for it (AquacultureHelper.TryGrowIntoRoom).
        public static void Hatch(int seed)
        {
            W.New(seed);
            var ras = Tank();
            Feed(ras);
            var roe = AquacultureItems.SpawnFertilizedRoe(FishSpecies.Tilapia);
            GeneralHelper.SetItemOwned(roe, true);
            GraphUtils.TryAcceptAll(MachineRAS.GetTankGrid(ras), roe, -1);
            int nightsToFry = 0, nightsToJuveniles = 0;
            for (int n = 1; n <= 6; n++)
            {
                Feed(ras);
                W.QuietDay();
                var g = MachineRAS.GetTankGrid(ras);
                if (nightsToFry == 0 && g.childItems.Any(i => i != null && i.IsTag("FISH_FRY_TAG"))) nightsToFry = n;
                if (nightsToJuveniles == 0 && g.childItems.Count(i => i != null && AquacultureHelper.IsFish(i)) >= 6) nightsToJuveniles = n;
            }
            R.Exact("fish hatch: fertilised roe -> fry after nights", 1, nightsToFry);
            R.Info("fish hatch: fertilised roe -> six juveniles after nights (fry ages two fed nights at 0.75-1.0 speed)", nightsToJuveniles);
        }
            // An item put at grid cell (x, y) of an inventory, trying each orientation (the player's drag and drop).
        static bool PlaceAt(GameGridInventory grid, GameItem it, int x, int y)
        {
            for (int o = 4; o >= 1; o--)
            {
                var b = new GridShapeBuilder(it.modifiedShape);
                b.SetTransform(x, y, false, o);
                var slot = grid.TryInventorySlot(it, it.unitCount, b);
                if (slot != null && slot.numTransfer > 0 && slot.TryAcceptOnce(it.unitCount) > 0) return true;
            }
            return false;
        }

        static string Box(GameItem it) => it.modifiedShape is GridShape s ? $"({s.minX},{s.minY})-({s.maxX},{s.maxY})" : "?";

        static int Charge(GameItem cell) => cell == null ? -1 : PowerHelper.GetAvailableEnergyFromItem(cell, false);

        // FORMULA-EXPLOITS §6: an electric eel tank charges its own battery and, in full, every edge-adjacent cell or
        // battery machine in the grid it stands in (AquacultureHelper.GeneratePower, early pass), ignoring blackouts.
        public static void EelHub(int seed)
        {
            W.New(seed);
            var stock = W.E.invElement;
            W.ClearInventory(stock);
            R.Info("eel hub: the stock grid (w x h)", $"{stock.inventoryShape.width} x {stock.inventoryShape.height}");
            var ras = W.Make("ras");
            GeneralHelper.SetItemOwned(ras, true);
            bool placed = PlaceAt(stock, ras, 0, 0);
            var tank = W.Make("ras_water_tank");
            ProductionExps.Put(MachineRAS.GetWaterSlot(ras), tank);
            while (AquacultureHelper.GetWaterMl(tank) < 12000)
            {
                var jug = ProductionExps.Owned("water_jug");
                WaterHelper.AddWater(jug, 1);
                WaterHelper.TransferLiquid(jug, tank);
            }
            var own = ProductionExps.Battery(0);
            ProductionExps.Put(MachineHelper.GetBatterySlot(ras), own);
            var eels = new List<GameItem>();
            for (int k = 0; k < 2; k++)
            {
                var e = Fish(ras, FishSpecies.ElectricEel, k == 0 ? "MALE" : "FEMALE", 18);
                e.ModifyTag("FISH_HUNGER_TAG", s => s.Enable().SetFloat(1100f));
                eels.Add(e);
            }
            int output = AquacultureHelper.GetTankPowerOutput(MachineRAS.GetTankGrid(ras));
            R.Info("eel hub: the tank (shape in the stock grid)", Box(ras), 1, $"placed {placed}; two elder electric eels, output {output} a night; tank draw {MachineryHelper.GetMachinePowerUsage(ras)}");
            var sh = ras.modifiedShape;
            // Right of the tank, below it, diagonal to it, and one cell clear of it.
            var right = ProductionExps.Battery(0); GeneralHelper.SetItemOwned(right, true);
            bool pr = PlaceAt(stock, right, sh.maxX + 1, sh.minY);
            var bay = W.Make("hydroponic"); GeneralHelper.SetItemOwned(bay, true);
            bool pb = PlaceAt(stock, bay, sh.minX, sh.maxY + 1);
            var bayCell = ProductionExps.Battery(0);
            ProductionExps.Put(MachineHelper.GetBatterySlot(bay), bayCell);
            var diag = ProductionExps.Battery(0); GeneralHelper.SetItemOwned(diag, true);
            bool pd = PlaceAt(stock, diag, sh.maxX + 1, sh.maxY + 1);
            var far = ProductionExps.Battery(0); GeneralHelper.SetItemOwned(far, true);
            bool pf = PlaceAt(stock, far, sh.maxX + 3, sh.minY);
            var adj = ras.FindAdjacentType<GameItem>().Where(i => i != ras).Select(i => i.identifier).ToList();
            R.Info("eel hub: placed (right cell, bay below, diagonal cell, far cell); adjacent per FindAdjacentType", $"{pr} {pb} {pd} {pf}", 1,
                $"right {Box(right)} bay {Box(bay)} diag {Box(diag)} far {Box(far)}; adjacent: {string.Join(",", adj)}");
            W.QuietDay();
            int o2 = AquacultureHelper.GetTankPowerOutput(MachineRAS.GetTankGrid(ras));
            R.Exact("eel hub: an edge-adjacent loose cell gains the tank's full output a night", output, Charge(right));
            R.Exact("eel hub: an edge-adjacent machine's cell (hydroponic bay, idle) gains the full output", output, Charge(bayCell));
            R.Exact("eel hub: a diagonal cell gains nothing", 0, Charge(diag));
            R.Exact("eel hub: a cell one gap away gains nothing", 0, Charge(far));
            R.Info("eel hub: the tank's own battery after one night (output - draw)", Charge(own), 1, $"output then {o2}; eel hunger {string.Join("/", eels.Select(e => AquacultureHelper.GetHunger(e).ToString("0")))}");
            // A blackout does not stop the eels.
            W.QuietDay();
            R.Info("eel hub: after two nights (right cell, bay cell)", $"{Charge(right)}, {Charge(bayCell)}");
        }
    }
}
