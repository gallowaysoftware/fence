using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The strategist's plan as the executor reads it (docs/PROTOCOL.md "Directives",
    // protocol/fixtures/directives.json). Every field is optional: null means "the executor's own rule".
    // Fields beyond the protocol's (marked gym) are executor extensions a planner may also set.
    internal sealed partial class Directives
    {
        // Per load context, like Knobs.Current: the plan in force for the day being played.
        public static Directives Current = new Directives();

        public int? Day;
        public int? CashFloor;

        public int? WholesaleMaxPct, SingleMaxPct, MaxSpend, HotStockCap;
        public List<string> PreferTypes = new List<string>(), AvoidTypes = new List<string>();

        public int? MinPct, SellDownToCash, LiquidateMinPct;
        public HashSet<int> HoldUids = new HashSet<int>();
        // gym: items the plan wants sold even though an engine would hold them.
        public HashSet<int> ReleaseUids = new HashSet<int>();

        // never | discount (gym) | ev_positive | aggressive
        public string Bargain;

        public List<int> ShowUids;
        public bool? AvoidStolen;

        public List<string> Services;
        public List<string> UpgradesBuy;
        // buy | refuse | bribe
        public string Permit;
        public JArray Production;
        // shoot | serve
        public string Augs;
        public bool? Scavenge;

        // gym: stop buying while stock (at estimate) exceeds this; production engine (wine | none);
        // sell_hot: stage stolen goods for buyers (false holds them until heat 0).
        public int? StockCap, RestockBelow, UpgradeReserve;
        public string Engine;
        public bool? SellHot;

        public JObject Raw = new JObject();

        static int? Int(JToken t) => t == null || t.Type == JTokenType.Null ? (int?)null : (int)Math.Round((double)t, MidpointRounding.AwayFromZero);
        static bool? Bool(JToken t) => t == null || t.Type == JTokenType.Null ? (bool?)null : (bool)t;
        static string Str(JToken t) => t == null || t.Type == JTokenType.Null ? null : (string)t;
        static List<string> Strs(JToken t) => t is JArray a ? a.Select(x => (string)x).Where(x => x != null).ToList() : null;
        static List<int> Ints(JToken t) => t is JArray a ? a.Select(x => (int)x).ToList() : null;

        public static Directives Parse(JObject o)
        {
            var d = new Directives { Raw = o ?? new JObject() };
            if (o == null) return d;
            d.Day = Int(o["day"]);
            d.CashFloor = Int(o["cash_floor"]);
            if (o["buy"] is JObject b)
            {
                d.WholesaleMaxPct = Int(b["wholesale_max_pct"]);
                d.SingleMaxPct = Int(b["single_max_pct"]);
                d.MaxSpend = Int(b["max_spend"]);
                d.HotStockCap = Int(b["hot_stock_cap"]);
                d.PreferTypes = Strs(b["prefer_types"]) ?? d.PreferTypes;
                d.AvoidTypes = Strs(b["avoid_types"]) ?? d.AvoidTypes;
                d.StockCap = Int(b["stock_cap"]);
                d.RestockBelow = Int(b["restock_below"]);
            }
            if (o["sell"] is JObject s)
            {
                d.MinPct = Int(s["min_pct"]);
                d.SellDownToCash = Int(s["sell_down_to_cash"]);
                d.LiquidateMinPct = Int(s["liquidate_min_pct"]);
                d.HoldUids = new HashSet<int>(Ints(s["hold_uids"]) ?? new List<int>());
                d.ReleaseUids = new HashSet<int>(Ints(s["release_uids"]) ?? new List<int>());
                d.SellHot = Bool(s["sell_hot"]);
            }
            d.Bargain = Str(o["bargain"]);
            if (o["window"] is JObject w)
            {
                d.ShowUids = Ints(w["show_uids"]);
                d.AvoidStolen = Bool(w["avoid_stolen"]);
            }
            d.Services = Strs(o["services"])?.Select(ServiceId).ToList();
            d.UpgradesBuy = Strs(o["upgrades_buy"]);
            d.Permit = Str(o["permit"]);
            d.Production = o["production"] as JArray;
            d.Augs = Str(o["augs"]);
            d.Scavenge = Bool(o["scavenge"]);
            d.StockCap ??= Int(o["stock_cap"]);
            d.Engine = o["engine"] is JArray engines ? string.Join("+", engines.Select(x => x.ToString())) : Str(o["engine"]);
            d.UpgradeReserve = Int(o["upgrade_reserve"]);
            if (d.Bargain != null && !new[] { "never", "discount", "ev_positive", "aggressive" }.Contains(d.Bargain)) throw new ArgumentException("directives.bargain=" + d.Bargain + " (never|discount|ev_positive|aggressive)");
            if (d.Permit != null && !new[] { "buy", "refuse", "bribe" }.Contains(d.Permit)) throw new ArgumentException("directives.permit=" + d.Permit + " (buy|refuse|bribe)");
            if (d.Augs != null && d.Augs != "shoot" && d.Augs != "serve") throw new ArgumentException("directives.augs=" + d.Augs + " (shoot|serve)");
            if (d.Engine != null && d.Engine.Split('+').Any(e => e != "wine" && e != "water" && e != "furnace" && e != "none")) throw new ArgumentException("directives.engine=" + d.Engine + " (wine|water|furnace joined by +, or a list of them; none)");
            return d;
        }

        // The fixture says INSURANCE / JANITORIAL; the game's ids end in _SERVICE (except a few, e.g. WILD_FIXER).
        static readonly string[] Bare = { "SEC_INFORMANT", "WILD_FIXER", "BUY_REFERAL", "SELL_REFERAL", "CRIMINEL_NETWORK", "CARTEL_PROTECTION" };

        public static string ServiceId(string s)
        {
            s = (s ?? "").Trim().ToUpperInvariant();
            return s.EndsWith("_SERVICE") || Bare.Contains(s) ? s : s + "_SERVICE";
        }

        // `over` wins field by field (a day's plan over the run's base file).
        public static Directives Merge(Directives under, Directives over)
        {
            if (under == null) return over ?? new Directives();
            if (over == null) return under;
            var raw = (JObject)under.Raw.DeepClone();
            raw.Merge(over.Raw, new JsonMergeSettings { MergeArrayHandling = MergeArrayHandling.Replace, MergeNullValueHandling = MergeNullValueHandling.Merge });
            return Parse(raw);
        }

        // Knobs (-human knobs vocabulary) seed the directives the executor starts from, so the old
        // experiment arms keep working on the player: bargain, max_heat, sell_hot, permit, ...
        public static Directives FromKnobs(Knobs k)
        {
            var o = new JObject();
            if (k.Has("bargain"))
                o["bargain"] = k.S("bargain", null) switch { "ev" => "ev_positive", "aggr" => "aggressive", var v => v };
            if (k.Has("permit")) o["permit"] = k.S("permit", null);
            var sell = new JObject();
            if (k.Has("sell_hot")) sell["sell_hot"] = k.B("sell_hot", true);
            if (sell.HasValues) o["sell"] = sell;
            var buy = new JObject();
            if (k.Has("stock_cap")) buy["stock_cap"] = k.I("stock_cap", 0);
            if (buy.HasValues) o["buy"] = buy;
            if (k.Has("scav")) o["scavenge"] = k.S("scav", "always") != "never";
            return Parse(o);
        }

        public JObject ToJson() => (JObject)Raw.DeepClone();

        public override string ToString() => Raw.ToString(Newtonsoft.Json.Formatting.None);
    }
}
