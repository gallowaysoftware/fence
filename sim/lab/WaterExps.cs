using System;
using System.Collections.Generic;
using System.Linq;
using static Fence.Lab.ProductionExps;

namespace Fence.Lab
{
    // docs/re/WATER.md: grades, premade fills, value, labels, filter, purifier, tablets.
    public static class WaterExps
    {
        static readonly int[] Floor = { 9990, 9900, 9800, 9600, 9000, 0 };
        static readonly int[] Ceil = { 10000, 9989, 9899, 9799, 9599, 8999 };

        static string Parts(GameItem c)
        {
            var parts = WaterHelper.GetLiquidParts(c);
            return string.Join(" ", Liquid.Liquids.Select((l, i) => (l.identifier, v: parts[i])).Where(x => x.v > 0).Select(x => $"{x.identifier}={x.v / 1000.0:0.#}ml"));
        }

        static int Durability(GameItem it) => (int)W.Call(typeof(DurabilityHelper), "GetRealDurability", it);

        public static void Grades(int seed)
        {
            W.New(seed);
            // Band edges.
            var edges = new[] { (10000, 0), (9990, 0), (9989, 1), (9900, 1), (9899, 2), (9800, 2), (9799, 3), (9600, 3), (9599, 4), (9000, 4), (8999, 5), (0, 5) };
            R.Exact("grade index at purity 100.00/99.90/99.89/99.00/98.99/98.00/97.99/96.00/95.99/90.00/89.99/0", string.Join(",", edges.Select(e => e.Item2)),
                string.Join(",", edges.Select(e => WaterFeatureHelper.GetPurityArrayIndex(e.Item1))));
            // Premade fills.
            int n = Math.Max(500, Program.N);
            for (int g = 0; g <= 5; g++)
            {
                var ps = new List<int>(); int bad = 0;
                for (int i = 0; i < n; i++)
                {
                    var b = W.Make("bottled_water");
                    WaterHelper.AddWater(b, g);
                    int p = WaterHelper.GetWaterPurity(b);
                    ps.Add(p);
                    if (WaterFeatureHelper.GetPurityArrayIndex(p) != g) bad++;
                }
                double floor = g == 5 ? 3500 : Floor[g];
                double meanAbove = ps.Average() - floor;
                double band = (g == 5 ? 8999 : Ceil[g]) - floor + 1;
                R.Exact($"AddWater grade {g}: every fill lands in its band", 0, bad, n,
                    $"purity bp min {ps.Min()} mean {ps.Average():0.0} max {ps.Max()}; mean above floor = {100 * meanAbove / band:0.0}% of band");
            }
            // Values of a 750 ml bottle by grade (label = truth for premade fills).
            var want = new[] { 70.0, 52.5, 35, 23.45, 8.75, 3.5 };
            for (int g = 0; g <= 5; g++)
            {
                var b = W.Make("bottled_water");
                WaterHelper.AddWater(b, g);
                long v = b.GetCurrentValue(false, true, true, false, 0);
                R.Near($"value of a full 750 ml bottled_water, grade {g}: (round(40 x 0.75) + 5) x condition", want[g], v, 1.01, 1,
                    $"label {WaterFeatureHelper.GetCurrentWaterDisplay(b)}; ml {WaterHelper.GetTotalVolume(b) / 1000}");
            }
        }

        public static void Labels(int seed)
        {
            W.New(seed);
            // Pour into empty copies the source label; into non-empty keeps the target's.
            var ghost = Owned("water_jug"); WaterHelper.FillWithGhostwater(ghost);
            var empty = Owned("bottled_water");
            WaterHelper.TransferLiquid(ghost, empty);
            R.Exact("pour ghost into an empty bottle: label copies the source (ghost)", WaterFeatureHelper.GetCurrentWaterDisplay(ghost), WaterFeatureHelper.GetCurrentWaterDisplay(empty), 1,
                $"true purity {WaterHelper.GetWaterPurity(empty)}");
            var baseB = Owned("large_bottled_water"); WaterHelper.AddWater(baseB, 2);
            WaterHelper.RemoveMl(baseB, 1000);
            string before = WaterFeatureHelper.GetCurrentWaterDisplay(baseB);
            var rust = Owned("water_jug"); WaterHelper.FillWithRustWater(rust);
            WaterHelper.TransferLiquid(rust, baseB);
            var f = WaterFeatureHelper.GetWaterFeature(baseB);
            R.Exact("top up a half-full base bottle with rust: label stays base (silent mislabel)", before, WaterFeatureHelper.GetCurrentWaterDisplay(baseB), 1,
                $"true purity now {WaterHelper.GetWaterPurity(baseB)} bp; label {f?.customIntValue1} bp; match {f?.IsFeatureMatch()}");
            // Portable purifier relabels; tablet does not.
            var jug = Owned("water_jug"); WaterHelper.FillWithRustWater(jug);
            WaterHelper.PurifyToBaseWater(jug);
            R.Info("portable purifier on 6 L rust: purity / label after", $"{WaterHelper.GetWaterPurity(jug)} bp / {WaterFeatureHelper.GetCurrentWaterDisplay(jug)} ({WaterFeatureHelper.GetWaterFeature(jug)?.customIntValue1})");
            var tab = W.Own("water_jug"); WaterHelper.FillWithRustWater(tab);
            int p0 = WaterHelper.GetWaterPurity(tab); string l0 = WaterFeatureHelper.GetCurrentWaterDisplay(tab);
            int chem0 = WaterHelper.GetContaminantPercentage(tab, "chemical_contaminant");
            for (int i = 0; i < 6; i++) ChemicalHelper.DissolveInto(Owned("water_tablet"), tab);
            W.QuietDay();
            R.Info("6 water tablets in 6 L rust, one night: purity, chemical bp, microbe bp, label",
                $"{p0}->{WaterHelper.GetWaterPurity(tab)} bp; chem {chem0}->{WaterHelper.GetContaminantPercentage(tab, "chemical_contaminant")} (doc floor 70 bp = 0.7%); microbe {WaterHelper.GetContaminantPercentage(tab, "microbe")}; label {l0}->{WaterFeatureHelper.GetCurrentWaterDisplay(tab)}");
        }

        public static void Filter(int seed)
        {
            W.New(seed);
            foreach (var src in new[] { "rust", "ghost", "base" })
            {
                var pitcher = W.Own("water_pitcher");
                var filter = Owned("water_filter");
                bool inFilter = Put(pitcher, filter);
                int d0 = Durability(filter);
                var jug = Owned("water_jug");
                if (src == "rust") WaterHelper.FillWithRustWater(jug); else if (src == "ghost") WaterHelper.FillWithGhostwater(jug); else WaterHelper.FillWithBaseWater(jug);
                int p0 = WaterHelper.GetWaterPurity(jug);
                string before = Parts(jug);
                WaterHelper.TransferLiquid(jug, pitcher);
                int ml = WaterHelper.GetTotalVolume(pitcher) / 1000;
                int used = d0 - Durability(filter);
                R.Info($"pitcher + water_filter, pour 6 L {src} ({p0} bp): purity after, ml in pitcher, durability used",
                    $"{WaterHelper.GetWaterPurity(pitcher)} bp, {ml} ml, {used} dur ({(ml > 0 ? 1000.0 * used / ml : 0):0.#} per L)", 1,
                    $"filter seated {inFilter}; before: {before}; after: {Parts(pitcher)}");
                // Then tablets to reach base (doc: ~98.3 %).
                int tabs = (int)Math.Ceiling(ml / 1000.0);
                for (int i = 0; i < tabs; i++) ChemicalHelper.DissolveInto(Owned("water_tablet"), pitcher);
                W.QuietDay();
                R.Info($"... then {tabs} water tablet(s) and one night", $"{WaterHelper.GetWaterPurity(pitcher)} bp ({WaterFeatureHelper.GetWaterDisplayName(WaterHelper.GetWaterPurity(pitcher))})", 1, Parts(pitcher));
            }
        }

        public static void Purifier(int seed)
        {
            W.New(seed);
            var setups = new (string label, Action<GameItem> fill, string container)[]
            {
                ("6 L rust jug", j => WaterHelper.FillWithRustWater(j), "water_jug"),
                ("3 L base pitcher", j => WaterHelper.AddWater(j, 2), "water_pitcher"),
                ("3 L HQ pitcher", j => WaterHelper.AddWater(j, 1), "water_pitcher"),
            };
            var machines = new List<(string label, GameItem m, GameItem c, List<int> trace)>();
            foreach (var s in setups)
            {
                var m = W.Own("water_purifier");
                Put(MachineHelper.GetBatterySlot(m), Battery());
                var c = Owned(s.container);
                s.fill(c);
                bool ok = Put(m, c);
                machines.Add((s.label, m, c, new List<int> { WaterHelper.GetWaterPurity(c) }));
                R.Info($"purifier: container seated ({s.label})", ok, 1, "cap at quality 0: " + MachinePurifier.GetMaxPurityCap(MachineryHelper.GetCurrentQualityBonus(m)));
            }
            for (int night = 0; night < 8; night++)
            {
                W.QuietDay();
                foreach (var x in machines) x.trace.Add(WaterHelper.GetWaterPurity(x.c));
            }
            foreach (var x in machines)
            {
                R.Info($"purifier, unupgraded, {x.label}: purity by night (bp)", string.Join(",", x.trace), x.trace.Count - 1,
                    $"label after {WaterFeatureHelper.GetCurrentWaterDisplay(x.c)}; ml {WaterHelper.GetTotalVolume(x.c) / 1000}");
                R.Exact($"purifier, unupgraded, {x.label}: never exceeds the 99.01% cap", true, x.trace.Max() <= 9901 || x.trace[0] > 9901, x.trace.Count - 1);
            }
        }
    }
}
