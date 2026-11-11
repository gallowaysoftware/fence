using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using Fence.Gym;
using Fence.Protocol;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Rollout
{
    // The main line's decision log in the bridge's format (internal/decisionlog), so fenceshow can replay a
    // hybrid game like a league match, plus one `show` line per hybrid decision carrying the feed's
    // `candidates` event (internal/show). Branches log nothing: they are forked children of the main line.
    internal sealed class ShowLog : IBridge
    {
        readonly IBridge inner;
        readonly StreamWriter w;
        string session = "";

        public ShowLog(IBridge inner, string path)
        {
            this.inner = inner;
            System.IO.Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(path)));
            w = new StreamWriter(path) { NewLine = "\n" };
        }

        static string Ts() => DateTime.UtcNow.ToString("yyyy-MM-dd'T'HH:mm:ss.fffffff'Z'", CultureInfo.InvariantCulture);

        void Line(JObject j)
        {
            if (Search.IsChild) return;
            w.WriteLine(j.ToString(Formatting.None));
        }

        static JToken Wire(object body) => JToken.Parse(ProtocolJson.Serialize(body));

        // Called before every fork, so a branch inherits an empty buffer.
        public void Flush() => w.Flush();

        public string Name => inner.Name;

        public HelloResponse Hello(HelloRequest hello)
        {
            var r = inner.Hello(hello);
            session = r.SessionId;
            Line(new JObject { ["type"] = "hello", ["ts"] = Ts(), ["session_id"] = session, ["mode"] = r.Mode, ["policy"] = "hybrid", ["hello"] = Wire(hello) });
            return r;
        }

        public void Event(EventRequest e)
        {
            Line(new JObject { ["type"] = "event", ["ts"] = Ts(), ["session_id"] = session, ["event"] = Wire(e) });
            inner.Event(e);
        }

        public DecideResponse Decide(DecideRequest request, out long latencyMs)
        {
            var r = inner.Decide(request, out latencyMs);
            Line(new JObject
            {
                ["type"] = "decision", ["ts"] = Ts(), ["session_id"] = session, ["decision_id"] = request.DecisionId, ["policy"] = "hybrid",
                ["request"] = Wire(request), ["response"] = r == null ? null : Wire(r), ["latency_ms"] = latencyMs,
            });
            return r;
        }

        public void Outcome(OutcomeRequest outcome)
        {
            Line(new JObject { ["type"] = "outcome", ["ts"] = Ts(), ["session_id"] = session, ["decision_id"] = outcome.DecisionId, ["outcome"] = Wire(outcome) });
            inner.Outcome(outcome);
        }

        public SummaryResponse Summary(string sessionId) => inner.Summary(sessionId);

        public void Dispose()
        {
            if (!Search.IsChild) w.Dispose();
            inner.Dispose();
        }

        public void Show(JObject ev) => Line(new JObject { ["type"] = "show", ["ts"] = Ts(), ["show"] = ev });

        // The decision as the panel's "Alternatives considered" box shows it: every candidate rolled, its mean
        // score in points of buyout progress, the paired verdict in the caption. Each option carries its delta
        // and the proposer's reason whole, so the replay can show the chosen one as today's plan.
        public static JObject Candidates(int day, List<Trigger> triggers, IList<(Candidate c, double score, JArray samples)> evals, int baseIndex, Candidate chosen, string argmax, int samples, int horizon)
        {
            var keep = evals[0];
            var baseC = evals[baseIndex].c;
            var options = new JArray();
            foreach (var e in evals)
            {
                int lost = e.samples.Count(s => s["game_over"]?.Type == JTokenType.String && !((string)s["game_over"]).StartsWith("buyout", StringComparison.Ordinal));
                int won = e.samples.Count(s => ((string)s["game_over"] ?? "").StartsWith("buyout", StringComparison.Ordinal));
                double nw = e.samples.Where(s => s["nw_end"] != null).Select(s => (double)s["nw_end"]).DefaultIfEmpty(0).Average();
                var note = new List<string>();
                if (!string.IsNullOrEmpty(e.c.Why)) note.Add(e.c.Why);
                note.Add($"net worth {nw:N0} after {horizon} days" + (won > 0 ? $", {won}/{e.samples.Count} bought out" : "") + (lost > 0 ? $", {lost}/{e.samples.Count} lost" : ""));
                options.Add(new JObject
                {
                    ["label"] = Label(e.c), ["source"] = e.c.Source, ["directives"] = new JArray(Words(e.c.Directives)),
                    ["score"] = Math.Round(e.score / 100, 1), ["stderr"] = Math.Round(StdErr(e.samples) / 100, 1),
                    ["rollouts"] = e.samples.Count, ["chosen"] = e.c == chosen, ["note"] = string.Join("; ", note),
                    ["why"] = string.IsNullOrEmpty(e.c.Why) ? null : e.c.Why, ["delta"] = e.c.Directives,
                });
            }
            string labels = string.Join(" / ", evals.Select(e => Label(e.c)));
            string verdict;
            var top = evals.First(e => e.c.Name == argmax).c;
            if (chosen == top) verdict = chosen == keep.c ? $"the plan in force still scores best over {samples} simulated futures" : $"{samples} simulated futures say {Label(chosen)}";
            else if (chosen == baseC) verdict = $"{Label(top)} scores higher, but not clearly enough over the base plan to leave it";
            else verdict = $"{Label(top)} scores higher, but only {Label(chosen)} clearly beats the base plan";
            bool urgent = triggers != null && triggers.Any(t => t.Id == "mortgage_offer" || t.Id == "georgio" || t.Id == "payment_due");
            string why = triggers == null || triggers.Count == 0 ? "" : " (" + string.Join(", ", triggers.Select(t => t.Id.Replace('_', ' '))) + ")";
            string text = $"Day {day}{why}: considered {labels}; {verdict}.";
            if (text.Length > 200) text = $"Day {day}{why}: considered {evals.Count} plans; {verdict}.";
            // The reason rides in the caption only whole and only while the caption still fits a breath
            // (internal/show cuts captions at 200 characters); the panel shows it whole under the slate.
            string reason = (chosen.Why ?? "").Trim().TrimEnd('.');
            if (reason.Length > 0 && chosen != keep.c && text.Length + reason.Length + 9 <= 200) text += $" Why: {reason}.";
            return new JObject
            {
                ["v"] = 1, ["kind"] = "candidates", ["day"] = day, ["level"] = chosen != keep.c || urgent ? 2 : 1,
                ["text"] = text,
                ["candidates"] = new JObject { ["day"] = day, ["metric"] = $"buyout progress, % ({horizon} days)", ["options"] = options },
            };
        }

        static double StdErr(JArray samples)
        {
            var xs = samples.Select(s => (double?)s["score"] ?? 0).ToList();
            if (xs.Count < 2) return 0;
            double m = xs.Average();
            return Math.Sqrt(xs.Sum(x => (x - m) * (x - m)) / (xs.Count - 1) / xs.Count);
        }

        static string Label(Candidate c)
        {
            switch (c.Source)
            {
                case "keep": return c.Directives.Count == 0 ? "the base plan" : "keep the plan";
                case "base": return "back to the base plan";
            }
            string n = c.Name;
            foreach (var p in new[] { "llm_", "blind_" })
                if (n.StartsWith(p, StringComparison.Ordinal)) n = n.Substring(p.Length);
            return n.Replace('_', ' ');
        }

        // Each directive as "key: value", nested objects flattened one level.
        static IEnumerable<string> Words(JObject d)
        {
            foreach (var p in d.Properties())
            {
                if (p.Value is JObject o)
                    foreach (var q in o.Properties()) yield return $"{p.Name}.{q.Name}: {Flat(q.Value)}";
                else yield return $"{p.Name}: {Flat(p.Value)}";
            }
        }

        static string Flat(JToken t) => t.Type == JTokenType.String ? (string)t : t.ToString(Formatting.None);
    }
}
