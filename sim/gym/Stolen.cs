using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `stolen: {…}` (docs/re/STOLEN.md): the Fence start's native trade, off unless the directives carry a `stolen` object.
    //   `legal_only` (default on): from a seller whose lot mixes contraband with other goods, stage the other goods
    //     (a thief's lot nearly always carries a keycard or a drug, and the whole seller used to be sent away).
    //   `clean_value` (default on): a stolen item on a seller's counter shows its "five finger" discount (-10/-15/-20/-25 %
    //     by heat) in its features and estimate; bought, the feature goes and the clean estimate is back. Lots are judged
    //     against that clean estimate; `single_max_pct` / `wholesale_max_pct` (defaults: the plan's) apply to it.
    //   `crime_cap` (default 0 = never): a stolen item still warm may be sold to any buyer but Security while the
    //     trafficking ledger plus its FENCING (estimate x heat %, halved for Black Market buyers, a third for the
    //     Revolution) stays at or under the cap. Under 50 the inspection countdown never starts; crimes fall 20 a night.
    //   `max_heat` (default 100): the hottest item `crime_cap` will sell.
    //   `save_stop` (default on): while saving for the buyout or selling down for rent, no lot with a stolen item is
    //     bought (it ties up cash for the nights it takes to cool).
    //   `blackmail` (default off): a Black Market seller with stolen goods on the counter is a criminal: blackmail him first
    //     (-20 % on his goods, no walk-out; a refusal is -5 rep, x2.5 for BM on hard, and a success turns the trade's rep
    //     multiplier to -6) when the shown odds are at least `blackmail_min_odds` % (default 60).
    //   `bm_bargain` (default "never"): bargaining with Black Market sellers; a walk-out costs BM rep twice over.
    // Everything read here is on screen: heat and the discount feature in the item's tooltip, the crimes in the record.
    internal static class StolenTrade
    {
        static JObject O => Directives.Current.Raw["stolen"] as JObject;
        public static bool On => O != null && (O.Value<bool?>("on") ?? true);
        static bool Opt(string k, bool d = true) => On && (O.Value<bool?>(k) ?? d);
        static int Num(string k, int d) => O?.Value<int?>(k) ?? d;

        public static int HotSales, HotSaleCrime, LegalStaged;

        public static void Reset() => HotSales = HotSaleCrime = LegalStaged = 0;

        static bool Contraband(GameItem i) => ContrabandHelper.GetContrabandLevelInInt(i) > 0 || ContrabandHelper.IsSeriousContraband(i);

        // From a mixed lot, the goods the shop can legally carry.
        public static IList<GameItem> Legal(IList<GameItem> picked)
        {
            if (!Opt("legal_only") || (Directives.Current.Raw["buy"]?.Value<bool?>("contraband") ?? false)) return picked;
            var legal = picked.Where(i => !Contraband(i) && !Lab.ContrabandInput(i.identifier)).ToList();
            if (legal.Count == 0 || legal.Count == picked.Count) return picked;
            LegalStaged++;
            return legal;
        }

        // The discount a seller's stolen item shows, in percent (0 when none).
        public static int DiscountPct(GameItem i)
        {
            if (i == null || !StolenHelper.IsStolenItem(i)) return 0;
            var f = i.itemFeatures.FirstOrDefault(x => !x.isDisabled && (x.category ?? "").StartsWith("stolenBonusBuying", StringComparison.Ordinal));
            return f == null ? 0 : -f.GetCurrentValueMod();
        }

        // What the item will be worth on the shelf once bought: the shown estimate without the five-finger discount.
        public static long CleanEst(GameItem i, long shown) => Opt("clean_value") ? Clean(i, shown) : shown;

        public static long Clean(GameItem i, long shown)
        {
            int d = DiscountPct(i);
            return d <= 0 || d >= 100 ? shown : (long)Math.Round(shown * 100.0 / (100 - d));
        }

        public static long CleanEst(IEnumerable<JToken> lot) => lot.Sum(t =>
        {
            long shown = t.Value<long?>("estimated_value") ?? 0;
            return CleanEst(PlayerStock.OnCounter(t.Value<int?>("uid") ?? -1), shown);
        });

        public static double? SingleMax => O?.Value<int?>("single_max_pct") is int p ? p / 100.0 : (double?)null;
        public static double? WholesaleMax => O?.Value<int?>("wholesale_max_pct") is int p ? p / 100.0 : (double?)null;

        public static bool Stops(IEnumerable<JToken> lot)
        {
            var day = PlayerDay.Current;
            return Opt("save_stop") && day != null && (day.SavingForBuyout || day.Liquidating) && lot.Any(t => (t.Value<int?>("stolen_heat") ?? 0) > 0);
        }

        public static bool Blackmails(JObject c, IEnumerable<JToken> staged, double odds)
        {
            if (!Opt("blackmail", false) || c["client"]?.Value<string>("faction") != "black_market") return false;
            if (c["bargain"]?.Value<bool?>("blackmail_used") ?? false) return false;
            return staged.Any(t => (t.Value<int?>("stolen_heat") ?? 0) > 0) && odds * 100 >= Num("blackmail_min_odds", 60);
        }

        public static bool AllStolen(IEnumerable<JToken> lot) => lot.Any() && lot.All(t => (t.Value<int?>("stolen_heat") ?? 0) > 0);

        public static string BargainBuy(string faction, string mode) =>
            On && faction == "black_market" ? O.Value<string>("bm_bargain") ?? "never" : mode;

        static int Divisor(StoreClient c) => c?.clientFaction == "FACTION_BLACK_MARKET" ? 2 : c?.clientFaction == "FACTION_REVOLUTION" ? 3 : 1;

        public static int Trafficking() => PlayerStore.Instance.secData?.GetTotalTraffickingCrimes() ?? 0;

        // A warm item may go to this buyer: the FENCING it writes keeps the trafficking ledger under the cap.
        public static bool SellsWarm(GameItem i, StoreClient c)
        {
            int cap = Num("crime_cap", 0);
            if (!On || cap <= 0 || c == null || c.isSecurity || !StolenHelper.IsStolenItem(i)) return false;
            int heat = PlayerRole.Heat(i);
            if (heat > Num("max_heat", 100)) return false;
            return Trafficking() + Crime(i, c) <= cap;
        }

        static int Crime(GameItem i, StoreClient c) => (int)Math.Round(PlayerRole.Est(i) * PlayerRole.Heat(i) / 100.0) / Divisor(c);

        // Of the warm items staged for one buyer, the ones that fit the cap together (the hottest-margin first).
        public static IList<GameItem> CapWarm(IList<GameItem> picked, StoreClient c)
        {
            if (!On || Num("crime_cap", 0) <= 0) return picked;
            int room = Num("crime_cap", 0) - Trafficking();
            var keep = new List<GameItem>();
            foreach (var i in picked)
            {
                if (!StolenHelper.IsStolenItem(i)) { keep.Add(i); continue; }
                int crime = Crime(i, c);
                if (crime > room) continue;
                room -= crime;
                keep.Add(i);
            }
            return keep;
        }

        public static void Sold(GameItem i, StoreClient c)
        {
            if (!On || i == null || !StolenHelper.IsStolenItem(i)) return;
            HotSales++;
            HotSaleCrime += Crime(i, c);
        }

        public static JObject Status() => On ? new JObject
        {
            ["warm_sales"] = HotSales, ["warm_sale_crime"] = HotSaleCrime, ["legal_staged"] = LegalStaged, ["trafficking"] = Trafficking(),
        } : null;
    }
}
