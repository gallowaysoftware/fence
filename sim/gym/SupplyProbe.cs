using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Gym-only demand probe, not a player move: `supply_probe: {"caffeine_pill": {"per_day": 12, "purity": 99.5}}` puts
    // that many free, owned units into the stock grid each morning from `supply_probe_from` (default 30). What the counter
    // then sells is the ceiling an engine making that good could earn at this shop (docs/research/SECOND-ENGINE.md).
    // `supply_probe_capex` and `supply_probe_cost_per_day` charge it like an engine (docs/research/PRODUCTION-VERDICTS.md):
    // the probe starts on the first day from `supply_probe_from` when cash covers the capex, the rent due and 500, pays
    // the capex then, and pays the daily cost every morning after (as far as the cash goes). `supply_probe_cash_per_day`
    // credits a deterministic night machine's takings (the water fountain) each morning after the start.
    internal static class SupplyProbe
    {
        static JObject Plan => Directives.Current.Raw["supply_probe"] as JObject;

        public static long Injected, InjectedValue, Sold, Sales, Paid;
        public static int StartDay;
        static readonly HashSet<int> mine = new HashSet<int>();

        public static void Reset()
        {
            Injected = InjectedValue = Sold = Sales = Paid = 0;
            StartDay = 0;
            mine.Clear();
        }

        public static void Morning(int day)
        {
            if (Plan == null || day < (Directives.Current.Raw.Value<int?>("supply_probe_from") ?? 30)) return;
            var store = PlayerStore.Instance;
            if (StartDay == 0)
            {
                int capex = Directives.Current.Raw.Value<int?>("supply_probe_capex") ?? 0;
                int due = PlayerDay.Current?.RentDue ?? store.GetRentValue();
                if (capex > 0 && store.playerCash < capex + due + 500) return;
                store.playerCash -= capex;
                Paid += capex;
                StartDay = day;
            }
            else
            {
                int cost = Math.Min(Math.Max(0, store.playerCash), Directives.Current.Raw.Value<int?>("supply_probe_cost_per_day") ?? 0);
                store.playerCash -= cost;
                Paid += cost;
                int income = Directives.Current.Raw.Value<int?>("supply_probe_cash_per_day") ?? 0;
                store.playerCash += income;
                Sales += income;
            }
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var p in Plan.Properties().OrderBy(p => p.Name, StringComparer.Ordinal))
            {
                int n = p.Value.Value<int?>("per_day") ?? 0;
                double? purity = p.Value.Value<double?>("purity");
                for (int k = 0; k < n; k++)
                {
                    var it = DirectoryMaster.Item(p.Name);
                    if (purity is double pct && it.IsTag("CHEMICAL_PRODUCT_TAG")) SetPurity(it, pct);
                    if (p.Value.Value<int?>("wine_score") is int score && it.identifier == "wine_bottle") FinishWine(it, score);
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

        // A finished bottle of the given fermentation score (WineHelper.FinishFermentation: 6 Master Vintage, 5 Reserve).
        static void FinishWine(GameItem it, int score)
        {
            it.ModifyTag("WINE_SCORE_INT", t => t.Enable().SetInt(score));
            WineHelper.FinishFermentation(it);
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!sold || item == null || !mine.Contains(item.uniqueId)) return;
            Sold++;
            Sales += amount;
        }

        public static JObject Totals() => Plan == null ? null : new JObject { ["injected"] = Injected, ["injected_value"] = InjectedValue, ["sold"] = Sold, ["sales"] = Sales, ["paid"] = Paid, ["start_day"] = StartDay };
    }
}
