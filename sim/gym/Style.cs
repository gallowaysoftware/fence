using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Trading styles (docs/research/TRADING-STYLES.md): how the shop chooses what to buy and when to sell, on top
    // of the executor's counter policy. All off unless the directives name them.
    //   `specialty: {types, lot_share, buy_at_estimate}`: buy only goods of these types (a wholesale lot only when
    //     they make up `lot_share` of its estimate, default 0.5), stage only them from a mixed seller, and fill the
    //     display case from them.
    //   `speculate: {types, ul, events, max_days, buy_at_estimate}`: goods of these types are held back from
    //     ordinary buyers and sold to an Upper Levels buyer once their tips are unlocked (`ul`, default true), while
    //     a market event lifts their price (`events`, default true), or after `max_days` in stock (default 21);
    //     a shop short of cash sells them like any stock.
    //   `perfume: {buyers, sellers, min_sale, min_lot, buy_at_estimate}`: neuroactive perfume is kept as a tool and
    //     sprayed on a buyer (+15 % on what is staged, while that stays within the budget) and on a seller (−15 % on
    //     every item they bring, so on the wholesale price too) when the trade is worth it (AmenitiesItemDirectory,
    //     ItemFeatureList.NeuroActivePerfume*).
    internal static class Style
    {
        static Directives D => Directives.Current;

        static List<string> Types(JObject o) => o?["types"] is JArray a && a.Count > 0 ? a.Select(x => (string)x).Where(x => x != null).ToList() : null;

        public static List<string> SpecialtyTypes => Types(D.Raw["specialty"] as JObject);

        public static bool IsSpecialty(GameItem i) => i != null && SpecialtyTypes is List<string> t && PlayerRole.TypeIs(i, t);

        public static bool SpecialtyCarries(JArray lot)
        {
            double share = (D.Raw["specialty"] as JObject)?.Value<double?>("lot_share") ?? 0.5;
            long all = 0, mine = 0;
            foreach (var i in lot)
            {
                long v = i.Value<long?>("estimated_value") ?? 0;
                all += v;
                if (IsSpecialty(PlayerStock.OnCounter(i.Value<int?>("uid") ?? -1))) mine += v;
            }
            return all > 0 && mine >= all * share;
        }

        // Types the style buys up to their estimate (as `buy.prefer_types`).
        public static IEnumerable<string> BuyTypes()
        {
            if (Perfume != null && (Perfume.Value<bool?>("buy_at_estimate") ?? true)) yield return "neuroactive_perfume";
            foreach (var key in new[] { "specialty", "speculate" })
                if (D.Raw[key] is JObject o && (o.Value<bool?>("buy_at_estimate") ?? false) && Types(o) is List<string> t)
                    foreach (var x in t) yield return x;
        }

        static JObject Perfume => D.Raw["perfume"] as JObject;

        public static bool PerfumeOn => Perfume != null;

        public static bool IsPerfume(GameItem i) => i != null && (i.identifier == "neuroactive_perfume" || i.identifier == "self_refilling_neuroactive_perfume");

        public static bool KeepsPerfume(GameItem i) => Perfume != null && IsPerfume(i);

        // The bottle to spray on this client, or null: `value` is what the spray moves (the staged goods' price),
        // `limit` the most it may come to (a buyer's budget; long.MaxValue for none).
        public static GameItem PerfumeFor(StoreClient c, IEnumerable<GameItem> owned, bool buyer, long value, long limit)
        {
            var o = Perfume;
            if (o == null || c == null) return null;
            if (!(o.Value<bool?>(buyer ? "buyers" : "sellers") ?? true)) return null;
            if (value < (o.Value<int?>(buyer ? "min_sale" : "min_lot") ?? (buyer ? 200 : 300))) return null;
            if (buyer && value * 115 / 100 > limit) return null;
            // The self-refilling bottle first: its uses come back.
            return owned.Where(i => IsPerfume(i) && GeneralHelper.IsItemOwned(i) && UseCountHelper.CanUse(i))
                .OrderBy(i => i.identifier == "self_refilling_neuroactive_perfume" ? 0 : 1).ThenBy(i => i.uniqueId).FirstOrDefault();
        }

        // A seller is sprayed only when the lot, 15 % cheaper, is within the plan's buying limits (the executor's
        // hard defaults otherwise).
        public static bool SprayWorthIt(double price, long est, bool wholesale) =>
            price * 0.85 <= est * ((wholesale ? D.WholesaleMaxPct ?? 95 : D.SingleMaxPct ?? 85) / 100.0);

        // The first day each speculative item was weighed for a buyer: its time in stock as the player counts it.
        static readonly Dictionary<int, int> seen = new Dictionary<int, int>();

        public static bool Speculating(GameItem i, StoreClient c)
        {
            if (!(D.Raw["speculate"] is JObject sp) || !(Types(sp) is List<string> types) || !PlayerRole.TypeIs(i, types)) return false;
            var p = PlayerDay.Current;
            if (p == null || p.Liquidating) return false;
            if (!seen.TryGetValue(i.uniqueId, out int first)) seen[i.uniqueId] = first = p.Day;
            if (p.Day - first >= (sp.Value<int?>("max_days") ?? 21)) return false;
            if ((sp.Value<bool?>("ul") ?? true) && c != null && c.clientFaction == "FACTION_UPPER_LEVEL" && StoreReputation.IsPerkUnlocked("UL_TIPS")) return false;
            if ((sp.Value<bool?>("events") ?? true) && i.GetCurrentValue(false, true, true, false, 0L) > i.GetCurrentValue(false, true, false, false, 0L)) return false;
            return true;
        }
    }
}
