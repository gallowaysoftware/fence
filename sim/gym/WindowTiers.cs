using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // `window_mode: "tiers"` (docs/re/WINDOW.md): the case brings one buyer for each of its top N types by displayed value
    // (N = PlayerStore.GetShowcaseTypeVisitorCount), at a tier set by that type's real value in the case: 145 for tier 2,
    // 350 from day 14 for tier 3 (household 300). An item counts in full for every type it has (ShowcaseHelper
    // .CountsAsDisplayType), and the forecast is read only when the shutter opens, so the case can be sold that day.
    // Items go in one at a time, each the one that most raises the buyers' tiers (and the progress toward the next).
    internal static class WindowTiers
    {
        static readonly string[] Types = { "FOOD", "MEDICAL", "MATERIAL", "HOUSEHOLD_GOOD", "SUBSTANCE", "WEAPON", "WATER", "ARMOR", "TOOL", "MODULE", "AMMUNITION" };

        static bool Counts(GameItem i, string t)
        {
            var mine = i.GetGameItemType();
            return mine.Contains(t) || (t == "MATERIAL" && mine.Contains("ENERGY_CELL")) || (t == "WATER" && mine.Contains("WATER_RATION"));
        }

        static long Real(GameItem i) => i.GetCurrentValue(false, true, true, false, 0L);

        static double Score(List<GameItem> c, int buyers, int day)
        {
            var top = Types.Select(t => (t, shown: c.Where(i => Counts(i, t)).Sum(i => (long)i.GetNegociatedValue())))
                .Where(x => x.shown > 0).OrderByDescending(x => x.shown).Take(buyers);
            double s = 0;
            foreach (var (t, _) in top)
            {
                long v = c.Where(i => Counts(i, t)).Sum(Real);
                long t3 = day >= 14 ? (t == "HOUSEHOLD_GOOD" ? 300 : 350) : long.MaxValue;
                s += v >= t3 ? 3 : v >= 145 ? 1 : 0;
                s += 0.9 * Math.Min(v, day >= 14 ? t3 : 145) / (double)(day >= 14 ? t3 : 145);
            }
            return s;
        }

        public static List<GameItem> Pick(IList<GameItem> pool, int capacity, int day)
        {
            int buyers = Math.Max(1, PlayerStore.Instance.GetShowcaseTypeVisitorCount());
            var rest = pool.Where(i => Types.Any(t => Counts(i, t))).OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).ToList();
            var picked = new List<GameItem>();
            while (picked.Count < capacity && rest.Count > 0)
            {
                double now = Score(picked, buyers, day);
                GameItem best = null;
                double gain = 1e-9;
                foreach (var i in rest)
                {
                    picked.Add(i);
                    double g = Score(picked, buyers, day) - now;
                    picked.RemoveAt(picked.Count - 1);
                    if (g > gain) { gain = g; best = i; }
                }
                if (best == null) break;
                picked.Add(best);
                rest.Remove(best);
            }
            // The rest of the room, most valuable first, unless an item would push a tiered type out of the top N.
            foreach (var i in rest)
            {
                if (picked.Count >= capacity) break;
                double now = Score(picked, buyers, day);
                picked.Add(i);
                if (Score(picked, buyers, day) < now) picked.RemoveAt(picked.Count - 1);
            }
            return picked;
        }
    }
}
