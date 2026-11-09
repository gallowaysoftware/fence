using System.Collections.Generic;
using System.Text;
using System.Text.RegularExpressions;

namespace Fence.Barter
{
    public static class Factions
    {
        static readonly Dictionary<string, string> Known = new Dictionary<string, string>
        {
            ["FACTION_LOWER_LEVEL"] = "lower_levels",
            ["FACTION_UPPER_LEVEL"] = "upper_levels",
            ["FACTION_SECURITY"] = "security",
            ["FACTION_BLACK_MARKET"] = "black_market",
            ["FACTION_REVOLUTION"] = "revolution",
            ["FACTION_CARTEL"] = "cartel",
            ["FACTION_TOURIST"] = "tourist",
            ["FACTION_CHURCH"] = "church",
            ["FACTION_MIDDLE"] = "middle",
        };

        // The factions with a reputation bar (docs/PROTOCOL.md, Barter context).
        public static readonly HashSet<string> WithRepBar = new HashSet<string>
        {
            "lower_levels", "upper_levels", "security", "black_market", "revolution", "cartel",
        };

        public static string Name(string factionId)
        {
            if (string.IsNullOrEmpty(factionId)) return null;
            if (Known.TryGetValue(factionId, out var name)) return name;
            string s = factionId.StartsWith("FACTION_") ? factionId.Substring("FACTION_".Length) : factionId;
            return s.ToLowerInvariant();
        }
    }

    public static class TypeTags
    {
        // Item-type constants as the protocol spells them: the game's id, lower-cased ("LOWTECH" -> "lowtech").
        public static string Name(string typeConst) => string.IsNullOrEmpty(typeConst) ? null : typeConst.ToLowerInvariant();
    }

    public static class FeatureText
    {
        static readonly Regex Struck = new Regex("<s>(.*?)</s>", RegexOptions.Singleline);
        static readonly Regex Tag = new Regex("<[^>]*>");
        static readonly Regex Spaces = new Regex("\\s+");

        // A tablet feature line as plain text: rich-text tags dropped, the game's leading "-" bullet
        // removed, and an exposed fake ("<s>claimed</s>\n real") spelled out instead of struck through.
        public static string Plain(string raw)
        {
            if (string.IsNullOrEmpty(raw)) return "";
            var m = Struck.Match(raw);
            if (m.Success)
            {
                string claimed = Line(m.Groups[1].Value);
                string actual = Line(raw.Substring(m.Index + m.Length));
                if (claimed.Length == 0) return actual;
                if (actual.Length == 0) return claimed;
                return actual + " (exposed; claimed " + claimed + ")";
            }
            return Line(raw);
        }

        static string Line(string s)
        {
            s = Tag.Replace(s, " ");
            var parts = new List<string>();
            foreach (var piece in s.Split('\n'))
            {
                string p = Spaces.Replace(piece, " ").Trim();
                while (p.StartsWith("-")) p = p.Substring(1).TrimStart();
                if (p.Length > 0) parts.Add(p);
            }
            return string.Join(" ", parts);
        }

        public static string Truncate(string s, int max)
        {
            if (s == null) return "";
            s = Spaces.Replace(s, " ").Trim();
            if (s.Length <= max) return s;
            var sb = new StringBuilder(s.Substring(0, max - 1).TrimEnd());
            sb.Append('…');
            return sb.ToString();
        }
    }
}
