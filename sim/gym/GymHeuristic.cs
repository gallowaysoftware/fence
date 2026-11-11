using System;
using System.Linq;
using System.Text.RegularExpressions;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The in-process baseline for throughput runs: a few lines of the bridge's heuristic
    // (internal/policy/heuristic.go). Reads only `context` and `options`.
    // Sells everything staged (or the budget-maximising discount), buys when the estimate covers the
    // asking price and the rent stays covered, and turns away hot or serious goods.
    // Strategy experiments override each rule with a knob (Knobs.cs); unset knobs keep this baseline.
    internal static class GymHeuristic
    {
        public static DecideResponse Decide(DecideRequest req)
        {
            var r = new DecideResponse { DecisionId = req.DecisionId, Source = Sources.Heuristic };
            string why;
            string pick = req.Kind == "aug_check" ? AugRule.Pick(req, out why)
                : PlayerPolicy.Active ? PlayerPolicy.Pick(req, out why) : Pick(req, Knobs.Current, out why);
            r.Choice = pick == null ? Choice.MakeDefer() : new Choice { Option = pick };
            r.Rationale = why;
            return r;
        }

        static bool Has(DecideRequest req, string id) => req.Options.Any(o => o.Id == id);

        static string Pick(DecideRequest req, Knobs k, out string why)
        {
            var c = req.Context;
            string intent = c?["client"]?.Value<string>("intent");
            var theirs = c?["counter"]?["theirs"] as JArray ?? new JArray();
            var yours = c?["counter"]?["yours_staged"] as JArray ?? new JArray();
            bool sells = intent == "buy" || (intent == "sell_and_buy" && yours.Count > 0);
            bool buys = intent == "sell" || (intent == "sell_and_buy" && yours.Count == 0 && theirs.Count > 0);
            if (sells) return Sell(req, c, yours, k, out why);
            if (buys) return Buy(req, c, theirs, k, out why);
            why = "nothing to weigh";
            return Has(req, "dismiss") ? "dismiss" : null;
        }

        // Odds parsed from the option label (BarterRules.BargainLabel): accept and walk-out shares.
        static (double accept, double walk) Odds(DecideRequest req, string id)
        {
            string label = req.Options.FirstOrDefault(o => o.Id == id)?.Label ?? "";
            if (label.Contains("always accepted")) return (1, 0);
            var a = Regex.Match(label, @"accept (\d+)%");
            var w = Regex.Match(label, @"walk out (\d+)%");
            return (a.Success ? int.Parse(a.Groups[1].Value) / 100.0 : 0, w.Success ? int.Parse(w.Groups[1].Value) / 100.0 : 0);
        }

        static bool Nice(JObject c, Knobs k)
        {
            string f = k.S("nice_faction", null);
            if (f == null) return false;
            return f == "all" || f == c["client"]?.Value<string>("faction");
        }

        static bool SideUsed(JObject c, string side) => c["bargain"]?.Value<bool?>(side) ?? false;

        static readonly int[] Pcts = { 10, 25, 50, 75 };

        // BargainUIManager.GetMarkupBudgetCutPercent.
        static double MarkupBudgetCut(int pct) => pct <= 10 ? 0.10 : pct <= 25 ? 0.15 : pct <= 50 ? 0.20 : 0.25;

        static string Sell(DecideRequest req, JObject c, JArray yours, Knobs k, out string why)
        {
            if (yours.Count == 0 || !Has(req, "sell_all"))
            {
                why = "nothing sellable staged";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            long price = yours.Sum(i => i.Value<long?>("price") ?? 0);
            long? budget = c["client"]?.Value<long?>("budget");
            double B = budget ?? double.MaxValue;
            double baseValue = Math.Min(price, B);
            string mode = k.S("bargain", "discount");

            if (Nice(c, k))
            {
                string id = "offer_discount_" + k.I("nice_pct", 10);
                if (Has(req, id))
                {
                    why = "nice bargain for the faction";
                    return id;
                }
            }

            string best = "sell_all";
            double bestValue = baseValue;
            if (mode == "discount" || mode == "ev")
                foreach (int n in Pcts)
                {
                    string id = "offer_discount_" + n;
                    if (!budget.HasValue || !Has(req, id)) continue;
                    double v = Math.Min(price * (1 - n / 100.0), budget.Value * (1 + 2 * n / 100.0));
                    if (v > bestValue + 0.5) { best = id; bestValue = v; }
                }
            if (mode == "ev")
                foreach (int n in Pcts)
                {
                    string id = "add_markup_" + n;
                    if (!Has(req, id)) continue;
                    var (s, w) = Odds(req, id);
                    double up = Math.Min(price * (1 + n / 100.0), B * (1 - MarkupBudgetCut(n)));
                    double v = s * up + (1 - s - w) * baseValue;
                    if (v > bestValue + 0.5) { best = id; bestValue = v; }
                }
            if (mode == "aggr")
            {
                string id = "add_markup_" + k.I("bargain_pct", 25);
                if (Has(req, id)) best = id;
            }
            if (mode != "never" && mode != "discount" && mode != "ev" && mode != "aggr")
                throw new ArgumentException("bargain=" + mode + " (never|discount|ev|aggr)");
            why = $"{best} for about {bestValue:0}";
            return best;
        }

        static string Buy(DecideRequest req, JObject c, JArray theirs, Knobs k, out string why)
        {
            int maxHeat = k.I("max_heat", 50);
            bool contraband = k.B("buy_contraband", false);
            if (theirs.Any(i => (i.Value<int?>("stolen_heat") ?? 0) >= maxHeat || (!contraband && (i.Value<bool?>("serious_contraband") ?? false))))
            {
                why = "too hot";
                return Has(req, "dismiss") ? "dismiss" : null;
            }
            double margin = k.D("buy_margin", 1.0), wsMargin = k.D("ws_margin", margin);
            bool nice = Nice(c, k);
            int nicePct = k.I("nice_pct", 10);
            // A nice markup already applied raised their prices: that cost was paid on purpose.
            if (nice && SideUsed(c, "buy_side_used")) { margin /= 1 + nicePct / 100.0; wsMargin /= 1 + nicePct / 100.0; }

            long cash = c["store"]?.Value<long?>("cash") ?? 0;
            long rentDue = c["store"]?.Value<long?>("rent_due") ?? 0;
            int daysLeft = c["store"]?.Value<int?>("rent_days_left") ?? 7;
            long reserve = k.S("reserve", "rent2") switch
            {
                "rent2" => daysLeft <= 2 ? rentDue : 0,
                "rent" => rentDue,
                "none" => 0,
                var s => throw new ArgumentException("reserve=" + s + " (rent2|rent|none)"),
            };
            reserve += k.I("floor", 0);
            // Saving for the business permit: refusing it is a weekly crime that ends the run in an arrest
            // around day 80. Read from the game, as the player knows it.
            int day = c["store"]?.Value<int?>("day") ?? 0;
            if (k.Has("save_permit") && day >= k.I("save_permit", 0) && !PlayerStore.Instance.hasBusinessPermit)
                reserve += PlayerStore.GetBusinessPermitPrice() + (k.S("reserve", "rent2") == "rent2" ? rentDue : 0);

            if (k.Has("stock_cap"))
            {
                var ps = PlayerStore.Instance;
                long stock = ps.GetTotalEstimatedValue() - ps.playerCash;
                if (stock > k.I("stock_cap", int.MaxValue))
                {
                    why = $"stock {stock} over the cap";
                    return Has(req, "dismiss") ? "dismiss" : null;
                }
            }
            long est = theirs.Sum(i => i.Value<long?>("estimated_value") ?? 0);
            long price = theirs.Sum(i => i.Value<long?>("price") ?? 0);
            long? wholesale = c["client"]?["wholesale"]?.Type == JTokenType.Object ? c["client"]["wholesale"].Value<long?>("price") : null;
            bool wsOk = wholesale.HasValue && Has(req, "accept_wholesale") && est >= wholesale.Value * wsMargin && cash - wholesale.Value >= reserve;
            bool buyOk = Has(req, "buy_all") && est >= price * margin && cash - price >= reserve;

            if (nice && (wsOk || buyOk))
            {
                string id = "offer_markup_" + nicePct;
                if (Has(req, id))
                {
                    why = "nice bargain for the faction";
                    return id;
                }
            }

            string mode = k.S("bargain", "discount");
            if (mode == "ev" || mode == "aggr")
            {
                double g0 = Math.Max(0, Math.Max(wsOk ? est - wholesale.Value : 0, buyOk ? est - price : 0));
                string best = null;
                double bestV = g0;
                foreach (int n in Pcts)
                {
                    string id = "demand_discount_" + n;
                    if (!Has(req, id)) continue;
                    if (mode == "aggr")
                    {
                        if (n == k.I("bargain_pct", 25)) { best = id; break; }
                        continue;
                    }
                    var (s, w) = Odds(req, id);
                    double f = 1 - n / 100.0;
                    bool affordP = cash - price * f >= reserve, affordW = wholesale.HasValue && cash - wholesale.Value * f >= reserve;
                    double gs = Math.Max(0, Math.Max(Has(req, "buy_all") && affordP && est >= price * f * margin ? est - price * f : 0,
                        affordW && est >= wholesale.Value * f * wsMargin ? est - wholesale.Value * f : 0));
                    double v = s * gs + (1 - s - w) * g0;
                    if (v > bestV + 0.5) { best = id; bestV = v; }
                }
                if (best != null)
                {
                    why = $"{best} (EV {bestV:0} vs {g0:0})";
                    return best;
                }
            }

            if (wsOk)
            {
                why = $"wholesale {wholesale} for an estimate of {est}";
                return "accept_wholesale";
            }
            if (buyOk)
            {
                why = $"buy {price} for an estimate of {est}";
                return "buy_all";
            }
            why = $"asking {price} against {est} leaves no margin";
            return Has(req, "dismiss") ? "dismiss" : null;
        }
    }
}
