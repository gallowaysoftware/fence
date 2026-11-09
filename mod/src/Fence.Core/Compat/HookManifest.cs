using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Newtonsoft.Json;

namespace Fence.Compat
{
    public static class MemberKinds
    {
        public const string Event = "event";
        public const string Method = "method";
        public const string Field = "field";
        public const string Property = "property";

        public static readonly string[] All = { Event, Method, Field, Property };
    }

    public sealed class HookSpec
    {
        public const string EventFeaturePrefix = "event.";

        [JsonProperty("id")] public string Id;
        [JsonProperty("type")] public string Type;
        [JsonProperty("member")] public string Member;
        [JsonProperty("kind")] public string Kind;
        [JsonProperty("params", NullValueHandling = NullValueHandling.Ignore)] public List<string> Params;
        [JsonProperty("returns")] public string Returns;
        [JsonProperty("features")] public List<string> Features = new List<string>();
        [JsonProperty("required")] public bool Required;
        [JsonProperty("why", NullValueHandling = NullValueHandling.Ignore)] public string Why;

        // The bridge-facing event kind of a forwarded ModHook event, from its "event.<kind>" feature.
        [JsonIgnore]
        public string EventKind
        {
            get
            {
                var f = Features?.FirstOrDefault(x => x.StartsWith(EventFeaturePrefix, StringComparison.Ordinal));
                return f?.Substring(EventFeaturePrefix.Length);
            }
        }

        // Same shape as hello.compat.missing in docs/PROTOCOL.md: "Type::Member(params)" for methods.
        [JsonIgnore]
        public string Display => Kind == MemberKinds.Method
            ? Type + "::" + Member + "(" + string.Join(", ", Params ?? new List<string>()) + ")"
            : Type + "::" + Member;
    }

    public sealed class HookManifest
    {
        public const string ResourceName = "Fence.hooks.json";

        [JsonProperty("hooks")] public List<HookSpec> Hooks = new List<HookSpec>();

        public HookSpec Find(string id) => Hooks.FirstOrDefault(h => h.Id == id);

        public static HookManifest Parse(string json) => JsonConvert.DeserializeObject<HookManifest>(json);

        public static HookManifest LoadEmbedded()
        {
            using (var stream = typeof(HookManifest).Assembly.GetManifestResourceStream(ResourceName))
            {
                if (stream == null) throw new InvalidOperationException(ResourceName + " is not embedded");
                using (var reader = new StreamReader(stream)) return Parse(reader.ReadToEnd());
            }
        }

        // Structural problems; an empty list means the manifest is usable.
        public List<string> Validate()
        {
            var problems = new List<string>();
            var seen = new HashSet<string>();
            foreach (var h in Hooks)
            {
                string where = h.Id ?? "<no id>";
                if (string.IsNullOrEmpty(h.Id)) problems.Add("hook without id");
                else if (!seen.Add(h.Id)) problems.Add("duplicate id " + h.Id);
                if (string.IsNullOrEmpty(h.Type)) problems.Add(where + ": no type");
                if (string.IsNullOrEmpty(h.Member)) problems.Add(where + ": no member");
                if (!MemberKinds.All.Contains(h.Kind)) problems.Add(where + ": unknown kind '" + h.Kind + "'");
                if (h.Kind == MemberKinds.Method && h.Params == null) problems.Add(where + ": method without params");
                if (h.Kind != MemberKinds.Method && h.Params != null && h.Params.Count > 0) problems.Add(where + ": params on a " + h.Kind);
                if (string.IsNullOrEmpty(h.Returns)) problems.Add(where + ": no returns");
                if (h.Features == null || h.Features.Count == 0) problems.Add(where + ": no features");
            }
            return problems;
        }
    }
}
