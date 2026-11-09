using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Runtime.CompilerServices;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // fence-gym: the real game, headless, as an environment for bridge policies.
    //   run    one episode (in-process heuristic, or -bridge URL for any bridge policy)
    //   smoke  N seeds × D days in parallel child processes; prints outcomes
    //   bench  throughput of the in-process mode (and -bridge for HTTP)
    static class Program
    {
        static int Main(string[] args)
        {
            string cmd = args.Length > 0 && !args[0].StartsWith("-") ? args[0] : "run";
            var a = Args.Parse(args.SkipWhile(x => !x.StartsWith("-")).ToArray());
            Boot.Init();
            try
            {
                switch (cmd)
                {
                    case "run": return RunOne(a);
                    case "smoke": return Smoke(a);
                    case "bench": return Bench(a);
                    case "probe": return Probe.Run();
                    default:
                        Console.Error.WriteLine("usage: fence-gym run|smoke|bench [-seed N] [-days D] [-difficulty normal] [-start none] [-bridge URL] [-out FILE] [-no-trajectory] [-summary FILE] [-v]");
                        return 2;
                }
            }
            catch (ArgumentException e)
            {
                Console.Error.WriteLine(e.Message);
                return 2;
            }
        }

        static GymOptions Options(Args a) => new GymOptions
        {
            Seed = a.Int("seed", 1),
            Days = a.Int("days", 30),
            Difficulty = a.Str("difficulty", "normal"),
            Start = a.Str("start", "none"),
            SkipIntro = a.Flag("skip-intro"),
            DeadlineMs = a.Int("deadline-ms", 10000),
            Verbose = a.Flag("v"),
            Human = a.Str("human", "default"),
            Parity = a.Flag("parity"),
            WireLog = a.Str("wire", null),
        };

        static string RunName(GymOptions o, string bridge) =>
            $"sim-{o.Difficulty}-{o.Start.ToLowerInvariant()}-s{o.Seed}-{(bridge == null ? "local" : "bridge")}{(o.Human == "default" ? "" : "-" + o.Human)}";

        // -repeat N plays N episodes in this process (an in-process reset); files after the first get .2, .3, …
        static int RunOne(Args a)
        {
            var o = Options(a);
            string bridge = a.Str("bridge", null);
            string name = RunName(o, bridge);
            string outPath = a.Str("out", Boot.Var("sim", "runs", name + ".jsonl"));
            int repeat = a.Int("repeat", 1);
            for (int k = 1; k <= repeat; k++)
            {
                string path = k == 1 ? outPath : Path.ChangeExtension(outPath, null) + "." + k + ".jsonl";
                string traj = a.Flag("no-trajectory") ? null : path;
                var result = k == 1 ? Play(o, bridge, traj) : JObject.Parse(EpisodeContext.Play(new JObject
                {
                    ["seed"] = o.Seed, ["days"] = o.Days, ["difficulty"] = o.Difficulty, ["start"] = o.Start, ["skip_intro"] = o.SkipIntro,
                    ["deadline_ms"] = o.DeadlineMs, ["verbose"] = o.Verbose, ["human"] = o.Human, ["bridge"] = bridge, ["trajectory"] = traj, ["parity"] = o.Parity, ["wire"] = o.WireLog,
                }.ToString()));
                string summary = k == 1 ? a.Str("summary", Path.ChangeExtension(outPath, null) + ".summary.json") : Path.ChangeExtension(path, null) + ".summary.json";
                System.IO.Directory.CreateDirectory(Path.GetDirectoryName(summary));
                File.WriteAllText(summary, result.ToString(Formatting.Indented) + "\n");
                Console.WriteLine($"{name}: day {result["last_day"]}, cash {result["cash"]}, {(string)result["game_over"] ?? "alive"}, " +
                                  $"{result["stats"]["customers"]} customers, {result["stats"]["decisions"]} decisions, {result["seconds"]} s ({result["days_per_minute"]} days/min)");
                if (a.Flag("v"))
                    foreach (var kv in (JObject)result["defaults"]) Console.WriteLine($"  default {kv.Key}: {kv.Value}");
            }
            return 0;
        }

        // The guard that keeps a headless game away from real saves: the engine call is stubbed to null.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static string PersistentDataPath() => UnityEngine.Application.persistentDataPath;

        // Entry point inside an EpisodeContext.
        static string PlayJson(string json)
        {
            Boot.Init();
            var j = JObject.Parse(json);
            var o = new GymOptions
            {
                Seed = (int)j["seed"], Days = (int)j["days"], Difficulty = (string)j["difficulty"], Start = (string)j["start"],
                SkipIntro = (bool)j["skip_intro"], DeadlineMs = (int)j["deadline_ms"], Verbose = (bool)j["verbose"], Human = (string)j["human"],
                Parity = (bool?)j["parity"] ?? false, PhaseLog = (bool?)j["phase_log"] ?? false, WireLog = (string)j["wire"],
            };
            return Play(o, (string)j["bridge"], (string)j["trajectory"]).ToString(Formatting.None);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static JObject Play(GymOptions o, string bridgeUrl, string trajectoryPath)
        {
            var trajectory = trajectoryPath != null ? new Trajectory(RunName(o, bridgeUrl)) : null;
            IBridge inner = bridgeUrl != null ? new HttpBridge(bridgeUrl) : new LocalBridge(o.Seed);
            using IBridge bridge = o.WireLog != null ? new WireLog(inner, o.WireLog) : inner;
            var ep = new Episode(o, bridge, trajectory);
            if (o.Verbose) ep.Defaults.Log = (k, d) => Console.Error.WriteLine($"  [day {ep.Day}] default {k}: {d}");
            var sw = Stopwatch.StartNew();
            string crash = null;
            int startDay = 0;
            try
            {
                ep.Step();
                startDay = ep.Day;
                while (ep.Step()) { }
            }
            catch (Exception e)
            {
                crash = Fence.Sim.Game.Inner(e);
            }
            sw.Stop();
            trajectory?.Write(trajectoryPath);
            int played = Math.Max(0, ep.Day - startDay);
            var s = ep.Stats;
            return new JObject
            {
                ["seed"] = o.Seed,
                ["difficulty"] = o.Difficulty,
                ["start"] = o.Start,
                ["human_role"] = o.Human,
                ["bridge"] = bridgeUrl ?? "local",
                ["days_requested"] = o.Days,
                ["days_played"] = s.CashByDay.Count,
                ["last_day"] = ep.Day,
                ["cash"] = ep.Cash,
                ["game_over"] = ep.GameOver,
                ["crash"] = crash,
                ["cash_by_day"] = new JArray(s.CashByDay),
                ["net_worth_by_day"] = new JArray(s.NetWorthByDay),
                ["stats"] = new JObject
                {
                    ["customers"] = s.Customers, ["covered"] = s.Covered, ["decisions"] = s.Decisions, ["deferred"] = s.Deferred,
                    ["executed"] = s.Executed, ["sales"] = s.Sales, ["purchases"] = s.Purchases, ["walkouts"] = s.Walkouts,
                    ["forced_dismissals"] = s.Forced, ["dialogue_stalls"] = s.Stalls, ["game_errors"] = s.GameErrors,
                    ["events"] = ep.Mod.EventsSent, ["decision_failures"] = ep.Mod.DecisionFailures,
                },
                ["defaults"] = JObject.FromObject(ep.Defaults.Counts),
                ["errors"] = new JArray(s.Errors),
                ["parity"] = new JObject { ["checked"] = s.ParityChecked, ["problems"] = new JArray(s.ParityProblems) },
                ["phases"] = new JArray(s.Phases),
                ["persistent_data_path"] = PersistentDataPath(),
                ["seconds"] = Math.Round(sw.Elapsed.TotalSeconds, 3),
                ["days_per_minute"] = sw.Elapsed.TotalMinutes > 0 ? Math.Round(s.CashByDay.Count / sw.Elapsed.TotalMinutes, 1) : 0,
                ["decisions_per_second"] = sw.Elapsed.TotalSeconds > 0 ? Math.Round(s.Decisions / sw.Elapsed.TotalSeconds, 1) : 0,
            };
        }

        // Child processes: each run gets a fresh game (the game keeps static state).
        static int Smoke(Args a)
        {
            int seeds = a.Int("seeds", 20), days = a.Int("days", 30), parallel = a.Int("parallel", Math.Max(1, Environment.ProcessorCount / 2));
            string dir = a.Str("dir", Path.Combine(Boot.FindRepo(), "var", "sim", "smoke"));
            System.IO.Directory.CreateDirectory(dir);
            string self = Environment.ProcessPath;
            string dll = typeof(Program).Assembly.Location;
            var extra = a.Passthrough("difficulty", "start", "bridge", "human");
            var sw = Stopwatch.StartNew();
            var results = new JObject[seeds];
            Parallel.For(1, seeds + 1, new ParallelOptions { MaxDegreeOfParallelism = parallel }, seed =>
            {
                string summary = Path.Combine(dir, $"s{seed}.summary.json");
                string traj = Path.Combine(dir, $"s{seed}.jsonl");
                var psi = new ProcessStartInfo(self.EndsWith("dotnet") ? self : self) { RedirectStandardOutput = true, RedirectStandardError = true };
                if (Path.GetFileNameWithoutExtension(self) == "dotnet") psi.ArgumentList.Add(dll);
                foreach (var x in new[] { "run", "-seed", seed.ToString(), "-days", days.ToString(), "-out", traj, "-summary", summary }.Concat(extra)) psi.ArgumentList.Add(x);
                using var p = Process.Start(psi);
                string stdout = p.StandardOutput.ReadToEnd();
                string stderr = p.StandardError.ReadToEnd();
                p.WaitForExit();
                results[seed - 1] = File.Exists(summary) && p.ExitCode == 0
                    ? JObject.Parse(File.ReadAllText(summary))
                    : new JObject { ["seed"] = seed, ["crash"] = "exit " + p.ExitCode + ": " + Tail(stderr + stdout) };
            });
            sw.Stop();
            Report(results, sw.Elapsed, dir);
            return results.Any(r => r["crash"]?.Type == JTokenType.String) ? 1 : 0;
        }

        static string Tail(string s) => s.Length > 600 ? s.Substring(s.Length - 600) : s;

        static void Report(JObject[] results, TimeSpan wall, string dir)
        {
            var inv = CultureInfo.InvariantCulture;
            Console.WriteLine("seed  days  cash   ending        cust  cov  dec  def  sold bought walk forced err");
            foreach (var r in results)
            {
                var st = r["stats"] as JObject ?? new JObject();
                Console.WriteLine(string.Format(inv, "{0,4} {1,5} {2,6}   {3,-12} {4,5} {5,4} {6,4} {7,4} {8,5} {9,6} {10,4} {11,6} {12,3}",
                    r["seed"], r["days_played"], r["cash"], (string)r["game_over"] ?? (r["crash"]?.Type == JTokenType.String ? "CRASH" : "alive"),
                    st["customers"], st["covered"], st["decisions"], st["deferred"], st["sales"], st["purchases"], st["walkouts"], st["forced_dismissals"], st["game_errors"]));
            }
            var defaults = new SortedDictionary<string, int>(StringComparer.Ordinal);
            foreach (var r in results)
                if (r["defaults"] is JObject d)
                    foreach (var kv in d) defaults[kv.Key] = defaults.GetValueOrDefault(kv.Key) + (int)kv.Value;
            Console.WriteLine("defaults taken (all seeds):");
            foreach (var kv in defaults) Console.WriteLine($"  {kv.Key,-36} {kv.Value}");
            var errors = results.SelectMany(r => (r["errors"] as JArray ?? new JArray()).Select(e => (string)e)).GroupBy(e => e.Substring(e.IndexOf(' ', 4) + 1)).OrderByDescending(g => g.Count()).Take(15);
            Console.WriteLine("game errors (top):");
            foreach (var g in errors) Console.WriteLine($"  {g.Count(),4}  {g.Key}");
            int crashes = results.Count(r => r["crash"]?.Type == JTokenType.String);
            double days = results.Sum(r => (double?)r["days_played"] ?? 0);
            Console.WriteLine(string.Format(inv, "{0} runs, {1} crashes, {2} days in {3:0.0} s wall ({4:0} days/min aggregate)", results.Length, crashes, days, wall.TotalSeconds, days / wall.TotalMinutes));
            File.WriteAllText(Path.Combine(dir, "smoke.json"), new JArray(results).ToString(Formatting.Indented) + "\n");
        }

        static int Bench(Args a)
        {
            Boot.Init();
            var o = Options(a);
            o.Days = a.Int("days", 200);
            var r = Play(o, a.Str("bridge", null), a.Flag("trajectory") ? Boot.Var("sim", "bench", "bench.jsonl") : null);
            Console.WriteLine($"{(a.Str("bridge", null) == null ? "in-process" : "http")}: {r["days_played"]} days, {r["stats"]["decisions"]} decisions, " +
                              $"{r["stats"]["customers"]} customers in {r["seconds"]} s = {r["days_per_minute"]} days/min, {r["decisions_per_second"]} decisions/s" +
                              (r["game_over"]?.Type == JTokenType.String ? $" (run ended: {r["game_over"]})" : ""));
            return 0;
        }
    }

    sealed class Args
    {
        readonly Dictionary<string, string> values = new Dictionary<string, string>(StringComparer.Ordinal);
        readonly HashSet<string> flags = new HashSet<string>(StringComparer.Ordinal);

        public static Args Parse(string[] args)
        {
            var a = new Args();
            for (int i = 0; i < args.Length; i++)
            {
                string k = args[i].TrimStart('-');
                if (i + 1 < args.Length && !args[i + 1].StartsWith("-")) a.values[k] = args[++i];
                else a.flags.Add(k);
            }
            return a;
        }

        public string Str(string k, string def) => values.TryGetValue(k, out var v) ? v : def;
        public int Int(string k, int def) => values.TryGetValue(k, out var v) ? int.Parse(v, CultureInfo.InvariantCulture) : def;
        public bool Flag(string k) => flags.Contains(k);

        public IEnumerable<string> Passthrough(params string[] keys)
        {
            foreach (var k in keys)
                if (values.TryGetValue(k, out var v)) { yield return "-" + k; yield return v; }
        }
    }
}
