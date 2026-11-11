using System.Collections.Generic;
using System.Linq;
using Fence.Aug;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The executor's answer to aug_check (docs/re/AUGS.md §5), shared by the gym and the live autopilot. In `tells`
    // detection (the default, and the only one live) it reads what the context shows a player: a zero-false-positive
    // tell (stillness, twitching, an aug portrait, the aug script's lines, a generic name in the wrong bubble) or the
    // scanner's verdict. `perfect` reads the truth flag and exists for gym analysis only.
    public static class AugRule
    {
        // What a verdict rests on, for the feed's hidden_used marker.
        public const string Visible = "visible";
        public const string Hidden = "hidden:is_aug";

        public sealed class Verdict
        {
            public bool Aug;
            public string Basis = Visible;
            public List<string> Tells = new List<string>();
        }

        public static Verdict Judge(JObject context)
        {
            var client = context?["client"] as JObject;
            var v = new Verdict();
            if ((string)context?["detection"] == AugDetection.Perfect)
            {
                v.Basis = Hidden;
                v.Aug = client?.Value<bool?>("aug") ?? false;
                return v;
            }
            if (client?["aug_tells"] is JArray tells) v.Tells.AddRange(tells.Select(t => (string)t));
            if ((string)client?["scan"] == "AUG DETECTED") v.Tells.Add("the aug scanner reads AUG DETECTED");
            v.Aug = v.Tells.Count > 0 && (string)client?["scan"] != "No Aug detected";
            return v;
        }

        public static string Pick(DecideRequest req, out string why)
        {
            var v = Judge(req.Context);
            string mode = Directives.Current.Augs ?? "shoot";
            if (v.Aug && mode == "shoot" && req.Options.Any(o => o.Id == AugOptionIds.Shoot))
            {
                why = v.Basis == Hidden ? "an aug at the counter: shoot" : "an aug: " + string.Join("; ", v.Tells) + ". Shoot";
                return AugOptionIds.Shoot;
            }
            why = v.Aug ? "plan says serve" : v.Basis == Hidden ? "not an aug" : "no sure tell of an aug";
            return req.Options.Any(o => o.Id == AugOptionIds.Serve) ? AugOptionIds.Serve : null;
        }
    }
}
