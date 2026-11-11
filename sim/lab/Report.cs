using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using Newtonsoft.Json.Linq;

namespace Fence.Lab
{
    // One JSON line per measured claim, appended to var/lab/results.jsonl and echoed to stdout.
    internal static class R
    {
        public static string Out;
        public static string Exp = "?";
        static readonly CultureInfo Inv = CultureInfo.InvariantCulture;

        // Wilson score interval at z (2.576 = 99 %).
        public static (double lo, double hi) Wilson(int k, int n, double z = 2.576)
        {
            if (n == 0) return (0, 1);
            double p = (double)k / n, d = 1 + z * z / n;
            double c = (p + z * z / (2 * n)) / d, h = z * Math.Sqrt(p * (1 - p) / n + z * z / (4.0 * n * n)) / d;
            return (Math.Max(0, c - h), Math.Min(1, c + h));
        }

        // A rate claim: expected p (0..1) vs k of n. Verdict by the 99 % Wilson interval.
        public static void Rate(string claim, double expected, int k, int n, string note = null)
        {
            var (lo, hi) = Wilson(k, n);
            bool ok = expected >= lo - 1e-9 && expected <= hi + 1e-9;
            Emit(claim, n, string.Format(Inv, "{0:0.0}% (99% CI {1:0.0}–{2:0.0})", 100.0 * k / Math.Max(1, n), 100 * lo, 100 * hi),
                string.Format(Inv, "{0:0.0}%", 100 * expected), ok, note);
        }

        // An exact claim: every observation must equal the expectation.
        public static void Exact(string claim, object expected, object measured, int n = 1, string note = null)
        {
            bool ok = Equals(Norm(expected), Norm(measured));
            Emit(claim, n, Str(measured), Str(expected), ok, note);
        }

        public static void Near(string claim, double expected, double measured, double tol, int n = 1, string note = null)
        {
            Emit(claim, n, measured.ToString("0.####", Inv), expected.ToString("0.####", Inv), Math.Abs(expected - measured) <= tol, note);
        }

        public static void Info(string claim, object measured, int n = 1, string note = null) => Emit(claim, n, Str(measured), null, null, note);

        static object Norm(object o) => o switch { int i => (long)i, long l => l, double d when d == Math.Floor(d) => (long)d, IFormattable f => f.ToString(null, Inv), _ => o };

        static string Str(object o) => o switch
        {
            null => "null",
            double d => d.ToString("0.####", Inv),
            float f => f.ToString("0.####", Inv),
            IFormattable f => f.ToString(null, Inv),
            _ => o.ToString(),
        };

        static void Emit(string claim, int n, string measured, string expected, bool? ok, string note)
        {
            var j = new JObject
            {
                ["exp"] = Exp, ["claim"] = claim, ["n"] = n, ["measured"] = measured, ["expected"] = expected,
                ["verdict"] = ok == null ? "INFO" : ok.Value ? "CONFIRMED" : "CONTRADICTED",
            };
            if (note != null) j["note"] = note;
            string line = j.ToString(Newtonsoft.Json.Formatting.None);
            Console.WriteLine(line);
            if (Out != null) lock (Inv) File.AppendAllText(Out, line + "\n");
        }
    }
}
