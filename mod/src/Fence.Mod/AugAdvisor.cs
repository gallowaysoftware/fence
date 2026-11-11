using System;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Aug;
using Fence.Barter;
using Fence.Bridge;
using Fence.Decisions;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // aug_check advice (docs/re/AUGS.md §5): when an aug (perfect detection) or a suspected aug (tells) is
    // at the counter and the gun can be raised, ask the bridge and show the answer in the overlay. Advice
    // only, in every mode: nothing here raises the gun, shoots, scans, probes or threatens. Also records
    // what the gun did: shots, kills, captures and augs that left alive.
    sealed class AugAdvisor
    {
        readonly FenceLog log;
        readonly BridgeSession session;
        readonly BridgeClient client;
        readonly GameBarter game = new GameBarter();
        readonly AugTracker tracker = new AugTracker();
        readonly Recorder.Emit emitRecord;
        readonly Action<string> noteOutcome;
        readonly string launchTag = Guid.NewGuid().ToString("N").Substring(0, 8);
        readonly HashSet<string> loggedOnce = new HashSet<string>();
        int decisionCounter;
        IList<FactionRep> repBefore;
        int? cashBefore, presenceBefore;
        string shotName;
        // The last dialogue line before this customer arrived: their lines are the ones after it.
        string linesBefore;

        public bool Enabled { get; set; }
        // Recording of shots and kills (record.aug); independent of the advice.
        public bool RecordEnabled { get; set; }

        public AugAdvisor(FenceLog log, BridgeSession session, BridgeClient client, Recorder.Emit emitRecord, Action<string> noteOutcome)
        {
            this.log = log;
            this.session = session;
            this.client = client;
            this.emitRecord = emitRecord;
            this.noteOutcome = noteOutcome;
        }

        // Called from BarterAdvisor's poll with the customer it tracks.
        public void Tick(ClientInfo info, int clientSeq, long now)
        {
            if (info == null) return;
            if (!ReferenceEquals(info.Key, tracker.ClientKey))
            {
                tracker.NewClient(info.Key, clientSeq);
                linesBefore = null;
                try
                {
                    linesBefore = GameAug.Lines(null).LastOrDefault();
                }
                catch (Exception)
                {
                }
            }
            if (!Enabled) return;
            AugQuestion q;
            try
            {
                q = AugCheck.Build(GameAug.Situation(game, info, clientSeq, linesBefore), AugDetection.Current);
            }
            catch (Exception e)
            {
                Once("read", "cannot read the aug state (aug advice off until restart): " + e.GetType().Name + ": " + e.Message);
                Enabled = false;
                return;
            }
            if (!tracker.Show(q) || !session.WantsDecisions(now)) return;
            string decisionId = "a-" + launchTag + "-" + (++decisionCounter);
            var request = AugCheck.Request(q, decisionId, DecisionDeadlines.ForMode(session.Mode));
            tracker.Sent(q, decisionId);
            object key = tracker.ClientKey;
            client.Decide(request, outcome => OnDecided(key, q, request, outcome));
        }

        void OnDecided(object key, AugQuestion q, DecideRequest request, DecisionOutcome outcome)
        {
            if (!ReferenceEquals(key, tracker.ClientKey)) return;
            var advice = new Advice
            {
                DecisionId = request.DecisionId,
                Kind = request.Kind,
                Signature = q.Signature,
                Deferred = outcome.Choice == null || outcome.Choice.IsDefer,
                OptionId = outcome.Choice?.Option,
                Rationale = outcome.Response?.Rationale,
                ReceivedMs = BridgeClient.NowMs(),
                Options = request.Options,
                Notes = outcome.Response?.OptionNotes,
            };
            advice.Label = request.Options.FirstOrDefault(o => o.Id == advice.OptionId)?.Label;
            tracker.Answered(q.Signature, advice);
            log.Info("aug " + request.DecisionId + " [" + q.Signature + "]: " + (advice.Deferred ? "defer (" + outcome.Reason + ")" : advice.OptionId)
                     + (string.IsNullOrEmpty(advice.Rationale) ? "" : " | " + FeatureText.Truncate(advice.Rationale, 300)));
        }

        public string OverlayLine()
        {
            if (!Enabled || tracker.ClientKey == null || !ReferenceEquals(GameBarter.CurrentClient(), tracker.ClientKey)) return "";
            return AugText.Line(tracker.Current, tracker.CurrentAnswer, tracker.Thinking);
        }

        // The aug decision a gun move attaches to, or null.
        public AdviceInForce InForce()
        {
            if (tracker.LastDecisionId == null || !ReferenceEquals(GameBarter.CurrentClient(), tracker.ClientKey)) return null;
            return new AdviceInForce { ClientSeq = tracker.ClientSeq, DecisionId = tracker.LastDecisionId, Advised = tracker.LastAdvised };
        }

        // The tells on screen when the player fires, in the mode's words.
        public JObject ShotContext()
        {
            var o = new JObject();
            try
            {
                var info = game.Client();
                if (info == null) return o;
                o["name"] = info.Name;
                var c = new JObject();
                AugDetection.Describe(c, info);
                foreach (var p in c.Properties()) o[p.Name] = p.Value;
            }
            catch (Exception)
            {
            }
            return o;
        }

        // ---- hooks ----

        public void Unholstered() => tracker.Holstered = false;

        public void Holstered(bool shot)
        {
            if (!shot) tracker.Holstered = true;
        }

        public void Reported() => tracker.Reported = true;

        public void Dismissing(bool on) => tracker.Dismissing = on;

        public void BeforeShot()
        {
            shotName = null;
            try
            {
                shotName = GameBarter.CurrentClient()?.displayName;
            }
            catch (Exception)
            {
            }
        }

        public void BeforeResult()
        {
            try
            {
                repBefore = game.Reputations();
                cashBefore = game.Store().Cash;
                presenceBefore = GameAug.Presence();
            }
            catch (Exception)
            {
                repBefore = null;
            }
        }

        // OnGoodKill postfix: cash and rep moved, presence dropped.
        public void GoodKill() => Result(AugEvents.GoodKill, null);

        // CleanupKill postfix: a human shot; the crime is the day's MANSLAUGHTER amount.
        public void WrongfulKill(string name)
        {
            int day = 0;
            try
            {
                day = game.Store().Day ?? 0;
            }
            catch (Exception)
            {
            }
            Result(AugEvents.WrongfulKill, new JObject { ["crime"] = new JObject { ["id"] = "MANSLAUGHTER", ["amount"] = AugCheck.Manslaughter(day) } }, name);
        }

        public void WantedCapture(string name) => Result(AugEvents.WantedCapture, null, name);

        public void LeftAlive()
        {
            bool threatened = false;
            try
            {
                threatened = game.Bargain().ThreatenOffered;
            }
            catch (Exception)
            {
            }
            Result(AugEvents.LeftAlive, new JObject { ["cause"] = tracker.LeftCause(threatened) }, CurrentName());
        }

        string CurrentName()
        {
            try
            {
                return GameBarter.CurrentClient()?.displayName;
            }
            catch (Exception)
            {
                return null;
            }
        }

        void Result(string ev, JObject extra, string name = null)
        {
            if (!RecordEnabled) return;
            var data = AugEvents.Data(ev, tracker.ClientSeq, name ?? shotName, extra);
            JObject hidden = null;
            try
            {
                var store = game.Store();
                if (cashBefore.HasValue && store.Cash.HasValue && ev != AugEvents.LeftAlive) data["cash_delta"] = store.Cash.Value - cashBefore.Value;
                if (repBefore != null && ev != AugEvents.LeftAlive) data["rep_delta"] = Outcomes.RepDelta(repBefore, game.Reputations());
                int? after = GameAug.Presence();
                hidden = new JObject { ["is_aug"] = ev == AugEvents.GoodKill || ev == AugEvents.LeftAlive };
                if (presenceBefore.HasValue) hidden["presence_before"] = presenceBefore.Value;
                if (after.HasValue) hidden["presence_after"] = after.Value;
            }
            catch (Exception)
            {
            }
            repBefore = null;
            cashBefore = presenceBefore = null;
            emitRecord(RecordKinds.Aug, data, hidden);
            var result = Outcomes.WithActor(AugEvents.Outcome(data), Actors.Human);
            noteOutcome?.Invoke(AugEvents.Describe(result));
            string decisionId = tracker.LastDecisionId;
            if (decisionId == null) return;
            int day = 0;
            try
            {
                day = game.Store().Day ?? 0;
            }
            catch (Exception)
            {
            }
            client.SendOutcome(new OutcomeRequest { DecisionId = decisionId, GameDay = day, Result = result });
        }

        void Once(string key, string message)
        {
            if (loggedOnce.Add(key)) log.Warn(message);
        }
    }

    // Aug reads around the counter; members listed in compat/hooks.json under advise.aug / record.aug.
    static class GameAug
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static AugSituation Situation(GameBarter game, ClientInfo info, int clientSeq, string linesBefore)
        {
            var ps = PlayerStore.Instance;
            var ui = StoreUIManager.Instance;
            var c = GameBarter.CurrentClient();
            var flags = game.Bargain() ?? new BargainFlags();
            var s = new AugSituation
            {
                ClientSeq = clientSeq,
                Client = info,
                Ui = game.Ui(),
                Flags = flags,
                Store = game.Store(),
                AugsActive = ps != null && ps.isAugIntroduced && !ps.isAugEnded,
                ShootingUnlocked = ps != null && ps.isShootingUnlocked,
                ShootingMode = ui != null && ui.isShootingMode,
                BeingArrested = ps != null && ps.isClientBeingArrested,
                HandlingShootout = ps != null && ps.isHandlingShootout,
                Presence = ps?.augPresence,
                Difficulty = ps?.difficultyId,
            };
            if (c != null)
            {
                s.Arrestable = c.arrestable;
                s.ForceShootable = c.forceShootable;
                s.NonShootable = c.nonShootable;
                s.ShotCountsAsCapture = c.shotCountsAsCapture;
            }
            if (!s.AugsActive) return s;
            s.IntroDay = PlayerStore.GetAugIntroducedDay();
            s.AugMenace = StartingPerk.IsAugMenace();
            var rules = Difficulty.Rules;
            if (rules != null)
            {
                s.PresenceRise = rules.augLeftAlivePresenceRise;
                s.KillPresenceDrop = rules.augKillPresenceDrop;
                s.GameOverAtFull = rules.augGameOverAtFullPresence;
            }
            s.ScannerOwned = ScannerOwned();
            s.Lines = Lines(linesBefore);
            try
            {
                s.MarkupRefused = flags.MarkupOffered && !GameReflect.MarkupApplied();
                s.DiscountRefused = flags.BuyingDiscountOffered && !GameReflect.BuyingDiscountApplied();
            }
            catch (Exception)
            {
            }
            s.MarkupOdds = game.Odds(BargainKind.Markup, AugCheck.ProbePercent);
            s.DiscountOdds = game.Odds(BargainKind.BuyingDiscount, AugCheck.ProbePercent);
            return s;
        }

        // The dialogue history after `after` (the last line before this customer), at most the last six.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<string> Lines(string after)
        {
            var all = PlayerStore.Instance?.dialogHistory?.ToList() ?? new List<string>();
            if (after != null)
            {
                int i = all.LastIndexOf(after);
                if (i >= 0) all = all.Skip(i + 1).ToList();
            }
            return all.Skip(Math.Max(0, all.Count - 6)).ToList();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool ScannerOwned()
        {
            var entry = EmporiumEntry.Instance;
            if (entry == null) return false;
            foreach (var item in entry.GetAllOwnedItems(false, false))
                if (item != null && item.identifier == "aug_scanner") return true;
            return false;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int? Presence() => PlayerStore.Instance?.augPresence;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool IsShot() => StoreUIManager.Instance != null && StoreUIManager.Instance.isShot;

        // AugHelper.CleanupKill clears the shot client; read the name first.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string ShotClientName() => AugHelper.GetShotClient()?.GetClientBlueprint()?.displayName;
    }
}
