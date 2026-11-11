using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The appraisal board (docs/re/APPRAISAL.md). `appraisal: "join"` (gym, off by default) joins on the
    // board's offer, accepts promotions, keeps the certificate in the display case (the only thing that
    // brings appraisal clients) and answers each client's items with an estimate made from what the
    // inspection view shows and the guides' value tables. The true value is never read for the estimate:
    // it is logged as hidden, for evaluation, like the slip a player is handed afterwards.
    internal static class Appraisal
    {
        public static bool On => Guides != null && Directives.Current.Raw.Value<string>("appraisal") == "join";
        // `appraisal_aim_pct` (default 105): the estimate as a percent of the value read off the item. The fee
        // is a percent of the estimate, and a perfect grade spans 95–105 % of the true value.
        static int AimPct => Math.Clamp(Directives.Current.Raw.Value<int?>("appraisal_aim_pct") ?? 105, 95, 105);

        public static readonly string[] Kit = { "magnifier", "hand_guide", "ribwich_guide", "fuel_guide", "cert1", "cert2", "cert3", "cert4" };

        public static int JoinedDay = -1, Visits, Items, Refusals, Walkouts, Mismatches, FineCueItems;
        public static long Fees;
        public static readonly int[] Grades = new int[5];
        public static readonly List<int> Promotions = new List<int>();

        public static void Reset()
        {
            client = null;
            answered.Clear();
            JoinedDay = -1;
            Visits = Items = Refusals = Walkouts = Mismatches = FineCueItems = 0;
            Fees = 0;
            Array.Clear(Grades, 0, Grades.Length);
            Promotions.Clear();
        }

        public static bool IsKit(GameItem i) => On && Kit.Contains(i.identifier);

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        public static bool Owns(string id) => Owned().Any(i => i.identifier == id);

        public static GameItem Certificate() => On && PlayerStore.Instance.isAppraisalIntroduced ? Owned().FirstOrDefault(AppraisalRankHelper.IsCertificate) : null;

        // The board member's last line: join, decline, come back in a week (StoreClientListStory.CreateStoryAppraisalBoard,
        // ...Board2). Join while the fee leaves the day's floor covered; otherwise ask him back.
        // The promotion offer (StoreClientListAppraisal.CreatePromotionOffer): accept, later (7 days), never.
        public static int Dialogue(DialogueView v, int floor)
        {
            if (!On) return -1;
            var ps = PlayerStore.Instance;
            if (v.ClientId == "appraisalBoardIntro" || v.ClientId == "appraisalBoardIntro2")
            {
                int cost = ps.GetAppraisalJoinCost();
                string joinText = LocHelper.GetLocalizedDialogue("dialog_appraisal_board_choice_join", cost);
                string laterText = LocHelper.GetLocalizedDialogue("dialog_appraisal_board_choice_later");
                var join = v.Choices.FirstOrDefault(c => c.Enabled && c.Text == joinText);
                var later = v.Choices.FirstOrDefault(c => c.Enabled && c.Text == laterText);
                if (join != null && v.Cash - cost >= floor) { JoinedDay = ModSide.CurrentDay(); return join.Index; }
                return later?.Index ?? -1;
            }
            if (v.ClientId == AppraisalRankHelper.PROMOTION_CLIENT_ID)
            {
                int cost = AppraisalRankHelper.GetPromotionCost(Math.Min(3, AppraisalRankHelper.GetRank() + 1));
                string acceptText = LocHelper.GetLocalizedDialogue("dialog_chem_feedback_choice_accept", cost);
                string laterText = LocHelper.GetLocalizedDialogue("dialog_landlord_vending_player_decline");
                var accept = v.Choices.FirstOrDefault(c => c.Enabled && c.Text == acceptText);
                var later = v.Choices.FirstOrDefault(c => c.Enabled && c.Text == laterText);
                if (accept != null && v.Cash - cost >= floor) { Promotions.Add(ModSide.CurrentDay()); return accept.Index; }
                return later?.Index ?? -1;
            }
            return -1;
        }

        // What the magnifier's inspection view draws for an appraisal item (AppHandUI, RibwichUI, FuelUI .OnInit),
        // in words. A sprite variant is read as "matches the guide's reference" or not, as an expert with the
        // guide open would (`fine` marks the cues that need that comparison). Parts the view hides are absent.
        public static JObject Perceive(GameItem i)
        {
            string S(string tag) => i.GetTagReadonly(tag).GetString();
            int N(string tag) => i.GetTagReadonly(tag).GetInt();
            if (i.IsTag("HAND_TAG"))
            {
                // The interface marking (full label or symbol, the guide maps both) is absent when unspecified.
                string iface = S("HAND_INTERFACE");
                return new JObject
                {
                    ["type"] = "hand",
                    ["material"] = S("HAND_MATERIAL") switch { "GOLD_PLATED" => "gold", "CARBON_COMPOSITE" => "carbon", _ => "steel" },
                    ["interface"] = iface switch { "NEURAL" => "neural", "EMG_PLUS" => "emg_plus", "EMG" => "emg", _ => "none" },
                    ["condition"] = S("HAND_CONDITION") switch { "WEAR" => "wear", "DAMAGE" => "damage", _ => "intact" },
                    ["armored"] = S("HAND_ADDON_ARMORED") == "ARMORED",
                    ["bladed"] = S("HAND_ADDON_BLADED") == "BLADED",
                };
            }
            if (i.IsTag("RIBWICH_TAG"))
            {
                var o = new JObject { ["type"] = "ribwich" };
                bool dbl = N("RIBWICH_MEAT_TAG") == 1;
                o["meat"] = dbl ? "double" : "normal";
                // Double meat hides the top bun and the pickles.
                if (!dbl)
                {
                    o["bun"] = N("RIBWICH_BUN_TAG") == 1 ? "deluxe" : "normal";
                    o["pickles"] = N("RIBWICH_PICKLES_TAG") == 1 ? "extra" : "normal";
                }
                o["wrap"] = N("RIBWICH_WRAP_TAG") == 0 ? "sealed" : "open";
                int davre = N("RIBWICH_DAVRE_LOGO_TAG"), rib = N("RIBWICH_RIBWICH_LOGO_TAG");
                o["davre_logo"] = davre < 0 ? "absent" : davre == 0 ? "reference" : "off";
                o["ribwich_logo"] = rib < 0 ? "absent" : rib == 0 ? "reference" : "off";
                o["fine"] = davre > 0 || rib > 0;
                if (N("RIBWICH_PRODUCTION_TAG") == 0) { o["month"] = N("RIBWICH_PRODUCTION_MONTH_TAG"); o["year"] = N("RIBWICH_PRODUCTION_YEAR_TAG"); }
                if (N("RIBWICH_FACILITY_TAG") == 0) o["facility"] = N("RIBWICH_FACILITY_NUMBER_TAG");
                return o;
            }
            if (i.IsTag("PLASMA_CELL_TAG"))
            {
                return new JObject
                {
                    ["type"] = "plasma",
                    ["color"] = S("PLASMA_COLOR") switch { "HIGH_QUALITY" => "high", "POOR1" => "poor", "POOR2" => "poor", _ => "standard" },
                    ["seal"] = S("PLASMA_SEAL").ToLowerInvariant(),
                    ["wear"] = S("PLASMA_CASING_CONDITION_WEAR") switch { "WEAR" => "wear", "WEAR_HEAVY" => "heavy", _ => "none" },
                    ["crack"] = S("PLASMA_CASING_CONDITION_CRACK") == "CRACK",
                    ["uncovered"] = S("PLASMA_CASING_CONDITION_UNCOVERED") == "UNCOVERED",
                    ["reinforced"] = S("PLASMA_CASING_CONDITION_REINFORCED") == "REINFORCED",
                };
            }
            return null;
        }

        public static string Guide(JObject cues) => (string)cues?["type"] switch { "hand" => "hand_guide", "ribwich" => "ribwich_guide", "plasma" => "fuel_guide", _ => null };

        // The guides' value tables (`local/appraisal_guides.json`, never committed: docs/re/APPRAISAL.md), set by
        // the host before play. Without them the board is not joined: an estimate needs the table.
        public static JObject Guides;

        // Per item type: `base`, plus each `add` rule whose `when` matches the cues (of rules sharing a `group`,
        // only the first that matches), floored at `min`; any `invalid_when` match is worth `invalid`.
        public static int Value(JObject c)
        {
            if (!(Guides?[(string)c?["type"] ?? ""] is JObject t)) return 0;
            if (t["invalid_when"] is JArray bad && bad.Any(w => Matches(c, (JObject)w))) return t.Value<int>("invalid");
            int v = t.Value<int>("base");
            var groups = new HashSet<string>();
            foreach (JObject r in (t["add"] as JArray) ?? new JArray())
            {
                string g = r.Value<string>("group");
                if ((g != null && groups.Contains(g)) || !Matches(c, (JObject)r["when"])) continue;
                if (g != null) groups.Add(g);
                v += r.Value<int>("add");
            }
            return Math.Max(v, t.Value<int>("min"));
        }

        static bool Matches(JObject cues, JObject when) => when.Properties().All(p => Match(cues[p.Name], p.Value));

        // A matcher is a value (equal), an array (any of), {"range": [lo, hi]} or {"not": matcher}.
        static bool Match(JToken cue, JToken m)
        {
            if (m is JArray any) return any.Any(x => Match(cue, x));
            if (m is JObject o)
            {
                if (o["not"] != null) return !Match(cue, o["not"]);
                if (o["range"] is JArray r) return cue != null && cue.Type == JTokenType.Integer && (int)cue >= (int)r[0] && (int)cue <= (int)r[1];
                return false;
            }
            return cue != null && JToken.DeepEquals(cue, m);
        }

        // The client at the counter, and per item of theirs: estimates refused, the last one, and its feedback
        // (-1 "too low", 1 "too high").
        static object client;
        static readonly Dictionary<int, (int tries, int last, int feedback)> answered = new Dictionary<int, (int, int, int)>();

        public sealed class Answer
        {
            public GameItem Item;
            public JObject Cues;
            public int Value, Estimate, Try;
        }

        // The next estimate to give an appraisal client (the first of their items, by id, that the guides cover
        // and that has not been refused twice), or null. The magnifier opens the inspection view on it.
        public static Answer Next(StoreClient c, IEnumerable<GameItem> front)
        {
            if (!On || c == null || c.clientIntent != StoreClient.ClientIntent.APPRAISAL_SERVICE) return null;
            if (!ReferenceEquals(client, c))
            {
                client = c;
                answered.Clear();
                Visits++;
            }
            if (!Owns("magnifier")) return null;
            foreach (var item in front.Where(i => i != null && !GeneralHelper.IsItemOwned(i) && i.IsTag(AppraisalHelper.APPRAISAL_ITEM)).OrderBy(i => i.uniqueId).ToList())
            {
                var cues = Perceive(item);
                if (cues == null || !Owns(Guide(cues)) || Guides[(string)cues["type"]] == null) continue;
                answered.TryGetValue(item.uniqueId, out var seen);
                // A third refusal is a walk-out (49 impatience each): leave the item rather than risk it.
                if (seen.tries >= 2) continue;
                int value = Value(cues);
                return new Answer { Item = item, Cues = cues, Value = value, Estimate = Estimate(value, seen.tries, seen.last, seen.feedback), Try = seen.tries };
            }
            return null;
        }

        // The client refused the estimate; their reply says which way it was off. Returns the feedback.
        public static int Refused(Answer a, Dialogue reply)
        {
            Refusals++;
            int feedback = Says(reply, "appraisal_too_low_2") ? -1 : 1;
            answered[a.Item.uniqueId] = (a.Try + 1, a.Estimate, feedback);
            return feedback;
        }

        static bool Says(Dialogue d, string key)
        {
            string line = LocHelper.GetLocalizedMechanic(key);
            for (int n = 0; d != null && n < 8; d = d.nextDialogue, n++)
                if (d.plainText == line) return true;
            return false;
        }

        // The board hands its kit (certificate, magnifier, guides) over on the counter, as the tutorial does the
        // labeler, and the counter is wiped every night: it goes into the dossier, which takes documents, the
        // magnifier and the labeler.
        public static List<GameItem> KitOnCounter() =>
            EmporiumEntry.Instance.frontInvinvElement.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) && (IsKit(i) || Labels.IsTool(i))).ToList();

        public static GameInventory Dossier() =>
            GraphUtils.FindAllChildrenType<GameItem>(EmporiumEntry.Instance.docInvElement).FirstOrDefault(i => i.identifier == "dossier")?.contentWindow?.child as GameInventory;

        // Each accepted estimate leaves a slip (paper, worth nothing) on the counter: read, then binned.
        public static List<GameItem> Slips()
        {
            string name = LocHelper.GetLocalizedMechanic("appraisal_slip_name");
            return EmporiumEntry.Instance.frontInvinvElement.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) && i.identifier == "slip" && i.name == name).ToList();
        }

        public static int Estimate(int value, int attempt, int lastEstimate, int feedback)
        {
            if (attempt == 0) return value * AimPct / 100;
            // After "too low"/"too high" (only when the reading was wrong): a quarter up, a fifth down.
            return feedback < 0 ? lastEstimate * 125 / 100 : lastEstimate * 80 / 100;
        }

        public static JObject Status()
        {
            if (JoinedDay < 0 && !(PlayerStore.Instance?.isAppraisalIntroduced ?? false)) return null;
            var ps = PlayerStore.Instance;
            return new JObject
            {
                ["joined_day"] = JoinedDay,
                ["rank"] = AppraisalRankHelper.GetRank(),
                ["progress"] = Math.Round(ps.appraisalProgress, 2),
                ["certificate_shown"] = AppraisalRankHelper.GetDisplayedCertificate() != null,
                ["visits"] = Visits,
                ["items"] = Items,
                ["fees_total"] = Fees,
                ["grades"] = new JArray(Grades),
                ["refusals"] = Refusals,
                ["walkouts"] = Walkouts,
                ["misread"] = Mismatches,
                ["fine_cue_items"] = FineCueItems,
                ["promotions"] = new JArray(Promotions),
            };
        }
    }
}
