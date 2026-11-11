using System;
using System.Diagnostics;
using System.Net.Http;
using System.Text;
using Fence.Protocol;

namespace Fence.Gym
{
    // The gym's side of docs/PROTOCOL.md. HttpBridge talks to a real bridge exactly as the mod does;
    // LocalBridge answers in-process with the gym heuristic, for throughput.
    internal interface IBridge : IDisposable
    {
        string Name { get; }
        HelloResponse Hello(HelloRequest hello);
        void Event(EventRequest e);
        // Null when the bridge failed; the caller treats that like the mod does (defer).
        DecideResponse Decide(DecideRequest request, out long latencyMs);
        void Outcome(OutcomeRequest outcome);
        SummaryResponse Summary(string sessionId);
    }

    internal sealed class HttpBridge : IBridge
    {
        readonly HttpClient http;
        readonly string baseUrl;
        public int Failures { get; private set; }

        public HttpBridge(string baseUrl)
        {
            this.baseUrl = baseUrl.TrimEnd('/');
            http = new HttpClient { Timeout = TimeSpan.FromSeconds(120) };
        }

        public string Name => "http " + baseUrl;

        string Post(string path, object body, out int status)
        {
            using var content = new StringContent(ProtocolJson.Serialize(body), Encoding.UTF8, "application/json");
            using var resp = http.PostAsync(baseUrl + path, content).GetAwaiter().GetResult();
            status = (int)resp.StatusCode;
            return resp.Content.ReadAsStringAsync().GetAwaiter().GetResult();
        }

        public HelloResponse Hello(HelloRequest hello)
        {
            string body = Post("/v1/hello", hello, out int status);
            if (status != 200) throw new InvalidOperationException("bridge refused hello: " + status + " " + body);
            return ProtocolJson.Deserialize<HelloResponse>(body);
        }

        public void Event(EventRequest e)
        {
            try
            {
                Post("/v1/event", e, out int status);
                if (status >= 300) Failures++;
            }
            catch (HttpRequestException)
            {
                Failures++;
            }
        }

        public DecideResponse Decide(DecideRequest request, out long latencyMs)
        {
            var sw = Stopwatch.StartNew();
            try
            {
                string body = Post("/v1/decide", request, out int status);
                latencyMs = sw.ElapsedMilliseconds;
                if (status != 200)
                {
                    Failures++;
                    return null;
                }
                return ProtocolJson.Deserialize<DecideResponse>(body);
            }
            catch (Exception)
            {
                latencyMs = sw.ElapsedMilliseconds;
                Failures++;
                return null;
            }
        }

        public void Outcome(OutcomeRequest outcome)
        {
            try
            {
                Post("/v1/outcome", outcome, out int status);
                if (status >= 300) Failures++;
            }
            catch (HttpRequestException)
            {
                Failures++;
            }
        }

        public SummaryResponse Summary(string sessionId)
        {
            try
            {
                using var resp = http.GetAsync(baseUrl + "/v1/summary?session_id=" + Uri.EscapeDataString(sessionId)).GetAwaiter().GetResult();
                if (!resp.IsSuccessStatusCode) return null;
                return ProtocolJson.Deserialize<SummaryResponse>(resp.Content.ReadAsStringAsync().GetAwaiter().GetResult());
            }
            catch (Exception)
            {
                return null;
            }
        }

        // The plan's directives from /v1/summary (docs/PROTOCOL.md "Directives"); null when absent.
        public Directives SummaryDirectives(string sessionId)
        {
            try
            {
                using var resp = http.GetAsync(baseUrl + "/v1/summary?session_id=" + Uri.EscapeDataString(sessionId)).GetAwaiter().GetResult();
                if (!resp.IsSuccessStatusCode) return null;
                var o = Newtonsoft.Json.Linq.JObject.Parse(resp.Content.ReadAsStringAsync().GetAwaiter().GetResult());
                return o["directives"] is Newtonsoft.Json.Linq.JObject d ? Directives.Parse(d) : null;
            }
            catch (Exception e) when (!(e is ArgumentException))
            {
                return null;
            }
        }

        public void Dispose() => http.Dispose();
    }

    // `-bridge defer:`: a bridge in observe mode with no planner, in-process: hello and events go nowhere and
    // every decide is deferred (the mod's answer when the bridge only watches). Tests the fallbacks.
    internal sealed class DeferBridge : IBridge
    {
        readonly string session;

        public DeferBridge(int seed) => session = "defer-s" + seed;

        public string Name => "defer";

        public HelloResponse Hello(HelloRequest hello) => new HelloResponse { SessionId = session, Protocol = ProtocolVersion.Current, Mode = Modes.Observe };

        public void Event(EventRequest e) { }

        public DecideResponse Decide(DecideRequest request, out long latencyMs)
        {
            latencyMs = 0;
            return new DecideResponse { DecisionId = request.DecisionId, Choice = Choice.MakeDefer(), Source = Sources.Heuristic, Rationale = "observe" };
        }

        public void Outcome(OutcomeRequest outcome) { }

        public SummaryResponse Summary(string sessionId) => null;

        public void Dispose() { }
    }

    internal sealed class LocalBridge : IBridge
    {
        readonly string session;

        public LocalBridge(int seed) => session = "local-s" + seed;

        public string Name => "local heuristic";

        public HelloResponse Hello(HelloRequest hello) => new HelloResponse { SessionId = session, Protocol = ProtocolVersion.Current, Mode = Modes.Act };

        public void Event(EventRequest e) { }

        public DecideResponse Decide(DecideRequest request, out long latencyMs)
        {
            latencyMs = 0;
            return GymHeuristic.Decide(request);
        }

        public void Outcome(OutcomeRequest outcome) { }

        public SummaryResponse Summary(string sessionId) => null;

        public void Dispose() { }
    }
}

namespace Fence.Gym
{
    // Writes every request the gym sends (and each decide response) as one JSON line: the wire
    // view, for schema tests and debugging.
    internal sealed class WireLog : IBridge
    {
        readonly IBridge inner;
        readonly System.IO.StreamWriter w;

        public WireLog(IBridge inner, string path)
        {
            this.inner = inner;
            System.IO.Directory.CreateDirectory(System.IO.Path.GetDirectoryName(System.IO.Path.GetFullPath(path)));
            w = new System.IO.StreamWriter(path) { NewLine = "\n" };
        }

        void Line(string type, object body) => w.WriteLine("{\"type\":\"" + type + "\",\"body\":" + ProtocolJson.Serialize(body) + "}");

        public string Name => inner.Name;

        public HelloResponse Hello(HelloRequest hello)
        {
            Line("hello", hello);
            return inner.Hello(hello);
        }

        public void Event(EventRequest e)
        {
            Line("event", e);
            inner.Event(e);
        }

        public DecideResponse Decide(DecideRequest request, out long latencyMs)
        {
            Line("decide", request);
            var r = inner.Decide(request, out latencyMs);
            if (r != null) Line("decide_response", r);
            return r;
        }

        public void Outcome(OutcomeRequest outcome)
        {
            Line("outcome", outcome);
            inner.Outcome(outcome);
        }

        public SummaryResponse Summary(string sessionId) => inner.Summary(sessionId);

        public void Dispose()
        {
            w.Dispose();
            inner.Dispose();
        }
    }
}
