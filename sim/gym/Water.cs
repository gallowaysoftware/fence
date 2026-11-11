using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Tap water made clean enough to ferment, grow and sell (docs/re/WATER.md §1.4–1.5, docs/sim/VERIFIED.md
    // "Filter + tablets: rust → ≈ 98.3 % base"). Each morning, as a player at the sink: fill a container at
    // the faucet, pour it through the filter pitcher into an empty container, drop in a purification tablet
    // per litre; the next morning a test strip and Match put the true grade on the label. The faucet needs
    // GUTTERFLOW_TAP (Gutterflow, < 90 %) or RUSTWATER_TAP (90–91.5 %, which ends at Basewater).
    internal static partial class Production
    {
        public static int WaterTreatedMl, WaterRelabelled;
        static readonly Dictionary<int, int> treatedOn = new Dictionary<int, int>();
        // Clean containers kept for the bottles and bays when water is also sold.
        static readonly HashSet<int> keepWater = new HashSet<int>();
        static GameItem faucet;

        static readonly HashSet<string> Treatable = new HashSet<string>(StringComparer.Ordinal) { "water_jug", "large_bottled_water", "bottled_water", "small_bottled_water" };

        public static bool TapOn => Up("RUSTWATER_TAP") || Up("GUTTERFLOW_TAP");

        static bool Up(string id)
        {
            try { return NetworkUpgrade.GetUpgradeById(id)?.IsUnlocked() ?? false; }
            catch (Exception) { return false; }
        }

        static bool InBox(GameItem i) => Parent(i) == null || (Parent(i).identifier ?? "").EndsWith("_container", StringComparison.Ordinal);

        static GameItem Pitcher() => Owned().FirstOrDefault(i => i.identifier == "water_pitcher" && Parent(i) == null);

        static int FilterLeft(GameItem pitcher)
        {
            var f = pitcher == null ? null : WaterHelper.GetFilterItem(pitcher);
            return f == null ? 0 : DurabilityHelper.GetRealDurability(f);
        }

        static IEnumerable<GameItem> Tablets() => Owned().Where(i => i.identifier == "water_tablet" && InBox(i));
        static IEnumerable<GameItem> Strips() => Owned().Where(i => i.identifier == "water_test_strip" && !i.IsTag("IS_SPENT") && InBox(i));

        // Containers the engine keeps for water (never sold, used for wine and the bays).
        static bool WaterVessel(GameItem i) => Treatable.Contains(i.identifier ?? "") && Parent(i) == null;

        // ---- purifiers: Basewater to High Quality (99.01 %) overnight, ~3 L a night ----------------

        static List<GameItem> Purifiers() => Owned().Where(i => i.identifier == "water_purifier" && Parent(i) == null).ToList();

        static GameItem PurifierSlotItem(GameItem m)
        {
            foreach (var inv in GraphUtils.FindChildrenType<GameSlotInventory>(m))
                if (inv.childItem != null && inv.childItem.IsTag("LIQUID_CONTAINER_TAG")) return inv.childItem;
            return null;
        }

        static bool PutIn(GameItem machine, GameItem item)
        {
            foreach (var inv in GraphUtils.FindChildrenType<GameSlotInventory>(machine))
                if (inv.childItem == null && Chore.Accept(inv, item, -1) == 0 && Parent(item) == machine) return true;
            return false;
        }

        public static int PurifiedMl;

        static void TendPurifiers(IExecutorHost ep)
        {
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var m in Purifiers())
            {
                var bat = MachineHelper.GetBatterySlot(m);
                if (bat != null && (bat.childItem == null || PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) < 8))
                {
                    var cell = Owned().Where(i => Charged(i) && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).FirstOrDefault();
                    if (cell != null)
                    {
                        if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                        Chore.Accept(bat, cell, -1);
                    }
                }
                var c = PurifierSlotItem(m);
                // Done when it reached the cap (HQ) or the machine stopped removing anything.
                if (c != null && (WaterHelper.GetWaterPurity(c) >= 9900 || m.GetTagReadonly("TOTAL_CONTAMINANT_REMOVED_TAG").GetInt() == 0 && treatedOn.TryGetValue(c.uniqueId, out int put) && put < ep.Day - 1))
                {
                    Chore.Accept(stock, c, -1);
                    if (Parent(c) == null)
                    {
                        PurifiedMl += WaterHelper.GetCurrentCapacityML(c);
                        treatedOn[c.uniqueId] = ep.Day - 1; // relabel at the next pass
                    }
                    c = null;
                }
                if (c == null)
                {
                    var next = Owned().Where(i => WaterVessel(i) && !WaterHelper.IsEmpty(i) && !treatedOn.ContainsKey(i.uniqueId) && !keepWater.Contains(i.uniqueId)
                            && WaterHelper.GetWaterPurity(i) >= 9600 && WaterHelper.GetWaterPurity(i) < 9900)
                        .OrderByDescending(WaterHelper.GetCurrentCapacityML).FirstOrDefault();
                    if (next != null && PutIn(m, next)) treatedOn[next.uniqueId] = ep.Day;
                }
            }
        }

        // ---- the bottle printer: empty bottles for the water (a button press, real time, battery power) ----

        public static int Printed;

        // MachineBottlePrinter's buttons: small (450 ml, 4 power), normal (750 ml, 6; 3 upgrades), large (2 L, 11; 9 upgrades).
        public static int Print(IExecutorHost ep, GameItem printer, string size, int count)
        {
            string key = size == "large" ? "mech_bottle_printer_btn_print_large" : size == "normal" ? "mech_bottle_printer_btn_print_normal" : "mech_bottle_printer_btn_print_small";
            string label = size == "small" ? LocHelper.GetLocalizedMechanicOrDefault(key, "Print Small Bottle") : LocHelper.GetLocalizedMechanic(key);
            var press = ep.MachineButton(printer, label);
            if (press == null) return 0;
            var stock = EmporiumEntry.Instance.invElement;
            int made = 0;
            for (int k = 0; k < count; k++)
            {
                int before = GraphUtils.FindAllChildrenType<GameItem>(printer).Count(i => i.IsTag("LIQUID_CONTAINER_TAG"));
                press();
                var outs = GraphUtils.FindAllChildrenType<GameItem>(printer).Where(i => i.IsTag("LIQUID_CONTAINER_TAG")).ToList();
                if (outs.Count <= before) break;
                made++;
                foreach (var b in outs)
                {
                    Chore.Act("own", b, () => GeneralHelper.SetItemOwned(b, true));
                    Chore.Accept(stock, b, -1);
                }
            }
            Printed += made;
            return made;
        }

        static void AutoPrint(IExecutorHost ep)
        {
            var printer = Owned().FirstOrDefault(i => i.identifier == "bottle_printer" && Parent(i) == null);
            if (printer == null) return;
            int empties = Owned().Count(i => WaterVessel(i) && WaterHelper.IsEmpty(i));
            if (empties >= 4) return;
            int n = Print(ep, printer, MachineBottlePrinter.IsNormalUnlocked(printer) ? "normal" : "small", 4 - empties);
            if (n > 0) ep.Human("production", null, ep.Uid(printer), new JObject { ["action"] = "print_bottles", ["count"] = n }, "player:water");
        }

        public static JObject WaterStatus()
        {
            var owned = Owned().ToList();
            var p = Pitcher();
            return new JObject
            {
                ["empty_vessels"] = owned.Count(i => WaterVessel(i) && WaterHelper.IsEmpty(i)),
                ["filled_vessels"] = owned.Count(i => WaterVessel(i) && !WaterHelper.IsEmpty(i)),
                ["treating"] = treatedOn.Count,
                ["pitcher"] = p != null, ["filter_left"] = FilterLeft(p),
                ["spare_filters"] = owned.Count(i => i.identifier == "water_filter" && Parent(i) == null),
                ["tablets"] = Tablets().Count(), ["strips"] = Strips().Count(),
                ["portable_purifier_charge"] = owned.Where(i => i.identifier == "portable_water_purifier").Select(DurabilityHelper.GetRealDurability).DefaultIfEmpty(0).Max(),
                ["treated_total_ml"] = WaterTreatedMl,
                ["purifiers"] = Purifiers().Count, ["purified_total_ml"] = PurifiedMl, ["printed_total"] = Printed,
            };
        }

        // Empty bottles and jugs to fill and sell (selling water): a few of each size in hand.
        public static bool WantsEmptyVessel(GameItem i, List<GameItem> already)
        {
            if (!WaterSellOn || !TapOn || !Treatable.Contains(i.identifier ?? "") || !WaterHelper.IsEmpty(i)) return false;
            int have = Owned().Count(x => x.identifier == i.identifier && Parent(x) == null && WaterHelper.IsEmpty(x)) + already.Count(x => x.identifier == i.identifier);
            return have < (i.identifier == "water_jug" ? 3 : 6);
        }

        public static int WaterWants(string id)
        {
            if (!TapOn) return 0;
            var owned = Owned().ToList();
            switch (id)
            {
                case "water_pitcher": return Pitcher() == null ? 1 : 0;
                case "water_filter": return Math.Max(0, 2 - owned.Count(i => i.identifier == "water_filter" && Parent(i) == null));
                // Supplies in proportion to throughput: two boxes at a time while selling water.
                case "water_tablet_container":
                case "water_tablet": return Tablets().Count() < (WaterSellOn ? 30 : 12) ? (WaterSellOn ? 2 : 1) : 0;
                case "water_test_container":
                case "water_test_strip": return owned.Any(i => i.identifier == "aquascan") ? 0 : Strips().Count() < (WaterSellOn ? 12 : 4) ? (WaterSellOn ? 2 : 1) : 0;
                case "water_jug": return Math.Max(0, (WaterSellOn ? 5 : 3) - owned.Count(i => i.identifier == "water_jug" && Parent(i) == null && WaterHelper.IsEmpty(i)));
                case "aquascan": return WaterSellOn && !owned.Any(i => i.identifier == "aquascan") ? 1 : 0;
                case "water_purifier": return (WaterSellOn && Purifiers().Count < 2) || (WinePurifierOn && Purifiers().Count < 1) ? 1 : 0;
                case "energy_credit": return WaterSellOn && Purifiers().Any() && owned.Count(i => Charged(i) && Parent(i) == null) < 2 ? 1 : 0;
                case "portable_water_purifier": return WaterSellOn && !owned.Any(i => i.identifier == "portable_water_purifier" && DurabilityHelper.GetRealDurability(i) >= 30) ? 1 : 0;
            }
            return 0;
        }

        public static bool IsWaterInput(string id) => id == "aquascan" || id == "water_purifier" || id == "portable_water_purifier" || id == "water_pitcher" || id == "water_filter" || id == "water_tablet_container" || id == "water_tablet" || id == "water_test_container" || id == "water_test_strip";

        static bool WaterReserved(GameItem i)
        {
            if (!TapOn) return false;
            if (i.identifier == "water_pitcher" || (i.identifier == "water_filter" && DurabilityHelper.GetRealDurability(i) > 0)) return true;
            if (i.identifier == "portable_water_purifier" && DurabilityHelper.GetRealDurability(i) > 0) return true;
            if (WaterSellOn && (i.identifier == "water_purifier" || i.identifier == "aquascan" || (Charged(i) && Purifiers().Any()))) return true;
            if (WaterSellOn && Treatable.Contains(i.identifier ?? "") && Parent(i) == null && WaterHelper.IsEmpty(i)) return true;
            if (i.identifier == "water_tablet" || i.identifier == "water_tablet_container" || i.identifier == "water_test_container" || (i.identifier == "water_test_strip" && !i.IsTag("IS_SPENT"))) return true;
            return i.identifier == "water_jug" && Parent(i) == null && WaterHelper.IsEmpty(i);
        }

        public static void TreatWater(IExecutorHost ep)
        {
            if (!TapOn) return;
            int day = ep.Day;
            // Yesterday's batches: test and match the label to the truth.
            // Strips are scarce: the purest water (what sells for the most) is tested first; water the bottles and
            // bays will use needs no label (they read the truth).
            foreach (var c in Owned().Where(i => treatedOn.TryGetValue(i.uniqueId, out int d) && d < day && Parent(i) == null)
                         .OrderByDescending(i => WaterHelper.IsEmpty(i) ? 0 : WaterHelper.GetWaterPurity(i)).ThenByDescending(WaterHelper.GetCurrentCapacityML).ToList())
            {
                treatedOn.Remove(c.uniqueId);
                if (!WaterSellOn) continue;
                if (WaterHelper.IsEmpty(c)) continue;
                // The AquaScan reads and Matches for free; otherwise a strip is spent.
                bool scanner = Owned().Any(i => i.identifier == "aquascan");
                var strip = scanner ? null : Strips().FirstOrDefault();
                if (!scanner && strip == null) continue;
                Chore.Act("test_strip", c, () => strip?.onItemTargetItemFunc?.Invoke(strip, c));
                var f = WaterFeatureHelper.GetWaterFeature(c);
                if (f != null && GeneralHelper.IsItemOwned(c) && !f.IsExposed() && !f.IsFeatureMatch())
                {
                    // WaterStripUIManager.OnMatch.
                    Chore.Act("test_strip", c, () => f.ExposeFeature(lowkey: true));
                    Chore.Act("relabel", c, () => WaterFeatureHelper.OnWaterLabelUpdated(c));
                    WaterRelabelled++;
                }
            }
            faucet ??= DirectoryMaster.Item("faucet");
            if (WaterSellOn) PortablePurify(ep);
            if (WaterSellOn) TendPurifiers(ep);
            if (WaterSellOn && (Directives.Current.Raw.Value<bool?>("auto_print") ?? false)) AutoPrint(ep);
            var pitcher = Pitcher();
            if (pitcher == null) { KeepForWine(); return; }
            if (WaterHelper.GetFilterItem(pitcher) == null || FilterLeft(pitcher) <= 0)
            {
                var spent = WaterHelper.GetFilterItem(pitcher);
                if (spent != null) Chore.Accept(EmporiumEntry.Instance.invElement, spent, -1);
                var fresh = Owned().Where(i => i.identifier == "water_filter" && Parent(i) == null && DurabilityHelper.GetRealDurability(i) > 0).OrderByDescending(DurabilityHelper.GetRealDurability).FirstOrDefault();
                if (fresh == null) return;
                foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(pitcher))
                    if (Chore.Accept(inv, fresh, -1) == 0 && Parent(fresh) == pitcher) break;
                if (FilterLeft(pitcher) <= 0) return;
            }
            // Keep about two bottles' worth of clean water ahead of the empty bottles and the bays.
            var owned = Owned().ToList();
            int haveMl = owned.Where(i => CleanWater(i)).Sum(WaterHelper.GetCurrentCapacityML);
            int wantMl = (WineOn ? owned.Count(EmptyBottle) * 750 + Bays().Count * 1500 + 1500 - owned.Where(WineHelper.IsWaterRation).Sum(r => r.identifier == "water_ration" ? 750 : 375) : 0)
                + (WaterSellOn ? 12000 : 0);
            if (WaterSellOn) haveMl = 0;
            var empties = owned.Where(i => WaterVessel(i) && WaterHelper.IsEmpty(i)).OrderByDescending(i => i.identifier == "water_jug").ThenBy(i => i.uniqueId).ToList();
            if (WaterHelper.GetCurrentCapacityML(pitcher) > 0) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
            while (haveMl < wantMl && empties.Count >= 2 && FilterLeft(pitcher) > 0 && (Tablets().Any() || (WaterSellOn && Purifiers().Any())))
            {
                var raw = empties[empties.Count - 1];
                var clean = empties[0];
                empties.RemoveAt(empties.Count - 1);
                empties.RemoveAt(0);
                Chore.Act("tap", raw, () => raw.onThisTargetItemFunc?.Invoke(raw, faucet));
                int guard = 0;
                // Refilled at the faucet until the clean vessel is full (one fill of a small raw bottle is not).
                while (!WaterHelper.IsFull(clean) && FilterLeft(pitcher) > 0 && guard++ < 40)
                {
                    if (WaterHelper.IsEmpty(raw)) Chore.Act("tap", raw, () => raw.onThisTargetItemFunc?.Invoke(raw, faucet));
                    Chore.Act("pour", pitcher, () => WaterHelper.TransferLiquid(raw, pitcher));
                    Chore.Act("pour", clean, () => WaterHelper.TransferLiquid(pitcher, clean));
                }
                Chore.Act("empty", raw, () => WaterHelper.EmptyContainer(raw));
                if (WaterHelper.GetCurrentCapacityML(pitcher) > 0) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
                int ml = WaterHelper.GetCurrentCapacityML(clean);
                // Tablets always: filtered tap water sits near 95 % and the purifier only takes >= 96 %.
                foreach (var tab in Tablets().Take(Math.Max(1, (ml + 999) / 1000)).ToList())
                    Chore.Act("dissolve", clean, () => ChemicalHelper.DissolveInto(tab, clean));
                treatedOn[clean.uniqueId] = day;
                WaterTreatedMl += ml;
                haveMl += ml;
                empties.Add(raw);
                ep.Human("production", null, ep.Uid(clean), new JObject { ["action"] = "treat_water", ["ml"] = ml }, "player:water");
            }
            KeepForWine();
        }

        // Selling water: the cleanest containers up to the bottles' and bays' need stay home.
        static void KeepForWine() => Earmark();

        // The portable purifier lifts a container to exactly Basewater and updates its label: one charge
        // per 100 ml. Smallest vessels first (they sell to the most buyers).
        static void PortablePurify(IExecutorHost ep)
        {
            var unit = Owned().Where(i => i.identifier == "portable_water_purifier" && Parent(i) == null).OrderByDescending(DurabilityHelper.GetRealDurability).FirstOrDefault();
            if (unit == null) return;
            foreach (var c in Owned().Where(i => WaterVessel(i) && WaterHelper.IsEmpty(i)).OrderBy(i => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt()).ToList())
            {
                int cap = c.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt() / 1000;
                if (DurabilityHelper.GetRealDurability(unit) < (cap + 99) / 100) break;
                Chore.Act("tap", c, () => c.onThisTargetItemFunc?.Invoke(c, faucet));
                Chore.Act("purify", c, () => unit.onThisTargetItemFunc?.Invoke(unit, c));
                WaterTreatedMl += WaterHelper.GetCurrentCapacityML(c);
                ep.Human("production", null, ep.Uid(c), new JObject { ["action"] = "purify_water", ["ml"] = WaterHelper.GetCurrentCapacityML(c), ["purity"] = WaterHelper.GetWaterPurity(c) }, "player:water");
            }
        }
    }
}
