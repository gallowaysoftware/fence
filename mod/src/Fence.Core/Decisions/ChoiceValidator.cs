using System.Collections.Generic;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Decisions
{
    // The mod may only execute an option it enumerated itself, with parameters inside the bounds it
    // advertised. Anything else becomes defer.
    public static class ChoiceValidator
    {
        public static Choice Validate(DecideRequest request, DecideResponse response, out string reason)
        {
            reason = null;
            if (response == null) return Reject("no response", out reason);
            if (response.DecisionId != request.DecisionId) return Reject("decision_id mismatch", out reason);
            var choice = response.Choice;
            if (choice == null || string.IsNullOrEmpty(choice.Option)) return Reject("no choice", out reason);
            if (choice.IsDefer)
            {
                reason = "bridge deferred";
                return Choice.MakeDefer();
            }

            DecisionOption option = null;
            foreach (var o in request.Options)
            {
                if (o.Id == choice.Option)
                {
                    option = o;
                    break;
                }
            }
            if (option == null) return Reject("unknown option '" + choice.Option + "'", out reason);

            var specs = option.Params ?? new Dictionary<string, ParamSpec>();
            var given = choice.Params ?? new JObject();
            foreach (var prop in given.Properties())
            {
                if (!specs.ContainsKey(prop.Name)) return Reject("unexpected param '" + prop.Name + "'", out reason);
            }

            var clean = new JObject();
            foreach (var kv in specs)
            {
                if (!given.TryGetValue(kv.Key, out var value)) return Reject("missing param '" + kv.Key + "'", out reason);
                if (!ParamOk(kv.Value, value)) return Reject("param '" + kv.Key + "' out of bounds", out reason);
                clean[kv.Key] = value.DeepClone();
            }

            return new Choice { Option = option.Id, Params = clean.Count > 0 ? clean : null };
        }

        static bool ParamOk(ParamSpec spec, JToken value)
        {
            if (spec == null || spec.Type != ParamSpec.Int) return false;
            if (value.Type != JTokenType.Integer) return false;
            long v = value.Value<long>();
            return (!spec.Min.HasValue || v >= spec.Min.Value) && (!spec.Max.HasValue || v <= spec.Max.Value);
        }

        static Choice Reject(string why, out string reason)
        {
            reason = why;
            return Choice.MakeDefer();
        }
    }
}
