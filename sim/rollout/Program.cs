using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Gym;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    // fence-rollout: rollout search over the gym's directives executor (docs/research/ROLLOUT.md).
    //   play     one game; every `-every` days the plan is chosen by forked rollouts (or, with -follow base,
    //            the base plan is played and the rollouts only label it)
    //   check    fork fidelity: an un-reseeded base branch must replay the main line day for day
    static class Program
    {
        const string Usage = "usage: fence-rollout play|check -seed N [-difficulty normal] [-days 1000] [-every 7] [-horizon 14] [-samples 2] [-par 7]\n" +
                             "       [-follow best|base] [-margin 0] [-max-upgrades 3] [-exclude name,prefix_,...] [-focus mortgage] [-to-payoff] [-screen] [-rows FILE.jsonl] [-summary FILE.json] [-work DIR] [-directives FILE]\n" +
                             "       [-start none] [-perks ids+...] [-deadline 91 -gate 2.0 -rules FILE.json] [-decision-log FILE.jsonl] [-hybrid llm|blind -propose-dir DIR -propose-timeout 600 -blind 3 -review 7 -cooldown 2 -window-horizon 42 -switch-se 1.0 -no-commit-payoff]";

        static int Main(string[] args)
        {
            string cmd = args.Length > 0 && !args[0].StartsWith("-") ? args[0] : "";
            var a = Args.Parse(args.SkipWhile(x => !x.StartsWith("-")).ToArray());
            if (cmd != "play" && cmd != "check")
            {
                Console.Error.WriteLine(Usage);
                return 2;
            }
            // W^X maps JIT code MAP_SHARED from a memfd, so a forked branch that JITs or patches code would
            // write into its parent's code pages (both crash). Without W^X, code pages are private copy-on-write.
            if (Environment.GetEnvironmentVariable("DOTNET_EnableWriteXorExecute") != "0")
            {
                var psi = new ProcessStartInfo(Environment.ProcessPath);
                if (Path.GetFileNameWithoutExtension(Environment.ProcessPath) == "dotnet") psi.ArgumentList.Add(typeof(Program).Assembly.Location);
                foreach (var x in args) psi.ArgumentList.Add(x);
                psi.Environment["DOTNET_EnableWriteXorExecute"] = "0";
                using var p = Process.Start(psi);
                p.WaitForExit();
                return p.ExitCode;
            }
            Boot.Init();
            return Run(cmd, a);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static int Run(string cmd, Args a)
        {
            var o = new GymOptions
            {
                Seed = a.Int("seed", 1), Days = a.Int("days", 1000), Difficulty = a.Str("difficulty", "normal"), Start = a.Str("start", "none"),
                Human = "player", DirectivesFile = a.Str("directives", ""), Knobs = a.Str("knobs", ""), Perks = a.Str("perks", ""),
            };
            var cfg = new SearchConfig
            {
                Every = a.Int("every", 7), Horizon = a.Int("horizon", 14), Samples = a.Int("samples", 2), Par = a.Int("par", 7),
                MaxUpgrades = a.Int("max-upgrades", 3), Follow = a.Str("follow", "best") == "best", Margin = a.Int("margin", 0),
                RowsPath = a.Str("rows", null),
                Focus = a.Str("focus", null),
                ToPayoff = a.Flag("to-payoff"),
                Screen = a.Flag("screen"),
                Exclude = a.Str("exclude", "").Split(',', StringSplitOptions.RemoveEmptyEntries),
                WorkDir = a.Str("work", Path.Combine("/dev/shm", "fence-rollout", Environment.ProcessId.ToString())),
                WindowHorizon = a.Int("window-horizon", 42),
                SwitchSE = a.Dbl("switch-se", 1.0),
                CommitPayoff = !a.Flag("no-commit-payoff"),
            };
            if (a.Int("deadline", 0) is int deadline && deadline > 0)
            {
                cfg.Sprint = new SprintConfig { Deadline = deadline, Gate = a.Dbl("gate", 2.0) };
                if (a.Str("rules", null) is string rules) cfg.Sprint.Rules = Sprint.LoadRules(rules);
            }
            string hybrid = a.Str("hybrid", null);
            if (hybrid != null)
            {
                if (hybrid != "llm" && hybrid != "blind") { Console.Error.WriteLine("-hybrid llm|blind"); return 2; }
                string dir = a.Str("propose-dir", null);
                if (hybrid == "llm" && dir == null) { Console.Error.WriteLine("-hybrid llm needs -propose-dir"); return 2; }
                cfg.Hybrid = new HybridConfig
                {
                    Source = hybrid, ProposeDir = dir, TimeoutSec = a.Int("propose-timeout", 600), Blind = a.Int("blind", 3),
                    Review = a.Int("review", 7), Cooldown = a.Int("cooldown", 2),
                };
                cfg.Horizon = a.Int("horizon", 28);
            }
            if (cmd == "check")
            {
                cfg.NoReseed = true;
                cfg.OnlyBase = true;
                cfg.Follow = false;
                cfg.Samples = 1;
                cfg.Every = cfg.Horizon = a.Int("every", 10);
            }
            if (cfg.RowsPath != null) System.IO.Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(cfg.RowsPath)));

            Knobs.Current = new Knobs(o.Knobs);
            string decisionLog = a.Str("decision-log", null);
            if (decisionLog != null) cfg.Show = new ShowLog(new LocalBridge(o.Seed), decisionLog);
            var ep = new Episode(o, (IBridge)cfg.Show ?? new LocalBridge(o.Seed), null);
            ep.BaseDirectives = Directives.Merge(Directives.FromKnobs(Knobs.Current), string.IsNullOrEmpty(o.DirectivesFile) ? null : Directives.Load(o.DirectivesFile));
            var search = new Search(cfg, o);
            ep.Planner = search.Plan;
            var sw = Stopwatch.StartNew();
            string crash = null;
            try
            {
                ep.Step();
                while (ep.Step()) { }
            }
            catch (Exception e)
            {
                crash = Fence.Sim.Game.Inner(e);
            }
            if (Search.IsChild) search.ChildFinish(ep, crash);
            sw.Stop();
            cfg.Show?.Dispose();
            try { System.IO.Directory.Delete(cfg.WorkDir, true); } catch (Exception) { }

            var s = ep.Stats;
            var result = new JObject
            {
                ["seed"] = o.Seed, ["difficulty"] = o.Difficulty, ["start"] = o.Start, ["human_role"] = "player",
                ["policy"] = cfg.Follow ? "rollout" : "player",
                ["search"] = new JObject { ["deadline"] = cfg.Sprint?.Deadline, ["gate"] = cfg.Sprint?.Gate, ["rules"] = cfg.Sprint?.Rules != null, ["focus"] = cfg.Focus, ["every"] = cfg.Every, ["horizon"] = cfg.Horizon, ["samples"] = cfg.Samples, ["par"] = cfg.Par, ["margin"] = cfg.Margin, ["max_upgrades"] = cfg.MaxUpgrades,
                    ["hybrid"] = cfg.Hybrid?.Source, ["window_horizon"] = cfg.Hybrid == null ? null : cfg.WindowHorizon, ["review"] = cfg.Hybrid?.Review, ["cooldown"] = cfg.Hybrid?.Cooldown, ["blind"] = cfg.Hybrid?.Blind, ["switch_se"] = cfg.Hybrid == null ? null : cfg.SwitchSE, ["commit_payoff"] = cfg.Hybrid == null ? null : cfg.CommitPayoff },
                ["perks"] = o.Perks,
                ["hybrid"] = search.Hybrid == null ? null : new JObject
                {
                    ["proposer_calls"] = search.Hybrid.Calls, ["proposer_failures"] = search.Hybrid.CallFailures, ["proposer_seconds"] = Math.Round(search.Hybrid.CallSeconds, 1),
                    ["triggers"] = JObject.FromObject(search.Decisions.SelectMany(d => d["triggers"] as JArray ?? new JArray()).GroupBy(t => (string)t).OrderBy(g => g.Key, StringComparer.Ordinal).ToDictionary(g => g.Key, g => g.Count())),
                    ["played_sources"] = JObject.FromObject(search.Decisions.GroupBy(d => (string)d["played_source"] ?? "").OrderBy(g => g.Key, StringComparer.Ordinal).ToDictionary(g => g.Key, g => g.Count())),
                },
                ["days_requested"] = o.Days, ["days_played"] = s.CashByDay.Count, ["last_day"] = ep.Day, ["cash"] = ep.Cash,
                ["game_over"] = ep.GameOver, ["crash"] = crash,
                ["cash_by_day"] = new JArray(s.CashByDay), ["net_worth_by_day"] = new JArray(s.NetWorthByDay),
                ["end"] = End(),
                // The executor's per-customer reasons, which fenceshow's replay shows (internal/show/match.go).
                ["player_days"] = new JArray(ep.Ledger.Days),
                ["decisions"] = search.Decisions,
                ["played"] = JObject.FromObject(search.Decisions.GroupBy(d => (string)d["played"]).OrderBy(g => g.Key, StringComparer.Ordinal).ToDictionary(g => g.Key, g => g.Count())),
                ["branches"] = search.Branches, ["branch_days"] = search.BranchDays,
                ["seconds"] = Math.Round(sw.Elapsed.TotalSeconds, 1), ["branch_seconds"] = Math.Round(search.BranchSeconds, 1),
            };
            string summary = a.Str("summary", null);
            if (summary != null)
            {
                System.IO.Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(summary)));
                File.WriteAllText(summary, result.ToString(Formatting.Indented) + "\n");
            }
            Console.WriteLine($"seed {o.Seed} {o.Difficulty}: day {ep.Day}, {(string)ep.GameOver ?? "alive"}, nw {s.NetWorthByDay.LastOrDefault()}, " +
                              $"{search.Decisions.Count} decisions, {search.Branches} branches, {search.BranchDays} branch days in {result["seconds"]} s " +
                              $"({(sw.Elapsed.TotalMinutes > 0 ? Math.Round((search.BranchDays + s.CashByDay.Count) / sw.Elapsed.TotalMinutes) : 0)} days/min)" + (crash != null ? " CRASH " + crash : ""));
            return cmd == "check" ? Check(search, s) : crash == null ? 0 : 1;
        }

        // A base branch on the real stream plays the same days as the main line: its net worth path must match.
        static int Check(Search search, EpisodeStats s)
        {
            int bad = 0, compared = 0;
            foreach (JObject d in search.Decisions)
            {
                int day = (int)d["day"], i0 = (int)d["nw_index"];
                var path = d["base_nw_path"] as JArray ?? new JArray();
                for (int k = 0; k < path.Count; k++)
                {
                    int idx = i0 + k;
                    if (idx >= s.NetWorthByDay.Count) break;
                    compared++;
                    if ((int)path[k] != s.NetWorthByDay[idx])
                    {
                        bad++;
                        Console.WriteLine($"  day {day}+{k}: branch {path[k]} main {s.NetWorthByDay[idx]}");
                        break;
                    }
                }
            }
            Console.WriteLine(bad == 0 ? $"check ok: {compared} branch days match the main line" : $"check FAILED: {bad} branches diverge");
            return bad == 0 ? 0 : 1;
        }

        static JObject End()
        {
            var ps = PlayerStore.Instance;
            var j = new JObject();
            try
            {
                j["permit"] = ps.hasBusinessPermit;
                j["crime"] = ps.secData?.GetTotalCrimeValue() ?? -1;
                j["evidence"] = ps.secData?.evidenceLevel ?? -1;
                var crimes = new JObject();
                for (int i = 0; i < (ps.secData?.crimeTypes?.Count ?? 0); i++)
                    crimes[ps.secData.crimeTypes[i]] = new JArray(ps.secData.crimeAmount[i], ps.secData.crimeAmountTotal[i]);
                j["crimes"] = crimes;
                j["rent"] = ps.GetRentValue();
                j["net_worth"] = ps.GetTotalEstimatedValue();
            }
            catch (Exception e)
            {
                j["error"] = e.Message;
            }
            return j;
        }
    }
}
