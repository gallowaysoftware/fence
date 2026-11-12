using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `crops: {"coffee_bean": 2, "kotton_seed": 1}`: bays beyond the cellar's `bays`, each growing a cash crop that goes
    // straight to stock (docs/research/SECOND-ENGINE.md). The bays are the wine engine's (bought, powered, watered
    // and fed the same way); only the seed differs. A seed crop keeps its own seeds for replanting (coffee replants one
    // of its own beans; kotton and fillerweed return seeds); everything else it harvests is stock.
    internal static class Crops
    {
        static JObject Plan => Directives.Current.Raw["crops"] as JObject;

        public static bool On => Plan != null && Plan.Count > 0;

        static IEnumerable<(string seed, int bays)> Targets() =>
            On ? Plan.Properties().Select(p => (p.Name, (int?)p.Value ?? 0)).Where(t => t.Item2 > 0).OrderBy(t => t.Name, StringComparer.Ordinal) : Enumerable.Empty<(string, int)>();

        public static int ExtraBays => Targets().Sum(t => t.bays);

        static bool IsCropSeed(string id) => id != null && Targets().Any(t => t.seed == id);

        // `crops_tap` (default true): Wild's WATER_TRADER and GUTTERFLOW_TAP, and the crop bays drink raw tap water.
        // Coffee keeps half its harvest on any grade (4 beans of 8; docs/research/SECOND-ENGINE.md), while the clean
        // water the shop can buy is too scarce to keep a bay wet.
        static bool TapWater => On && (Directives.Current.Raw.Value<bool?>("crops_tap") ?? true);

        public static IEnumerable<string> Upgrades()
        {
            if (!TapWater) yield break;
            yield return "WATER_TRADER";
            yield return "GUTTERFLOW_TAP";
        }

        public static bool WantsUpgrade(string id) => TapWater && (id == "WATER_TRADER" || id == "GUTTERFLOW_TAP");

        static GameItem faucet;
        static readonly string[] Vessels = { "water_jug", "large_bottled_water", "bottled_water", "small_bottled_water" };

        // A crop bay short of a night's water is filled at the faucet (its container, or an empty vessel put in the slot).
        public static bool Water(IExecutorHost ep, GameItem bay)
        {
            if (!TapWater || !IsCrop(bay) || !Production.TapOn) return false;
            var ws = MachineHydroponic.GetWaterSlot(bay);
            var c = ws.childItem;
            if (c == null)
            {
                var empty = Owned().Where(i => Vessels.Contains(i.identifier) && Parent(i) == null && WaterHelper.IsEmpty(i) && !Lab.Holds(i)).OrderByDescending(i => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt()).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (empty == null || Chore.Accept(ws, empty, -1) != 0) return false;
                c = empty;
            }
            if (WaterHelper.GetCurrentCapacityML(c) >= 600) return true;
            faucet ??= Chore.Faucet();
            int before = WaterHelper.GetCurrentCapacityML(c);
            Chore.Act("tap", c, () => c.onThisTargetItemFunc?.Invoke(c, faucet));
            TapMl += WaterHelper.GetCurrentCapacityML(c) - before;
            return true;
        }

        public static long TapMl;
        public static int Harvested, Planted;
        public static long HarvestValue, Sales;
        public static readonly Dictionary<string, int> SoldById = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly HashSet<int> keepSeeds = new HashSet<int>(), harvested = new HashSet<int>();

        public static void Reset()
        {
            Harvested = Planted = 0;
            TapMl = 0;
            faucet = null;
            HarvestValue = Sales = 0;
            SoldById.Clear();
            keepSeeds.Clear();
            harvested.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        static int Planted_(List<GameItem> bays, string seed) => bays.Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == seed);

        static IEnumerable<GameItem> LooseSeeds(string seed) => Owned().Where(i => i.identifier == seed && Parent(i) == null && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId);

        // The seeds held for replanting: one per bay of the crop and one spare.
        public static void Earmark(List<GameItem> bays)
        {
            keepSeeds.Clear();
            foreach (var (seed, n) in Targets())
                foreach (var s in LooseSeeds(seed).Take(n + 1))
                    keepSeeds.Add(s.uniqueId);
        }

        public static bool Reserved(GameItem i) => On && keepSeeds.Contains(i.uniqueId);

        public static bool IsInput(string id) => On && IsCropSeed(id);

        // A crop seed is bought only while a crop bay has none to plant.
        public static int Wants(string id, List<GameItem> bays)
        {
            if (!On || !IsCropSeed(id)) return 0;
            int target = Targets().First(t => t.seed == id).bays;
            int have = Planted_(bays, id) + LooseSeeds(id).Count();
            return Math.Max(0, Math.Min(target, bays.Count) - have);
        }

        // The seed for an empty bay once the cellar has its `bays` of bloomberries: a crop still short of its bays, in
        // plan order; null leaves the bay to the cellar.
        public static GameItem SeedFor(List<GameItem> bays, int wineBays)
        {
            if (!On || Planted_(bays, "bloomberry_seed") < wineBays) return null;
            foreach (var (seed, n) in Targets())
                if (Planted_(bays, seed) < n && LooseSeeds(seed).FirstOrDefault() is GameItem s)
                    return s;
            return null;
        }

        public static bool IsCrop(GameItem bay) => IsCropSeed(MachineHydroponic.GetInputSlot(bay).childItem?.identifier);

        public static void Harvest(GameItem item)
        {
            if (!harvested.Add(item.uniqueId)) return;
            Harvested++;
            HarvestValue += PlayerRole.Est(item);
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || !sold || item == null || !harvested.Contains(item.uniqueId)) return;
            Sales += amount;
            string id = item.identifier ?? "?";
            SoldById[id] = (SoldById.TryGetValue(id, out int n) ? n : 0) + 1;
        }

        public static JObject Totals(List<GameItem> bays) => !On ? null : new JObject
        {
            ["bays"] = bays.Count(IsCrop), ["planted"] = Planted, ["harvested"] = Harvested, ["harvest_value"] = HarvestValue,
            ["sales"] = Sales, ["sold"] = JObject.FromObject(SoldById), ["tap_ml"] = TapMl,
        };
    }
}
