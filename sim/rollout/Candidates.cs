using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    internal sealed class Candidate
    {
        public string Name;
        public JObject Directives;
        // Hybrid: where the candidate came from (keep, base, llm, blind) and the proposer's one-line reason.
        public string Source, Why;
    }

    // The directive variants tried at a decision: the base plan ({}) and one-lever deviations from it
    // along the levers the strategist owns (docs/PROTOCOL.md "Directives"). Read-only on the game: the
    // main line must play on exactly as if nothing had been asked.
    internal static class Candidates
    {
        // The mortgage window (docs/research/ROLLOUT.md "Focus"): from the cash that makes the loan shark's offer
        // near (one down payment) until the last payment.
        public static bool MortgageWindow(Episode ep)
        {
            var ps = PlayerStore.Instance;
            return ps.isPayingMortgage || (!ps.IsPropertyPaid && ep.Cash >= ps.GetMortgageDownPayment());
        }

        // The levers that decide a mortgage run: take it now, later or never; how big a cushion to keep
        // through the twelve payments; selling down or buying less before a payment.
        public static List<Candidate> Mortgage(Episode ep, int every)
        {
            var ps = PlayerStore.Instance;
            int cash = ep.Cash;
            var l = new List<Candidate>();
            void Add(string name, JObject d) => l.Add(new Candidate { Name = name, Directives = d });
            Add("base", new JObject());
            if (!ps.isPayingMortgage)
            {
                int down = ps.GetMortgageDownPayment();
                // The offer comes at 2 down payments of cash: below 1.5 the choice cannot arise within a hold.
                if (cash >= 3 * down / 2)
                {
                    Add("mortgage_take", new JObject { ["mortgage"] = "take" });
                    Add("mortgage_later", new JObject { ["mortgage_window"] = new JArray(ep.Day + every, 100000) });
                    Add("mortgage_float_500", new JObject { ["mortgage_float"] = 500 });
                    Add("mortgage_float_2000", new JObject { ["mortgage_float"] = 2000 });
                    Add("mortgage_margin_1.5", new JObject { ["mortgage_margin"] = 1.5 });
                }
                Add("save_mortgage", new JObject { ["cash_floor"] = Math.Min(cash, 2 * down), ["upgrade_reserve"] = 2 * down });
            }
            else
            {
                int weekly = ps.mortgageWeeklyPayment;
                Add("cushion_3w", new JObject { ["cash_floor"] = Math.Min(cash, 3 * weekly), ["upgrade_reserve"] = 3 * weekly });
                Add("repo_reserve_2.5", new JObject { ["repo_reserve_payments"] = 2.5 });
                Add("repo_reserve_0.5", new JObject { ["repo_reserve_payments"] = 0.5 });
                Add("mortgage_max_spend_250", new JObject { ["mortgage_max_spend"] = 250 });
                Add("selldown_early", new JObject { ["mortgage_selldown_days"] = 7, ["mortgage_selldown_margin"] = 500 });
            }
            Add("no_buy", new JObject { ["buy"] = new JObject { ["max_spend"] = 0 } });
            Add("buy_tight", new JObject { ["buy"] = new JObject { ["wholesale_max_pct"] = 65, ["single_max_pct"] = 65 } });
            Add("buy_loose", new JObject { ["buy"] = new JObject { ["wholesale_max_pct"] = 95, ["single_max_pct"] = 95 } });
            Add("no_growth", new JObject { ["growth"] = new JArray() });
            return l;
        }

        // The executor's default `growth` list (PlayerRole.Upgrades).
        static readonly string[] Growth = { "RENOVATION1", "SHOWCASE_I", "RENOVATION2", "RENOVATION3", "SHOWCASE_II" };

        // Upgrades worth a branch, in order; anything else buyable is tried cheapest first.
        static readonly string[] UpgradePriority =
        {
            "RENOVATION1", "SHOWCASE_I", "RENOVATION2", "RENOVATION3", "SHOWCASE_II", "BUY_REFERAL", "SELL_REFERAL",
            "INSPECTION_INFORMANT", "EVIDENCE_REMOVAL",
        };

        public static List<Candidate> Build(Episode ep, int maxUpgrades)
        {
            var ps = PlayerStore.Instance;
            int rent = ps.isPayingMortgage ? ps.mortgageWeeklyPayment : ps.GetRentValue(), cash = ep.Cash, day = ep.Day;
            var baseD = ep.BaseDirectives ?? new Directives();
            var l = new List<Candidate>();
            void Add(string name, string json) => l.Add(new Candidate { Name = name, Directives = JObject.Parse(json) });

            Add("base", "{}");
            Add("reserve_rent", $"{{\"cash_floor\":{rent},\"sell\":{{\"sell_down_to_cash\":{rent}}}}}");
            Add("sell_down", $"{{\"sell\":{{\"sell_down_to_cash\":{rent + 500},\"min_pct\":80}}}}");
            Add("sell_high", "{\"sell\":{\"min_pct\":110}}");
            Add("buy_tight", "{\"buy\":{\"wholesale_max_pct\":65,\"single_max_pct\":65}}");
            Add("buy_loose", "{\"buy\":{\"wholesale_max_pct\":95,\"single_max_pct\":95}}");
            Add("no_buy", "{\"buy\":{\"max_spend\":0}}");
            Add("bargain_aggressive", "{\"bargain\":\"aggressive\"}");
            Add("sell_hot", "{\"sell\":{\"sell_hot\":true}}");

            bool wine = (baseD.Engine ?? "wine") == "wine";
            Add(wine ? "engine_none" : "engine_wine", wine ? "{\"engine\":\"none\"}" : "{\"engine\":\"wine\"}");

            var using_ = ps.storeService.Where(s => s.unlocked && s.isUsing && !s.isMandatory).Select(s => s.id).OrderBy(x => x, StringComparer.Ordinal).ToList();
            if (using_.Count > 0) Add("services_none", "{\"services\":[]}");
            if (!(using_.Contains("INSURANCE_SERVICE") && using_.Contains("JANITORIAL_SERVICE")))
                Add("services_ins_jan", "{\"services\":[\"INSURANCE_SERVICE\",\"JANITORIAL_SERVICE\"]}");

            if (!ps.hasBusinessPermit && day >= 36)
            {
                int fee = PlayerStore.GetBusinessPermitPrice();
                // The executor's default buys when cash covers fee + rent + 300, else bribes: branch the other way.
                if (cash >= fee + rent) Add(cash >= fee + rent + 300 ? "permit_bribe" : "permit_buy", cash >= fee + rent + 300 ? "{\"permit\":\"bribe\"}" : "{\"permit\":\"buy\"}");
            }

            // Wild's Network: the executor buys its engine's upgrades, then the `growth` list with favor or
            // surplus. A branch puts one buyable upgrade at the head of that list, or buys no growth at all.
            if (WildNetwork.Reachable())
            {
                var v = WildNetwork.View();
                var buyable = v.Upgrades.Where(u => u.Buyable && u.Id != "LANDLORD_GRACE" && u.Id != "WINEMAKER" && (u.Cost <= v.Favor || v.Cash + v.Favor - u.Cost >= v.RentDue))
                    .OrderBy(u => Array.IndexOf(UpgradePriority, u.Id) is var i && i >= 0 ? i : int.MaxValue).ThenBy(u => u.Cost).ThenBy(u => u.Id, StringComparer.Ordinal)
                    .Take(maxUpgrades).ToList();
                string firstGrowth = Growth.FirstOrDefault(g => v.Upgrades.Any(u => u.Id == g && u.Buyable));
                foreach (var u in buyable.Where(u => u.Id != firstGrowth))
                    l.Add(new Candidate { Name = "growth_" + u.Id, Directives = new JObject { ["growth"] = new JArray(new[] { u.Id }.Concat(Growth.Where(g => g != u.Id))) } });
                if (v.Upgrades.Any(u => u.Buyable && Growth.Contains(u.Id)))
                    Add("no_growth", "{\"growth\":[]}");
            }
            Add("upgrade_reserve", "{\"upgrade_reserve\":1000}");
            Add("liquidate_deep", "{\"sell\":{\"liquidate_min_pct\":50}}");

            // The mortgage (the other way to buy the shop): the executor's default takes it on a cautious
            // earnings test when the loan shark offers it; branch to always take or always decline.
            // The loan shark offers it once cash reaches twice the down payment (PlayerStore.CanBeOfferedMortgage):
            // saving toward that is the only line that escapes a rent compounding 10 % a week.
            if (!ps.isPayingMortgage)
            {
                int target = 2 * ps.GetMortgageDownPayment();
                l.Add(new Candidate { Name = "save_mortgage", Directives = new JObject { ["cash_floor"] = Math.Min(cash, target), ["upgrade_reserve"] = target, ["mortgage"] = "take" } });
            }
            if (!ps.isPayingMortgage && cash >= ps.GetMortgageDownPayment())
            {
                Add("mortgage_take", "{\"mortgage\":\"take\"}");
                Add("mortgage_decline", "{\"mortgage\":\"decline\"}");
            }
            return l;
        }
    }
}
