using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Barter;
using Fence.Bridge;
using Fence.Decisions;
using Fence.Inspect;
using Fence.Protocol;
using Fence.Recording;

namespace Fence.Mod
{
    // Inspect advice (docs/re/INSPECT.md): when the magnifier opens a customer's item that the game
    // lets the player confront or appraise, ask the bridge and show the answer in the overlay. Advice
    // only, in every mode: nothing here confronts, talks or submits an estimate. Also attaches the
    // player's verdict and the game's response to the decision.
    sealed class InspectAdvisor
    {
        readonly FenceLog log;
        readonly BridgeSession session;
        readonly BridgeClient client;
        readonly GameBarter game = new GameBarter();
        readonly InspectTracker tracker = new InspectTracker();
        readonly Func<AdviceInForce> barterInForce;
        readonly Action<string> noteOutcome;
        readonly string launchTag = Guid.NewGuid().ToString("N").Substring(0, 8);
        readonly HashSet<string> loggedOnce = new HashSet<string>();
        int decisionCounter;

        public bool Enabled { get; set; }

        public InspectAdvisor(FenceLog log, BridgeSession session, BridgeClient client, Func<AdviceInForce> barterInForce, Action<string> noteOutcome)
        {
            this.log = log;
            this.session = session;
            this.client = client;
            this.barterInForce = barterInForce;
            this.noteOutcome = noteOutcome;
        }

        int ClientSeq() => barterInForce?.Invoke()?.ClientSeq ?? 0;

        // Inspections belong to the customer at the counter; a new one starts a fresh tracker.
        bool SyncClient()
        {
            var c = GameBarter.CurrentClient();
            if (c == null) return false;
            if (!ReferenceEquals(c, tracker.ClientKey)) tracker.NewClient(c);
            return true;
        }

        // Postfix of InspectionUIManager.OnInspectItem: the panel is built, so the game's own choice of
        // confront or appraisal panel is readable.
        public void OnInspected(GameItem item)
        {
            if (!Enabled || item == null || !SyncClient()) return;
            InspectQuestion q;
            try
            {
                q = Question(item);
            }
            catch (Exception e)
            {
                Once("read", "cannot read the inspected item (inspect advice off until restart): " + e.GetType().Name + ": " + e.Message);
                Enabled = false;
                return;
            }
            if (!tracker.Show(q)) return;
            long now = BridgeClient.NowMs();
            if (!session.WantsDecisions(now)) return;
            string decisionId = "i-" + launchTag + "-" + (++decisionCounter);
            var request = InspectContext.Request(q, decisionId, DecisionDeadlines.ForMode(session.Mode));
            tracker.Sent(q, decisionId);
            object key = tracker.ClientKey;
            client.Decide(request, outcome => OnDecided(key, q, request, outcome));
        }

        InspectQuestion Question(GameItem item)
        {
            var c = GameBarter.CurrentClient();
            int uid = game.Uid(item);
            var s = new InspectSituation
            {
                ClientSeq = ClientSeq(),
                Uid = uid,
                ItemId = GameInspect.Identifier(item),
                Name = GameInspect.Name(item),
                Price = GameInspect.Price(item),
                Owned = GameInspect.Owned(item),
                Client = game.Client(),
                Reputations = game.Reputations(),
                ConfrontPanelOpen = GameInspect.ConfrontPanelOpen(),
                AppraisalService = GameInspect.AppraisalService(c),
                GameDay = game.Store().Day ?? 0,
            };
            if (s.AppraisalService && !s.Owned)
            {
                s.Appraisal = tracker.Terms(uid, GameInspect.Terms(item));
                s.TrueValue = GameInspect.TrueValue(item);
            }
            return InspectContext.Build(new GameInspectItem(item), s);
        }

        void OnDecided(object key, InspectQuestion q, DecideRequest request, DecisionOutcome outcome)
        {
            if (!ReferenceEquals(key, tracker.ClientKey)) return;
            var advice = new Advice
            {
                DecisionId = request.DecisionId,
                Kind = request.Kind,
                Signature = q.Signature,
                Deferred = outcome.Choice == null || outcome.Choice.IsDefer,
                OptionId = outcome.Choice?.Option,
                Params = outcome.Choice?.Params,
                Rationale = outcome.Response?.Rationale,
                ReceivedMs = BridgeClient.NowMs(),
                Options = request.Options,
                Notes = outcome.Response?.OptionNotes,
            };
            advice.Label = request.Options.FirstOrDefault(o => o.Id == advice.OptionId)?.Label;
            tracker.Answered(q.Signature, advice);
            log.Info("inspect " + request.DecisionId + " [" + q.Signature + "]: " + (advice.Deferred ? "defer (" + outcome.Reason + ")" : advice.OptionId
                     + (advice.Params != null ? " " + advice.Params.ToString(Newtonsoft.Json.Formatting.None) : "")
                     + (string.IsNullOrEmpty(advice.Rationale) ? "" : " | " + FeatureText.Truncate(advice.Rationale, 300))));
        }

        public string OverlayLine()
        {
            if (!Enabled || tracker.ClientKey == null || !ReferenceEquals(GameBarter.CurrentClient(), tracker.ClientKey)) return "";
            return InspectText.Line(tracker.Current, tracker.CurrentAnswer, tracker.Thinking);
        }

        // The inspect decision human moves on the item attach to, or null for the barter one.
        public AdviceInForce InForce()
        {
            if (tracker.ClientKey == null || !ReferenceEquals(GameBarter.CurrentClient(), tracker.ClientKey)) return null;
            return tracker.InForce(ClientSeq());
        }

        // Prefix of InspectionUIManager.OnConfront: the issue the player submits (null when nothing is
        // checked, so the game ignores the click).
        public string BeforeConfront()
        {
            var item = GameInspect.Inspecting();
            if (item == null || !SyncClient()) return null;
            var inspected = new GameInspectItem(item);
            string kind = ItemKinds.Of(inspected);
            var toggles = GameInspect.Toggles();
            if (kind == null || toggles == null || !toggles.Any(t => t)) return null;
            string shown = inspected.ShownCondition(ItemKinds.ClaimCategory(kind)) ?? "";
            string issue = shown.StartsWith("genuine", StringComparison.Ordinal) ? ConfrontIssues.Chosen(kind, toggles) : InspectOptionIds.Genuine;
            tracker.Verdict(GameInspect.Identifier(item), issue);
            return issue;
        }

        public string VerdictFor(string itemId) => tracker.VerdictFor(itemId);

        // Postfix of EmporiumAnalytics.TrackConfront: the customer's answer to the verdict.
        public void Confronted(string itemId, string confrontType, string outcome)
        {
            if (!SyncClient()) return;
            string decisionId = tracker.DecisionFor(itemId, out _);
            Send(decisionId, InspectOutcomes.Confront(tracker.VerdictFor(itemId), confrontType, outcome));
        }

        public void Estimating(GameItem item, int value)
        {
            if (item == null || !SyncClient()) return;
            tracker.Estimated(game.Uid(item), value);
        }

        // Postfixes of the appraisal dialogues: what the customer said to the estimate in flight.
        public void AppraisalAnswer(string feedback, GameItem item)
        {
            if (!SyncClient()) return;
            var current = tracker.Current;
            int? estimate = tracker.Feedback(item != null ? game.Uid(item) : (int?)null, feedback, out int uid);
            if (!estimate.HasValue) return;
            int? truth = null;
            if (feedback == AppraisalFeedback.Accepted && item != null)
            {
                try
                {
                    truth = GameInspect.TrueValue(item);
                }
                catch (Exception)
                {
                }
            }
            string decisionId = current != null && current.Uid == uid ? tracker.DecisionFor(current.ItemId, out _) : null;
            Send(decisionId, InspectOutcomes.Appraisal(estimate.Value, feedback, truth));
        }

        void Send(string decisionId, Newtonsoft.Json.Linq.JObject result)
        {
            Outcomes.WithActor(result, Actors.Human);
            noteOutcome?.Invoke(OverlayText.Describe(result));
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
}
