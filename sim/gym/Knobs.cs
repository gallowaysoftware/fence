using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;

namespace Fence.Gym
{
    // Strategy knobs for controlled experiments: `-knobs "services=both,bargain=ev,..."`. One set per
    // episode; `Current` is per load context, so -repeat and smoke children each see their own.
    // An unset knob keeps the documented default behaviour (sim/gym/README.md), so an empty set
    // reproduces `-human default` with the stock GymHeuristic.
    internal sealed class Knobs
    {
        public static Knobs Current = new Knobs("");

        static readonly HashSet<string> Known = new HashSet<string>(StringComparer.Ordinal)
        {
            // human role
            "services", "svc_nights", "scav", "scav_risk", "window", "window_hot", "window_n", "sell_hot", "sell_contraband",
            "permit", "dlg", "dlglog", "max_stage", "spec",
            // counter policy (GymHeuristic)
            "bargain", "bargain_pct", "buy_margin", "max_heat", "buy_contraband", "ws_margin", "reserve", "floor",
            "nice_faction", "nice_pct", "save_permit", "stock_cap",
        };

        readonly Dictionary<string, string> kv = new Dictionary<string, string>(StringComparer.Ordinal);
        public readonly string Raw;

        public Knobs(string spec)
        {
            Raw = spec ?? "";
            foreach (var part in Raw.Split(new[] { ',', ';' }, StringSplitOptions.RemoveEmptyEntries))
            {
                int eq = part.IndexOf('=');
                if (eq <= 0) throw new ArgumentException("knob without value: " + part);
                string k = part.Substring(0, eq).Trim(), v = part.Substring(eq + 1).Trim();
                if (!Known.Contains(k)) throw new ArgumentException("unknown knob " + k + " (known: " + string.Join(" ", Known.OrderBy(x => x)) + ")");
                kv[k] = v;
            }
        }

        public bool Has(string k) => kv.ContainsKey(k);
        public string S(string k, string def) => kv.TryGetValue(k, out var v) ? v : def;
        public double D(string k, double def) => kv.TryGetValue(k, out var v) ? double.Parse(v, CultureInfo.InvariantCulture) : def;
        public int I(string k, int def) => kv.TryGetValue(k, out var v) ? int.Parse(v, CultureInfo.InvariantCulture) : def;
        public bool B(string k, bool def) => kv.TryGetValue(k, out var v) ? v == "1" || v == "true" || v == "yes" : def;
    }
}
