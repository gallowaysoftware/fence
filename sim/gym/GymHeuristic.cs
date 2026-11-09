using System;
using System.Linq;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The in-process baseline for throughput runs: a few lines of the bridge's heuristic
    // (internal/policy/heuristic.go) without odds parsing. Reads only `context` and `options`.
    // Sells everything staged (or the budget-maximising discount), buys when the estimate covers the
    // asking price and the rent stays covered, and turns away hot or serious goods.
    internal static class GymHeuristic
    {
        // The retail markup is the margin: buying at or under the estimate resells at a profit.
        const double BuyMargin = 1.0;
        const int MaxHeat = 50;

        public static DecideResponse Decide(DecideRequest req)
        {
            var r = new DecideResponse { DecisionId = req.DecisionId, Source = Sources.Heuristic };
            string pick = Pick(req, out string why);
            r.Choice = pick == null ? Choice.MakeDefer() : new Choice { Option = pick };
            r.Rationale = why;
            return r;
        }

        static bool Has(DecideRequest req, string id) => req.Options.Any(o => o.Id == id);

        static string Pick(DecideRequest req, out string why)
        {
            var c = req.Context;
            string intent = c?["client"]?.Value<string>("intent");
            var theirs = c?["counter"]?["theirs"] as JArray ?? new JArray();
            var yours = c?["counter"]?["yours_staged"] as JArray ?? new JArray();
            bool sells = intent == "buy" || (intent == "sell_and_buy" && yours.Count > 0);
            bool buys = intent == "sell" || (intent == "sell_and_buy" && yours.Count == 0 && theirs.Count > 0);
            if (sells) return Sell(req, c, yours, out why);
            if (buys) return Buy(req, c, theirs, out why);
            why = "nothing to weigh";
            return Has(req, "dismiss") ? "dismiss" : null;
        }

        static string Sell(DecideRequest req, JObject c, JArray yours, out string why)
        {
            if (yours.Count == 0 || !Has(req, "sell_all"))
            {
                why = "nothing sellable staged";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            long price = yours.Sum(i => i.Value<long?>("price") ?? 0);
            long? budget = c["client"]?.Value<long?>("budget");
            string best = "sell_all";
            double bestValue = budget.HasValue ? Math.Min(price, budget.Value) : price;
            foreach (int n in new[] { 10, 25, 50, 75 })
            {
                string id = "offer_discount_" + n;
                if (!budget.HasValue || !Has(req, id)) continue;
                double v = Math.Min(price * (1 - n / 100.0), budget.Value * (1 + 2 * n / 100.0));
                if (v > bestValue + 0.5)
                {
                    best = id;
                    bestValue = v;
                }
            }
            why = $"sell for about {bestValue:0}";
            return best;
        }

        static string Buy(DecideRequest req, JObject c, JArray theirs, out string why)
        {
            if (theirs.Any(i => (i.Value<int?>("stolen_heat") ?? 0) >= MaxHeat || (i.Value<bool?>("serious_contraband") ?? false)))
            {
                why = "too hot";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            long cash = c["store"]?.Value<long?>("cash") ?? 0;
            long reserve = (c["store"]?.Value<int?>("rent_days_left") ?? 7) <= 2 ? c["store"]?.Value<long?>("rent_due") ?? 0 : 0;
            long est = theirs.Sum(i => i.Value<long?>("estimated_value") ?? 0);
            long price = theirs.Sum(i => i.Value<long?>("price") ?? 0);
            long? wholesale = c["client"]?["wholesale"]?.Type == JTokenType.Object ? c["client"]["wholesale"].Value<long?>("price") : null;
            if (wholesale.HasValue && Has(req, "accept_wholesale") && est >= wholesale.Value * BuyMargin && cash - wholesale.Value >= reserve)
            {
                why = $"wholesale {wholesale} for an estimate of {est}";
                return "accept_wholesale";
            }
            if (Has(req, "buy_all") && est >= price * BuyMargin && cash - price >= reserve)
            {
                why = $"buy {price} for an estimate of {est}";
                return "buy_all";
            }
            why = $"asking {price} against {est} leaves no margin";
            return Has(req, "dismiss") ? "dismiss" : null;
        }
    }
}
