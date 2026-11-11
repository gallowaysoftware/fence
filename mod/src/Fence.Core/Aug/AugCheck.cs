using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Fence.Barter;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Aug
{
    // Everything aug_check reads around the customer at the counter (docs/re/AUGS.md §3, §5.1).
    public sealed class AugSituation
    {
        public int ClientSeq;
        public ClientInfo Client;
        public UiState Ui;
        public BargainFlags Flags;
        public StoreInfo Store;
        // Augs introduced and the all-clear not yet given.
        public bool AugsActive;
        // Shotgun.CanPickup's inputs: the gun is owned, and the customer can be shot now.
        public bool ShootingUnlocked, ShootingMode, BeingArrested, HandlingShootout;
        public bool Arrestable = true, ForceShootable, NonShootable;
        public bool ScannerOwned;
        public int? IntroDay;
        public string Difficulty;
        public int? PresenceRise, KillPresenceDrop;
        public bool? GameOverAtFull;
        public bool AugMenace;
        // The dialogue lines shown so far, oldest first.
        public List<string> Lines = new List<string>();
        // A markup (customer buys) or demanded discount (customer sells) was refused and they stayed.
        public bool MarkupRefused, DiscountRefused;
        // The tablet's odds for the 10% probe on the open side.
        public Odds MarkupOdds, DiscountOdds;
        // HIDDEN labels.
        public int? Presence;
        public bool ShotCountsAsCapture;
    }

    public sealed class AugQuestion
    {
        public int ClientSeq;
        public string Signature;
        public JObject Context;
        public JObject Hidden;
        public List<DecisionOption> Options;
        public int GameDay;
    }

    public static class AugOptionIds
    {
        public const string Shoot = "shoot";
        public const string Scan = "scan";
        public const string ProbeMarkup = "probe_markup_10";
        public const string ProbeDiscount = "probe_discount_10";
        public const string Threaten = "threaten";
        public const string Serve = "serve";
        public const string Dismiss = "dismiss";
    }

    // The aug_check decision (docs/PROTOCOL.md, docs/re/AUGS.md §5.1): an aug (perfect detection) or a
    // suspected aug (tells) is at the counter and the gun can be raised. Advice only: the mod never raises
    // the gun, shoots, scans, probes or threatens on this kind.
    public static class AugCheck
    {
        public const string Kind = "aug_check";
        public const int ProbePercent = 10;

        // Shotgun.CanPickup, or the gun is already raised.
        public static bool GunAvailable(AugSituation s)
        {
            var c = s?.Client;
            if (c == null || !s.ShootingUnlocked) return false;
            if (s.ShootingMode) return true;
            if (!s.Arrestable && !s.ForceShootable) return false;
            if (!c.Dismissable && !s.ForceShootable) return false;
            if (c.IsSecurity || s.NonShootable || s.BeingArrested || s.HandlingShootout) return false;
            return s.Flags == null || !s.Flags.ThreatenOffered;
        }

        // Tells mode asks on suspicion only, from visible state: a zero-false-positive tell, a scan, a haggle
        // refusal, or the shopping pattern augs share (medical, or materials).
        public static bool Suspected(AugSituation s)
        {
            var c = s.Client;
            if (c.Aug != null && c.Aug.Scanned) return c.IsAug;
            if (AugTells.Definitive(c) || s.MarkupRefused || s.DiscountRefused) return true;
            return BarterRules.IsBuying(c.Intent) && c.InterestTags.Any(t => t == "MEDICAL" || t == "MATERIAL");
        }

        public static AugQuestion Build(AugSituation s, string detection)
        {
            var c = s?.Client;
            if (c == null || s.Ui == null || !s.AugsActive || !s.Ui.ClientPresent || !s.Ui.ClientArrived || c.IsSecurity) return null;
            if (!GunAvailable(s)) return null;
            bool perfect = AugDetection.IsPerfect(detection);
            if (!perfect) SeeLines(s);
            if (perfect ? !c.IsAug : !Suspected(s)) return null;

            int day = s.Store?.Day ?? 0;
            var options = Options(s, day);
            return new AugQuestion
            {
                ClientSeq = s.ClientSeq,
                Signature = Signature(s, perfect, options),
                Context = Context(s, perfect, day),
                Hidden = Hidden(s),
                Options = options,
                GameDay = day,
            };
        }

        // The aug script's lines among what this customer said, recognised by their text (AugLines).
        public static void SeeLines(AugSituation s)
        {
            if (s?.Client?.Aug != null) s.Client.Aug.AugOnlyLines = AugLines.AnyShown(s.Lines);
        }

        public static int KillReward(int day, int? introDay, bool menace)
        {
            int reward = Math.Max(100, Math.Min(250, 100 + day - (introDay ?? 12)));
            return menace ? (int)Math.Round(reward * 1.25) : reward;
        }

        // AugHelper.CleanupKill: MANSLAUGHTER by game day.
        public static int Manslaughter(int day) => day >= 50 ? 750 : day >= 25 ? 500 : 250;

        static List<DecisionOption> Options(AugSituation s, int day)
        {
            var c = s.Client;
            var list = new List<DecisionOption>();
            string drop = s.KillPresenceDrop.HasValue ? "presence -" + N(s.KillPresenceDrop.Value) : "presence drops";
            string rise = Rise(s);
            list.Add(Option(AugOptionIds.Shoot,
                "Shoot: if an aug, about +" + N(KillReward(day, s.IntroDay, s.AugMenace)) + " cash, rep +2 LL/+2 BM/+2 Rev/+8 Sec, " + drop
                + "; if human, MANSLAUGHTER crime " + N(Manslaughter(day)) + " and rep -15 LL/-15 Sec/-10 Rev/-10 BM (no game over); the sale is lost either way"));
            if (s.ScannerOwned && (c.Aug == null || !c.Aug.Scanned))
                list.Add(Option(AugOptionIds.Scan, "Scan with the aug scanner (25 energy from its battery): an exact answer, and the gun stays available"));
            var f = s.Flags ?? new BargainFlags();
            if (BarterRules.BargainPanelUsable(s.Ui, c))
            {
                if (BarterRules.IsBuying(c.Intent) && BarterRules.CanBargain(BargainKind.Markup, c, f, s.Ui))
                    list.Add(Option(AugOptionIds.ProbeMarkup, "Add 10% markup as a probe: an aug always refuses and stays; a human " + Roll(s.MarkupOdds)));
                else if (BarterRules.IsSelling(c.Intent) && BarterRules.CanBargain(BargainKind.BuyingDiscount, c, f, s.Ui))
                    list.Add(Option(AugOptionIds.ProbeDiscount, "Demand 10% discount as a probe: an aug always refuses and stays; a human " + Roll(s.DiscountOdds)));
                if (BarterRules.CanBargain(BargainKind.Threaten, c, f, s.Ui))
                    list.Add(Option(AugOptionIds.Threaten, "Threaten: an aug accepts (+20% deal) but can no longer be shot and escapes (" + rise
                        + "); a human denies it: -7 rep, or 50% -10 rep and they leave"));
            }
            list.Add(Option(AugOptionIds.Serve, "Serve them normally: an aug leaves alive (" + rise + ")"));
            if (s.Ui.LockNextBell <= 0 && c.Dismissable) list.Add(Option(AugOptionIds.Dismiss, "Send them away: an aug leaves alive (" + rise + ")"));
            return list;
        }

        static string Rise(AugSituation s)
        {
            string r = s.PresenceRise.HasValue ? "presence +" + N(s.PresenceRise.Value) : "presence rises";
            return s.GameOverAtFull == true || s.AugMenace ? r + "; 100 ends the run" : r;
        }

        static string Roll(Odds o)
        {
            if (o == null) return "may accept, refuse or walk out (-3 rep)";
            int refuse = Math.Max(0, 100 - o.Leave - o.Success);
            return "accepts " + N(o.Success) + "%, refuses " + N(refuse) + "%, walks out " + N(o.Leave) + "% (-3 rep)";
        }

        static JObject Context(AugSituation s, bool perfect, int day)
        {
            var c = s.Client;
            var client = new JObject
            {
                ["name"] = c.Name,
                ["faction"] = Factions.Visible(c),
                ["intent"] = Intent(c.Intent),
                ["wants"] = new JArray((BarterRules.IsBuying(c.Intent) ? c.InterestTags : new List<string>()).Select(TypeTags.Name).Cast<object>().ToArray()),
                ["budget"] = BarterRules.IsBuying(c.Intent) && c.UsesBudget ? new JValue(c.Budget) : JValue.CreateNull(),
            };
            AugDetection.Describe(client, c, perfect ? AugDetection.Perfect : AugDetection.Tells);
            if (!perfect)
            {
                string portrait = AugTells.PortraitWords(c.Aug?.Sprite);
                if (portrait != null) client["portrait"] = portrait;
                string idle = AugTells.IdleWords(c.Aug?.Idle);
                if (idle != null) client["idle"] = idle;
                client["lines"] = new JArray(s.Lines.Select(FeatureText.Plain).Where(l => l.Length > 0).Cast<object>().ToArray());
            }
            string haggle = Haggle(s);
            if (haggle != null) client["haggle"] = haggle;
            if (c.Aug != null && c.Aug.Scanned) client["scan"] = c.IsAug ? "AUG DETECTED" : "No Aug detected";

            var run = new JObject { ["day"] = day };
            if (s.IntroDay.HasValue) run["days_since_intro"] = day - s.IntroDay.Value;
            if (s.Difficulty != null) run["difficulty"] = s.Difficulty;
            if (s.PresenceRise.HasValue) run["presence_rise"] = s.PresenceRise.Value;
            if (s.KillPresenceDrop.HasValue) run["kill_presence_drop"] = s.KillPresenceDrop.Value;
            run["game_over_at_100"] = s.GameOverAtFull == true || s.AugMenace;
            run["aug_menace"] = s.AugMenace;
            run["kill_reward"] = KillReward(day, s.IntroDay, s.AugMenace);
            run["manslaughter_crime"] = Manslaughter(day);

            return new JObject
            {
                ["detection"] = perfect ? AugDetection.Perfect : AugDetection.Tells,
                ["client"] = client,
                ["gun_raised"] = s.ShootingMode,
                ["threaten_used"] = s.Flags != null && s.Flags.ThreatenOffered,
                ["run"] = run,
            };
        }

        static string Haggle(AugSituation s)
        {
            if (s.MarkupRefused) return "refused a markup and stayed";
            if (s.DiscountRefused) return "refused a demanded discount and stayed";
            return null;
        }

        static JObject Hidden(AugSituation s)
        {
            var c = s.Client;
            var o = BarterContext.HiddenClient(c);
            string result = c.IsAug ? "good_kill" : c.IsWanted && s.ShotCountsAsCapture ? "wanted_capture" : "wrongful_kill";
            var hidden = new JObject { ["client"] = o, ["shoot_result_if_shot"] = result };
            if (s.Presence.HasValue) hidden["aug_presence"] = s.Presence.Value;
            return hidden;
        }

        // A new question when what the player can see or do changes: tells, scan, haggle, threaten, gun, options.
        public static string Signature(AugSituation s, bool perfect, IList<DecisionOption> options)
        {
            var c = s.Client;
            string tells = perfect ? (c.IsAug ? "A" : "H") : string.Join("|", AugTells.Words(c).Select(w => w.Substring(0, Math.Min(6, w.Length))));
            string scan = c.Aug != null && c.Aug.Scanned ? (c.IsAug ? "1" : "0") : "-";
            return "a" + N(s.ClientSeq) + ":" + tells + ":s" + scan + ":h" + (s.MarkupRefused ? "m" : "") + (s.DiscountRefused ? "d" : "")
                   + ":g" + (s.ShootingMode ? "1" : "0") + ":o" + string.Join(",", options.Select(o => o.Id));
        }

        public static DecideRequest Request(AugQuestion q, string decisionId, int deadlineMs) => new DecideRequest
        {
            DecisionId = decisionId,
            Kind = Kind,
            GameDay = q.GameDay,
            DeadlineMs = deadlineMs,
            StateSignature = q.Signature,
            Context = q.Context,
            Hidden = q.Hidden,
            Options = q.Options,
        };

        static string Intent(ClientIntent i) => i == ClientIntent.Buy ? "buy" : i == ClientIntent.Sell ? "sell" : i == ClientIntent.SellAndBuy ? "sell_and_buy" : "other";

        static DecisionOption Option(string id, string label) => new DecisionOption { Id = id, Label = label };

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }

    // One customer's aug_check questions: ask once per signature, remember the answer, and tell later
    // shots, kills and escapes which decision they belong to.
    public sealed class AugTracker
    {
        sealed class Asked
        {
            public AugQuestion Question;
            public string DecisionId;
            public Advice Answer;
        }

        readonly Dictionary<string, Asked> bySignature = new Dictionary<string, Asked>();
        Asked current;
        Asked lastSent;

        public object ClientKey { get; private set; }
        public int ClientSeq { get; private set; }
        // Set by the hooks: what ended the visit, for the left-alive record.
        public bool Reported, Holstered, Dismissing;

        public void NewClient(object key, int clientSeq)
        {
            ClientKey = key;
            ClientSeq = clientSeq;
            bySignature.Clear();
            current = lastSent = null;
            Reported = Holstered = Dismissing = false;
        }

        // Makes q the question on screen (null: nothing to ask); true when q has not been asked yet.
        public bool Show(AugQuestion q)
        {
            if (q == null)
            {
                current = null;
                return false;
            }
            if (bySignature.TryGetValue(q.Signature, out var a))
            {
                current = a;
                return false;
            }
            current = new Asked { Question = q };
            return true;
        }

        public void Sent(AugQuestion q, string decisionId)
        {
            if (current == null || current.Question != q) current = new Asked { Question = q };
            current.DecisionId = decisionId;
            bySignature[q.Signature] = current;
            lastSent = current;
        }

        public void Answered(string signature, Advice advice)
        {
            if (bySignature.TryGetValue(signature, out var a)) a.Answer = advice;
        }

        public AugQuestion Current => current?.Question;
        public Advice CurrentAnswer => current?.Answer;
        public bool Thinking => current != null && current.DecisionId != null && current.Answer == null;

        // The newest decision asked about this customer, for outcomes after the question left the screen.
        public string LastDecisionId => lastSent?.DecisionId;

        public string LastAdvised => lastSent?.Answer == null ? null : lastSent.Answer.Deferred ? Choice.Defer : lastSent.Answer.OptionId;

        public string LeftCause(bool threatened)
        {
            if (threatened) return "threatened";
            if (Reported) return "reported";
            if (Holstered) return "holstered";
            return Dismissing ? "dismissed" : "served";
        }
    }

    public static class AugText
    {
        public static string Line(AugQuestion q, Advice a, bool thinking)
        {
            if (q == null) return "";
            if (thinking) return "aug: thinking…";
            if (a == null) return "";
            if (a.Deferred) return "aug: deferred to you";
            string rationale = OverlayText.Clean(a.Rationale, 160);
            string head = "aug: " + (a.OptionId ?? "").Replace('_', ' ');
            return rationale.Length > 0 ? head + " — " + rationale : head;
        }
    }

    public static class AugEvents
    {
        public const string GoodKill = "good_kill";
        public const string WrongfulKill = "wrongful_kill";
        public const string WantedCapture = "wanted_capture";
        public const string LeftAlive = "left_alive";

        // The `aug` record event's data; the same object (plus kind "aug") is the aug_check outcome.
        public static JObject Data(string ev, int clientSeq, string name, JObject extra = null)
        {
            var o = new JObject { ["event"] = ev };
            if (clientSeq > 0) o["client_seq"] = clientSeq;
            if (name != null) o["name"] = name;
            if (extra != null)
                foreach (var p in extra.Properties())
                    o[p.Name] = p.Value;
            return o;
        }

        public static JObject Outcome(JObject data)
        {
            var o = new JObject { ["kind"] = "aug", ["result"] = (string)data["event"] };
            foreach (var p in data.Properties())
                if (p.Name != "event" && p.Name != "client_seq") o[p.Name] = p.Value;
            return o;
        }

        public static string Describe(JObject result)
        {
            switch ((string)result?["result"])
            {
                case GoodKill: return "aug shot: good kill";
                case WrongfulKill: return "shot a human: manslaughter";
                case WantedCapture: return "shot a wanted criminal: capture";
                case LeftAlive: return "an aug left alive (" + ((string)result["cause"] ?? "served") + ")";
            }
            return null;
        }
    }
}
