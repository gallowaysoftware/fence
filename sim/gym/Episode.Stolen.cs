using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The stolen-goods trade as the summary reports it (gym only; docs/re/STOLEN.md): every item bought while it carried
    // the stolen tag, followed until it leaves the shop by a counter sale, the vending machine, or a loss (seizure,
    // burglary, the front counter's nightly wipe).
    internal static class StolenLedger
    {
        sealed class Lot
        {
            public GameItem Item;
            public int Paid, Heat, Est, Day;
            public string Seller, Parent;
        }

        static readonly Dictionary<int, Lot> lots = new Dictionary<int, Lot>();
        static int bought, paid, estBought, heatBought, sold, soldFor, soldCost, soldHot, soldHotFor, heldDays;
        static int vend, vendEst, vendCost, lost, lostCost, inspections, failed, seized, offers, offersEst, offersLegal, offersLegalEst;
        static readonly SortedDictionary<string, int[]> declinedBy = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        static readonly List<string> hotExamples = new List<string>();
        static readonly int[] soldByHeat = new int[5], boughtByHeat = new int[5];
        static readonly SortedDictionary<string, int[]> byBuyer = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        static readonly SortedDictionary<string, int[]> bySeller = new SortedDictionary<string, int[]>(StringComparer.Ordinal);

        public static void Reset()
        {
            lots.Clear();
            bought = paid = estBought = heatBought = sold = soldFor = soldCost = soldHot = soldHotFor = heldDays = 0;
            vend = vendEst = vendCost = lost = lostCost = inspections = failed = seized = offers = offersEst = offersLegal = offersLegalEst = 0;
            hotExamples.Clear();
            declinedBy.Clear();
            Array.Clear(soldByHeat, 0, 5);
            Array.Clear(boughtByHeat, 0, 5);
            byBuyer.Clear();
            bySeller.Clear();
        }

        static int Band(int heat) => heat <= 0 ? 0 : Math.Min(4, (heat + 24) / 25);
        static int Day => StoreStation.GetDayCounter();

        static void Add(SortedDictionary<string, int[]> d, string k, int value)
        {
            if (!d.TryGetValue(k ?? "?", out var v)) d[k ?? "?"] = v = new int[2];
            v[0]++;
            v[1] += value;
        }

        // A client leaving: the stolen goods still on the counter were offered and not bought.
        public static void Declined()
        {
            foreach (var i in EmporiumEntry.Instance.GetAllNonOwnedItem().Where(i => i != null && StolenHelper.IsStolenItem(i)))
            {
                offers++;
                offersEst += (int)PlayerRole.Est(i);
                if (ContrabandHelper.GetContrabandLevelInInt(i) > 0) continue;
                var c = PlayerStore.Instance.currentClientInstance?.GetClientBlueprint();
                if (!declinedBy.TryGetValue(c?.identifier ?? "?", out var v)) declinedBy[c?.identifier ?? "?"] = v = new int[3];
                v[0]++;
                v[1] += (int)i.GetNegociatedValue();
                v[2] += (int)StolenTrade.Clean(i, PlayerRole.Est(i));
                offersLegal++;
                offersLegalEst += (int)PlayerRole.Est(i);
            }
        }

        public static void Bought(GameItem i, int cost)
        {
            if (i == null || !StolenHelper.IsStolenItem(i) || lots.ContainsKey(i.uniqueId)) return;
            var c = PlayerStore.Instance.currentClientInstance?.GetClientBlueprint();
            int heat = PlayerRole.Heat(i);
            lots[i.uniqueId] = new Lot { Item = i, Paid = cost, Heat = heat, Est = (int)PlayerRole.Est(i), Day = Day, Seller = c?.identifier };
            bought++;
            paid += cost;
            estBought += (int)PlayerRole.Est(i);
            heatBought += heat;
            boughtByHeat[Band(heat)]++;
            Add(bySeller, c?.identifier, cost);
        }

        public static void Sold(StoreClient c, GameItem i, int cost)
        {
            if (i == null || !lots.TryGetValue(i.uniqueId, out var lot)) return;
            lots.Remove(i.uniqueId);
            int heat = PlayerRole.Heat(i);
            StolenTrade.Sold(i, c);
            sold++;
            soldFor += cost;
            soldCost += lot.Paid;
            heldDays += Day - lot.Day;
            soldByHeat[Band(heat)]++;
            if (heat > 0)
            {
                if (hotExamples.Count < 12) hotExamples.Add($"day {Day} {i.identifier} heat {heat} to {c?.identifier} for {cost}{(i.parentInventory?.GetParentItem() is GameItem p ? " in " + p.identifier : "")}");
                soldHot++;
                soldHotFor += cost;
            }
            Add(byBuyer, c?.clientFaction, cost);
        }

        public static void Inspection(bool passed, int seizedValue)
        {
            inspections++;
            if (!passed) failed++;
            seized += seizedValue;
        }

        static HashSet<int> OwnedUids() => new HashSet<int>(EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null).Select(i => i.uniqueId));

        // Before the night: where each lot sits (the vending machine sells overnight).
        public static void Evening()
        {
            foreach (var lot in lots.Values) lot.Parent = lot.Item.parentInventory?.GetParentItem()?.identifier;
        }

        // The morning after: lots no longer owned left by the vending machine or were lost.
        public static void Morning()
        {
            var owned = OwnedUids();
            foreach (var kv in lots.Where(kv => !owned.Contains(kv.Key)).ToList())
            {
                lots.Remove(kv.Key);
                if ((kv.Value.Parent ?? "").IndexOf("vending", StringComparison.OrdinalIgnoreCase) >= 0)
                {
                    vend++;
                    vendEst += kv.Value.Est;
                    vendCost += kv.Value.Paid;
                }
                else
                {
                    lost++;
                    lostCost += kv.Value.Paid;
                }
            }
        }

        static JObject Map(SortedDictionary<string, int[]> d)
        {
            var o = new JObject();
            foreach (var kv in d) o[kv.Key] = new JArray(kv.Value[0], kv.Value[1]);
            return o;
        }

        public static JObject Json()
        {
            if (bought == 0 && offers == 0 && inspections == 0) return null;
            var held = lots.Values.ToList();
            return new JObject
            {
                ["declined"] = offers, ["declined_est"] = offersEst, ["declined_legal"] = offersLegal, ["declined_legal_est"] = offersLegalEst, ["declined_by"] = JObject.FromObject(declinedBy),
                ["bought"] = bought, ["paid"] = paid, ["est_bought"] = estBought, ["mean_heat_bought"] = bought == 0 ? 0 : Math.Round((double)heatBought / bought, 1),
                ["bought_by_heat"] = new JArray(boughtByHeat), ["by_seller"] = Map(bySeller),
                ["sold"] = sold, ["sold_for"] = soldFor, ["sold_cost"] = soldCost, ["sold_hot"] = soldHot, ["sold_hot_for"] = soldHotFor,
                ["sold_by_heat"] = new JArray(soldByHeat), ["mean_days_held"] = sold == 0 ? 0 : Math.Round((double)heldDays / sold, 1), ["by_buyer"] = Map(byBuyer),
                ["vended"] = vend, ["vended_est"] = vendEst, ["vended_cost"] = vendCost,
                ["lost"] = lost, ["lost_cost"] = lostCost,
                ["held_at_end"] = held.Count, ["held_cost"] = held.Sum(l => l.Paid), ["held_est"] = held.Sum(l => (int)PlayerRole.Est(l.Item)),
                ["margin"] = soldFor - soldCost + vendEst - vendCost,
                ["inspections"] = inspections, ["inspections_failed"] = failed, ["seized_value"] = seized,
                ["engine"] = StolenTrade.Status(),
                ["hot_sales"] = new JArray(hotExamples),
            };
        }
    }

    internal sealed partial class Episode
    {
        void RegisterStolenHooks()
        {
            StolenLedger.Reset();
            Hooks.On("PlayerStore::OnItemBought", (self, a) => { if (IsPlayer) StolenLedger.Bought(a[0] as GameItem, a.Length > 1 && a[1] is int c ? c : 0); });
            Hooks.On("StoreClient::OnItemBought", (self, a) => { if (IsPlayer) StolenLedger.Sold(self as StoreClient, a[0] as GameItem, a.Length > 1 && a[1] is int c ? c : 0); });
            Hooks.On("PlayerStore::DismissCurrentClient", (self, a) => { if (IsPlayer) StolenLedger.Declined(); });
            Hooks.On("RunStats::OnInspection", (self, a) => { if (IsPlayer) StolenLedger.Inspection(a[0] is bool p && p, a.Length > 1 && a[1] is int v ? v : 0); });
            Hooks.On("PlayerStore::EndDay", (self, a) => { if (IsPlayer) StolenLedger.Evening(); });
            Hooks.On("PlayerStore::BeginDay", (self, a) => { if (IsPlayer) StolenLedger.Morning(); });
        }
    }
}
