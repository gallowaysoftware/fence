using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Nodes;
using Xunit;

namespace Fence.Gym.Tests
{
    // Item 5 (R6): with the wine engine on but the winemaker still locked, bottled water was classified an
    // engine input (IsInputAny includes WaterIds under WineOn), took the lenient input floor and spend cap,
    // and the counter bought it forever. Water now counts as an input only while Production.WantsWaterId
    // accepts it; a water route with no winemaker prices it by the normal caps.
    [Collection("gym")]
    public sealed class InputWaterTests
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
                ["deadline_ms"] = 10000, ["verbose"] = false, ["human"] = "player", ["bridge"] = null, ["trajectory"] = trajectory,
            };
            extra?.Invoke(args);
            lock (BootLock) Boot.Init();
            var result = JsonNode.Parse(EpisodeContext.Play(args.ToJsonString())).AsObject();
            Assert.True(result["crash"] == null, "episode crashed: " + result["crash"]);
            return result;
        }

        static readonly string[] WaterIds = { "bottled_water", "large_bottled_water", "small_bottled_water", "bottled_water_premium", "water_jug" };

        // What the counter bought from sellers, per barter decision: the water in the lot the player took,
        // and the reason the player rule gave. A rationale carrying the "engine inputs" marker went
        // through the lenient input path; otherwise the ordinary caps path.
        sealed class WaterBuy
        {
            public int Day;
            public string Id;
            public long Paid;
            public string Why;
        }

        static List<WaterBuy> WaterBuys(string traj)
        {
            var out_ = new List<WaterBuy>();
            foreach (var line in File.ReadLines(traj))
            {
                var day = JsonNode.Parse(line);
                var decisions = day["decisions"] as JsonArray;
                if (decisions == null) continue;
                int d = (int?)(day["game_day"] as JsonValue) ?? -1;
                foreach (var dec in decisions)
                {
                    if ((string)dec["kind"] != "barter") continue;
                    var resp = dec["response"] as JsonObject;
                    if (resp == null) continue;
                    string choice = resp["choice"] is JsonObject co ? (string)co["option"] ?? "" : (string)resp["choice"] ?? "";
                    if (choice != "buy_all" && choice != "accept_wholesale") continue;
                    string why = (string)resp["rationale"] ?? "";
                    var theirs = (dec["context"]?["counter"]?["theirs"]) as JsonArray;
                    if (theirs == null) continue;
                    foreach (var o in theirs)
                    {
                        string id = (string)o["id"];
                        if (id != null && WaterIds.Contains(id))
                            out_.Add(new WaterBuy { Day = d, Id = id, Paid = (long?)(o["price"] as JsonValue) ?? 0, Why = why });
                    }
                }
            }
            return out_;
        }

        static bool IsInputBuy(WaterBuy w) => w.Why.Contains("engine inputs");

        static string Describe(IList<WaterBuy> list) => string.Join("; ", list.Take(6).Select(w => $"day {w.Day} {w.Id} {w.Paid} (\"{w.Why}\")"));

        // Wine engine, no winemaker obtainable (`upgrades_buy: []` empties the plan's list, which also
        // suppresses the engine's baseline WINEMAKER). Bottled water still reaches the counter with the
        // wine engine on, and before the fix it took the lenient input path ("engine inputs ...").
        [Theory]
        [InlineData(11)]
        [InlineData(31)]
        [InlineData(52)]
        public void WaterIsNotBoughtAsEngineInputWhileTheWinemakerIsLocked(int seed)
        {
            string d = Dir("input_water");
            File.WriteAllText(Path.Combine(d, "wine.json"), "{\"engine\":\"wine\",\"upgrades_buy\":[]}");
            var r = Play(seed, 10, Path.Combine(d, "t.jsonl"), x => x["directives"] = Path.Combine(d, "wine.json"));
            Assert.True((int?)(r["stats"]["game_errors"] as JsonValue) == 0, string.Join("; ", r["errors"].AsArray()));
            var inputBuys = WaterBuys(Path.Combine(d, "t.jsonl")).Where(IsInputBuy).ToList();
            Assert.True(inputBuys.Count == 0, "water bought as engine inputs while the winemaker was locked: " + Describe(inputBuys));
        }

        // Control arm: with the winemaker obtainable, the player reaches for water through the input path
        // once it is unlocked, so the previous test pins the classification, not an absence of water on
        // the counter.
        [Fact]
        public void WaterIsBoughtAsEngineInputOnceTheWinemakerIsUnlocked()
        {
            string d = Dir("input_water_unlocked");
            File.WriteAllText(Path.Combine(d, "wine.json"), "{\"engine\":\"wine\",\"upgrades_buy\":[\"WINEMAKER\"]}");
            var r = Play(11, 30, Path.Combine(d, "t.jsonl"), x => x["directives"] = Path.Combine(d, "wine.json"));
            Assert.True((int?)(r["stats"]["game_errors"] as JsonValue) == 0, string.Join("; ", r["errors"].AsArray()));
            var inputBuys = WaterBuys(Path.Combine(d, "t.jsonl")).Where(IsInputBuy).ToList();
            Assert.True(inputBuys.Count > 0, "no water bought as engine inputs after the winemaker: " + r["player"]);
        }
    }
}
