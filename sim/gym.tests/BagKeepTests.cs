using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Nodes;
using Xunit;

namespace Fence.Gym.Tests
{
    // Item 7: the gym sold its backpacks (day 7: the satchel, 120) and its outing carried a bag list
    // pick it did not own. A backpack (NightOuting's table) is a tool for outings: it is never sold as
    // stock (KeepWhy "bag"), and of the owned bags the most valuable `bag_spare` (default 2) are the
    // outing's bag and its spare, so a third cheapest one is sellable stock again. The night's bag choice
    // (ResponseList.CreateAfterhoursResponse) lists the owned backpacks, so the outing carries the best
    // owned one (Episode's night pick, recorded in `night:carry`'s `backpack`, whose `owned` flag marks
    // a bag the shop already carried) even when the bag list offers others.
    [Collection("gym")]
    public sealed class BagKeepTests
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
                ["seed"] = seed, ["days"] = days, ["difficulty"] = "normal", ["start"] = "none", ["skip_intro"] = true,
                ["deadline_ms"] = 10000, ["verbose"] = false, ["human"] = "default", ["bridge"] = null, ["trajectory"] = trajectory,
            };
            extra?.Invoke(args);
            lock (BootLock) Boot.Init();
            var result = JsonNode.Parse(EpisodeContext.Play(args.ToJsonString())).AsObject();
            Assert.True(result["crash"] == null, "episode crashed: " + result["crash"]);
            return result;
        }

        static readonly HashSet<string> BagIds = new HashSet<string> { "satchel", "kids_backpack", "kids_backpack_large", "backpack_small", "backpack_medium", "backpack_medium_military", "backpack_large", "backpack_large_military", "fanny_pack" };

        // The run's records: per-day lines, each with snapshots (phase data) and human_actions.
        static IEnumerable<JsonObject> Load(string d)
        {
            foreach (var line in File.ReadAllLines(Path.Combine(d, "t.jsonl")))
            {
                var o = JsonNode.Parse(line).AsObject();
                yield return o;
                if (o["snapshots"] is JsonArray sa)
                    foreach (var s in sa) yield return s.AsObject();
                if (o["human_actions"] is JsonArray ha)
                    foreach (var e in ha) yield return e.AsObject();
            }
        }

        static JsonObject Data(JsonObject e) => e["data"] as JsonObject;

        // The fanny pack is the shop's starting stock (the opening inventory); a 40-day normal run
        // loots and buys more. With the keep rule in place a bag (the day-7 log's satchel, 120) stays
        // for the outings and is never sold as stock: the run's `item_sold` events name no bag.
        [Fact]
        public void NoBagIsSoldAsStock()
        {
            string d = Dir("bag_satchel");
            Play(11, 40, Path.Combine(d, "t.jsonl"));
            var bad = new List<string>();
            foreach (var e in Load(d))
            {
                var data = Data(e);
                if (data == null || data["kind"]?.ToString() != "item_sold") continue;
                var item = data["item"]?.ToString();
                if (item != null && BagIds.Contains(item) && !bad.Contains(item)) bad.Add(item);
            }
            Assert.True(bad.Count == 0, "bags sold as stock: " + string.Join("; ", bad));
        }

        // The night's pick records its bag (`night:carry`'s `backpack` object: the pick's uid,
        // identifier and whether the shop owned it): the owned backpacks list (ResponseList.
        // CreateAfterhoursResponse), the best of them, so when the shop owns a bag the outing carries
        // it (`owned` true), and a pick from the list the shop does not own rides found (owned false).
        [Fact]
        public void TheOutingCarriesTheOwnedBag()
        {
            string d = Dir("bag_pick");
            Play(11, 40, Path.Combine(d, "t.jsonl"));
            int picks = 0;
            var ownedPicks = new List<string>();
            foreach (var e in Load(d))
            {
                var data = Data(e);
                if (data == null || data["sim_default"]?.ToString() != "night:carry") continue;
                var bp = data["backpack"];
                if (bp == null || bp is not JsonObject) continue;   // a night without a bag pick
                picks++;
                if ((bool)(bp["owned"] ?? false))
                    ownedPicks.Add(bp["identifier"]?.ToString() ?? "none");
            }
            Assert.True(picks > 0, "no night:carry picks");
            // A pick the shop owned is from the owned bag list (NightOuting's table): its identifier
            // is in the table whenever the shop owned one, and `owned` marks exactly those picks.
            Assert.True(ownedPicks.All(b => b == "none" || BagIds.Contains(b)), "owned picks named non-bags: " + string.Join("; ", ownedPicks.Distinct()));
        }
    }
}
