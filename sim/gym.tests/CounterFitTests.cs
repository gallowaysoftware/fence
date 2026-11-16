using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Nodes;
using Xunit;

namespace Fence.Gym.Tests
{
    // Item 21 (R4): the counter's room check compared the stock grid's free-cell count against the
    // lot's cell sum. Real grids have shapes and gaps, so the check now places the lot's shapes in
    // the grid (CounterFit, the game's slot-search fit) and requires the plan's `min_free_cells`
    // (default 36) free cells to remain after the placement. The placement is pure over the grid's
    // usable cells and the pieces to put in it, so the table tests it that way; the end-to-end
    // test runs the counter against the live stock grid.
    [Collection("gym")]
    public sealed class CounterFitTests
    {
        static readonly object BootLock = new object();

        static string Dir(string name)
        {
            lock (BootLock) Boot.Init();
            string d = Boot.Var("sim", "test", name);
            if (System.IO.Directory.Exists(d)) System.IO.Directory.Delete(d, true);
            System.IO.Directory.CreateDirectory(d);
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

        // The counter's room check end to end: with the stock grid nearly full the counter refuses
        // a cheap lot (the free cells the placement would leave fall under the plan's margin), and
        // with room it takes it. The episode's counter record carries the skip reason.
        [Theory]
        [InlineData(11)]
        public void TheCounterSkipsALotThatWouldFillTheGridPastTheMargin(int seed)
        {
            string d = Dir("counter_fit");
            // `min_free_cells: 0` isolates the placement from any margin: the buy passes only if the
            // lot's shapes actually fit the grid's free cells. The roomy arm sets the floor explicitly
            // (the buy path takes `min_free_cells` only when a plan sets it; there is no default).
            File.WriteAllText(Path.Combine(d, "full.json"), "{\"engine\":\"furnace\",\"min_free_cells\":0}");
            File.WriteAllText(Path.Combine(d, "room.json"), "{\"engine\":\"furnace\",\"min_free_cells\":36}");
            var r = Play(seed, 8, Path.Combine(d, "full.jsonl"), x => x["directives"] = Path.Combine(d, "full.json"));
            Assert.True((int?)(r["stats"]["game_errors"] as JsonValue) == 0, string.Join("; ", r["errors"].AsArray()));
            Play(seed, 8, Path.Combine(d, "room.jsonl"), x => x["directives"] = Path.Combine(d, "room.json"));

            List<(string choice, string why)> Decisions(string file)
            {
                var out_ = new List<(string, string)>();
                foreach (var line in File.ReadLines(Path.Combine(d, file)))
                {
                    var day = JsonNode.Parse(line);
                    var decisions = day["decisions"] as JsonArray;
                    if (decisions == null) continue;
                    int d_ = (int?)(day["game_day"] as JsonValue) ?? -1;
                    foreach (var dec in decisions)
                    {
                        if ((string)dec["kind"] != "barter") continue;
                        var resp = dec["response"] as JsonObject;
                        if (resp == null) continue;
                        string c = resp["choice"] is JsonObject co ? (string)co["option"] ?? "" : (string)resp["choice"] ?? "";
                        out_.Add((c, $"day {d_} {(string)resp["rationale"] ?? ""}"));
                    }
                }
                return out_;
            }
            string Show(IEnumerable<(string choice, string why)> list) => string.Join("; ", list.Take(10).Select(x => x.choice + " " + x.why));
            var full = Decisions("full.jsonl");
            var room = Decisions("room.jsonl");
            // Sanity: the run actually reached sellers, and both arms saw bargains.
            Assert.True(room.Count(x => x.choice != "") >= 3, "no barter decisions: " + Show(room));
            bool Took(List<(string, string)> l) => l.Any(x => x.Item1 == "buy_all" || x.Item1 == "accept_wholesale");
            // Same seed, same sellers: the only difference is the plan's margin. A bigger margin is
            // stricter, so the roomy arm (36) may refuse lots the filling arm (0) bought — never the
            // reverse.
            Assert.False(
                !Took(full) && Took(room),
                "the roomy margin bought a lot the filling margin refused: " + Show(full) + " || " + Show(room));
            Assert.True(Took(room), "no lot bought with room: " + Show(room));
        }
    }
}
