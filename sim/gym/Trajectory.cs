using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text;
using Fence.Protocol;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The gym's own copy of a run in the `fencedata export` shape (cmd/fencedata/build.go: one JSON
    // line per game day; hidden values only under `labels`). Timestamps are synthetic (a fixed epoch
    // plus one millisecond per record) so a run is byte-identical for the same seed and policy.
    internal sealed class Trajectory
    {
        static readonly DateTime Epoch = new DateTime(2000, 1, 1, 0, 0, 0, DateTimeKind.Utc);

        readonly string run;
        readonly SortedDictionary<int, JObject> days = new SortedDictionary<int, JObject>();
        JObject session = new JObject();
        string sessionId = "";
        long tick;
        readonly Dictionary<string, long?> clientOf = new Dictionary<string, long?>(StringComparer.Ordinal);
        readonly Dictionary<string, List<string>> optionsOf = new Dictionary<string, List<string>>(StringComparer.Ordinal);

        public Trajectory(string run) => this.run = run;

        string Ts() => Epoch.AddTicks(TimeSpan.TicksPerMillisecond * ++tick).ToString("yyyy-MM-dd'T'HH:mm:ss.fffK", CultureInfo.InvariantCulture);

        public void Hello(string id, string mode, string policy, string modVersion, string gameVersion)
        {
            sessionId = id;
            session = new JObject { ["session_id"] = id, ["mode"] = mode, ["policy"] = policy, ["mod_version"] = modVersion, ["game_version"] = gameVersion };
        }

        JObject Day(int day)
        {
            if (!days.TryGetValue(day, out var d))
            {
                d = new JObject
                {
                    ["run"] = run, ["run_id"] = 0, ["game_day"] = day, ["sessions"] = new JArray(session.DeepClone()),
                    ["snapshots"] = new JArray(), ["customers"] = new JArray(), ["decisions"] = new JArray(), ["human_actions"] = new JArray(),
                    ["outcomes"] = new JArray(), ["events"] = new JArray(), ["labels"] = new JArray(),
                };
                days[day] = d;
            }
            return d;
        }

        static bool HasHidden(JObject h) => h != null && h.HasValues;

        public void Event(EventRequest e)
        {
            var d = Day(e.GameDay);
            string ts = Ts();
            JToken clientSeq = JValue.CreateNull();
            switch (e.Kind)
            {
                case "day_snapshot":
                    ((JArray)d["snapshots"]).Add(new JObject { ["session_id"] = sessionId, ["seq"] = e.Seq, ["ts"] = ts, ["phase"] = e.Data.Value<string>("phase"), ["data"] = e.Data });
                    break;
                case "customer_observed":
                    clientSeq = e.Data["client_seq"] ?? JValue.CreateNull();
                    ((JArray)d["customers"]).Add(new JObject
                    {
                        ["session_id"] = sessionId, ["seq"] = e.Seq, ["ts"] = ts, ["client_seq"] = clientSeq, ["covered"] = e.Data["covered"],
                        ["reason"] = e.Data.Value<string>("reason") ?? "", ["context"] = e.Data["context"] ?? JValue.CreateNull(), ["options"] = e.Data["options"] ?? JValue.CreateNull(),
                    });
                    break;
                case "human_action":
                    clientSeq = e.Data["client_seq"] ?? JValue.CreateNull();
                    string decision = e.Data.Value<string>("decision_id") ?? "";
                    string advised = e.Data.Value<string>("advised") ?? "";
                    var row = new JObject
                    {
                        ["session_id"] = sessionId, ["seq"] = e.Seq, ["ts"] = ts, ["client_seq"] = clientSeq, ["action"] = e.Data.Value<string>("action"),
                        ["item_uid"] = e.Data["item_uid"] ?? JValue.CreateNull(),
                    };
                    if (decision != "") row["decision_id"] = decision;
                    if (advised != "") row["advised"] = advised;
                    row["matched"] = Matched(e.Data.Value<string>("action"), advised, decision != "" && optionsOf.TryGetValue(decision, out var o) ? o : null);
                    row["data"] = e.Data;
                    ((JArray)d["human_actions"]).Add(row);
                    break;
                default:
                    ((JArray)d["events"]).Add(new JObject { ["session_id"] = sessionId, ["seq"] = e.Seq, ["ts"] = ts, ["kind"] = e.Kind, ["data"] = e.Data ?? new JObject() });
                    break;
            }
            if (HasHidden(e.Hidden))
                ((JArray)d["labels"]).Add(new JObject { ["source"] = e.Kind, ["session_id"] = sessionId, ["seq"] = e.Seq, ["client_seq"] = clientSeq, ["hidden"] = e.Hidden });
        }

        // memory.AdviceMatched.
        static JToken Matched(string action, string advised, List<string> options)
        {
            if (string.IsNullOrEmpty(advised) || advised == Choice.Defer || string.IsNullOrEmpty(action)) return JValue.CreateNull();
            if (action == advised) return true;
            if (options != null && options.Contains(action)) return false;
            return JValue.CreateNull();
        }

        public void Decision(DecideRequest req, JObject response, string mode, string policy, long latencyMs)
        {
            long? client = null;
            var sig = req.StateSignature ?? "";
            if (sig.StartsWith("c"))
            {
                int i = sig.IndexOf(':');
                if (long.TryParse(i < 0 ? sig.Substring(1) : sig.Substring(1, i - 1), NumberStyles.None, CultureInfo.InvariantCulture, out var n)) client = n;
            }
            clientOf[req.DecisionId] = client;
            optionsOf[req.DecisionId] = req.Options.Select(o => o.Id).ToList();
            var d = Day(req.GameDay);
            var row = new JObject
            {
                ["session_id"] = sessionId, ["ts"] = Ts(), ["decision_id"] = req.DecisionId, ["kind"] = req.Kind,
                ["client_seq"] = client.HasValue ? new JValue(client.Value) : JValue.CreateNull(), ["mode"] = mode, ["policy"] = policy,
            };
            if (!string.IsNullOrEmpty(req.StateSignature)) row["state_signature"] = req.StateSignature;
            row["context"] = req.Context;
            row["options"] = JArray.FromObject(req.Options.Select(o => new JObject { ["id"] = o.Id, ["label"] = o.Label }));
            row["response"] = response ?? (JToken)JValue.CreateNull();
            row["latency_ms"] = latencyMs;
            ((JArray)d["decisions"]).Add(row);
            if (req.Hidden != null && req.Hidden.HasValues)
                ((JArray)d["labels"]).Add(new JObject
                {
                    ["source"] = "decision", ["session_id"] = sessionId, ["decision_id"] = req.DecisionId,
                    ["client_seq"] = client.HasValue ? new JValue(client.Value) : JValue.CreateNull(), ["hidden"] = req.Hidden,
                });
        }

        public void Outcome(OutcomeRequest o)
        {
            clientOf.TryGetValue(o.DecisionId ?? "", out var client);
            ((JArray)Day(o.GameDay)["outcomes"]).Add(new JObject
            {
                ["session_id"] = sessionId, ["ts"] = Ts(), ["decision_id"] = o.DecisionId,
                ["client_seq"] = client.HasValue ? new JValue(client.Value) : JValue.CreateNull(), ["known_decision"] = clientOf.ContainsKey(o.DecisionId ?? ""),
                ["result"] = o.Result,
            });
        }

        public void Write(string path)
        {
            System.IO.Directory.CreateDirectory(Path.GetDirectoryName(path));
            var sb = new StringBuilder();
            foreach (var d in days.Values) sb.Append(d.ToString(Formatting.None)).Append('\n');
            string tmp = path + ".tmp";
            File.WriteAllText(tmp, sb.ToString());
            File.Move(tmp, path, true);
        }
    }
}
