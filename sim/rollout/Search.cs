using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using Fence.Gym;
using Fence.Sim;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    internal sealed class SearchConfig
    {
        public int Every = 7, Horizon = 14, Samples = 2, Par = 7, MaxUpgrades = 3;
        // Play the best candidate (rollout policy) or always the base plan (player policy, for datasets).
        public bool Follow = true;
        // Test aids (`check`): branch on the real stream (a base branch must then replay the main line).
        public bool NoReseed, OnlyBase;
        // Deviate from the base plan only when the best candidate beats it by this much (mean score).
        public double Margin = 0;
        public string RowsPath, WorkDir;
        // Candidate names (or name prefixes ending in '_') not to branch on.
        public string[] Exclude = new string[0];
        // "mortgage": decide only inside the mortgage window, on the mortgage levers (Candidates.Mortgage).
        public string Focus;
        // Roll each branch to the end of the mortgage (the remaining payments, or a full term after a
        // possible signing in this hold) instead of a fixed horizon; branches end early at buyout or repo.
        public bool ToPayoff;
        public bool Screen;
        // "LLM proposes, gym decides" (Hybrid.cs): event-driven decisions over proposed candidates. Inside the
        // mortgage window branches roll WindowHorizon days instead of Horizon.
        public HybridConfig Hybrid;
        public int WindowHorizon = 42;
        // Hybrid: a challenger replaces the base policy only when its paired gain over base is SwitchSE
        // standard errors clear of zero.
        public double SwitchSE = 1.0;
        // Hybrid: a decision whose candidates sign, refuse or defer the mortgage or the cash buyout rolls to the
        // end of the term (as ToPayoff does); a 42-day window ends before the repossession it buys.
        public bool CommitPayoff = true;
        // The main line's decision log and the show feed's `candidates` events (ShowLog.cs).
        public ShowLog Show;
    }

    // One step of policy iteration by Monte Carlo (Bertsekas; docs/research/SOLVER.md §7): at a decision
    // morning, fork the running game once per (candidate, sample), play each branch for `Horizon` days
    // (the candidate held for `Every` days, then the base plan), score the ends, and play on with the best.
    // Each sample re-seeds the game's generators in the branch, so a branch sees a future drawn like the
    // real one, not the real one (no hindsight): the main line keeps its own stream and is untouched.
    internal sealed class Search
    {
        public static bool IsChild;
        static readonly bool Debug = Environment.GetEnvironmentVariable("FENCE_ROLLOUT_DEBUG") != null;
        static void Log(string m) { if (Debug) { Console.Error.WriteLine($"[{Environment.ProcessId}] {m}"); Console.Error.Flush(); } }

        readonly SearchConfig cfg;
        readonly GymOptions o;
        int firstDay = -1, nextDecision;
        Directives hold;
        JObject holdJson;
        int holdUntil;
        public readonly Hybrid Hybrid;
        public readonly JArray Decisions = new JArray();
        public long BranchDays, Branches;
        public double BranchSeconds;

        // Branch state (child only).
        Directives childPlan;
        int childUntil, childNw0, childDay0;
        string childOut;
        JObject childMeta;

        public Search(SearchConfig cfg, GymOptions o)
        {
            this.cfg = cfg;
            this.o = o;
            System.IO.Directory.CreateDirectory(cfg.WorkDir);
            if (cfg.Hybrid != null) Hybrid = new Hybrid(cfg.Hybrid, o);
        }

        public Directives Plan(Episode ep)
        {
            int day = ep.Day;
            if (firstDay < 0) { firstDay = day; nextDecision = day; }
            if (IsChild) return day < childUntil ? childPlan : null;
            if (Hybrid != null)
            {
                var triggers = Hybrid.Triggers(ep);
                if (triggers.Count > 0)
                {
                    if (Decide(ep, triggers)) return childPlan;
                    Hybrid.Decided(ep);
                }
                return hold;
            }
            if (day >= nextDecision && (cfg.Focus != "mortgage" || Candidates.MortgageWindow(ep)))
            {
                nextDecision = day + cfg.Every;
                if (Decide(ep)) return childPlan;
            }
            return day < holdUntil ? hold : null;
        }

        static int SampleSeed(int seed, int day, int m) => (int)((uint)(seed * 1000003 + day * 7919 + m * 104729 + 0x2545F491) & 0x7fffffff);

        // True in a branch (which then plays `childPlan`); false in the main line once it has chosen.
        bool Decide(Episode ep, List<Trigger> triggers = null)
        {
            int day = ep.Day;
            var ps = PlayerStore.Instance;
            int payoff = ps.isPayingMortgage ? ps.mortgageWeekLeft * 7 + 7 : PlayerStore.GetMortgageWeeks() * 7 + cfg.Every + 7;
            int horizon = cfg.ToPayoff ? payoff : Hybrid != null && Candidates.MortgageWindow(ep) ? cfg.WindowHorizon : cfg.Horizon;
            Log("decide day " + day);
            JObject step = null, proposer = null;
            var rejected = new JArray();
            List<Candidate> cands;
            bool commit = false;
            if (Hybrid != null)
            {
                step = StepSnapshot(ep);
                (cands, proposer, rejected) = HybridCandidates(ep, triggers, step, Math.Min(day + horizon, firstDay + o.Days) - day, cfg.CommitPayoff ? Math.Min(day + Math.Max(horizon, payoff), firstDay + o.Days) - day : 0);
                commit = cfg.CommitPayoff && Commitment(ep, cands);
                if (commit) horizon = Math.Max(horizon, payoff);
            }
            else cands = cfg.Focus == "mortgage" ? Candidates.Mortgage(ep, cfg.Every) : Candidates.Build(ep, cfg.MaxUpgrades);
            int end = Math.Min(day + horizon, firstDay + o.Days);
            Log(cands.Count + " candidates");
            if (cfg.OnlyBase) cands = cands.Take(1).ToList();
            cands = cands.Where(c => c.Name == "base" || !cfg.Exclude.Any(x => x.EndsWith("_") ? c.Name.StartsWith(x, StringComparison.Ordinal) : c.Name == x)).ToList();
            int par = Throttle.Par(cfg.Par);
            var running = new Dictionary<int, (int c, int m)>();
            var failed = new List<string>();
            var sw = Stopwatch.StartNew();
            Console.Out.Flush();
            Console.Error.Flush();
            cfg.Show?.Flush();
            // With -screen, the base plan is rolled first; when every sample of it wins, nothing can beat it by
            // more than a few days and the other candidates are not branched.
            for (int stage = cfg.Screen ? 0 : 1; stage < 2; stage++)
            {
            var jobs = new List<(int c, int m)>();
            for (int c = stage == 0 ? 0 : cfg.Screen ? 1 : 0; c < (stage == 0 ? 1 : cands.Count); c++)
                for (int m = 0; m < cfg.Samples; m++) jobs.Add((c, m));
            foreach (var job in jobs)
            {
                while (running.Count >= par) Reap(running, failed);
                Log("fork c" + job.c + " m" + job.m);
                int pid = Libc.fork();
                if (pid < 0) throw new InvalidOperationException("fork failed: errno " + System.Runtime.InteropServices.Marshal.GetLastWin32Error());
                if (pid == 0)
                {
                    IsChild = true;
                    Log("child up");
                    childPlan = Directives.Parse(cands[job.c].Directives);
                    childUntil = day + cfg.Every;
                    childDay0 = day;
                    childNw0 = ep.Stats.NetWorthByDay.Count;
                    childOut = JobPath(day, job.c, job.m);
                    childMeta = new JObject { ["candidate"] = cands[job.c].Name, ["sample"] = job.m, ["sample_seed"] = SampleSeed(o.Seed, day, job.m) };
                    ep.Options.Days = end - firstDay;
                    if (!cfg.NoReseed) Headless.Seed(SampleSeed(o.Seed, day, job.m));
                    return true;
                }
                running[pid] = job;
            }
            while (running.Count > 0) Reap(running, failed);
            if (stage == 0 && Enumerable.Range(0, cfg.Samples).All(m => BaseWon(day, m))) cands = cands.Take(1).ToList();
            if (cands.Count == 1) break;
            }
            sw.Stop();

            var evals = new List<(Candidate c, double score, JArray samples)>();
            for (int c = 0; c < cands.Count; c++)
            {
                var samples = new JArray();
                double sum = 0;
                for (int m = 0; m < cfg.Samples; m++)
                {
                    string p = JobPath(day, c, m);
                    JObject r = File.Exists(p) ? JObject.Parse(File.ReadAllText(p)) : new JObject { ["crash"] = "no result" };
                    if (File.Exists(p)) File.Delete(p);
                    double s = Score(r, day, end) + (Hybrid != null ? Risk(r, day, end) : 0);
                    r["score"] = Math.Round(s, 1);
                    sum += s;
                    samples.Add(r);
                    if (r["crash"] == null) BranchDays += r.Value<int?>("days") ?? 0;
                    Branches++;
                }
                evals.Add((cands[c], sum / cfg.Samples, samples));
            }
            BranchSeconds += sw.Elapsed.TotalSeconds;
            // The base policy: first in the plain search; in the hybrid, `keep` when no delta is in force, else `base`.
            int bi = Hybrid == null ? 0 : evals.FindIndex(e => e.c.Directives.Count == 0);
            var baseEval = evals[bi];
            double baseScore = baseEval.score;
            var best = evals.OrderByDescending(e => e.score).ThenBy(e => evals.IndexOf(e)).First();
            string argmax = best.c.Name;
            if (Hybrid == null && best.score < baseScore + cfg.Margin) best = baseEval;
            // Hybrid: a candidate, the plan in force included, replaces the base policy only when its paired gain
            // over base (common random numbers) is SwitchSE standard errors clear of zero; with few samples the max
            // of noisy means wins too often (docs/research/ROLLOUT.md, "One sample"). Going back to base needs no gate.
            if (Hybrid != null)
                best = evals.Where(e => e.c == baseEval.c || cfg.Samples < 2 || PairedZ(baseEval.samples, e.samples) >= cfg.SwitchSE)
                    .OrderByDescending(e => e.score).ThenBy(e => evals.IndexOf(e)).First();
            var chosen = cfg.Follow ? best : baseEval;
            hold = chosen.c.Name == "base" || chosen.c.Directives.Count == 0 ? null : Directives.Parse(chosen.c.Directives);
            holdJson = hold == null ? null : chosen.c.Directives;
            holdUntil = Hybrid != null ? int.MaxValue : day + cfg.Every;

            Decisions.Add(new JObject
            {
                ["day"] = day, ["nw_index"] = ep.Stats.NetWorthByDay.Count, ["best"] = best.c.Name, ["played"] = chosen.c.Name, ["base_score"] = Math.Round(baseScore, 1),
                ["best_score"] = Math.Round(best.score, 1), ["candidates"] = cands.Count, ["seconds"] = Math.Round(sw.Elapsed.TotalSeconds, 2),
                ["failed"] = failed.Count, ["base_nw_path"] = baseEval.samples[0]["nw_path"], ["base_game_over"] = baseEval.samples[0]["game_over"],
                ["horizon"] = end - day, ["commitment"] = Hybrid == null ? null : commit,
                ["triggers"] = triggers == null ? null : new JArray(triggers.Select(t => t.Id)),
                ["sources"] = new JArray(cands.Select(c => c.Source)),
                ["played_source"] = chosen.c.Source,
            });
            if (Hybrid != null) cfg.Show?.Show(ShowLog.Candidates(day, triggers, evals, bi, chosen.c, argmax, cfg.Samples, end - day));
            if (cfg.RowsPath != null)
            {
                var row = new JObject
                {
                    ["seed"] = o.Seed, ["difficulty"] = o.Difficulty, ["start"] = o.Start, ["day"] = day,
                    ["policy"] = cfg.Follow ? "rollout" : "player",
                    ["search"] = new JObject { ["focus"] = cfg.Focus, ["to_payoff"] = cfg.ToPayoff, ["commitment"] = Hybrid == null ? null : commit, ["screened"] = cands.Count == 1, ["every"] = cfg.Every, ["horizon"] = end - day, ["samples"] = cfg.Samples, ["score"] = ScoreDoc },
                    ["snapshot"] = step ?? StepSnapshot(ep),
                    ["triggers"] = triggers == null ? null : new JArray(triggers.Select(t => new JObject { ["id"] = t.Id, ["text"] = t.Text })),
                    ["in_force"] = Hybrid == null ? null : evals[0].c.Directives,
                    ["proposer"] = proposer,
                    ["rejected"] = Hybrid == null ? null : rejected,
                    ["played"] = chosen.c.Name,
                    ["candidates"] = new JArray(evals.Select(e => new JObject
                    {
                        ["name"] = e.c.Name, ["source"] = e.c.Source, ["why"] = e.c.Why, ["directives"] = e.c.Directives, ["score"] = Math.Round(e.score, 1),
                        ["z_vs_first"] = Hybrid == null || e.samples.Count < 2 ? null : Math.Round(Math.Min(99, Math.Max(-99, PairedZ(evals[0].samples, e.samples))), 2),
                        ["z_vs_base"] = Hybrid == null || e.samples.Count < 2 ? null : Math.Round(Math.Min(99, Math.Max(-99, PairedZ(baseEval.samples, e.samples))), 2),
                        ["rollouts"] = new JArray(e.samples.Select(Summary)),
                    })),
                    ["best"] = best.c.Name,
                    ["argmax"] = argmax,
                    ["best_directives"] = best.c.Directives,
                    ["margin_over_base"] = Math.Round(best.score - baseScore, 1),
                };
                if (Hybrid != null) row["search"]["score"] = ScoreDoc + "; " + RiskDoc;
                File.AppendAllText(cfg.RowsPath, row.ToString(Formatting.None) + "\n");
            }
            return false;
        }

        static double PairedZ(JArray a, JArray b)
        {
            var d = Enumerable.Range(0, Math.Min(a.Count, b.Count)).Select(i => (double)b[i]["score"] - (double)a[i]["score"]).ToList();
            double mean = d.Average(), sd = Math.Sqrt(d.Sum(x => (x - mean) * (x - mean)) / Math.Max(1, d.Count - 1));
            return sd < 1e-9 ? (mean > 0 ? double.PositiveInfinity : 0) : mean / (sd / Math.Sqrt(d.Count));
        }

        bool BaseWon(int day, int m)
        {
            string p = JobPath(day, 0, m);
            return File.Exists(p) && (((string)JObject.Parse(File.ReadAllText(p))["game_over"]) ?? "").StartsWith("buyout", StringComparison.Ordinal);
        }

        string JobPath(int day, int c, int m) => Path.Combine(cfg.WorkDir, $"d{day:000}-c{c:00}-m{m}.json");

        void Reap(Dictionary<int, (int c, int m)> running, List<string> failed)
        {
            int pid = Libc.waitpid(-1, out int status, 0);
            if (pid <= 0) throw new InvalidOperationException("waitpid failed");
            if (!running.Remove(pid, out var job)) return;
            if (!Libc.Exited(status, out int code) || code != 0) failed.Add($"c{job.c} m{job.m}: status {status}");
        }

        public const string ScoreDoc = "buyout progress toward a win: 10000 * (net worth + mortgage equity - rent shortfall) / buyout price at the horizon " +
            "(equity = the principal share of the down payment and weekly payments made; shortfall = rent or mortgage payment due within 3 days not covered by cash); " +
            "a buyout scores 20000 - days taken; any other ending -10000 + 100 * days survived in the branch";

        public static double Score(JObject r, int day, int end)
        {
            if (r["crash"] != null) return -20000;
            string over = (string)r["game_over"];
            int days = r.Value<int?>("days") ?? 0;
            if (over != null && over.StartsWith("buyout", StringComparison.Ordinal)) return 20000 - days;
            if (over != null) return -10000 + 100.0 * days;
            double price = Math.Max(1, r.Value<double>("buyout_price"));
            int due = r.Value<int>("rent_due"), left = r.Value<int>("rent_days_left"), cash = r.Value<int>("cash_end");
            double shortfall = left <= 3 ? Math.Max(0, due - cash) : 0;
            return 10000.0 * (r.Value<double>("nw_end") + (r.Value<double?>("mortgage_equity") ?? 0) - shortfall) / price;
        }

        public const string RiskDoc = "hybrid risk, alive at the horizon: -40 * (evidence - 40) when evidence > 40 (a quarter of it under 300 crime: no arrest), " +
            "-3000 while paying the mortgage with cash + repossessable stock under one payment (the next collection ends the run), " +
            "and while paying, when cash + repossessable stock + the last 28 days' net-worth trend do not reach the last payment, the score of a repossession on the day they run out";

        // What the horizon state says about the arrest and repossession to come; added to Score in hybrid mode.
        public static double Risk(JObject r, int day, int end)
        {
            if (r["crash"] != null || r["game_over"]?.Type == JTokenType.String) return 0;
            double risk = 0;
            int evidence = r.Value<int?>("evidence") ?? 0, crime = r.Value<int?>("crime") ?? 0;
            if (evidence > 40) risk -= 40.0 * (evidence - 40) * (crime >= 300 ? 1 : 0.25);
            if (r.Value<bool?>("mortgage") == true && r.Value<int>("cash_end") + (r.Value<int?>("repo_stock") ?? 0) < r.Value<int>("rent_due")) risk -= 3000;
            else if (RunsOut(r) is int lasts)
            {
                // A branch cut mid-term: it scores as the repossession its trend projects, not the equity it holds.
                double repo = -10000 + 100.0 * ((r.Value<int?>("days") ?? 0) + lasts);
                risk += Math.Min(0, repo - Score(r, day, end));
                r["runs_out_in"] = lasts;
            }
            r["risk"] = risk;
            return risk;
        }

        // Days until cash + repossessable stock, falling at the net-worth trend since signing (at most 28 days,
        // at least 7), are gone, when that comes before the last payment; null when they last.
        static int? RunsOut(JObject r)
        {
            int left = r.Value<int?>("mortgage_weeks_left") ?? 0, paid = (r.Value<int?>("mortgage_weeks") ?? 0) - left;
            var path = r["nw_path"] as JArray;
            if (r.Value<bool?>("mortgage") != true || left <= 0 || path == null) return null;
            int k = Math.Min(28, Math.Min(path.Count - 1, 7 * paid));
            if (k < 7) return null;
            double slope = ((double)path[path.Count - 1] - (double)path[path.Count - 1 - k]) / k;
            double reserve = r.Value<int>("cash_end") + (r.Value<int?>("repo_stock") ?? 0);
            if (slope >= 0 || reserve + slope * 7 * left >= 0) return null;
            return (int)(reserve / -slope);
        }

        // Commitment keys: they sign, refuse or put off the mortgage, or aim at the cash buyout.
        static readonly string[] CommitKeys = { "mortgage", "mortgage_window", "mortgage_margin", "mortgage_float", "mortgage_cover", "mortgage_save_from_day", "buyout", "buyout_by_day", "buyout_check_from" };

        // A decision commits when the offer can still come (or be bought out of) and some candidate, the plan in
        // force included, moves a commitment key away from the base policy.
        static bool Commitment(Episode ep, List<Candidate> cands)
        {
            var ps = PlayerStore.Instance;
            if (ps.isPayingMortgage || ps.IsPropertyPaid || ps.IsMortgageOfferDeclined || !Candidates.MortgageWindow(ep)) return false;
            return cands.Any(c => CommitKeys.Any(c.Directives.ContainsKey));
        }

        // The plan in force (always first: ties keep it), the base policy, then the proposals; a proposal the
        // executor cannot parse, or that repeats an earlier candidate, is dropped and logged.
        (List<Candidate>, JObject, JArray) HybridCandidates(Episode ep, List<Trigger> triggers, JObject step, int horizon, int commitHorizon)
        {
            var l = new List<Candidate> { new Candidate { Name = "keep", Source = "keep", Directives = holdJson ?? new JObject() } };
            if (holdJson != null && holdJson.Count > 0) l.Add(new Candidate { Name = "base", Source = "base", Directives = new JObject() });
            JObject meta = null;
            List<Candidate> proposed;
            if (cfg.Hybrid.Source == "llm")
            {
                var request = new JObject
                {
                    ["day"] = ep.Day, ["seed"] = o.Seed, ["difficulty"] = o.Difficulty,
                    ["triggers"] = new JArray(triggers.Select(t => new JObject { ["id"] = t.Id, ["text"] = t.Text })),
                    ["in_force"] = holdJson ?? new JObject(), ["base_directives"] = ep.BaseDirectives?.ToJson() ?? new JObject(),
                    ["hold_days"] = cfg.Every, ["horizon_days"] = horizon, ["commit_horizon_days"] = commitHorizon, ["samples"] = cfg.Samples,
                    ["step"] = step,
                };
                (proposed, meta) = Hybrid.Propose(request);
            }
            else proposed = Hybrid.Blind(ep, cfg.Every);
            var rejected = new JArray();
            foreach (var c in proposed)
            {
                c.Directives.Remove("day");
                string why = null;
                try
                {
                    Directives.Parse(c.Directives);
                }
                catch (Exception e)
                {
                    why = e.Message;
                }
                if (why == null && l.Any(x => JToken.DeepEquals(x.Directives, c.Directives))) why = "same as " + l.First(x => JToken.DeepEquals(x.Directives, c.Directives)).Name;
                if (why == null && l.Any(x => x.Name == c.Name)) c.Name += "_" + l.Count;
                if (why != null) rejected.Add(new JObject { ["name"] = c.Name, ["directives"] = c.Directives, ["why_rejected"] = why });
                else l.Add(c);
            }
            return (l, meta, rejected);
        }

        static JObject Summary(JToken t)
        {
            var r = (JObject)t;
            return new JObject
            {
                ["sample_seed"] = r["sample_seed"], ["score"] = r["score"], ["game_over"] = r["game_over"], ["days"] = r["days"],
                ["nw_end"] = r["nw_end"], ["cash_end"] = r["cash_end"], ["nw_delta"] = r["nw_delta"], ["rent_due"] = r["rent_due"],
                ["rent_days_left"] = r["rent_days_left"], ["buyout_price"] = r["buyout_price"], ["mortgage"] = r["mortgage"], ["mortgage_equity"] = r["mortgage_equity"], ["crime"] = r["crime"], ["evidence"] = r["evidence"], ["crash"] = r["crash"],
                ["repo_stock"] = r["repo_stock"], ["risk"] = r["risk"], ["runs_out_in"] = r["runs_out_in"], ["mortgage_weeks_left"] = r["mortgage_weeks_left"],
            };
        }

        // The child's last act: its branch's end state, then _exit (never return into the caller's process image).
        public void ChildFinish(Episode ep, string crash)
        {
            var ps = PlayerStore.Instance;
            var nw = ep.Stats.NetWorthByDay;
            var r = (JObject)childMeta.DeepClone();
            try
            {
                int nw0 = childNw0 > 0 ? nw[childNw0 - 1] : 0;
                r["day0"] = childDay0;
                r["last_day"] = ep.Day;
                r["days"] = nw.Count - childNw0;
                r["game_over"] = ep.GameOver;
                r["nw_end"] = nw.Count > 0 ? nw[^1] : ps.GetTotalEstimatedValue();
                r["nw_delta"] = (nw.Count > 0 ? nw[^1] : 0) - nw0;
                r["cash_end"] = ep.Cash;
                r["rent_due"] = ps.isPayingMortgage ? ps.mortgageWeeklyPayment : ps.GetRentValue();
                r["buyout_price"] = ps.GetStoreBuyPrice(false);
                r["mortgage"] = ps.isPayingMortgage;
                if (ps.isPayingMortgage)
                {
                    // Interest removed: what has been paid is that share of the price.
                    double total = ps.mortgageDownPaid + (double)ps.mortgageWeeklyPayment * ps.mortgageTotalWeeks;
                    double paid = ps.mortgageDownPaid + (double)ps.mortgageWeeklyPayment * (ps.mortgageTotalWeeks - ps.mortgageWeekLeft);
                    r["mortgage_weeks_left"] = ps.mortgageWeekLeft;
                    r["mortgage_weeks"] = ps.mortgageTotalWeeks;
                    r["mortgage_equity"] = Math.Round(total > 0 ? ps.GetStoreBuyPrice(false) * paid / total : 0);
                }
                r["rent_days_left"] = ps.dayUntilRent;
                r["grace"] = NetworkUpgrade.IsLandlordGraceUnlocked();
                r["repo_stock"] = PlayerStock.RepoValue();
                r["crime"] = ps.secData?.GetTotalCrimeValue() ?? 0;
                r["evidence"] = ps.secData?.evidenceLevel ?? 0;
                r["nw_path"] = new JArray(nw.Skip(childNw0));
                if (crash != null) r["crash"] = crash;
            }
            catch (Exception e)
            {
                r["crash"] = (crash ?? "") + " finish: " + e.Message;
            }
            Log("child done day " + ep.Day);
            File.WriteAllText(childOut + ".tmp", r.ToString(Formatting.None));
            File.Move(childOut + ".tmp", childOut, overwrite: true);
            Libc._exit(0);
        }

        static readonly MethodInfo SnapshotMethod = typeof(Episode).Assembly.GetType("Fence.Gym.StepMode").GetMethod("Snapshot", BindingFlags.Static | BindingFlags.NonPublic);

        // Exactly what `-step-dir` shows an agent each morning (sim/gym/README.md), minus the file-exchange fields.
        JObject StepSnapshot(Episode ep)
        {
            var s = (JObject)SnapshotMethod.Invoke(null, new object[] { ep, o });
            s.Remove("respond_with");
            s.Remove("timeout_seconds");
            return s;
        }
    }
}
