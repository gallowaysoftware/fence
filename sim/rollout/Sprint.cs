using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Gym;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    internal sealed class SprintConfig
    {
        // A buyout on or before this day wins; branches roll to it, so every label is the format's own outcome.
        public int Deadline = 91;
        // A challenger replaces the plan in force only when its paired gain is this many standard errors clear.
        public double Gate = 2.0;
        // Rule policy (`-rules FILE`): forks are answered by fitted rules on the morning's features, no rollouts.
        public JArray Rules;
    }

    // Deadline sprints (docs/research/SEARCH-LABELS.md): the forks a day-91 cash buyout turns on, their
    // candidate directive changes, the format-aware score and the visible features a rule can read.
    internal static class Sprint
    {
        // Fork days. Rent falls on multiples of 7, and a buyout can only happen on a rent morning, so the
        // selldown forks sit one rent ahead of the buyouts they aim at (84 and 91) and on the eve of 91.
        static readonly (int day, string fork)[] Schedule =
        {
            (28, "style"), (42, "save"), (56, "save"), (70, "save"), (77, "selldown"), (84, "selldown"), (88, "selldown"),
        };

        public static string ForkAt(int day) => Schedule.FirstOrDefault(s => s.day == day).fork;

        static JObject J(string json) => JObject.Parse(json);

        // The options at a fork, as deltas over the plan in force. The model-agency forks (internal/strategy/forks.go)
        // that fit a day-91 deadline are reused: the sprint fork (insured / sprint / all-in) and the style fork (the
        // styles the lab found as fast as the generalist). The pilot also tried Georgio's loan (lend at the peak, lend 25 %). Its mortgage fork does not fit (16 payments end long after
        // day 91), nor its engine and fountain forks (the sprinter already runs wine + furnace, and no water).
        public static List<(string name, JObject delta)> Options(string fork, int day, long price = 0)
        {
            var l = new List<(string, JObject)>();
            switch (fork)
            {
                case "style":
                    l.Add(("fair_dealer", J("{\"bargain\":\"never\",\"bargain_buy\":\"never\"}")));
                    l.Add(("clean_shop", J("{\"buy\":{\"hot_stock_cap\":0}}")));
                    break;
                case "save":
                    // `buyout_save: ["insurance", "upgrades"]` from 56 plays byte-identical to the route (screened on
                    // 8700001-060: the upgrades are bought by then), so from day 56 only the deeper cut remains.
                    l.Add(("insured", J("{\"buyout_save_from\":999}")));
                    if (day < 56)
                    {
                        l.Add(("save_now", J($"{{\"buyout_save_from\":{day}}}")));
                        l.Add(("all_in", J($"{{\"buyout_save\":[\"insurance\",\"upgrades\"],\"buyout_save_from\":{day}}}")));
                    }
                    l.Add(("save_all", J($"{{\"buyout_save\":[\"insurance\",\"upgrades\",\"vending\",\"inputs\"],\"buyout_save_from\":{Math.Min(day, 56)}}}")));
                    break;
                case "selldown":
                    // The executor sells down for the buyout only once cash + 70 % of stock covers today's price
                    // (ExecutorMorning.Plan), and `buyout_selldown_days` alone changed nothing on 8700001-060; these
                    // force the sell-down toward the deadline's price instead (it covers the earlier rents too).
                    l.Add(("force_selldown", J($"{{\"sell\":{{\"sell_down_to_cash\":{price + 100}}}}}")));
                    l.Add(("force_selldown_liq60", J($"{{\"sell\":{{\"sell_down_to_cash\":{price + 100},\"liquidate_min_pct\":60}}}}")));
                    l.Add(("hold", J("{\"buyout\":\"hold_cash_for_buyout\"}")));
                    l.Add(("no_buy", J("{\"buy\":{\"max_spend\":0}}")));
                    break;
            }
            return l.Where(o => !Dropped.Contains(o.Item1)).ToList();
        }

        // Options the 20-game pilot (8700001-020, 8 samples) found only losing against keep, paired samples won/lost:
        // fair_dealer +11/-15, insured +4/-33, no_buy +4/-13; force_selldown_liq60 played exactly as force_selldown.
        // The pilot's georgio fork (day 38: lend at the peak +7/-11, lend 25 % +2/-31) is gone with them.
        static readonly HashSet<string> Dropped = new HashSet<string> { "fair_dealer", "insured", "no_buy", "force_selldown_liq60" };

        // `keep` (the plan in force) first, then each option merged over it; an option that changes nothing is dropped.
        public static List<Candidate> Candidates(string fork, int day, JObject inForce, long price)
        {
            var keep = (JObject)(inForce ?? new JObject()).DeepClone();
            var l = new List<Candidate> { new Candidate { Name = "keep", Source = "keep", Directives = keep } };
            foreach (var (name, delta) in Options(fork, day, price))
            {
                var d = Over(keep, delta);
                if (l.Any(c => JToken.DeepEquals(c.Directives, d))) continue;
                l.Add(new Candidate { Name = name, Source = fork, Directives = d });
            }
            return l;
        }

        public static JObject Over(JObject under, JObject delta)
        {
            var d = (JObject)under.DeepClone();
            d.Merge(delta, new JsonMergeSettings { MergeArrayHandling = MergeArrayHandling.Replace });
            return d;
        }

        public const string ScoreDoc = "deadline sprint: a buyout on or before the deadline scores 10000 + 25 * days early; " +
            "alive and unbought at the deadline 4000 * min(1.2, net worth / buyout price); any other ending 0";

        public static bool Won(JObject r, int deadline) =>
            r["crash"] == null && ((string)r["game_over"] ?? "").StartsWith("buyout", StringComparison.Ordinal) && (r.Value<int?>("last_day") ?? int.MaxValue) <= deadline;

        public static double Score(JObject r, int deadline)
        {
            if (r["crash"] != null) return -1000;
            if (Won(r, deadline)) return 10000 + 25.0 * (deadline - r.Value<int>("last_day"));
            if (r["game_over"]?.Type == JTokenType.String) return 0;
            return 4000 * Math.Min(1.2, r.Value<double>("nw_end") / Math.Max(1, r.Value<double>("buyout_price")));
        }

        // What a player sees on a fork morning, as numbers a rule can threshold. `proj_gap` is the executor's own
        // cash-buyout projection (PlayerRole.CashBuyoutBy) carried to the deadline, less the price on that day.
        // The buyout price on day d (DifficultyRules: base + the daily rise after day 50, capped).
        public static long PriceOn(int d)
        {
            var r = Difficulty.Rules;
            return r == null ? PlayerStore.Instance.GetStoreBuyPrice(false) : r.propertyBasePrice + Math.Min((long)r.propertyPriceIncreaseCap, (long)r.propertyPriceIncreasePerDay * Math.Max(0, d - 50));
        }

        public static JObject Features(Episode ep, int deadline)
        {
            var ps = PlayerStore.Instance;
            int day = ep.Day, cash = ep.Cash;
            long stock = PlayerStock.Value();
            var nw = ep.Stats.NetWorthByDay;
            double Gain(int k) => nw.Count > k ? (nw[^1] - nw[^(k + 1)]) * 7.0 / k : 0;
            long price = PriceOn(deadline);
            double gap = cash + 0.7 * stock - price, gain4 = Gain(28);
            bool insured = ps.storeService.Any(s => s.unlocked && s.isUsing && s.id == "INSURANCE_SERVICE");
            return new JObject
            {
                ["day"] = day, ["cash"] = cash, ["stock"] = stock, ["net_worth"] = nw.Count > 0 ? nw[^1] : 0,
                ["price_deadline"] = price, ["gap"] = Math.Round(gap), ["gain_week4"] = Math.Round(gain4), ["gain_week2"] = Math.Round(Gain(14)),
                ["proj_gap"] = Math.Round(gap + Math.Max(0, gain4) * (deadline - day) / 7.0),
                ["cash_gap"] = cash - price, ["stock_share"] = cash + stock > 0 ? Math.Round((double)stock / (cash + stock), 3) : 0,
                ["rent"] = ps.GetRentValue(), ["days_to_rent"] = ps.dayUntilRent, ["insured"] = insured ? 1 : 0,
                ["evidence"] = ps.secData?.evidenceLevel ?? 0, ["georgio_lent"] = ps.loanOfferedToGeorgio,
            };
        }

        // Rules: [{"fork": "selldown", "day": 84, "option": "selldown_7", "when": [["proj_gap", ">=", -1500], ...]}]; every
        // rule that fires at today's fork merges its option over the plan in force, in file order.
        public static JArray LoadRules(string path) => JArray.Parse(File.ReadAllText(path));

        public static List<string> Fire(JArray rules, string fork, int day, JObject features)
        {
            var fired = new List<string>();
            foreach (JObject rule in rules)
            {
                if ((string)rule["fork"] != fork || (rule["day"] != null && (int)rule["day"] != day)) continue;
                bool ok = (rule["when"] as JArray ?? new JArray()).All(c =>
                {
                    double v = features.Value<double>((string)c[0]), x = (double)c[2];
                    return (string)c[1] switch { ">=" => v >= x, ">" => v > x, "<" => v < x, "<=" => v <= x, var op => throw new ArgumentException("rule op " + op) };
                });
                if (ok) fired.Add((string)rule["option"]);
            }
            return fired;
        }
    }
}
