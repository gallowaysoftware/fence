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

        // Strategy experiments compare knob arms against `-human knobs` with no knobs; it must be the default role.
        [Fact]
        public void KnobsWithoutKnobsPlayLikeTheDefaultRole()
        {
            string d = Dir("knobs");
            var a = Play(3, 15, Path.Combine(d, "default.jsonl"));
            var b = Play(3, 15, Path.Combine(d, "knobs.jsonl"), x => x["human"] = "knobs");
            string Norm(string f) => File.ReadAllText(Path.Combine(d, f)).Replace("\"human_role\":\"knobs\"", "\"human_role\":\"default\"").Replace("-knobs", "");
            Assert.Equal(Norm("default.jsonl"), Norm("knobs.jsonl"));
            Assert.Equal(a["net_worth_by_day"].ToJsonString(), b["net_worth_by_day"].ToJsonString());
        }

        // The protocol's directives fixture is what a planner sends: the executor must read every field.
        [Fact]
        public void DirectivesFixtureParses()
        {
            lock (BootLock) Boot.Init();
            var d = Directives.Load(Path.Combine(Boot.Repo, "protocol", "fixtures", "directives.json"));
            Assert.Equal(600, d.CashFloor);
            Assert.Equal(70, d.WholesaleMaxPct);
            Assert.Equal(1350, d.SellDownToCash);
            Assert.Contains(2231, d.HoldUids);
            Assert.Equal(new[] { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" }, d.Services);
            Assert.Equal("shoot", d.Augs);
            string tmp = Dir("directives");
            File.WriteAllText(Path.Combine(tmp, "over.json"), "{\"bargain\":\"never\",\"sell\":{\"min_pct\":80}}");
            File.WriteAllText(Path.Combine(tmp, "bad.json"), "{\"permit\":\"maybe\"}");
            var over = Directives.Merge(d, Directives.Load(Path.Combine(tmp, "over.json")));
            Assert.Equal("never", over.Bargain);
            Assert.Equal(80, over.MinPct);
            Assert.Equal(1350, over.SellDownToCash);
            Assert.Throws<ArgumentException>(() => Directives.Load(Path.Combine(tmp, "bad.json")));
        }

        // The executor is deterministic too, and by day 20 on normal it has met an aug (scripted on days 13
        // and 18) and shot it, and bought the wine upgrade.
        [Fact]
        public void PlayerIsDeterministicAndShootsAugs()
        {
            string d = Dir("player");
            var a = Play(5, 20, Path.Combine(d, "a.jsonl"), x => x["human"] = "player");
            Play(5, 20, Path.Combine(d, "b.jsonl"), x => x["human"] = "player");
            Assert.Equal(File.ReadAllText(Path.Combine(d, "a.jsonl")), File.ReadAllText(Path.Combine(d, "b.jsonl")));
            Assert.True((int)a["player"]["aug_good_kills"] >= 1, "no aug shot: " + a["player"]);
            Assert.Equal(0, (int)a["player"]["aug_wrongful_kills"]);
            Assert.True((int)a["stats"]["game_errors"] == 0, string.Join("; ", a["errors"].AsArray()));
            string traj = File.ReadAllText(Path.Combine(d, "a.jsonl"));
            Assert.Contains("\"good_kill\"", traj);
            Assert.Contains("\"aug_check\"", traj);
            // The default is the fair detector; its verdicts are scored against the hidden truth.
            Assert.Equal("tells", (string)a["aug_detection"]);
            Assert.Equal(0, (int)a["aug_eval"]["false_positive"]);
            Assert.True((int)a["aug_eval"]["true_positive"] >= 1, "aug_eval: " + a["aug_eval"]);
            Assert.DoesNotContain("\"detection\":\"perfect\"", traj);
        }

        // Perfect detection (the truth flag) stays as a labelled analysis option.
        [Fact]
        public void PerfectDetectionIsAnAnalysisOption()
        {
            string d = Dir("perfect");
            var a = Play(5, 20, Path.Combine(d, "a.jsonl"), x =>
            {
                x["human"] = "player";
                x["aug_detection"] = "perfect";
            });
            Assert.Equal("perfect", (string)a["aug_detection"]);
            Assert.Null(a["aug_eval"]);
            Assert.True((int)a["player"]["aug_good_kills"] >= 1, "no aug shot: " + a["player"]);
            Assert.Contains("\"detection\":\"perfect\"", File.ReadAllText(Path.Combine(d, "a.jsonl")));
        }

        // The shared aug rule reads what the context shows: a visible tell or the scanner in tells mode, the truth
        // flag only in perfect mode.
        [Fact]
        public void AugRuleReadsTheDetectionModesContext()
        {
            static Fence.Protocol.DecideRequest Req(string context) => new Fence.Protocol.DecideRequest
            {
                Kind = "aug_check", Context = Newtonsoft.Json.Linq.JObject.Parse(context),
                Options = new List<Fence.Protocol.DecisionOption> { new() { Id = "shoot" }, new() { Id = "serve" } },
            };
            lock (BootLock) Boot.Init();
            Directives.Current = new Directives();
            Assert.Equal("shoot", AugRule.Pick(Req("{\"detection\":\"tells\",\"client\":{\"aug_tells\":[\"perfectly still\"]}}"), out _));
            Assert.Equal("serve", AugRule.Pick(Req("{\"detection\":\"tells\",\"client\":{\"aug\":true}}"), out _));
            Assert.Equal("serve", AugRule.Pick(Req("{\"detection\":\"tells\",\"client\":{\"aug_tells\":[\"x\"],\"scan\":\"No Aug detected\"}}"), out _));
            Assert.Equal("shoot", AugRule.Pick(Req("{\"detection\":\"tells\",\"client\":{\"scan\":\"AUG DETECTED\"}}"), out _));
            Assert.Equal("shoot", AugRule.Pick(Req("{\"detection\":\"perfect\",\"client\":{\"aug\":true}}"), out _));
            Assert.Equal(AugRule.Hidden, AugRule.Judge(Req("{\"detection\":\"perfect\",\"client\":{\"aug\":false}}").Context).Basis);
        }

        // Plan mode: a bridge in observe mode only supplies directives; when it defers, the executor's own
        // counter and aug rules play, so the game is the in-process player's.
        [Fact]
        public void PlayerUnderADeferringBridgePlaysItsOwnCounter()
        {
            var a = Play(5, 20, null, x => x["human"] = "player");
            var b = Play(5, 20, null, x => { x["human"] = "player"; x["bridge"] = "defer:"; });
            Assert.True((int)b["stats"]["deferred"] > 50, "the bridge did not defer: " + b["stats"]);
            Assert.Equal(a["net_worth_by_day"].ToJsonString(), b["net_worth_by_day"].ToJsonString());
            Assert.Equal(a["cash_by_day"].ToJsonString(), b["cash_by_day"].ToJsonString());
            Assert.Equal((int)a["player"]["aug_good_kills"], (int)b["player"]["aug_good_kills"]);
        }

        // Step mode: a snapshot each morning, the day played on the directives file, a result after it.
        [Fact]
        public void StepModeExchangesFiles()
        {
            string d = Dir("step");
            var t = System.Threading.Tasks.Task.Run(() => Play(4, 2, null, x => { x["human"] = "player"; x["step_dir"] = d; x["step_timeout"] = 60; }));
            for (int day = 1; day <= 2; day++)
            {
                string dir = Path.Combine(d, "day-" + day.ToString("000"));
                var until = DateTime.UtcNow.AddSeconds(60);
                while (!File.Exists(Path.Combine(dir, "snapshot.json")) && DateTime.UtcNow < until) System.Threading.Thread.Sleep(50);
                var snap = JsonNode.Parse(File.ReadAllText(Path.Combine(dir, "snapshot.json"))).AsObject();
                Assert.Equal(day, (int)snap["day"]);
                Assert.NotNull(snap["snapshot"]["store"]);
                File.WriteAllText(Path.Combine(dir, "directives.json"), "{\"bargain\":\"never\"}");
            }
            Assert.True(t.Wait(TimeSpan.FromSeconds(120)));
            var result = JsonNode.Parse(File.ReadAllText(Path.Combine(d, "day-002", "result.json"))).AsObject();
            Assert.Equal("never", (string)result["directives_used"]["bargain"]);
            Assert.True(File.Exists(Path.Combine(d, "final.json")));
        }

        [Fact]
        public void WarmStartAndPerksReachTheGame()
        {
            var r = Play(2, 2, null, x =>
            {
                x["difficulty"] = "hard";
                x["warm"] = "day=108,cash=6180,rent=1347,rent_days=4,permit=1,rep=lower_levels:88.27,upgrades=SHOWCASE_I+RENOVATION1";
                x["perks"] = "alert+bad_lease";
            });
            var st = r["start_state"].AsObject();
            Assert.Equal(108, (int)st["day"]);
            Assert.Equal(6180, (int)st["cash"]);
            Assert.Equal(1347, (int)st["rent"]);
            Assert.True((bool)st["permit"]);
            Assert.Equal(88.27, (double)st["rep"]["FACTION_LOWER_LEVEL"]);
            // RENOVATION1's own unlock action: +150 on the 250 base.
            Assert.Equal(400, (int)st["attract"]);
            Assert.Contains("SHOWCASE_I", st["upgrades"].AsArray().Select(x => (string)x));
            Assert.Equal(new[] { "alert", "bad_lease" }, r["perks"].AsArray().Select(x => (string)x));
            Assert.Equal(110, (int)r["last_day"]);
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
            // Keys the mod gained after the recorded live sessions; drop an entry once live logs carry it.
            var addedSinceRecording = new HashSet<string> { "client.aug" };
            var unknown = simKeys.Except(liveKeys).Where(k => !addedSinceRecording.Contains(k)).ToList();
            Assert.True(unknown.Count == 0 || liveKeys.Count == 0, "keys the live mod never sent: " + string.Join(", ", unknown));
            var missing = liveKeys.Except(simKeys).Where(k => k.Count(c => c == '.') <= 1).ToList();
            Assert.True(missing.Count == 0, "top-level keys live play sends and the gym does not: " + string.Join(", ", missing));
        }

        static IEnumerable<JsonNode> LiveContexts()
        {
            lock (BootLock) Boot.Init();
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

        // Every night's outing arrives (the travel's blackscreen callback once threw headless, so only the
        // first night's five searches ever happened): each night resets the attempts and searches again.
        [Fact]
        public void OutingsScavengeEveryNight()
        {
            string d = Dir("outings");
            var r = Play(1, 6, Path.Combine(d, "t.jsonl"), x => x["human"] = "player");
            int nights = File.ReadLines(Path.Combine(d, "t.jsonl"))
                .Count(l => JsonNode.Parse(l)["human_actions"]?.AsArray().Any(a => (string)a["action"] == "scavenge") == true);
            Assert.True(nights >= 5, "nights with a search: " + nights);
            Assert.True((int)r["stats"]["game_errors"] == 0, string.Join("; ", r["errors"].AsArray()));
        }

        // Outings pass the store door's gates (StoreDoor.OnClick): nobody goes out before the mentor introduces
        // after-hours, and with the agoraphobia perk a night out rests the next two.
        [Fact]
        public void OutingsPassTheDoorsGates()
        {
            string d = Dir("door");
            bool[] Out(string traj) => File.ReadLines(traj)
                .Select(l => JsonNode.Parse(l)["human_actions"]?.AsArray().Any(a => (string)a["action"] == "go_out") == true).ToArray();
            var intro = Play(1, 3, Path.Combine(d, "intro.jsonl"), x => x["human"] = "player");
            Assert.False(Out(Path.Combine(d, "intro.jsonl"))[0], "went out on night 1, before after-hours was introduced");
            Assert.Equal(1, (int)intro["defaults"]["night:door_shut"]);

            Play(1, 12, Path.Combine(d, "agoraphobia.jsonl"), x => { x["human"] = "player"; x["perks"] = "agoraphobia"; x["skip_intro"] = true; });
            var nights = Out(Path.Combine(d, "agoraphobia.jsonl"));
            Assert.True(nights.Count(n => n) >= 3, "nights out: " + nights.Count(n => n));
            for (int i = 0; i < nights.Length; i++)
                if (nights[i])
                    Assert.False(nights.Skip(i + 1).Take(2).Any(n => n), "went out during the agoraphobia rest after night " + (i + 1));
        }

        // `jackson_every: 0` means never; PlayerRole.Night once took the modulo before checking it.
        [Fact]
        public void JacksonEveryZeroNeverVisits()
        {
            string d = Dir("jackson-never");
            string file = Path.Combine(d, "never.json");
            File.WriteAllText(file, "{\"jackson_every\":0}");
            var r = Play(1, 8, Path.Combine(d, "t.jsonl"), x => { x["human"] = "player"; x["directives"] = file; });
            Assert.True((int)r["stats"]["game_errors"] == 0, string.Join("; ", r["errors"].AsArray()));
            Assert.Null(r["defaults"]["player:jackson_visit"]);
        }

        // `engine: wine+furnace` on hard: Dr. Jackson's day-6 furnace is bought, scrap and ore go in, ingots come
        // out and sell; the furnace, its modules and inputs are never sold. A list parses like the joined form.
        [Fact]
        public void FurnaceEngineSmeltsAndSellsIngots()
        {
            string d = Dir("furnace");
            File.WriteAllText(Path.Combine(d, "wf.json"), "{\"engine\":[\"wine\",\"furnace\"]}");
            Assert.Equal("wine+furnace", Directives.Load(Path.Combine(d, "wf.json")).Engine);
            File.WriteAllText(Path.Combine(d, "bad.json"), "{\"engine\":\"wine+forge\"}");
            Assert.Throws<ArgumentException>(() => Directives.Load(Path.Combine(d, "bad.json")));
            JsonObject Run(string traj) => Play(1, 30, traj, x =>
            {
                x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; x["directives"] = Path.Combine(d, "wf.json");
            });
            var a = Run(Path.Combine(d, "a.jsonl"));
            Run(Path.Combine(d, "b.jsonl"));
            Assert.Equal(File.ReadAllText(Path.Combine(d, "a.jsonl")), File.ReadAllText(Path.Combine(d, "b.jsonl")));
            var f = a["player"]["furnace"];
            Assert.True(f != null, "no furnace totals: " + a["player"]);
            Assert.True((int)f["first_day"] is > 0 and <= 8, "furnace not bought early: " + f);
            Assert.True((int)f["ingots_made"] >= 5, "too few ingots: " + f);
            Assert.True((int)f["ingots_sold"] >= 1, "no ingot sold: " + f);
            Assert.True((int)a["stats"]["game_errors"] == 0, string.Join("; ", a["errors"].AsArray()));
            string traj = File.ReadAllText(Path.Combine(d, "a.jsonl"));
            Assert.Contains("\"player:furnace\"", traj);
        }

        // Dr. Jackson is the counter's client at his shop (DrJacksonLocation.Arrive), so what the player buys there
        // is paid for, at the difficulty's inventor markup (normal: +15 %).
        [Fact]
        public void JacksonShopPurchasesBuy()
        {
            string d = Dir("jackson");
            File.WriteAllText(Path.Combine(d, "j7.json"), "{\"jackson_every\":7}");
            var r = Play(1, 28, null, x => { x["human"] = "player"; x["directives"] = Path.Combine(d, "j7.json"); });
            var j = r["player"]["jackson"];
            Assert.True((int)j["shop_visits"] >= 1, "never visited: " + j);
            Assert.Equal(115, (int)j["shop"]["sign_water"]["cash"]);
            Assert.True((int)r["stats"]["game_errors"] == 0, string.Join("; ", r["errors"].AsArray()));
        }

        // The `vending` perk's machine is stocked at bedtime, sells overnight and is emptied into the cash each
        // morning; common electronics raise its attractiveness. Without the perk nothing changes.
        [Fact]
        public void VendingMachineStocksAndSells()
        {
            string d = Dir("vending");
            var a = Play(1, 30, Path.Combine(d, "a.jsonl"), x =>
            {
                x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; x["perks"] = "vending";
            });
            var v = a["player"]["vending"];
            Assert.True(v != null, "no vending totals: " + a["player"]);
            Assert.True((int)v["income_total"] >= 200, "too little vending income: " + v);
            Assert.True((int)v["attractiveness"] > 15, "never upgraded: " + v);
            Assert.True((int)a["stats"]["game_errors"] == 0, string.Join("; ", a["errors"].AsArray()));
            var b = Play(1, 10, Path.Combine(d, "b.jsonl"), x => { x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; });
            Assert.True(b["player"]["vending"] == null, "vending without a machine: " + b["player"]["vending"]);
        }

        // The value-table interpreter, on a made-up table (the real one is local only): base plus matching rules,
        // first match per group, any invalid_when wins, floored at min; matchers equal, any-of, range, not.
        [Fact]
        public void AppraisalValueTableRules()
        {
            var saved = Appraisal.Guides;
            try
            {
                Appraisal.Guides = Newtonsoft.Json.Linq.JObject.Parse(@"{""widget"": {""base"": 10, ""min"": 5, ""invalid"": 1,
                    ""invalid_when"": [{""year"": {""not"": [{""range"": [1, 3]}, {""range"": [7, 9]}]}}, {""a"": ""x"", ""b"": {""not"": ""y""}}],
                    ""add"": [{""when"": {""a"": ""x""}, ""add"": 4}, {""when"": {""flag"": true, ""a"": [""x"", ""z""]}, ""add"": 2},
                              {""group"": ""g"", ""when"": {""year"": 2}, ""add"": 100}, {""group"": ""g"", ""when"": {""year"": {""range"": [1, 9]}}, ""add"": 50},
                              {""when"": {""c"": ""bad""}, ""add"": -100}]}}");
                int V(string cues) => Appraisal.Value(Newtonsoft.Json.Linq.JObject.Parse(@"{""type"": ""widget"", " + cues + "}"));
                Assert.Equal(60, V(@"""year"": 8"));
                Assert.Equal(110, V(@"""year"": 2"));
                Assert.Equal(1, V(@"""year"": 5"));
                Assert.Equal(1, V(@"""a"": ""x"", ""year"": 2"));
                Assert.Equal(116, V(@"""a"": ""x"", ""b"": ""y"", ""flag"": true, ""year"": 2"));
                Assert.Equal(1, V(@"""c"": ""bad"""));
                Assert.Equal(5, V(@"""c"": ""bad"", ""year"": 8"));
                Assert.Equal(0, Appraisal.Value(Newtonsoft.Json.Linq.JObject.Parse(@"{""type"": ""hand""}")));
            }
            finally
            {
                Appraisal.Guides = saved;
            }
        }

        // `appraisal: join` joins the board on day 8, shows the certificate, and reads every client's item off its
        // cues to the true value (the slip's). Without it the board is declined and nothing is reported.
        [Fact]
        public void AppraisalBoardJoinsAndEstimates()
        {
            string d = Dir("appraisal");
            Assert.True(Appraisal.Guides != null, "needs the guides' value tables in local/appraisal_guides.json (docs/re/APPRAISAL.md)");
            File.WriteAllText(Path.Combine(d, "join.json"), "{\"appraisal\":\"join\"}");
            var a = Play(1, 30, Path.Combine(d, "a.jsonl"), x =>
            {
                x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; x["directives"] = Path.Combine(d, "join.json");
            });
            var ap = a["player"]["appraisal"];
            Assert.True(ap != null, "no appraisal totals: " + a["player"]);
            Assert.Equal(8, (int)ap["joined_day"]);
            Assert.True((int)ap["items"] >= 10, "too few items appraised: " + ap);
            Assert.Equal((int)ap["items"], (int)ap["grades"][0]);
            Assert.Equal(0, (int)ap["misread"]);
            Assert.True((bool)ap["certificate_shown"], "certificate not in the case: " + ap);
            Assert.True((int)a["stats"]["game_errors"] == 0, string.Join("; ", a["errors"].AsArray()));
            var b = Play(1, 10, Path.Combine(d, "b.jsonl"), x => { x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; });
            Assert.True(b["player"]["appraisal"] == null, "appraisal without joining: " + b["player"]["appraisal"]);
        }

        // `fakes: "honest"` labels the injectors and cigarette packs whose cues show a fault to their truth, with the
        // day-4 labeler put away; `labeler: "per_buyer"` relabels them per buyer and puts them back after.
        [Fact]
        public void LabelerRelabelsIdentifiedFakes()
        {
            string d = Dir("labels");
            File.WriteAllText(Path.Combine(d, "honest.json"), "{\"fakes\":\"honest\"}");
            File.WriteAllText(Path.Combine(d, "per_buyer.json"), "{\"labeler\":\"per_buyer\"}");
            foreach (var arm in new[] { "honest", "per_buyer" })
            {
                var a = Play(6, 40, Path.Combine(d, arm + ".jsonl"), x =>
                {
                    x["human"] = "player"; x["difficulty"] = "hard"; x["start"] = "apprentice"; x["directives"] = Path.Combine(d, arm + ".json");
                });
                var l = a["player"]["labels"];
                Assert.True(l != null, "no label totals: " + a["player"]);
                Assert.True((int)l["honest_items"] >= 1, "no fake identified: " + l);
                Assert.True((int)l["mornings_without_labeler"] <= 4, "labeler lost: " + l);
                Assert.True((int)a["stats"]["game_errors"] == 0, string.Join("; ", a["errors"].AsArray()));
            }
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
