using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Wine levers (docs/re/WINE.md), all off by default, and wine sales accounting (the summary's
    // `production.wine_sold`): price, grade, age and buyer of every bottle sold.
    internal static partial class Production
    {
        // `wine_score: N`: berries go in until the bottle's score reaches N (6 = Master Vintage). Short of it, a
        // filled bottle waits up to `wine_wait_days` mornings (default 2) for the next harvest.
        static int? TargetScore => Directives.Current.Raw.Value<int?>("wine_score");
        // `wine_purifier: true`: a water purifier (Dr. Jackson, or a seller) lifts the cellar's water to High
        // Quality, which adds a point to every bottle and keeps the bays' plants healthy.
        public static bool WinePurifierOn => WineOn && (Directives.Current.Raw.Value<bool?>("wine_purifier") ?? false);
        // `wine_racks: N`: Dr. Jackson's AgeWell racks, up to N, for wine held by `wine_min_age`.
        static int WineRacks => WineOn ? Directives.Current.Raw.Value<int?>("wine_racks") ?? 0 : 0;

        // The machines the wine levers buy at Dr. Jackson's, and how many of each.
        public static int JacksonWineWants(string id) => id == "wine_rack" ? WineRacks : id == "water_purifier" && WinePurifierOn ? 1 : id == "recharger" && Power.CellarWantsRecharger ? 1 : id == "uv_filter" && PureOn ? 1 : id == "water_tank" && ReedOn ? Owned().Count(i => i.identifier == "water_tank") + CleanWants(id) : JacksonDnaWants(id);

        // HQ (99.00 %) and better: the wine term +1 and a slow enough health loss in the bays.
        const int HqPurity = 9900;

        static readonly Dictionary<int, int> filledSince = new Dictionary<int, int>(), purifyingSince = new Dictionary<int, int>();

        static int Cond(GameItem berry) => berry.GetTagReadonly("BERRY_CONDITION_INT").GetInt();

        static int Points(int cond) => cond == 2 ? 2 : cond == 1 ? 1 : cond == -1 ? 0 : -1;

        // Berries for `wine_score`: the best first, a normal one to close a one-point gap, an under/over-ripe one for
        // the pair bonus. False leaves the bottle filled and waiting (berries untouched).
        static bool BerriesToScore(IExecutorHost ep, GameItem bottle, int target)
        {
            var free = Owned().Where(i => Usable(i) && Parent(i) == null).OrderByDescending(Cond).ThenBy(i => i.uniqueId).ToList();
            bool yeast = GraphUtils.FindAllChildrenType<GameItem>(bottle).Any(i => Yeasts.Contains(i.identifier)) || Owned().Any(i => Yeasts.Contains(i.identifier) && Parent(i) == null);
            int fixedPart = KnownWaterTerm(bottle) + (yeast ? 1 : 0) + 1;
            int reachable = fixedPart + free.Where(b => Cond(b) > 0).Sum(b => Points(Cond(b)));
            if (free.Count < 2) reachable--;
            if (reachable < target)
            {
                if (!filledSince.TryGetValue(bottle.uniqueId, out int since)) filledSince[bottle.uniqueId] = since = ep.Day;
                if (ep.Day - since < (Directives.Current.Raw.Value<int?>("wine_wait_days") ?? 2)) return false;
                // Production covers scores 3 and 4: never spend a point on the 4.
                target = reachable == 4 ? 3 : reachable;
            }
            filledSince.Remove(bottle.uniqueId);
            var pick = new List<GameItem>();
            var good = free.Where(b => Cond(b) > 0).ToList();
            int need = target - fixedPart;
            while (need > 0 && good.Count > 0)
            {
                var b = need == 1 ? good.FirstOrDefault(x => Cond(x) == 1) ?? good[0] : good[0];
                good.Remove(b);
                pick.Add(b);
                need -= Points(Cond(b));
            }
            if (pick.Count < 2)
                foreach (var b in free.Where(x => !pick.Contains(x)).OrderBy(Cond).ThenBy(x => x.uniqueId))
                {
                    if (pick.Count >= 2) break;
                    pick.Add(b);
                }
            int put = 0;
            foreach (var b in pick)
                if (Put(bottle, b)) put++;
            return put > 0;
        }

        // Earmark for `wine_score`: every normal and best berry, and an under/over-ripe one per empty bottle.
        static void EarmarkForScore(List<GameItem> owned, int bottles)
        {
            foreach (var b in owned.Where(i => Usable(i) && Parent(i) == null && Cond(i) > 0)) keepBerries.Add(b.uniqueId);
            foreach (var b in owned.Where(i => Usable(i) && Parent(i) == null && Cond(i) < 0).OrderBy(i => i.uniqueId).Take(bottles + 2)) keepBerries.Add(b.uniqueId);
        }

        // High Quality water for the bottles first, when the purifier lever runs.
        static void PourHq(GameItem bottle)
        {
            if (!WinePurifierOn || WaterHelper.GetCurrentCapacityML(bottle) != 0) return;
            var hq = Owned().Where(i => CleanWater(i) && Label(i) >= HqPurity).OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();
            if (hq.Sum(WaterHelper.GetCurrentCapacityML) < 749) return;
            foreach (var src in hq)
            {
                if (WaterHelper.GetCurrentCapacityML(bottle) >= 749) break;
                Poured(bottle, LabelGrade(src));
                Chore.Act("pour", bottle, () => WaterHelper.TransferLiquid(src, bottle));
            }
        }

        // The cellar's purifier: each night one container under HQ (the largest; a full 10 L batch reaches the
        // 99.01 % cap overnight), powered from the charged cells.
        static void TendWinePurifier(IExecutorHost ep)
        {
            if (!WinePurifierOn || WaterSellOn) return;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var m in Purifiers())
            {
                var bat = MachineHelper.GetBatterySlot(m);
                if (bat != null && (bat.childItem == null || PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) < 8))
                {
                    var cell = Owned().Where(i => Charged(i) && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (cell != null)
                    {
                        if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                        Chore.Accept(bat, cell, -1);
                    }
                }
                var c = PurifierSlotItem(m);
                if (c != null && (PurifierDone(m, c) || (purifyingSince.TryGetValue(c.uniqueId, out int put) && put < ep.Day - 2)))
                {
                    Chore.Accept(stock, c, -1);
                    if (Parent(c) == null) PurifiedMl += WaterHelper.GetCurrentCapacityML(c);
                    // Its label is stale (the purifier never relabels): tested, so the bottles and bays see HQ.
                    if (Parent(c) == null) TestWater(c);
                    purifyingSince.Remove(c.uniqueId);
                    c = null;
                }
                if (c != null) continue;
                var next = Owned().Where(i => CleanWater(i) && Label(i) < HqPurity && !Bottle(i))
                    .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (next != null && PutIn(m, next)) purifyingSince[next.uniqueId] = ep.Day;
            }
        }

        static int soldBottles, soldValue, soldAge, soldUl, soldBerries;
        static readonly int[] soldByTier = new int[6];
        static readonly int[] harvestByCond = new int[5], startedByScore = new int[9];
        static readonly SortedDictionary<string, int> soldByBuyer = new SortedDictionary<string, int>(StringComparer.Ordinal);

        static void ResetSales()
        {
            filledSince.Clear();
            DnaOps = DnaReplanted = 0;
            purifyingSince.Clear();
            soldBottles = soldValue = soldAge = soldUl = soldBerries = 0;
            Array.Clear(soldByTier, 0, soldByTier.Length);
            soldByBuyer.Clear();
            Array.Clear(harvestByCond, 0, harvestByCond.Length);
            Array.Clear(startedByScore, 0, startedByScore.Length);
        }

        // Berry grades -2..2 at harvest (index +2), and the score of every bottle started (clamped 0..8).
        static void HarvestSeen(GameItem berry) => harvestByCond[Math.Max(0, Math.Min(4, berry.GetTagReadonly("BERRY_CONDITION_INT").GetInt() + 2))]++;

        static void StartSeen(GameItem bottle) => startedByScore[Math.Max(0, Math.Min(8, bottle.GetTagReadonly("WINE_SCORE_INT").GetInt()))]++;

        public static void Sold(GameItem item, int amount, StoreClient buyer)
        {
            if (item.identifier == "wine_berry") soldBerries++;
            Quality.WaterSold(item, amount);
            if (item.identifier != "wine_bottle" || !WineHelper.IsFinishedWine(item)) return;
            soldBottles++;
            soldValue += amount;
            soldAge += item.GetTagReadonly("AGE_INT").GetInt();
            soldByTier[Math.Max(0, Math.Min(5, WineHelper.GetWineQualityTier(item)))]++;
            string who = buyer?.identifier ?? "?";
            soldByBuyer[who] = (soldByBuyer.TryGetValue(who, out int n) ? n : 0) + 1;
            if (buyer?.clientFaction == "FACTION_UPPER_LEVEL") soldUl++;
        }

        static JObject SalesTotals() => new JObject
        {
            ["bottles"] = soldBottles,
            ["value"] = soldValue,
            ["mean_price"] = soldBottles == 0 ? 0 : Math.Round(soldValue / (double)soldBottles, 1),
            ["mean_age"] = soldBottles == 0 ? 0 : Math.Round(soldAge / (double)soldBottles, 1),
            ["upper_levels"] = soldUl,
            ["by_tier"] = new JArray(soldByTier),
            ["by_buyer"] = JObject.FromObject(soldByBuyer),
            ["berries_sold"] = soldBerries,
            ["harvest_by_condition"] = new JArray(harvestByCond),
            ["started_by_score"] = new JArray(startedByScore),
        };
    }
}
