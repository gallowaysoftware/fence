using System.Collections.Generic;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Protocol
{
    // Wire shapes for docs/PROTOCOL.md. Every fixture in protocol/fixtures must round-trip
    // through these types (Fence.Tests.ProtocolFixtureTests).

    public static class ProtocolVersion
    {
        public const int Current = 1;
    }

    public static class Modes
    {
        public const string Observe = "observe";
        public const string Advise = "advise";
        public const string Act = "act";
    }

    public static class Sources
    {
        public const string Llm = "llm";
        public const string Heuristic = "heuristic";
        public const string Fallback = "fallback";
    }

    public sealed class HealthResponse
    {
        [JsonProperty("ok")] public bool Ok;
        [JsonProperty("protocol")] public int Protocol;
    }

    public sealed class HelloRequest
    {
        [JsonProperty("protocol")] public int Protocol = ProtocolVersion.Current;
        [JsonProperty("mod_version")] public string ModVersion;
        [JsonProperty("game")] public GameInfo Game;
        [JsonProperty("compat")] public HelloCompat Compat;
    }

    public sealed class GameInfo
    {
        [JsonProperty("version")] public string Version;
        [JsonProperty("unity")] public string Unity;
        [JsonProperty("assembly_sha256")] public string AssemblySha256;
        [JsonProperty("assembly_mvid", NullValueHandling = NullValueHandling.Ignore)] public string AssemblyMvid;
    }

    public sealed class HelloCompat
    {
        [JsonProperty("ok")] public bool Ok;
        [JsonProperty("missing")] public List<string> Missing = new List<string>();
        [JsonProperty("disabled_features")] public List<string> DisabledFeatures = new List<string>();
    }

    public sealed class HelloResponse
    {
        [JsonProperty("session_id")] public string SessionId;
        [JsonProperty("protocol")] public int Protocol;
        [JsonProperty("mode")] public string Mode;
        // The appraisal guides' value tables, when the bridge has them locally (never in the mod or the repo).
        [JsonProperty("appraisal_guides", NullValueHandling = NullValueHandling.Ignore)] public JObject AppraisalGuides;
    }

    public sealed class EventRequest
    {
        [JsonProperty("session_id")] public string SessionId;
        [JsonProperty("seq")] public long Seq;
        [JsonProperty("kind")] public string Kind;
        [JsonProperty("game_day")] public int GameDay;
        [JsonProperty("data")] public JObject Data;
        // Values the player cannot see; logged for training, never prompted (docs/PROTOCOL.md, Recording events).
        [JsonProperty("hidden", NullValueHandling = NullValueHandling.Ignore)] public JObject Hidden;
    }

    public sealed class DecideRequest
    {
        [JsonProperty("session_id")] public string SessionId;
        [JsonProperty("decision_id")] public string DecisionId;
        [JsonProperty("kind")] public string Kind;
        [JsonProperty("game_day")] public int GameDay;
        [JsonProperty("deadline_ms")] public int DeadlineMs;
        // Opaque digest of the state sent; the bridge uses it to spot a stale decision (docs/PROTOCOL.md).
        [JsonProperty("state_signature")] public string StateSignature;
        [JsonProperty("context")] public JObject Context;
        // State the player cannot see; the bridge keeps it out of the prompt (docs/PROTOCOL.md, Information boundary).
        [JsonProperty("hidden")] public JObject Hidden;
        [JsonProperty("options")] public List<DecisionOption> Options = new List<DecisionOption>();
    }

    public sealed class DecisionOption
    {
        [JsonProperty("id")] public string Id;
        [JsonProperty("label")] public string Label;
        [JsonProperty("params")] public Dictionary<string, ParamSpec> Params;
    }

    public sealed class ParamSpec
    {
        public const string Int = "int";

        [JsonProperty("type")] public string Type;
        [JsonProperty("min")] public long? Min;
        [JsonProperty("max")] public long? Max;
    }

    public sealed class DecideResponse
    {
        [JsonProperty("decision_id")] public string DecisionId;
        [JsonProperty("choice")] public Choice Choice;
        [JsonProperty("rationale")] public string Rationale;
        [JsonProperty("source")] public string Source;
        // Option id -> a short trade-off note, for the options the model did not pick too.
        [JsonProperty("option_notes")] public Dictionary<string, string> OptionNotes;
    }

    public sealed class Choice
    {
        public const string Defer = "defer";

        [JsonProperty("option")] public string Option;
        [JsonProperty("params")] public JObject Params;

        [JsonIgnore] public bool IsDefer => Option == Defer;

        public static Choice MakeDefer() => new Choice { Option = Defer };
    }

    // GET /v1/summary: the day (meta-turn) view for the overlay.
    public sealed class SummaryResponse
    {
        [JsonProperty("session_id")] public string SessionId;
        [JsonProperty("mode")] public string Mode;
        [JsonProperty("policy")] public string Policy;
        [JsonProperty("model")] public string Model;
        [JsonProperty("day")] public int Day;
        [JsonProperty("today")] public DayStats Today;
        [JsonProperty("lessons")] public string Lessons;
        [JsonProperty("lessons_day")] public int LessonsDay;
        [JsonProperty("plan", NullValueHandling = NullValueHandling.Ignore)] public string Plan;
        [JsonProperty("plan_day", NullValueHandling = NullValueHandling.Ignore)] public int? PlanDay;
        // The plan as executor directives (protocol/schema/directives.schema.json); the autopilot plays them.
        [JsonProperty("directives", NullValueHandling = NullValueHandling.Ignore)] public JObject Directives;
    }

    public sealed class DayStats
    {
        [JsonProperty("decisions")] public int Decisions;
        [JsonProperty("sales")] public int Sales;
        [JsonProperty("purchases")] public int Purchases;
        [JsonProperty("cash_delta")] public long CashDelta;
        [JsonProperty("walkouts")] public int Walkouts;
        [JsonProperty("rep_delta")] public Dictionary<string, double> RepDelta;
    }

    public sealed class OutcomeRequest
    {
        [JsonProperty("session_id")] public string SessionId;
        [JsonProperty("decision_id")] public string DecisionId;
        [JsonProperty("game_day")] public int GameDay;
        [JsonProperty("result")] public JObject Result;
    }
}
