using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Nodes;
using Json.Schema;
using Xunit;

namespace Fence.Gym.Tests
{
    // Each episode runs in its own EpisodeContext (fresh game statics), driven through the same
    // JSON entry point `fence-gym run -repeat` uses. Outputs go to var/sim/test/ (never committed).
    [CollectionDefinition("gym", DisableParallelization = true)]
    public sealed class GymCollection { }

    [Collection("gym")]
    public sealed class GymTests
    {
        static readonly object BootLock = new object();

        static string Dir(string name)
        {
            lock (BootLock) Boot.Init();
            string d = Boot.Var("sim", "test", name);
            if (Directory.Exists(d)) Directory.Delete(d, true);
            Directory.CreateDirectory(d);
            return d;
        }

        static JsonObject Play(int seed, int days, string trajectory, Action<JsonObject> extra = null)
        {
            var args = new JsonObject
            {
                ["seed"] = seed, ["days"] = days, ["difficulty"] = "normal", ["start"] = "none", ["skip_intro"] = false,
                ["deadline_ms"] = 10000, ["verbose"] = false, ["human"] = "default", ["bridge"] = null, ["trajectory"] = trajectory,
            };
            extra?.Invoke(args);
            lock (BootLock) Boot.Init();
            var result = JsonNode.Parse(EpisodeContext.Play(args.ToJsonString())).AsObject();
            Assert.True(result["crash"] == null, "episode crashed: " + result["crash"]);
            return result;
        }

        [Fact]
        public void SameSeedSamePolicyIsByteIdentical()
        {
            string d = Dir("determinism");
            Play(11, 12, Path.Combine(d, "a.jsonl"));
            Play(11, 12, Path.Combine(d, "b.jsonl"));
            Play(12, 12, Path.Combine(d, "c.jsonl"));
            byte[] a = File.ReadAllBytes(Path.Combine(d, "a.jsonl")), b = File.ReadAllBytes(Path.Combine(d, "b.jsonl"));
            Assert.True(a.Length > 10000, "trajectory too small: " + a.Length);
            Assert.Equal(Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(a)), Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(b)));
            Assert.NotEqual(a, File.ReadAllBytes(Path.Combine(d, "c.jsonl")));
        }

        [Fact]
        public void PhasesRunInTheGamesOrder()
        {
            var r = Play(5, 3, null, a => a["phase_log"] = true);
            var phases = r["phases"].AsArray().Select(x => ((string)x).Split(':')).Select(p => (day: int.Parse(p[0]), phase: p[1])).ToList();
            string[] order = { "Morning", "Window", "Open", "Customer", "Close", "Night", "Sleep" };
            int prevDay = phases[0].day, prevRank = -1, customers = 0;
            foreach (var (day, phase) in phases.Where(p => p.phase != "Over"))
            {
                int rank = Array.IndexOf(order, phase);
                Assert.True(rank >= 0, "unknown phase " + phase);
                if (phase == "Morning")
                {
                    Assert.True(prevRank == -1 || prevRank == 6, "a day must end with Sleep before the next Morning");
                    // The day counter advances during sleep, so Sleep already reports the next day.
                    if (prevRank == 6) Assert.Equal(prevDay, day);
                }
                else Assert.True(rank > prevRank || (phase == "Customer" && prevRank == 3), $"{phase} after {order[prevRank]} on day {day}");
                if (phase == "Customer") customers++;
                if (phase == "Sleep") Assert.Equal(prevDay + 1, day);
                else Assert.Equal(prevDay, day);
                prevRank = rank;
                prevDay = day;
            }
            Assert.True(3 == (int)r["days_played"], "days played " + r["days_played"] + ": " + r["phases"].ToJsonString());
            Assert.True(customers >= 3, "no customers served");
        }

        // Fence.Core's replica of the tablet's gates against the game's own methods, for every customer
        // the gym asks about over a month.
        [Fact]
        public void OptionSetsMatchTheGamesOwnGates()
        {
            var r = Play(3, 30, null, a => a["parity"] = true);
            int checkedCount = (int)r["parity"]["checked"];
            var problems = r["parity"]["problems"].AsArray().Select(x => (string)x).ToList();
            Assert.True(checkedCount >= 50, "only " + checkedCount + " decisions checked");
            Assert.True(problems.Count == 0, string.Join("\n", problems));
        }

        // Every request matches the schema the live mod's requests are held to (protocol/schema).
        [Fact]
        public void WireMatchesProtocolSchemas()
        {
            string d = Dir("wire");
            string wire = Path.Combine(d, "wire.jsonl");
            Play(4, 8, null, a => a["wire"] = wire);
            var schemas = new Dictionary<string, JsonSchema>
            {
                ["hello"] = Schema("hello.request"), ["event"] = Schema("event.request"), ["decide"] = Schema("decide.request"),
                ["decide_response"] = Schema("decide.response"), ["outcome"] = Schema("outcome.request"),
            };
            var seen = new Dictionary<string, int>();
            foreach (var line in File.ReadLines(wire))
            {
                var node = JsonNode.Parse(line);
                string type = (string)node["type"];
                seen[type] = seen.GetValueOrDefault(type) + 1;
                var result = schemas[type].Evaluate(node["body"], new EvaluationOptions { OutputFormat = OutputFormat.List });
                Assert.True(result.IsValid, type + " fails its schema: " + string.Join("; ", result.Details.Where(x => x.HasErrors).SelectMany(x => x.Errors.Select(e => x.InstanceLocation + " " + e.Value))) + "\n" + line);
            }
            foreach (var t in new[] { "hello", "event", "decide", "decide_response", "outcome" })
                Assert.True(seen.GetValueOrDefault(t) > 0, "no " + t + " sent");
            var hello = JsonNode.Parse(File.ReadLines(wire).First())["body"];
            Assert.EndsWith(ModSide.VersionSuffix, (string)hello["mod_version"]);
        }

        static JsonSchema Schema(string name) => JsonSchema.FromFile(Path.Combine(Boot.Repo, "protocol", "schema", name + ".schema.json"));

        // The decide context has the shape of real play's, key for key, where the bridge has logged a live session.
        [Fact]
        public void ContextShapeMatchesRecordedLivePlay()
        {
            var live = LiveContexts().Take(200).ToList();
            if (live.Count == 0) return; // no recorded live session on this machine
            string d = Dir("shape");
            Play(6, 10, Path.Combine(d, "t.jsonl"));
            var sim = File.ReadLines(Path.Combine(d, "t.jsonl")).Select(l => JsonNode.Parse(l)).SelectMany(day => day["decisions"].AsArray()).Select(x => x["context"]).ToList();
            Assert.NotEmpty(sim);
            var liveKeys = new HashSet<string>(live.SelectMany(c => Keys(c, "")));
            var simKeys = new HashSet<string>(sim.SelectMany(c => Keys(c, "")));
            Assert.True(simKeys.IsSubsetOf(liveKeys) || liveKeys.Count == 0, "keys the live mod never sent: " + string.Join(", ", simKeys.Except(liveKeys)));
            var missing = liveKeys.Except(simKeys).Where(k => k.Count(c => c == '.') <= 1).ToList();
            Assert.True(missing.Count == 0, "top-level keys live play sends and the gym does not: " + string.Join(", ", missing));
        }

        static IEnumerable<JsonNode> LiveContexts()
        {
            string log = Path.Combine(Boot.Repo, "var", "log");
            if (!Directory.Exists(log)) yield break;
            foreach (var f in Directory.GetFiles(log, "*.jsonl").OrderBy(x => x))
                foreach (var line in File.ReadLines(f))
                {
                    if (!line.Contains("\"type\":\"decision\"")) continue;
                    JsonNode n;
                    try { n = JsonNode.Parse(line); } catch (JsonException) { continue; }
                    var ctx = n?["request"]?["context"];
                    if (ctx != null && (string)n["request"]["kind"] == "barter") yield return ctx;
                }
        }

        static IEnumerable<string> Keys(JsonNode n, string prefix)
        {
            if (n is JsonObject o)
                foreach (var kv in o)
                {
                    string k = prefix + kv.Key;
                    yield return k;
                    foreach (var sub in Keys(kv.Value, k + ".")) yield return sub;
                }
            else if (n is JsonArray a)
                foreach (var x in a)
                    foreach (var sub in Keys(x, prefix)) yield return sub;
        }

        // A headless game must never write a save, a log or an analytics file: the engine's data path
        // stays null, and no file outside var/ written during the run carries the run's id (a save
        // would). The live game or another editor may be writing next to us, so the check is by content.
        [Fact]
        public void NoWritesOutsideVar()
        {
            const int seed = 80871;
            string runId = "sim-s" + seed.ToString(System.Globalization.CultureInfo.InvariantCulture);
            var start = DateTime.UtcNow.AddSeconds(-1);
            string d = Dir("writes");
            var r = Play(seed, 8, Path.Combine(d, "t.jsonl"));
            Assert.True(r["persistent_data_path"] == null, "persistentDataPath must stay null: " + r["persistent_data_path"]);
            Assert.Equal(new[] { "t.jsonl" }, Directory.GetFiles(d).Select(Path.GetFileName).OrderBy(x => x).ToArray());
            string saves = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
                ".local/share/Steam/steamapps/compatdata/4348910/pfx/drive_c/users/steamuser/AppData/LocalLow/Questing Goose Studio/Probably Stolen");
            var suspects = Touched(saves, start, _ => true)
                .Concat(Touched(Boot.Repo, start, f => !f.Contains("/var/") && !f.Contains("/bin/") && !f.Contains("/obj/") && !f.Contains("/.git/")))
                .Where(f => new FileInfo(f).Length < 64 << 20 && File.ReadAllText(f).Contains(runId)).ToList();
            Assert.True(suspects.Count == 0, "written outside var/: " + string.Join(", ", suspects));
        }

        static IEnumerable<string> Touched(string dir, DateTime since, Func<string, bool> keep) => !Directory.Exists(dir) ? Enumerable.Empty<string>() :
            Directory.EnumerateFiles(dir, "*", new EnumerationOptions { RecurseSubdirectories = true, IgnoreInaccessible = true })
                .Where(f => keep(f) && File.GetLastWriteTimeUtc(f) >= since);
    }
}
