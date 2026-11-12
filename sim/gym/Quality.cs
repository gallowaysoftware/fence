using System;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The output grades the engines reach (docs/research/QUALITY.md), for the summary only: these counters read the true
    // grades (hidden from a player until tested), and nothing the executor decides reads them.
    internal static class Quality
    {
        // Grade index 0..5 = pure, HQ, base, ghost, rust, gutter (WATER.md §1.3); bay nights by cost bucket.
        static readonly int[] bayNights = new int[6], bottleWater = new int[6], ingots = new int[6];
        static readonly int[] waterSoldLabel = new int[6], waterSoldTrue = new int[6], waterCreditsLabel = new int[6];
        static int mineralLow, mineralIn, mineralHigh;
        static long bayCost;

        public static void Reset()
        {
            Array.Clear(bayNights, 0, 6);
            Array.Clear(bottleWater, 0, 6);
            Array.Clear(ingots, 0, 6);
            Array.Clear(waterSoldLabel, 0, 6);
            Array.Clear(waterSoldTrue, 0, 6);
            Array.Clear(waterCreditsLabel, 0, 6);
            mineralLow = mineralIn = mineralHigh = 0;
            bayCost = 0;
        }

        public static int Grade(GameItem c) => WaterHelper.IsEmpty(c) ? 5 : WaterFeatureHelper.GetPurityArrayIndex(WaterHelper.GetWaterPurity(c));

        // Each morning, a seeded bay's water as the night will see it: the health a progress point will cost the plant
        // (HydroponicHelper.ApplyContaminantEffect's step table per contaminant, in hundredths of a percent), bucketed
        // 0 / < 1 % / < 2 % / < 4 % / >= 4 %, and its minerals against the plant's band.
        public static void Bay(GameItem bay)
        {
            var seed = MachineHydroponic.GetInputSlot(bay).childItem;
            var w = MachineHydroponic.GetWaterSlotItem(bay);
            if (seed == null || seed.identifier != "bloomberry_seed" || w == null || WaterHelper.IsEmpty(w)) return;
            double eff = HydroponicHelper.GetContaminantEffect(seed), sens = HydroponicHelper.GetContaminantSensitivity(seed);
            int cost = 0;
            foreach (var id in new[] { "heavy_metal", "chemical_contaminant", "microbe", "organic_waste", "acid", "base" })
            {
                int x = (int)Math.Round(WaterHelper.GetContaminantPercentage(w, id) * sens);
                int step = x <= 10 ? 0 : x <= 15 ? 15 : x <= 20 ? 20 : x <= 25 ? 30 : x <= 30 ? 40 : x <= 40 ? 60 : x <= 60 ? 80 : x <= 100 ? 160 : x <= 200 ? 320 : 640;
                cost += (int)(step * eff);
            }
            bayNights[cost == 0 ? 0 : cost < 100 ? 1 : cost < 200 ? 2 : cost < 400 ? 3 : 4]++;
            bayCost += cost;
            int m = WaterHelper.GetContaminantPercentage(w, "mineral"), lvl = HydroponicHelper.GetNutrimentLevel(seed), rng = HydroponicHelper.GetNutrimentRange(seed);
            if (m < lvl - rng) mineralLow++;
            else if (m > lvl + rng) mineralHigh++;
            else mineralIn++;
        }

        public static void Bottle(GameItem bottle) => bottleWater[Grade(bottle)]++;

        static readonly string[] Purities = { "INGOT_PURITY_VERY_LOW", "INGOT_PURITY_LOW", "INGOT_PURITY_FAIR", "INGOT_PURITY_HIGH", "INGOT_PURITY_VERY_HIGH", "INGOT_PURITY_PERFECT" };

        public static void Ingot(GameItem ingot)
        {
            int i = Array.IndexOf(Purities, IngotPurityHelper.GetPurity(ingot));
            if (i >= 0) ingots[i]++;
        }

        // Water sold at the counter: litres by the grade it was priced at (the label until tested) and by its true grade.
        public static void WaterSold(GameItem c, int amount)
        {
            if (c == null || !c.IsTag("LIQUID_CONTAINER_TAG") || c.identifier == "wine_bottle" || WaterHelper.IsEmpty(c)) return;
            var f = WaterFeatureHelper.GetWaterFeature(c);
            if (f == null) return;
            int label = f.IsExposed() ? Grade(c) : WaterFeatureHelper.GetPurityArrayIndex(f.customIntValue1);
            int ml = WaterHelper.GetCurrentCapacityML(c);
            waterSoldLabel[label] += ml;
            waterSoldTrue[Grade(c)] += ml;
            waterCreditsLabel[label] += amount;
        }

        public static JObject Totals(JObject engine) => new JObject
        {
            ["engine"] = engine,
            ["bay_nights_by_cost"] = new JArray(bayNights.Take(5)),
            ["bay_cost_per_point"] = bayNights.Take(5).Sum() == 0 ? 0 : Math.Round(bayCost / 100.0 / bayNights.Take(5).Sum(), 2),
            ["bay_minerals_low_in_high"] = new JArray(mineralLow, mineralIn, mineralHigh),
            ["bottle_water_by_grade"] = new JArray(bottleWater),
            ["ingots_by_purity"] = new JArray(ingots),
            ["water_sold_ml_by_label"] = new JArray(waterSoldLabel),
            ["water_sold_ml_by_true"] = new JArray(waterSoldTrue),
            ["water_sold_credits_by_label"] = new JArray(waterCreditsLabel),
        };
    }
}
