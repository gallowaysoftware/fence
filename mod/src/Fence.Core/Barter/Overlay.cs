using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text;
using Fence.Bridge;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Barter
{
    public enum BridgeStatus
    {
        Connecting,
        Connected,
        Down,
        Vanilla,
        Off,
    }

    // How acting stands, for display. ConfigOff: the bridge says act, the act_enabled setting says no,
    // so the mod advises exactly like P1.
    public enum ActDisplay
    {
        Advise,
        ConfigOff,
        Active,
        Paused,
        Faulted,
    }

    public sealed class DayEntry
    {
        public string Name;
        public List<string> Outcomes = new List<string>();
    }

    // Today's customers (the meta-turn), one entry per arrival; cleared when the game day changes.
    public sealed class DayLog
    {
        public const int MaxEntries = 40;
        readonly List<DayEntry> entries = new List<DayEntry>();
        int? day;

        public IList<DayEntry> Entries => entries;
        public DayEntry Current => entries.Count > 0 ? entries[entries.Count - 1] : null;

        public void SetDay(int? gameDay)
        {
            if (!gameDay.HasValue || gameDay == day) return;
            if (day.HasValue) entries.Clear();
            day = gameDay;
        }

        public void Arrive(string name)
        {
            entries.Add(new DayEntry { Name = string.IsNullOrEmpty(name) ? "customer" : name });
            if (entries.Count > MaxEntries) entries.RemoveAt(0);
        }

        public void Note(string outcome)
        {
            if (string.IsNullOrEmpty(outcome)) return;
            if (Current == null) Arrive(null);
            Current.Outcomes.Add(outcome);
        }
    }

    // GET /v1/summary at most every MinIntervalMs, and only once something asked for it.
    public sealed class SummaryPoller
    {
        public const long MinIntervalMs = 5000;
        bool wanted = true;
        bool inFlight;
        bool everAsked;
        long lastMs;

        public bool LastFailed { get; private set; }

        public void Want() => wanted = true;

        public bool TryBegin(long nowMs)
        {
            if (!wanted || inFlight || (everAsked && nowMs - lastMs < MinIntervalMs)) return false;
            wanted = false;
            inFlight = true;
            everAsked = true;
            lastMs = nowMs;
            return true;
        }

        public void Done(bool ok)
        {
            inFlight = false;
            LastFailed = !ok;
        }
    }

    public sealed class OverlayInput
    {
        public string Mode;
        public ActDisplay Act;
        public string ActFault;
        public BridgeStatus Bridge;
        public ClientInfo Client;
        public string CoverageGap;
        public bool Ready;
        public AdviceView View;
        public IList<DayEntry> Today;
        public SummaryResponse Summary;
        public bool SummaryFailed;
        public RecordingView Recording;
        // Morning, shutter not yet opened today: the plan's WINDOW part matters now.
        public bool BeforeOpen;
        // The inspect advice line for the item under the magnifier (Inspect.InspectText), or empty.
        public string Inspect;
        // The stage shortlist (Fence.Core's own ranking) while a stage moment holds; null otherwise.
        public IList<StageCandidate> Shortlist;
        public long? ShortlistBudgetLeft;
        // The autopilot's status line (empty when its setting is off), whether it is a stop that needs the
        // human, and the pause button's label while it is on (it then pauses acting too).
        public string Autopilot;
        public bool AutopilotAlert;
        public string AutopilotPause;
    }

    public sealed class OverlayView
    {
        public string Header;
        public string Customer;
        public string Status;
        public string Move;
        public string Inspect;
        public string Rationale;
        public string Options;
        public string Outcome;
        public string Today;
        public string Summary;
        public string Lessons;
        public string Recording;
        public bool NotRecording;
        // Empty hides the plan section.
        public string PlanHeader;
        public string PlanWindow;
        public string Plan;
        // Null hides the button (not in act mode).
        public string PauseLabel;
        public bool PauseInteractable;
        // The collapsed one-line bar.
        public string Bar;
        public string Autopilot;
        public bool AutopilotAlert;
    }

    // The strategist's plan is free text with headings (GOALS, CASH, WINDOW, STOCK, ...).
    public static class PlanText
    {
        // The body under `heading`, or "" when the plan has none. A heading is a line whose text, minus
        // markdown marks and a trailing colon, is upper case; text after the colon belongs to the body.
        public static string Section(string plan, string heading)
        {
            if (string.IsNullOrEmpty(plan)) return "";
            var lines = plan.Replace("\r", "").Split('\n');
            var body = new List<string>();
            bool inside = false;
            foreach (var raw in lines)
            {
                if (Heading(raw, out var name, out var rest))
                {
                    if (inside) break;
                    if (name == heading)
                    {
                        inside = true;
                        if (rest.Length > 0) body.Add(rest);
                    }
                    continue;
                }
                if (inside) body.Add(raw.TrimEnd());
            }
            return string.Join("\n", body).Trim();
        }

        static bool Heading(string line, out string name, out string rest)
        {
            name = rest = "";
            string t = line.Trim().Trim('#', '*', ' ');
            int colon = t.IndexOf(':');
            string head = (colon >= 0 ? t.Substring(0, colon) : t).Trim().TrimEnd('*').Trim();
            if (head.Length < 3 || head.Length > 24 || colon < 0 && t != head) return false;
            foreach (char ch in head)
                if (!(char.IsUpper(ch) || ch == ' ' || ch == '_' || ch == '&' || ch == '/')) return false;
            name = head;
            rest = colon >= 0 ? t.Substring(colon + 1).Trim().TrimStart('*').Trim() : "";
            return true;
        }
    }

    // Everything the overlay shows, as text, from the advisor's state. The Unity side only copies
    // these strings into widgets.
    public static class OverlayText
    {
        public const int MaxOptions = 12;

        public static OverlayView Build(OverlayInput s)
        {
            var v = new OverlayView
            {
                Header = Header(s),
                Customer = Customer(s.Client),
                Today = Today(s.Today),
                Summary = Summary(s.Summary, s.SummaryFailed),
                Lessons = Lessons(s.Summary),
                PauseLabel = s.Act == ActDisplay.Active ? "Pause acting" : s.Act == ActDisplay.Paused ? "Resume acting" : s.Act == ActDisplay.Faulted ? "Acting off (error)" : null,
                PauseInteractable = s.Act == ActDisplay.Active || s.Act == ActDisplay.Paused,
                Recording = s.Recording?.Text ?? "",
                NotRecording = s.Recording != null && !s.Recording.Recording,
            };
            if (!string.IsNullOrEmpty(s.AutopilotPause))
            {
                v.PauseLabel = s.AutopilotPause;
                v.PauseInteractable = true;
            }
            v.Autopilot = s.Autopilot ?? "";
            v.AutopilotAlert = s.AutopilotAlert && v.Autopilot.Length > 0;
            Plan(s, v);
            var advice = s.View?.Advice;
            v.Status = Status(s);
            v.Move = advice != null && s.Client != null && s.CoverageGap == null ? Move(advice, s.View.Status) : "";
            v.Rationale = advice != null && v.Move.Length > 0 ? Clean(advice.Rationale, 300) : "";
            v.Options = advice != null && v.Move.Length > 0 ? Options(advice) : "";
            if (s.Shortlist != null && s.Shortlist.Count > 0 && s.Client != null && s.CoverageGap == null && (advice == null || advice.Kind == StageContext.Kind))
            {
                if (!IsStageOffer(advice)) v.Move = "offer: " + StageShortlist.Line(s.Shortlist[0], s.ShortlistBudgetLeft);
                v.Options = StageShortlist.Text(s.Shortlist, s.ShortlistBudgetLeft, advice);
            }
            v.Inspect = s.Client != null ? Clean(s.Inspect, 260) : "";
            var current = s.Client != null && s.Today != null && s.Today.Count > 0 ? s.Today[s.Today.Count - 1] : null;
            v.Outcome = current != null && current.Outcomes.Count > 0 ? "Result: " + string.Join("; ", current.Outcomes) : "";
            v.Bar = (v.NotRecording ? v.Recording + "\n" : "") + (v.Autopilot.Length > 0 ? v.Autopilot + "\n" : "") + "fence · " + ModeText(s) + " · " + v.Status + (v.Move.Length > 0 ? " · " + v.Move : "")
                    + (v.Inspect.Length > 0 ? "\n" + v.Inspect : "");
            return v;
        }

        static void Plan(OverlayInput s, OverlayView v)
        {
            string plan = s.Summary?.Plan;
            if (string.IsNullOrWhiteSpace(plan))
            {
                v.PlanHeader = v.PlanWindow = v.Plan = "";
                return;
            }
            v.PlanHeader = "Today's plan" + (s.Summary.PlanDay.HasValue ? " (day " + N(s.Summary.PlanDay.Value) + ")" : "");
            string window = PlanText.Section(plan, "WINDOW");
            v.PlanWindow = s.BeforeOpen && window.Length > 0 ? "WINDOW (before you open):\n" + Clean(window, 500, keepNewlines: true) : "";
            v.Plan = Clean(plan, 2100, keepNewlines: true);
        }

        public static BridgeStatus BridgeOf(BridgeSession session, long nowMs)
        {
            if (session == null || !session.Enabled) return BridgeStatus.Off;
            switch (session.State)
            {
                case SessionState.Vanilla: return BridgeStatus.Vanilla;
                case SessionState.Active: return session.Breaker.IsOpen(nowMs) ? BridgeStatus.Down : BridgeStatus.Connected;
                default: return session.Breaker.IsOpen(nowMs) || session.Breaker.State != Decisions.BreakerState.Closed ? BridgeStatus.Down : BridgeStatus.Connecting;
            }
        }

        public static string ModeText(OverlayInput s)
        {
            switch (s.Act)
            {
                case ActDisplay.Active: return "act";
                case ActDisplay.Paused: return "act paused";
                case ActDisplay.Faulted: return "act off (error)";
                case ActDisplay.ConfigOff: return "advise (act off in settings)";
                default: return string.IsNullOrEmpty(s.Mode) ? "observe" : s.Mode;
            }
        }

        static string Header(OverlayInput s)
        {
            string bridge;
            switch (s.Bridge)
            {
                case BridgeStatus.Connected: bridge = "connected"; break;
                case BridgeStatus.Down: bridge = "bridge down"; break;
                case BridgeStatus.Vanilla: bridge = "vanilla"; break;
                case BridgeStatus.Off: bridge = "off in settings"; break;
                default: bridge = "connecting"; break;
            }
            string model = s.Summary != null && !string.IsNullOrEmpty(s.Summary.Model) ? " · " + Clean(s.Summary.Model, 40) : "";
            string fault = s.Act == ActDisplay.Faulted && !string.IsNullOrEmpty(s.ActFault) ? "\n" + Clean(s.ActFault, 120) : "";
            return ModeText(s) + " · " + bridge + model + fault;
        }

        static string Customer(ClientInfo c)
        {
            if (c == null) return "No customer at the counter";
            var parts = new List<string> { Clean(c.Name, 40) };
            string faction = Factions.Visible(c);
            if (faction != null) parts.Add(faction);
            parts.Add(Intent(c.Intent, c.RawIntent));
            if (BarterRules.IsBuying(c.Intent) && c.UsesBudget) parts.Add("budget " + N(c.Budget));
            return string.Join(" · ", parts);
        }

        static string Intent(ClientIntent i, string raw)
        {
            switch (i)
            {
                case ClientIntent.Buy: return "buying";
                case ClientIntent.Sell: return "selling";
                case ClientIntent.SellAndBuy: return "selling and buying";
                case ClientIntent.Dialogue: return "talking";
                default: return (raw ?? "other").ToLowerInvariant();
            }
        }

        static string Status(OverlayInput s)
        {
            if (s.Client == null) return "waiting for a customer";
            if (s.CoverageGap != null) return "not covered: " + s.CoverageGap;
            var view = s.View;
            var a = view?.Advice;
            if (a != null && a.Act == ActState.Pending) return "acting…";
            switch (view?.Status ?? AdviceStatus.None)
            {
                case AdviceStatus.Thinking: return "thinking…";
                case AdviceStatus.Ready:
                    if (a.Act == ActState.Executed) return "done";
                    if (a.Act == ActState.Stale) return "stale — dropped";
                    return "advice";
                case AdviceStatus.Stale:
                    return a != null && a.Act == ActState.Executed ? "done — waiting for the counter" : "stale — state changed";
            }
            if (s.Ready && s.Bridge == BridgeStatus.Connected && s.View != null && s.View.Deferred) return "deferred to you";
            return "waiting for the counter";
        }

        static string Move(Advice a, AdviceStatus status)
        {
            string label = Clean(a.Label ?? a.OptionId, 120);
            if (IsStageOffer(a)) return (status == AdviceStatus.Stale ? "was: " : "") + "offer: " + label;
            switch (a.Act)
            {
                case ActState.Pending: return "fence will: " + label;
                case ActState.Executed: return "fence did: " + label;
                case ActState.Stale: return "dropped (" + a.ActNote + "): " + label;
                case ActState.Blocked: return "fence suggests: " + label + " (not acting: " + a.ActNote + ")";
                default: return (status == AdviceStatus.Stale ? "was: " : "fence suggests: ") + label;
            }
        }

        // Stage advice names an item for the player to put on the counter; it is never executed.
        public static bool IsStageOffer(Advice a) =>
            a != null && a.Kind == StageContext.Kind && a.OptionId != null && a.OptionId.StartsWith(StageOptionIds.OfferPrefix, StringComparison.Ordinal);

        static string Options(Advice a)
        {
            if (a.Options == null || a.Options.Count == 0) return "";
            var sb = new StringBuilder();
            int shown = 0;
            foreach (var o in a.Options)
            {
                if (shown == MaxOptions)
                {
                    sb.Append("\n  … ").Append(N(a.Options.Count - shown)).Append(" more");
                    break;
                }
                if (shown > 0) sb.Append('\n');
                sb.Append(o.Id == a.OptionId ? "▶ " : "  ").Append(Clean(o.Label ?? o.Id, 90));
                if (a.Notes != null && a.Notes.TryGetValue(o.Id, out var note) && !string.IsNullOrEmpty(note))
                    sb.Append(" — ").Append(Clean(note, 80));
                shown++;
            }
            return sb.ToString();
        }

        static string Today(IList<DayEntry> today)
        {
            if (today == null || today.Count == 0) return "No customers yet today";
            return string.Join("\n", today.Select(e => Clean(e.Name, 30) + " → " + (e.Outcomes.Count > 0 ? string.Join("; ", e.Outcomes) : "…")));
        }

        static string Summary(SummaryResponse r, bool failed)
        {
            if (failed) return "bridge down";
            if (r == null) return "";
            // Memory off, or nothing dated yet: day 0 and zeros mean "no data", not a quiet day.
            if (r.Day <= 0 || r.Today == null) return "Day summary: no data yet";
            var t = r.Today;
            var sb = new StringBuilder();
            sb.Append("Day ").Append(N(r.Day)).Append(": ").Append(N(t.Decisions)).Append(" decisions · ")
                .Append(N(t.Sales)).Append(" sales · ").Append(N(t.Purchases)).Append(" purchases · cash ").Append(Signed(t.CashDelta))
                .Append(" · ").Append(N(t.Walkouts)).Append(t.Walkouts == 1 ? " walk-out" : " walk-outs");
            if (t.RepDelta != null && t.RepDelta.Count > 0)
                sb.Append("\nrep ").Append(string.Join(", ", t.RepDelta.OrderBy(kv => kv.Key, StringComparer.Ordinal)
                    .Select(kv => kv.Key + " " + kv.Value.ToString("+0.##;-0.##;0", CultureInfo.InvariantCulture))));
            return sb.ToString();
        }

        static string Lessons(SummaryResponse r)
        {
            if (r == null || string.IsNullOrEmpty(r.Lessons)) return r != null && r.Day > 0 ? "No lessons yet" : "";
            return "Lessons (day " + N(r.LessonsDay) + "):\n" + Clean(r.Lessons, 600, keepNewlines: true);
        }

        // One outcome result (docs/PROTOCOL.md, POST /v1/outcome) as a short line.
        public static string Describe(JObject result)
        {
            if (result == null) return null;
            string text;
            switch ((string)result["kind"])
            {
                case "trade":
                    int n = (result["items"] as JArray)?.Count ?? 0;
                    long cash = result.Value<long?>("cash_delta") ?? 0;
                    text = ((string)result["side"] == TradeBatch.Sold ? "sold " : "bought ") + N(n) + (n == 1 ? " item for " : " items for ") + N(Math.Abs(cash));
                    break;
                case "bargain":
                    var accepted = result.Value<bool?>("accepted");
                    text = (string)result["option"] + (accepted == true ? " accepted" : accepted == false ? " refused" : "");
                    if (result.Value<bool?>("walked_out") == true) text += ", walked out";
                    text += Rep(result["rep_delta"] as JObject);
                    break;
                case "left":
                    switch ((string)result["cause"])
                    {
                        case LeftCauses.Dismissed: text = "dismissed"; break;
                        case LeftCauses.WalkedOut: text = "walked out"; break;
                        case LeftCauses.Shutter: text = "left at closing"; break;
                        default: text = "left, done"; break;
                    }
                    break;
                default:
                    text = Fence.Inspect.InspectOutcomes.Describe(result);
                    if (text == null) return null;
                    break;
            }
            return (string)result["actor"] == Actors.Mod ? text + " (fence)" : text;
        }

        static string Rep(JObject delta)
        {
            if (delta == null || !delta.HasValues) return "";
            return " (" + string.Join(", ", delta.Properties().Select(p => p.Name + " " + ((double)p.Value).ToString("+0.##;-0.##;0", CultureInfo.InvariantCulture))) + " rep)";
        }

        // Model and game text go into TMP labels: no rich-text tags, bounded length.
        public static string Clean(string s, int max, bool keepNewlines = false)
        {
            if (string.IsNullOrEmpty(s)) return "";
            s = s.Replace('<', '‹').Replace('>', '›');
            // Truncate collapses all whitespace, newlines included.
            if (keepNewlines) return Lines(s, max);
            return FeatureText.Truncate(s, max);
        }

        static string Lines(string s, int max)
        {
            var sb = new StringBuilder();
            foreach (var raw in s.Split('\n'))
            {
                string line = FeatureText.Truncate(raw, 200);
                if (line.Length == 0) continue;
                if (sb.Length + line.Length + 1 > max)
                {
                    sb.Append(sb.Length > 0 ? "\n…" : "…");
                    break;
                }
                if (sb.Length > 0) sb.Append('\n');
                sb.Append(line);
            }
            return sb.ToString();
        }

        static string Signed(long n) => (n > 0 ? "+" : "") + N(n);

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
