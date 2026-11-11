using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Gym-only demand probe, not a player move: `supply_probe: {"caffeine_pill": {"per_day": 12, "purity": 99.5}}` puts
    // that many free, owned units into the stock grid each morning from `supply_probe_from` (default 30). What the counter
    // then sells is the ceiling an engine making that good could earn at this shop (docs/research/SECOND-ENGINE.md).
    internal static class SupplyProbe
    {
        static JObject Plan => Directives.Current.Raw["supply_probe"] as JObject;

        public static long Injected, InjectedValue, Sold, Sales;
        static readonly HashSet<int> mine = new HashSet<int>();

        public static void Reset()
        {
            Injected = InjectedValue = Sold = Sales = 0;
            mine.Clear();
        }

        public static void Morning(int day)
        {
            if (Plan == null || day < (Directives.Current.Raw.Value<int?>("supply_probe_from") ?? 30)) return;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var p in Plan.Properties().OrderBy(p => p.Name, StringComparer.Ordinal))
            {
                int n = p.Value.Value<int?>("per_day") ?? 0;
                double? purity = p.Value.Value<double?>("purity");
                for (int k = 0; k < n; k++)
                {
                    var it = DirectoryMaster.Item(p.Name);
                    if (purity is double pct && it.IsTag("CHEMICAL_PRODUCT_TAG")) SetPurity(it, pct);
                    GeneralHelper.SetItemOwned(it, true);
                    if (GraphUtils.TryAcceptAll(stock, it, -1) != 0) { it.Destroy(); break; }
                    mine.Add(it.uniqueId);
                    Injected++;
                    InjectedValue += PlayerRole.Est(it);
                }
            }
        }

        // The active liquid at `pct` of the unit's volume, the rest physical contaminant (the finisher's impurity).
        static void SetPurity(GameItem it, double pct)
        {
            string active = it.GetTagReadonly("ACTIVATE_INGREDIENT_TAG").GetString();
            int total = ChemicalHelper.GetTotalVolume(it);
            if (total <= 0) total = 5000;
            foreach (var l in Liquid.Liquids)
            {
                string tag = Liquid.GetCurrentPartFromId(l.identifier);
                int v = l.identifier == active ? (int)Math.Round(total * pct / 100) : l.identifier == "physical_contaminant" ? total - (int)Math.Round(total * pct / 100) : 0;
                it.ModifyTag(tag, s => s.Enable().SetInt(v));
            }
            ChemicalProductHelper.GradeProduct(it);
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!sold || item == null || !mine.Contains(item.uniqueId)) return;
            Sold++;
            Sales += amount;
        }

        public static JObject Totals() => Plan == null ? null : new JObject { ["injected"] = Injected, ["injected_value"] = InjectedValue, ["sold"] = Sold, ["sales"] = Sales };
    }
}
