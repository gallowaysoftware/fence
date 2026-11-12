using System;
using System.Collections.Generic;
using System.Linq;
using static Fence.Lab.ProductionExps;

namespace Fence.Lab
{
    // docs/research/PRODUCTION-VERDICTS.md: the measurements behind the re-examined production verdicts.
    public static class VerdictExps
    {
        static GameItem Tank(int grade, int litres)
        {
            var t = Owned("water_tank");
            WaterHelper.AddWater(t, grade, litres * 1000000);
            return t;
        }

        // Coffee and bloomberry bays on the High Quality water the filter + tablet + purifier pipeline makes (99.01 %), with
        // and without nutrient tablets; production.crops covers pure, base and the tap grades. 24 nights, 4 bays each.
        public static void Hqcrops(int seed)
        {
            W.New(seed);
            var setups = new List<(string crop, string water, Action<GameItem> fill)>();
            foreach (var crop in new[] { "coffee_bean", "bloomberry_seed", "kotton_seed" })
            {
                setups.Add((crop, "HQ", j => WaterHelper.AddWater(j, 1)));
                setups.Add((crop, "HQ+tablets", j => { WaterHelper.AddWater(j, 1); for (int i = 0; i < 6; i++) ChemicalHelper.DissolveInto(Owned("nutrient_tablet"), j); }));
            }
            const int reps = 4, nights = 24;
            var bays = new List<(string crop, string water, Action<GameItem> fill, GameItem bay, GameItem jug)>();
            foreach (var (crop, water, fill) in setups)
                for (int r = 0; r < reps; r++)
                {
                    var bay = W.Own("hydroponic");
                    Put(MachineHelper.GetBatterySlot(bay), Battery());
                    var jug = Owned("water_jug");
                    fill(jug);
                    Put(MachineHydroponic.GetWaterSlot(bay), jug);
                    Put(MachineHydroponic.GetInputSlot(bay), Owned(crop));
                    bays.Add((crop, water, fill, bay, jug));
                }
            var got = bays.ToDictionary(b => b.bay, b => new Dictionary<string, int>());
            var grades = bays.ToDictionary(b => b.bay, b => new int[5]);
            foreach (var night in Enumerable.Range(1, nights))
            {
                W.QuietDay();
                foreach (var b in bays)
                {
                    var outp = MachineHydroponic.GetOutputSlot(b.bay);
                    foreach (var x in outp.childItems.Where(x => x != null).ToList())
                    {
                        got[b.bay][x.identifier] = (got[b.bay].TryGetValue(x.identifier, out int n) ? n : 0) + 1;
                        if (x.IsTag("BERRY_CONDITION_INT")) grades[b.bay][Math.Clamp(x.GetTagReadonly("BERRY_CONDITION_INT").GetInt() + 2, 0, 4)]++;
                        outp.Expel(x);
                    }
                    var input = MachineHydroponic.GetInputSlot(b.bay);
                    if (input.childItem == null) Put(input, Owned(b.crop));
                    if (WaterHelper.GetCurrentCapacityML(b.jug) < 2000) { WaterHelper.RemoveMl(b.jug, WaterHelper.GetCurrentCapacityML(b.jug)); b.fill(b.jug); }
                }
            }
            foreach (var g in bays.GroupBy(b => (b.crop, b.water)))
            {
                var tot = g.SelectMany(b => got[b.bay]).GroupBy(kv => kv.Key).ToDictionary(kv => kv.Key, kv => kv.Sum(x => x.Value) / (double)(reps * nights));
                var cond = Enumerable.Range(0, 5).Select(i => g.Sum(b => grades[b.bay][i])).ToArray();
                R.Info($"crops on HQ water [{g.Key.crop}, {g.Key.water}]: items a night per bay; berry conditions -2..+2",
                    string.Join(" ", tot.Select(kv => $"{kv.Key}:{kv.Value:0.00}")), reps * nights, "berries " + string.Join("/", cond));
            }
        }

        // A hydroreed bay as a purifier: its innate `purifying` trait works on its own water slot by percentage, so a big tank
        // cleans as fast as a small one (PlantWaterQuality.Purify). Raw or filtered tap water, no tablets.
        public static void Hydroreed(int seed)
        {
            W.New(seed);
            foreach (var (label, filtered, litres) in new[] { ("raw rust 20 L", false, 20), ("filtered rust 20 L", true, 20), ("filtered rust 6 L", true, 6) })
            {
                var tank = Tank(4, litres);
                if (filtered)
                {
                    var dst = Owned("water_tank");
                    var pitcher = W.Own("water_pitcher");
                    Put(pitcher, Owned("water_filter"));
                    for (int pass = 0; pass < 20 && WaterHelper.GetTotalVolume(tank) > 0; pass++)
                    {
                        WaterHelper.TransferLiquid(tank, pitcher);
                        WaterHelper.TransferLiquid(pitcher, dst);
                    }
                    tank = dst;
                }
                var bay = W.Own("hydroponic");
                Put(MachineHelper.GetBatterySlot(bay), Battery());
                Put(MachineHydroponic.GetWaterSlot(bay), tank);
                Put(MachineHydroponic.GetInputSlot(bay), Owned("hydroreed_seed"));
                var trace = new List<int> { WaterHelper.GetWaterPurity(tank) };
                for (int n = 0; n < 8; n++)
                {
                    W.QuietDay();
                    trace.Add(WaterHelper.GetWaterPurity(tank));
                    var input = MachineHydroponic.GetInputSlot(bay);
                    if (input.childItem == null) Put(input, Owned("hydroreed_seed"));
                    foreach (var x in MachineHydroponic.GetOutputSlot(bay).childItems.Where(x => x != null).ToList()) MachineHydroponic.GetOutputSlot(bay).Expel(x);
                }
                R.Info($"hydroreed bay, {label}: tank purity bp by night", string.Join(",", trace), 8, $"ml left {WaterHelper.GetTotalVolume(tank) / 1000}");
            }
        }

        // The staged cellar's reed (docs/research/QUALITY.md, "The staged clean-water build"): gutter and rust tap water,
        // raw or through the pitcher's filter, 20 L in a hydroreed bay. Per night: purity, worst non-mineral contaminant
        // and minerals in bp of the water; and the filter durability the fill took.
        public static void Reedcure(int seed)
        {
            W.New(seed);
            string[] dirt = { "heavy_metal", "chemical_contaminant", "microbe", "organic_waste", "physical_contaminant" };
            // The faucet's own fill (WaterHelper's tap branch): grade 1's mix at the day's purity, gutter 80-90 %, rust 90-91.5 %.
            foreach (var (label, bp, filtered) in new[] { ("raw gutter 85 %", 8500, false), ("filtered gutter 85 %", 8500, true), ("raw rust 90.75 %", 9075, false), ("filtered rust 90.75 %", 9075, true) })
            {
                var tank = Owned("water_tank");
                WaterHelper.AddWater(tank, 1, 20000000, useRange: true, bp, bp);
                int used = 0;
                if (filtered)
                {
                    var dst = Owned("water_tank");
                    var pitcher = W.Own("water_pitcher");
                    var filter = Owned("water_filter");
                    Put(pitcher, filter);
                    int d0 = DurabilityHelper.GetRealDurability(filter);
                    for (int pass = 0; pass < 40 && WaterHelper.GetTotalVolume(tank) > 0; pass++)
                    {
                        if (DurabilityHelper.GetRealDurability(WaterHelper.GetFilterItem(pitcher)) <= 0)
                        {
                            used += d0;
                            var spent = WaterHelper.GetFilterItem(pitcher);
                            spent.parentInventory?.Expel(spent);
                            Put(pitcher, Owned("water_filter"));
                        }
                        WaterHelper.TransferLiquid(tank, pitcher);
                        WaterHelper.TransferLiquid(pitcher, dst);
                    }
                    used += d0 - DurabilityHelper.GetRealDurability(WaterHelper.GetFilterItem(pitcher));
                    tank = dst;
                }
                var bay = W.Own("hydroponic");
                Put(MachineHelper.GetBatterySlot(bay), Battery());
                Put(MachineHydroponic.GetWaterSlot(bay), tank);
                Put(MachineHydroponic.GetInputSlot(bay), Owned("hydroreed_seed"));
                var trace = new List<string>();
                int hq = -1, safe = -1;
                for (int n = 0; n <= 14; n++)
                {
                    if (n > 0) W.QuietDay();
                    int p = WaterHelper.GetWaterPurity(tank), worst = dirt.Max(id => WaterHelper.GetContaminantPercentage(tank, id));
                    trace.Add($"{p}/{worst}/{WaterHelper.GetContaminantPercentage(tank, "mineral")}");
                    if (hq < 0 && p >= 9900) hq = n;
                    if (safe < 0 && worst <= 10) safe = n;
                    var input = MachineHydroponic.GetInputSlot(bay);
                    if (input.childItem == null) Put(input, Owned("hydroreed_seed"));
                    foreach (var x in MachineHydroponic.GetOutputSlot(bay).childItems.Where(x => x != null).ToList()) MachineHydroponic.GetOutputSlot(bay).Expel(x);
                }
                R.Info($"reed cure, {label} 20 L: purity/worst contaminant/mineral bp by night 0..14", string.Join(" ", trace), 14,
                    $"HQ night {hq}, plant-safe night {safe}, filter durability {used}, ml {WaterHelper.GetTotalVolume(tank) / 1000}");
            }
        }

        // The fountain's nightly takings by attractiveness, with every slot holding 20 L of its own grade
        // (VendingFountainHelper.HandleNight called directly, as the machine's late night callback does).
        public static void Fountain(int seed)
        {
            W.New(seed, "hard");
            foreach (var att in new[] { 15, 20, 40, 60, 75, 85, 100 })
                foreach (var (label, grades) in new[] { ("honest base/HQ/pure", new[] { 2, 1, 0 }), ("HQ in all three slots", new[] { 1, 1, 1 }), ("base/HQ, pure slot empty", new[] { 2, 1, -1 }) })
                {
                    var m = Owned("vending_fountain");
                    m.ModifyTag("VENDING_ATTRACTIVENESS", s => s.SetInt(att));
                    var slots = Enumerable.Range(1, 3).Select(i => MachineHelper.GetSlot(m, i) as GameSlotInventory).ToArray();
                    var tanks = new GameItem[3];
                    for (int g = 0; g < 3; g++)
                    {
                        if (grades[g] < 0) continue;
                        tanks[g] = Tank(grades[g], 20);
                        Put(slots[g], tanks[g]);
                    }
                    int crime0 = W.PS.secData.mislabeledWaterTracker + W.PS.secData.dirtyWaterTracker;
                    VendingFountainHelper.HandleNight(m, slots[0], slots[1], slots[2]);
                    int cash = m.GetTagReadonly("VENDING_CASH").GetInt();
                    var used = tanks.Select(t => t == null ? 0 : 20000 - WaterHelper.GetTotalVolume(t) / 1000).ToArray();
                    R.Info($"fountain, attractiveness {att}, {label}: cash a night; ml poured base/HQ/pure; fraud+endangerment ml tracked",
                        cash, 1, $"ml {string.Join("/", used)}; trackers +{W.PS.secData.mislabeledWaterTracker + W.PS.secData.dirtyWaterTracker - crime0}");
                }
        }

        // Free rust tap water to High Quality in volume: filter pitcher passes into a 20 L tank, a water tablet a litre,
        // one night, then one unupgraded purifier night on the whole tank (its budget scales with volume to 10 L).
        public static void Hqpipeline(int seed)
        {
            W.New(seed, "hard");
            foreach (var litres in new[] { 6, 10, 20 })
                foreach (var tabs in new[] { true, false })
                {
                    var src = Tank(4, litres);
                    var dst = Owned("water_tank");
                    var pitcher = W.Own("water_pitcher");
                    var filter = Owned("water_filter");
                    Put(pitcher, filter);
                    int d0 = (int)W.Call(typeof(DurabilityHelper), "GetRealDurability", filter);
                    int p0 = WaterHelper.GetWaterPurity(src);
                    for (int pass = 0; pass < 20 && WaterHelper.GetTotalVolume(src) > 0; pass++)
                    {
                        WaterHelper.TransferLiquid(src, pitcher);
                        WaterHelper.TransferLiquid(pitcher, dst);
                    }
                    int dur = d0 - (int)W.Call(typeof(DurabilityHelper), "GetRealDurability", filter);
                    int ml = WaterHelper.GetTotalVolume(dst) / 1000;
                    int nt = tabs ? (int)Math.Ceiling(ml / 1000.0) : 0;
                    for (int i = 0; i < nt; i++) ChemicalHelper.DissolveInto(Owned("water_tablet"), dst);
                    W.QuietDay();
                    int p1 = WaterHelper.GetWaterPurity(dst);
                    var pur = W.Own("water_purifier");
                    Put(MachineHelper.GetBatterySlot(pur), Battery());
                    Put(pur, dst);
                    var trace = new List<int> { p1 };
                    for (int n = 0; n < 4; n++) { W.QuietDay(); trace.Add(WaterHelper.GetWaterPurity(dst)); }
                    R.Info($"rust {litres} L -> filter -> {(tabs ? "1 tablet/L" : "no tablets")} -> purifier: purity bp after filter+night, then by purifier night; ml; filter durability used",
                        string.Join(",", trace), 1, $"raw {p0}; {ml} ml; filter {dur} dur ({1000.0 * dur / Math.Max(1, ml):0.0}/L); tablets {nt}");
                }
        }
    }
}
