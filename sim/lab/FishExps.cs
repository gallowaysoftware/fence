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
    }
}
