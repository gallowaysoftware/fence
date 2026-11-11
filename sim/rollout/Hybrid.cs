using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Threading;
using Fence.Gym;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    internal sealed class HybridConfig
    {
        // "llm": candidates from a proposer answering files in ProposeDir; "blind": a fixed lever set, sampled.
        public string Source;
        public string ProposeDir;
        public int TimeoutSec = 600;
        // Blind candidates drawn per decision (the LLM is asked for 2-3).
        public int Blind = 3;
        // A decision at least every Review days; an event decides at most once per Cooldown days.
        public int Review = 7, Cooldown = 2;
    }

    internal sealed class Trigger
    {
        public string Id, Text;
    }

    // "LLM proposes, gym decides" (docs/research/HYBRID.md): decision points are a weekly review plus game
    // events, read off the main line's morning. Each decision's candidates are the plan in force, the base
    // policy and 2-3 proposals (the LLM's or blind ones); Search rolls them and keeps the best.
    internal sealed class Hybrid
    {
        readonly HybridConfig cfg;
        readonly GymOptions o;
        int lastDecision = int.MinValue / 2;
        // The previous morning, for edge-triggered events.
        int prevCash = -1, decidedEvidence, prevCrimeBand, lastPaymentDue = int.MinValue / 2;
        bool prevPermitAffordable, windowArmed = true, georgioAsked;
        // An upgrade triggers once, the first morning it is affordable; a rise once per item type a week.
        readonly HashSet<string> seenBuyable = new HashSet<string>();
        readonly Dictionary<string, int> riseSeen = new Dictionary<string, int>();
        public int Calls, CallFailures;
        public double CallSeconds;

        public Hybrid(HybridConfig cfg, GymOptions o)
        {
            this.cfg = cfg;
            this.o = o;
            if (cfg.Source == "llm") System.IO.Directory.CreateDirectory(cfg.ProposeDir);
        }

        // A market rise this large is a shortage worth a decision; smaller ones the executor stocks for itself.
        const int ShortageMinPct = 30;

        static int CrimeBand(int crime) => crime >= 1200 ? 4 : crime >= 850 ? 3 : crime >= 300 ? 2 : crime >= 100 ? 1 : 0;

        // Today's decision points, in the order the prompt shows them; empty when today is not one.
        public List<Trigger> Triggers(Episode ep)
        {
            var ps = PlayerStore.Instance;
            int day = ep.Day, cash = ep.Cash;
            var t = new List<Trigger>();
            void Add(string id, string text) => t.Add(new Trigger { Id = id, Text = text });

            bool paying = ps.isPayingMortgage, owned = ps.IsPropertyPaid;
            int down = ps.GetMortgageDownPayment();
            if (!paying && !owned && !ps.IsMortgageOfferDeclined)
            {
                // Rent mornings have dayUntilRent 0; the planner runs before the loan shark's visit.
                if (cash >= 2 * down && ps.dayUntilRent == 0)
                    Add("mortgage_offer", $"this rent morning the loan shark offers the mortgage: cash {cash} >= 2 x the down payment {down}; weekly payment {ps.GetMortgageWeeklyPaymentPreview()} for {PlayerStore.GetMortgageWeeks()} weeks");
                if (windowArmed && cash >= 3 * down / 2)
                {
                    windowArmed = false;
                    Add("mortgage_window", $"cash {cash} is past 1.5 down payments ({down}): the offer comes at {2 * down} on a rent morning");
                }
                else if (cash < down) windowArmed = true;
            }
            if (paying && ps.dayUntilRent <= 3 && day - lastPaymentDue >= 4 && cash + PlayerStock.RepoValue() < ps.mortgageWeeklyPayment * 3 / 2)
            {
                lastPaymentDue = day;
                Add("payment_due", $"mortgage payment {ps.mortgageWeeklyPayment} in {ps.dayUntilRent} days; cash {cash} + repossessable stock {PlayerStock.RepoValue()} is under 1.5 payments");
            }
            if (!georgioAsked && ps.loanOfferedToGeorgio == 0 && day >= 41 && day <= 43)
            {
                georgioAsked = true;
                Add("georgio", "Georgio asks for a loan on day 43: lend 25 % or 50 % of the cash on hand, repaid 4x on the first rent morning more than 5 days later that finds cash under the rent, else on day 69");
            }
            if (!ps.hasBusinessPermit && day >= 36)
            {
                int fee = PlayerStore.GetBusinessPermitPrice(), rent = paying ? 2 * ps.mortgageWeeklyPayment : ps.GetRentValue();
                bool affordable = cash >= fee + rent + 500;
                if (day == 36) Add("permit", $"the permit official visits from day 38 (fee {fee}); without the permit each refusal logs OPERATING_WITHOUT_LICENSE");
                else if (affordable && !prevPermitAffordable) Add("permit", $"cash {cash} now covers the permit fee {fee}, the rent {rent} and 500");
                prevPermitAffordable = affordable;
            }
            int crime = ps.secData?.GetTotalCrimeValue() ?? 0, evidence = ps.secData?.evidenceLevel ?? 0;
            if (evidence >= decidedEvidence + 15 || CrimeBand(crime) > prevCrimeBand)
                Add("investigation", $"the record is rising: evidence {evidence}/100 (100 brings a sentencing officer), crime {crime} (arrest at 300+)");
            prevCrimeBand = CrimeBand(crime);
            if (prevCash >= 0 && cash - prevCash >= Math.Max(800, prevCash / 2))
                Add("windfall", $"cash jumped from {prevCash} to {cash} since yesterday morning");
            prevCash = cash;

            var rises = new List<string>();
            try
            {
                var em = StoreStation.Instance.storeEventManager;
                for (int on = day; on <= day + 3; on++)
                    foreach (var ev in em.GetActiveEventOnDay(on, true, true))
                        foreach (var n in ev.negociationDatas ?? new List<NegociationData>())
                            if (n != null && n.value >= ShortageMinPct && n.itemType != null && !(riseSeen.TryGetValue(n.itemType, out int seen) && day - seen < 7))
                            {
                                riseSeen[n.itemType] = day;
                                rises.Add($"{n.itemType} +{n.value}% on day {on}");
                            }
            }
            catch (Exception) { }
            if (rises.Count > 0 && day > 1) Add("shortage", "a price rise within 3 days: " + string.Join(", ", rises));

            var newly = new List<string>();
            try
            {
                if (WildNetwork.Reachable())
                    foreach (var u in WildNetwork.View().Upgrades.Where(u => u.Buyable).OrderBy(u => u.Id, StringComparer.Ordinal))
                        if (seenBuyable.Add(u.Id)) newly.Add($"{u.Id} ({u.Cost})");
            }
            catch (Exception) { }
            if (newly.Count > 0 && day > 1) Add("upgrade", "first time affordable at Wild's: " + string.Join(", ", newly));

            bool review = day >= lastDecision + cfg.Review;
            bool urgent = t.Any(x => x.Id == "mortgage_offer" || x.Id == "georgio" || x.Id == "payment_due");
            if (!review && !(t.Count > 0 && (urgent || day >= lastDecision + cfg.Cooldown))) return new List<Trigger>();
            if (review) t.Insert(0, new Trigger { Id = "review", Text = $"weekly review (last decision day {(lastDecision < 0 ? "none" : lastDecision.ToString())})" });
            return t;
        }

        public void Decided(Episode ep)
        {
            lastDecision = ep.Day;
            decidedEvidence = PlayerStore.Instance.secData?.evidenceLevel ?? 0;
        }

        // The LLM's alternatives, via a file exchange with the proposer (fenceleague's hybrid runner or
        // `fenceleague propose`); a timeout or a bad answer leaves only the plan in force and the base.
        public (List<Candidate> cands, JObject meta) Propose(JObject request)
        {
            int day = (int)request["day"];
            string req = Path.Combine(cfg.ProposeDir, $"d{day:000}.request.json"), resp = Path.Combine(cfg.ProposeDir, $"d{day:000}.response.json");
            File.WriteAllText(req + ".tmp", request.ToString(Formatting.None));
            File.Move(req + ".tmp", req, overwrite: true);
            var sw = Stopwatch.StartNew();
            while (!File.Exists(resp) && sw.Elapsed.TotalSeconds < cfg.TimeoutSec) Thread.Sleep(200);
            Calls++;
            CallSeconds += sw.Elapsed.TotalSeconds;
            var list = new List<Candidate>();
            if (!File.Exists(resp))
            {
                CallFailures++;
                return (list, new JObject { ["error"] = $"no answer within {cfg.TimeoutSec} s", ["seconds"] = Math.Round(sw.Elapsed.TotalSeconds, 1) });
            }
            JObject r;
            try
            {
                r = JObject.Parse(File.ReadAllText(resp));
            }
            catch (Exception e)
            {
                CallFailures++;
                return (list, new JObject { ["error"] = "unreadable answer: " + e.Message });
            }
            if (!string.IsNullOrEmpty((string)r["error"])) CallFailures++;
            foreach (JObject c in r["candidates"] as JArray ?? new JArray())
                list.Add(new Candidate { Name = "llm_" + Slug((string)c["name"]), Source = "llm", Why = (string)c["why"], Directives = c["directives"] as JObject ?? new JObject() });
            r.Remove("candidates");
            r["wait_seconds"] = Math.Round(sw.Elapsed.TotalSeconds, 1);
            return (list, r);
        }

        static string Slug(string s)
        {
            var chars = (s ?? "").ToLowerInvariant().Select(ch => char.IsLetterOrDigit(ch) ? ch : '_').ToArray();
            string slug = new string(chars).Trim('_');
            return slug.Length == 0 ? "unnamed" : slug.Length > 40 ? slug.Substring(0, 40) : slug;
        }

        // The ablation: `Blind` candidates drawn from the rollout search's fixed one-lever set (general and
        // mortgage levers together), the draw seeded by (seed, day) and independent of the game's generators.
        public List<Candidate> Blind(Episode ep, int every)
        {
            var pool = new List<Candidate>();
            foreach (var c in Candidates.Build(ep, 3).Concat(Candidates.Mortgage(ep, every)))
                if (c.Name != "base" && !pool.Any(p => p.Name == c.Name)) pool.Add(c);
            var rng = new Random(unchecked(o.Seed * 7919 + ep.Day * 104729));
            var picked = pool.OrderBy(_ => rng.Next()).Take(cfg.Blind).ToList();
            foreach (var c in picked)
            {
                c.Source = "blind";
                c.Name = "blind_" + c.Name;
            }
            return picked;
        }
    }
}
