using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Protocol;

namespace Fence.Barter
{
    public enum ActKind
    {
        SellAll,
        BuyAll,
        Wholesale,
        Bargain,
        Dismiss,
    }

    // One executable barter move, parsed from an option id the mod itself enumerated.
    public sealed class ActCommand
    {
        public string OptionId;
        public ActKind Kind;
        public BargainKind Bargain;
        public int Percent;
    }

    public static class ActCommands
    {
        // Null for anything P2a does not execute (staging, appraisal, unknown ids).
        public static ActCommand Parse(string optionId)
        {
            if (string.IsNullOrEmpty(optionId)) return null;
            switch (optionId)
            {
                case OptionIds.SellAll: return new ActCommand { OptionId = optionId, Kind = ActKind.SellAll };
                case OptionIds.BuyAll: return new ActCommand { OptionId = optionId, Kind = ActKind.BuyAll };
                case OptionIds.AcceptWholesale: return new ActCommand { OptionId = optionId, Kind = ActKind.Wholesale };
                case OptionIds.Dismiss: return new ActCommand { OptionId = optionId, Kind = ActKind.Dismiss };
                case OptionIds.Blackmail: return Bargain(optionId, BargainKind.Blackmail, BarterRules.CoercionPercent);
                case OptionIds.Threaten: return Bargain(optionId, BargainKind.Threaten, BarterRules.CoercionPercent);
            }
            foreach (BargainKind kind in new[] { BargainKind.Discount, BargainKind.Markup, BargainKind.BuyingMarkup, BargainKind.BuyingDiscount })
            foreach (int pct in BarterRules.PricePercents)
                if (OptionIds.Bargain(kind, pct) == optionId) return Bargain(optionId, kind, pct);
            return null;
        }

        static ActCommand Bargain(string id, BargainKind kind, int pct) =>
            new ActCommand { OptionId = id, Kind = ActKind.Bargain, Bargain = kind, Percent = pct };

        // compat/hooks.json ids the executor touches for this command; one missing disables acting on
        // this option only (the advice is still shown).
        public static IList<string> RequiredHooks(ActCommand c)
        {
            switch (c.Kind)
            {
                case ActKind.SellAll: return new[] { "barter.nego_instance", "barter.sell_all", "act.client_has_budget", "barter.confirm_over_budget" };
                case ActKind.BuyAll: return new[] { "barter.nego_instance", "barter.buy_all" };
                case ActKind.Wholesale: return new[] { "barter.nego_instance", "barter.wholesale" };
                case ActKind.Dismiss: return new[] { "barter.dismiss" };
            }
            string method;
            switch (c.Bargain)
            {
                case BargainKind.Discount: method = "barter.bargain_offer_discount"; break;
                case BargainKind.Markup: method = "barter.bargain_markup"; break;
                case BargainKind.BuyingMarkup: method = "barter.bargain_offer_markup"; break;
                case BargainKind.BuyingDiscount: method = "barter.bargain_demand_discount"; break;
                case BargainKind.Blackmail: method = "barter.bargain_blackmail"; break;
                default: method = "barter.bargain_threaten"; break;
            }
            return new[] { "barter.bargain_instance", method, "act.bargain_refresh_ui" };
        }
    }

    // The player's brake on acting. Paused is the overlay's Pause/Resume button; a fault (any exception
    // in the executor) is permanent for the session.
    public sealed class ActSwitch
    {
        public bool Paused { get; private set; }
        public string FaultReason { get; private set; }
        public bool Faulted => FaultReason != null;

        public void Pause() => Paused = true;

        public void Resume()
        {
            if (!Faulted) Paused = false;
        }

        public void Fault(string reason) => FaultReason = reason ?? "unknown error";
    }

    // Proof that acting is allowed right now: the bridge session is in act mode AND the player turned
    // on the act_enabled setting AND the switch is neither paused nor faulted. The only way to get one
    // is TryOpen, and the executor takes one and re-checks it before every game call.
    public sealed class ActGate
    {
        readonly Func<string> mode;
        readonly Func<bool> setting;
        readonly ActSwitch actSwitch;

        ActGate(Func<string> mode, Func<bool> setting, ActSwitch actSwitch)
        {
            this.mode = mode;
            this.setting = setting;
            this.actSwitch = actSwitch;
        }

        public static bool Allows(string mode, bool setting, ActSwitch actSwitch) =>
            mode == Modes.Act && setting && actSwitch != null && !actSwitch.Paused && !actSwitch.Faulted;

        public static ActGate TryOpen(Func<string> mode, Func<bool> setting, ActSwitch actSwitch)
        {
            if (mode == null || setting == null) return null;
            return Allows(mode(), setting(), actSwitch) ? new ActGate(mode, setting, actSwitch) : null;
        }

        public bool StillOpen => Allows(mode(), setting(), actSwitch);
    }

    // Rate limits and settling: at most PerCustomer actions per customer and PerMinute per rolling
    // minute, and no new question for SettleMs after an action (the tracker's own stability window
    // and readiness predicate cover dialogue and walk-out animations after that).
    public sealed class ActLimiter
    {
        public const int PerCustomer = 4;
        public const int PerMinute = 12;
        public const long SettleMs = 1500;
        const long MinuteMs = 60000;

        readonly Queue<long> recent = new Queue<long>();
        int customerCount;
        long lastActionMs;
        bool acted;

        public int CustomerCount => customerCount;

        public void NewClient() => customerCount = 0;

        // Null when an action is allowed now; otherwise why not.
        public string Refusal(long nowMs)
        {
            while (recent.Count > 0 && nowMs - recent.Peek() >= MinuteMs) recent.Dequeue();
            if (customerCount >= PerCustomer) return "already " + PerCustomer + " actions for this customer";
            if (recent.Count >= PerMinute) return PerMinute + " actions in the last minute";
            if (Settling(nowMs)) return "settling after the last action";
            return null;
        }

        public void Record(long nowMs)
        {
            customerCount++;
            recent.Enqueue(nowMs);
            lastActionMs = nowMs;
            acted = true;
        }

        public bool Settling(long nowMs) => acted && nowMs - lastActionMs < SettleMs;
    }

    public enum ActVerdictKind
    {
        Execute,
        // Not ready yet (a dialogue line is playing): keep the action for a short grace period.
        Wait,
        // Dropped: the state the decision was made for is gone.
        Stale,
        // Still valid but not executed (limits, a missing member): shown as advice.
        Blocked,
    }

    public sealed class ActVerdict
    {
        public ActVerdictKind Kind;
        public string Reason;

        public static ActVerdict Of(ActVerdictKind kind, string reason = null) => new ActVerdict { Kind = kind, Reason = reason };
    }

    public static class ActPlanner
    {
        public const long ReadyGraceMs = 3000;

        // Runs on the main thread right before executing, against a freshly read snapshot. A human click
        // between decide and execute changes the signature, so the human wins.
        public static ActVerdict Evaluate(Advice pending, object clientKey, BarterSnapshot fresh, long nowMs, Func<ActCommand, bool> executable, ActLimiter limiter)
        {
            if (fresh == null || fresh.Client == null || !ReferenceEquals(fresh.Client.Key, clientKey)) return ActVerdict.Of(ActVerdictKind.Stale, "customer changed");
            if (fresh.CoverageGap != null) return ActVerdict.Of(ActVerdictKind.Stale, "not covered: " + fresh.CoverageGap);
            if (!fresh.Ready)
                return nowMs - pending.ReceivedMs < ReadyGraceMs ? ActVerdict.Of(ActVerdictKind.Wait) : ActVerdict.Of(ActVerdictKind.Stale, "counter not ready");
            if (fresh.Signature != pending.Signature) return ActVerdict.Of(ActVerdictKind.Stale, "state changed");
            if (fresh.Options == null || !fresh.Options.Any(o => o.Id == pending.OptionId)) return ActVerdict.Of(ActVerdictKind.Stale, "option no longer offered");
            var cmd = ActCommands.Parse(pending.OptionId);
            if (cmd == null) return ActVerdict.Of(ActVerdictKind.Blocked, "not an action fence executes");
            if (executable != null && !executable(cmd)) return ActVerdict.Of(ActVerdictKind.Blocked, "game member missing for this action");
            string refusal = limiter?.Refusal(nowMs);
            if (refusal != null) return ActVerdict.Of(ActVerdictKind.Blocked, refusal);
            return ActVerdict.Of(ActVerdictKind.Execute);
        }
    }

    public static class Actors
    {
        public const string Mod = "mod";
        public const string Human = "human";
    }

    // Which decision and actor an outcome belongs to. A hook firing while the executor runs is the
    // mod's doing and belongs to the executed decision; anything else is the human's and belongs to
    // the newest decision sent (the advice on screen). Consequences that surface later (the customer
    // leaving after the all-done dialogue) go to whoever made the last move.
    public sealed class Attribution
    {
        string acting;
        string lastDecision;
        string lastActor;

        public bool Executing => acting != null;

        public void NewClient()
        {
            acting = null;
            lastDecision = null;
            lastActor = null;
        }

        public void BeginMod(string decisionId) => acting = decisionId;

        public void EndMod() => acting = null;

        public (string decisionId, string actor) Direct(string newestDecision)
        {
            var r = acting != null ? (acting, Actors.Mod) : (newestDecision, Actors.Human);
            lastDecision = r.Item1;
            lastActor = r.Item2;
            return r;
        }

        public (string decisionId, string actor) Consequence(string newestDecision) =>
            acting != null ? (acting, Actors.Mod)
            : lastDecision != null ? (lastDecision, lastActor)
            : (newestDecision, Actors.Human);
    }
}
