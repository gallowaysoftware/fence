using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Barter
{
    // The overlay's "Today" as a story a viewer can follow: one sentence per customer with the customer as
    // subject, the shop's own beats (calls, Wild, the outing) between them, and runs of browsers folded.
    public static class DayStory
    {
        const string Left = "left";

        // A customer outcome (docs/PROTOCOL.md, POST /v1/outcome) as a predicate on the customer, or null.
        public static string Clause(JObject result)
        {
            switch ((string)result?["kind"])
            {
                case "trade":
                    int n = (result["items"] as JArray)?.Count ?? 0;
                    long cash = Math.Abs(result.Value<long?>("cash_delta") ?? 0);
                    string items = N(n) + (n == 1 ? " item" : " items");
                    return (string)result["side"] == TradeBatch.Sold ? "bought " + items + " for " + N(cash) : "sold the shop " + items + " for " + N(cash);
                case "bargain":
                    string b = Bargain((string)result["option"] ?? "", result.Value<bool?>("accepted")) + Rep(result["rep_delta"] as JObject);
                    return result.Value<bool?>("walked_out") == true ? b + " and walked out" : b;
                case "left":
                    switch ((string)result["cause"])
                    {
                        case LeftCauses.WalkedOut: return "walked out";
                        case LeftCauses.Dismissed: return "was sent away";
                        case LeftCauses.Shutter: return "was still waiting at closing";
                        default: return Left;
                    }
            }
            return OverlayText.Describe(result);
        }

        static string Bargain(string option, bool? accepted)
        {
            if (option == OptionIds.Blackmail)
                return accepted == true ? "gave in to blackmail" : accepted == false ? "would not be blackmailed" : "was blackmailed";
            if (option == OptionIds.Threaten)
                return accepted == true ? "backed down under a threat" : accepted == false ? "stood up to a threat" : "was threatened";
            int cut = option.LastIndexOf('_');
            string pct = cut > 0 ? option.Substring(cut + 1) + "%" : "";
            switch (cut > 0 ? option.Substring(0, cut) : option)
            {
                case "add_markup": return accepted == true ? "paid a " + pct + " markup" : accepted == false ? "refused a " + pct + " markup" : "was asked " + pct + " more";
                case "offer_discount": return "got a " + pct + " discount";
                case "offer_markup": return "was offered " + pct + " more";
                case "demand_discount": return accepted == true ? "came down " + pct : accepted == false ? "would not come down " + pct : "was asked to come down " + pct;
                default: return option.Replace('_', ' ');
            }
        }

        // An autopilot action that belongs to the customer at the counter, as a predicate; null otherwise.
        public static string CustomerBeat(string action, string why)
        {
            switch (action)
            {
                case "dialogue_choice": return string.IsNullOrEmpty(why) ? null : "was told \"" + why.Trim().TrimStart('[').TrimEnd(']').Trim() + "\"";
                case "report_wanted": return "was reported for the bounty";
                case "donate": return "took goods as a donation";
                case "gun_order":
                    if (string.IsNullOrEmpty(why)) return null;
                    int given = why.IndexOf(" (given", StringComparison.Ordinal);
                    return "got their order: " + (given > 0 ? why.Substring(0, given) : why);
            }
            return null;
        }

        // An autopilot action as a sentence about the shop, or null when a viewer would not miss it.
        public static string ShopBeat(string action, string target, string why)
        {
            why = why ?? "";
            switch (action)
            {
                case "open_shutter": return "Opened the shop.";
                case "close_shutter": return "Shutter down.";
                case "chore":
                    return why.StartsWith("pressed ", StringComparison.Ordinal) && !string.IsNullOrEmpty(target)
                        ? "Pressed " + why.Substring("pressed ".Length) + " on the " + target + "." : null;
                case "dial":
                    int colon = why.IndexOf(": ", StringComparison.Ordinal);
                    return colon > 0 ? "Called " + why.Substring(0, colon) + " to " + why.Substring(colon + 2) + "." : "Made a call.";
                case "wild_buy":
                    int at = why.IndexOf(" for ", StringComparison.Ordinal);
                    if (at <= 0) return "Bought on Wild's Network.";
                    int paren = why.IndexOf(" (", at, StringComparison.Ordinal);
                    return "Bought " + Words(why.Substring(0, at)) + " on Wild's Network for " + (paren > 0 ? why.Substring(at + 5, paren - at - 5) : why.Substring(at + 5)) + ".";
                case "jackson_buy": return "Bought from Dr. Jackson.";
                case "open_door": return Place(why, "going out to ", "Went out to ");
                case "return": return Place(why, "home from ", "Home from ");
                case "carry_left":
                case "leave_on_ground":
                case "shot_result":
                    return Sentence(why);
                case "services":
                    if (!why.StartsWith("tonight: ", StringComparison.Ordinal) || why.StartsWith("tonight: none", StringComparison.Ordinal)) return null;
                    string list = why.Substring("tonight: ".Length);
                    int semi = list.IndexOf(';');
                    return "Services tonight: " + Words(semi > 0 ? list.Substring(0, semi) : list) + ".";
            }
            return null;
        }

        static string Place(string why, string prefix, string lead) =>
            why.StartsWith(prefix, StringComparison.Ordinal) ? lead + PlaceName(why.Substring(prefix.Length)) + "." : Sentence(why);

        static string PlaceName(string id)
        {
            switch (id)
            {
                case "dumping_ground": return "the dumping ground";
                case "dr_jackson": return "Dr. Jackson's";
                default: return id.Replace('_', ' ');
            }
        }

        // LANDLORD_GRACE, JANITORIAL_SERVICE → landlord grace, janitorial service.
        static string Words(string ids) => ids.Replace('_', ' ').ToLowerInvariant().Replace(",", ", ").Replace(",  ", ", ");

        static string Sentence(string s)
        {
            s = (s ?? "").Trim();
            if (s.Length == 0) return null;
            s = char.ToUpperInvariant(s[0]) + s.Substring(1);
            return s.EndsWith(".", StringComparison.Ordinal) ? s : s + ".";
        }

        public static string Tell(IList<DayEntry> entries, bool customerAtCounter)
        {
            if (entries == null || entries.Count == 0) return "No customers yet today";
            var lines = new List<string>();
            int browsers = 0;
            void FoldBrowsers()
            {
                if (browsers == 1) lines.Add("One customer browsed and left.");
                else if (browsers > 1) lines.Add(N(browsers) + " customers browsed and left.");
                browsers = 0;
            }
            var atCounter = customerAtCounter ? DayLog.LastCustomer(entries) : null;
            for (int i = 0; i < entries.Count; i++)
            {
                var e = entries[i];
                if (e.Shop)
                {
                    FoldBrowsers();
                    lines.Add(Repeated(OverlayText.Clean(e.Name, 160), e.Repeat));
                    continue;
                }
                bool current = ReferenceEquals(e, atCounter);
                var said = Predicates(e.Clauses);
                var asides = e.Asides.Where(a => !string.IsNullOrEmpty(a)).Select(a => OverlayText.Clean(a, 140)).ToList();
                if (said.Count == 0 && asides.Count == 0)
                {
                    if (current) { FoldBrowsers(); lines.Add(Name(e) + " is at the counter."); }
                    else browsers++;
                    continue;
                }
                FoldBrowsers();
                string aside = string.Join("; ", asides);
                lines.Add(said.Count == 0 ? Name(e) + ": " + aside + "." : Name(e) + " " + Join(said) + (aside.Length > 0 ? "; " + aside : "") + ".");
            }
            FoldBrowsers();
            return string.Join("\n", lines);
        }

        static string Repeated(string sentence, int times)
        {
            if (times <= 1) return sentence;
            string mark = " (×" + N(times) + ")";
            return sentence.EndsWith(".", StringComparison.Ordinal) ? sentence.Substring(0, sentence.Length - 1) + mark + "." : sentence + mark;
        }

        // Drops what goes without saying: a plain "left" next to anything else, a second "walked out".
        static List<string> Predicates(IList<string> clauses)
        {
            var said = new List<string>();
            bool walked = false;
            foreach (var c in clauses)
            {
                if (string.IsNullOrEmpty(c) || c == Left) continue;
                if (c == "walked out" && walked) continue;
                walked |= c.EndsWith("walked out", StringComparison.Ordinal);
                said.Add(OverlayText.Clean(c, 140));
            }
            return said;
        }

        static string Name(DayEntry e)
        {
            string n = OverlayText.Clean(e.Name, 30);
            return n.Length == 0 ? "A customer" : char.ToUpperInvariant(n[0]) + n.Substring(1);
        }

        static string Join(IList<string> parts) =>
            parts.Count == 1 ? parts[0] : string.Join(", ", parts.Take(parts.Count - 1)) + " and " + parts[parts.Count - 1];

        static string Rep(JObject delta)
        {
            if (delta == null || !delta.HasValues) return "";
            return " (" + string.Join(", ", delta.Properties().Select(p => p.Name.Replace('_', ' ') + " " + ((double)p.Value).ToString("+0.##;-0.##;0", CultureInfo.InvariantCulture))) + ")";
        }

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
