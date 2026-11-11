using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.RegularExpressions;
using Fence.Barter;
using Newtonsoft.Json.Linq;

namespace Fence.Aug
{
    // What the mod read about a customer for aug detection (docs/re/AUGS.md §2). Visible reads describe
    // what the screen shows; Tier, IdleTell and Disguised are labels for `hidden` only.
    public sealed class AugInfo
    {
        // StoreClientMono's idle for this customer: AugIdle.*, or null when not decided for them yet.
        public string Idle;
        // StoreClient.spriteName: the portrait, described in words, never sent raw.
        public string Sprite;
        // The game draws an undisguised aug's lines from an aug-only pool (AUGS.md §2.2); with exact
        // recall of the game's lines an expert recognises them once spoken. Set from the displayed text
        // (AugLines) by whoever holds this customer's lines (AugCheck.SeeLines).
        public bool AugOnlyLines;
        // The faction the generic name claims ("upper_levels" for "Upper Level Citizen", "black_market"
        // for "Thief"); null for any other name.
        public string NameClaim;
        // StoreClient.isAugScanned: the scanner's verdict is on screen.
        public bool Scanned;
        public int Tier;
        public string IdleTell;
        public bool Disguised;
    }

    public static class AugIdle
    {
        public const string Normal = "normal";
        public const string Still = "still";
        public const string Twitch = "twitch";
    }

    // docs/PROTOCOL.md, Perception standard: `perfect` states each customer's aug status as client.aug;
    // `tells` leaves it out and describes the visible tells instead. Set from the mod's aug_detection setting.
    public static class AugDetection
    {
        public const string Perfect = "perfect";
        public const string Tells = "tells";

        public static string Current = Perfect;

        public static string Parse(string s) => string.Equals((s ?? "").Trim(), Tells, StringComparison.OrdinalIgnoreCase) ? Tells : Perfect;

        public static bool IsPerfect(string mode) => mode != Tells;

        // client.aug in perfect mode; client.aug_tells (only when there are any) in tells mode.
        public static void Describe(JObject client, ClientInfo c, string mode = null)
        {
            if (client == null || c == null) return;
            if (IsPerfect(mode ?? Current))
            {
                client["aug"] = c.IsAug;
                return;
            }
            var tells = AugTells.Words(c);
            if (tells.Count > 0) client["aug_tells"] = new JArray(tells.Cast<object>().ToArray());
        }

        // The truth labels added to hidden.client for an aug.
        public static void Hidden(JObject hiddenClient, ClientInfo c)
        {
            if (hiddenClient == null || c == null || !c.IsAug || c.Aug == null) return;
            hiddenClient["aug_tier"] = c.Aug.Tier;
            if (c.Aug.IdleTell != null) hiddenClient["aug_idle_tell"] = c.Aug.IdleTell;
            hiddenClient["disguised"] = c.Aug.Disguised;
        }
    }

    // The bubble DialogUIManager.SetClientBubbleColor draws: an explicit StoreClient.bubbleColor, or by faction.
    public static class Bubbles
    {
        public const string Plain = "plain";
        public const string Upper = "upper";
        public const string BlackMarket = "black_market";
        public const string Revolution = "revolution";
        public const string Security = "security";

        public static string Of(string bubbleColor, string factionId)
        {
            if (string.IsNullOrEmpty(bubbleColor) || bubbleColor == "DEFAULT")
            {
                switch (factionId)
                {
                    case "FACTION_SECURITY": return Security;
                    case "FACTION_BLACK_MARKET": return BlackMarket;
                    case "FACTION_REVOLUTION": return Revolution;
                    case "FACTION_UPPER_LEVEL":
                    case "FACTION_TOURIST": return Upper;
                    default: return Plain;
                }
            }
            switch (bubbleColor)
            {
                case "sec": return Security;
                case "dark": return BlackMarket;
                case "greenrev": return Revolution;
                case "upper": return Upper;
                case "default": return Plain;
                default: return bubbleColor.ToLowerInvariant();
            }
        }

        // The faction a bubble implies to the player; null when it implies none.
        public static string ImpliedFaction(string bubble)
        {
            switch (bubble)
            {
                case Plain: return "lower_levels";
                case Upper: return "upper_levels";
                case BlackMarket: return "black_market";
                case Revolution: return "revolution";
                case Security: return "security";
                default: return null;
            }
        }
    }

    public static class AugTells
    {
        static readonly Regex Digits = new Regex("[0-9]+$");

        // Zero-false-positive tells (AUGS.md §2.1–2.2), in words. The scanner and haggle results are
        // stated separately because they are events, not looks.
        public static List<string> Words(ClientInfo c)
        {
            var list = new List<string>();
            var a = c?.Aug;
            if (a == null) return list;
            if (a.Idle == AugIdle.Still) list.Add("perfectly still: does not bob at all at the counter");
            if (a.Idle == AugIdle.Twitch) list.Add("twitching: a small fast side-to-side shake instead of the slow bob");
            if (IsAugPortrait(a.Sprite)) list.Add("aug portrait: unusual pupils, an uncanny expression or a visible chip");
            if (a.AugOnlyLines) list.Add("speaks in aug-only lines: stiff, robotic or overly formal phrasing no human customer uses");
            if (NameBubbleMismatch(c)) list.Add("named \"" + c.Name + "\" but talks in a plain bubble; real ones talk in the " + BubbleWords(ExpectedBubble(a.NameClaim)) + " bubble");
            return list;
        }

        public static bool Definitive(ClientInfo c) => Words(c).Count > 0;

        public static bool IsAugPortrait(string sprite) => sprite != null && sprite.StartsWith("aug", StringComparison.Ordinal);

        static string ExpectedBubble(string claim) => claim == "upper_levels" ? Bubbles.Upper : claim == "black_market" ? Bubbles.BlackMarket : null;

        static string BubbleWords(string bubble) => bubble == Bubbles.BlackMarket ? "black-market" : bubble == Bubbles.Upper ? "upper-level" : bubble;

        public static bool NameBubbleMismatch(ClientInfo c)
        {
            string expected = ExpectedBubble(c?.Aug?.NameClaim);
            return expected != null && Bubbles.Of(c.BubbleColor, c.FactionId) == Bubbles.Plain;
        }

        public static string IdleWords(string idle)
        {
            switch (idle)
            {
                case AugIdle.Normal: return "bobs normally";
                case AugIdle.Still: return "perfectly still";
                case AugIdle.Twitch: return "small fast side-to-side twitch";
                default: return null;
            }
        }

        static readonly Dictionary<string, string> Families = new Dictionary<string, string>
        {
            ["Lower"] = "lower-level clothes",
            ["Upper"] = "upper-level clothes",
            ["BM"] = "black-market look",
            ["Rev"] = "revolution colours",
            ["Scav"] = "scavenger gear",
            ["Farmer"] = "farmer's clothes",
            ["Chef"] = "chef's whites",
            ["Med"] = "medic's outfit",
            ["Eng"] = "engineer's gear",
            ["Miner"] = "miner's work gear",
            ["Merc"] = "mercenary gear",
            ["Sci"] = "scientist's coat",
            ["Sec"] = "security uniform",
            ["SecCorpo"] = "corporate security uniform",
            ["SecOff"] = "security officer's uniform",
        };

        // SpriteDict pools are "<prefix><n>": maleLower3, femaleScav1, augLLMale2.
        public static string PortraitWords(string sprite)
        {
            if (string.IsNullOrEmpty(sprite)) return null;
            string s = Digits.Replace(sprite, "");
            if (s.StartsWith("aug", StringComparison.Ordinal))
            {
                string f = s.Substring(3).Replace("Male", "").Replace("Female", "");
                string clothes = f == "LL" ? "lower-level" : f == "UL" ? "upper-level" : f == "BM" ? "black-market" : "unfamiliar";
                return clothes + " clothes; unusual pupils, an uncanny expression or a visible chip";
            }
            if (s.StartsWith("male", StringComparison.Ordinal)) s = s.Substring(4);
            else if (s.StartsWith("female", StringComparison.Ordinal)) s = s.Substring(6);
            return Families.TryGetValue(s, out var words) ? words : null;
        }
    }
}
