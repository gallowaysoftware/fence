using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Barter;
using Fence.Bridge;
using Fence.Decisions;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;
using UnityEngine;

namespace Fence.Mod
{
    // Asks the bridge about the current customer and shows the answer in the overlay (P1); in act mode
    // with act_enabled on, also executes it through BarterExecutor (P2). Polls the decision-ready
    // predicate (docs/re/BARTER.md) instead of hooking private UI code, and reports what actually
    // happened (bargain rolls, trades, the customer leaving) with who did it.
    sealed class BarterAdvisor
    {
        const float PollSeconds = 0.25f;
        const long TradeQuietMs = 150;

        readonly FenceLog log;
        readonly BridgeSession session;
        readonly BridgeClient client;
        readonly Func<IEnumerator, bool> startCoroutine;
        readonly Action<string, JObject> emitEvent;
        readonly Recorder.Emit emitRecord;
        readonly GameBarter game = new GameBarter();
        readonly AdviceTracker tracker = new AdviceTracker();
        readonly TradeBatch trades = new TradeBatch();
        readonly ActSwitch actSwitch = new ActSwitch();
        readonly ActLimiter limiter = new ActLimiter();
        readonly Attribution attribution = new Attribution();
        readonly BarterExecutor executor = new BarterExecutor();
        readonly DayLog today = new DayLog();
        readonly SummaryPoller summaryPoller = new SummaryPoller();
        readonly Func<bool> actSetting;
        readonly Func<string, bool> hookUsable;
        readonly FenceOverlay overlay;
        readonly Recorder recorder;
        readonly CustomerObserver observer = new CustomerObserver();
        bool beforeOpen;
        SummaryResponse summary;
        BarterSnapshot lastSnap;
        readonly string launchTag = Guid.NewGuid().ToString("N").Substring(0, 8);
        readonly HashSet<string> loggedOnce = new HashSet<string>();
        int decisionCounter;
        bool polling;
        bool stopped;
        bool canAsk;
        bool covered;
        bool walkedOut;
        bool playerDismissing;
        bool leftReported;
        long lastTradeMs;
        string stageSig;
        BarterSnapshot stageSnap;
        int stageCandidates;
        IList<StageCandidate> stageAll;
        IList<StageCandidate> stageTop;
        long? stageBudget;

        public bool AdviseEnabled { get; set; }
        // Staging advice for buying customers (advise.stage); barter advice works without it.
        public bool StageEnabled { get; set; }
        // Size of the instant shortlist (stage_shortlist setting); 0 disables it.
        public Func<int> ShortlistSize { get; set; } = () => StageShortlist.DefaultSize;
        public bool DisplayEnabled { get; set; }
        // The inspect advice line for the overlay; set by FenceMod.
        public Func<string> InspectLine { get; set; }
        // aug_check advice for the customer at the counter; set by FenceMod.
        public AugAdvisor Augs { get; set; }
        public bool OverlayEnabled { get => overlay.Enabled; set => overlay.Enabled = value; }
        // The autopilot's overlay line and its pause toggle; set by FenceMod.
        public Func<(string line, bool alert, string pause)> AutopilotStatus { get; set; }
        // Toggles the autopilot; true when it now runs.
        public Func<string, bool> AutopilotToggle { get; set; }
        // Every trade the game books (sold: true when the shop sold), for the autopilot's day ledger.
        public Action<bool, GameItem, int> Traded { get; set; }
        public SummaryResponse LatestSummary => summary;

        public BarterAdvisor(FenceLog log, BridgeSession session, BridgeClient client, Func<IEnumerator, bool> startCoroutine, Action<string, JObject> emitEvent, Recorder.Emit recordSink,
            Func<bool> actSetting, Func<string, bool> hookUsable, Recorder recorder)
        {
            this.recorder = recorder;
            if (recorder != null) recorder.Advice = InForce;
            this.log = log;
            this.session = session;
            this.client = client;
            this.startCoroutine = startCoroutine;
            this.emitEvent = emitEvent;
            emitRecord = recordSink;
            this.actSetting = actSetting;
            this.hookUsable = hookUsable;
            overlay = new FenceOverlay(log, AskAgain, TogglePause);
        }

        public void Stop()
        {
            stopped = true;
            overlay.Close();
        }

        // ModHelper.StartCoroutine may return null during Init/OnEnable (docs/COMPAT.md #15), so the
        // loop starts from the first hook that fires in a running game.
        public void EnsurePolling()
        {
            if (polling || stopped || !AdviseEnabled) return;
            polling = startCoroutine(Poll());
        }

        IEnumerator Poll()
        {
            try
            {
                while (!stopped && AdviseEnabled)
                {
                    try
                    {
                        Tick();
                    }
                    catch (Exception e)
                    {
                        Once("tick", "advisor tick failed (advice off until restart): " + e);
                        AdviseEnabled = false;
                    }
                    yield return new WaitForSecondsRealtime(PollSeconds);
                }
            }
            finally
            {
                polling = false;
            }
        }

        static long Now() => BridgeClient.NowMs();

        void Tick()
        {
            long now = Now();
            if (!trades.IsEmpty && now - lastTradeMs >= TradeQuietMs) FlushTrades();
            if (summaryPoller.TryBegin(now))
                client.Summary(r =>
                {
                    if (r != null) summary = r;
                    summaryPoller.Done(r != null);
                });

            BarterSnapshot snap;
            try
            {
                snap = BarterContext.Read(game, tracker.ClientSeq);
            }
            catch (Exception e)
            {
                Once("read", "cannot read barter state (advice off until restart): " + e.GetType().Name + ": " + e.Message);
                AdviseEnabled = false;
                return;
            }
            lastSnap = snap;

            if (snap.Client == null)
            {
                tracker.Observe(null, now, false);
                ShowOverlay(now);
                return;
            }
            if (!ReferenceEquals(snap.Client.Key, tracker.ClientKey)) StartClient(snap.Client, false);
            if (observer.Pending && CounterReady(snap) && observer.Ready(tracker.ClientSeq)) Observe("ready", snap);
            Augs?.Tick(snap.Client, tracker.ClientSeq, now);

            covered = snap.CoverageGap == null;
            if (!covered)
            {
                Once("gap:" + tracker.ClientSeq, "not advising " + snap.Client.Name + ": " + snap.CoverageGap);
                tracker.Observe(null, now, false);
                ShowOverlay(now);
                return;
            }

            canAsk = session.WantsDecisions(now);
            bool holdAsks = false;
            var pending = tracker.PendingAct;
            if (pending != null) holdAsks = HandlePending(pending, snap, now);

            stageTop = null;
            var question = snap.Ready && StageEnabled ? StageQuestion(snap) ?? snap : snap;
            string signature = question.Ready ? question.Signature : null;
            bool mayAsk = canAsk && !holdAsks && !limiter.Settling(now) && question.Options != null && question.Options.Count > 0;
            string ask = tracker.Observe(signature, now, mayAsk);
            if (ask != null)
            {
                string decisionId = "d-" + launchTag + "-" + (++decisionCounter);
                var request = BarterContext.Request(question, decisionId, DecisionDeadlines.ForMode(session.Mode));
                tracker.Sent(ask, decisionId);
                object key = tracker.ClientKey;
                client.Decide(request, outcome => OnDecided(key, ask, request, outcome));
            }
            ShowOverlay(now);
        }

        // The stage question when a buying customer waits on what to offer; null to ask the barter one.
        // The shop is read once per counter state.
        BarterSnapshot StageQuestion(BarterSnapshot snap)
        {
            if (!StageContext.IsMoment(snap.Client, snap.Items, snap.Ui, out var budgetLeft, out var suitable)) return null;
            string sig = StageContext.Signature(snap.Signature, budgetLeft);
            if (sig != stageSig)
            {
                IList<ShopItem> shop;
                try
                {
                    shop = game.ShopItems(snap.Client);
                }
                catch (Exception e)
                {
                    Once("stage", "cannot read the shop's items (staging advice off until restart): " + e.GetType().Name + ": " + e.Message);
                    StageEnabled = false;
                    return null;
                }
                var candidates = StageContext.Rank(snap.Client, shop, budgetLeft, out int total);
                stageSig = sig;
                stageSnap = StageContext.Build(snap, candidates, total, budgetLeft, suitable);
                stageCandidates = candidates.Count;
                stageAll = candidates;
            }
            if (!StageContext.Ask(suitable, stageCandidates, tracker.AnswerFor(sig))) return null;
            int n = SafeShortlistSize();
            stageTop = n > 0 ? stageAll.Take(n).ToList() : null;
            stageBudget = budgetLeft;
            return stageSnap;
        }

        void ForgetStage()
        {
            stageSig = null;
            stageSnap = null;
            stageCandidates = 0;
            stageAll = null;
            stageTop = null;
        }

        int SafeShortlistSize()
        {
            try
            {
                return ShortlistSize?.Invoke() ?? 0;
            }
            catch (Exception)
            {
                return StageShortlist.DefaultSize;
            }
        }

        // ---- act (P2) ----

        bool ActSettingOn()
        {
            try
            {
                return actSetting != null && actSetting();
            }
            catch (Exception)
            {
                return false;
            }
        }

        bool Executable(ActCommand c) => ActCommands.RequiredHooks(c).All(hookUsable);

        // Returns true while new questions must wait (an action is pending or was just executed).
        bool HandlePending(Advice pending, BarterSnapshot snap, long now)
        {
            var verdict = ActPlanner.Evaluate(pending, tracker.ClientKey, snap, now, Executable, limiter);
            switch (verdict.Kind)
            {
                case ActVerdictKind.Wait:
                    return true;
                case ActVerdictKind.Stale:
                    pending.Act = ActState.Stale;
                    pending.ActNote = verdict.Reason;
                    log.Info("act " + pending.DecisionId + " stale (" + verdict.Reason + "): " + pending.OptionId + " not executed");
                    return false;
                case ActVerdictKind.Blocked:
                    pending.Act = ActState.Blocked;
                    pending.ActNote = verdict.Reason;
                    log.Info("act " + pending.DecisionId + " not executed (" + verdict.Reason + "): " + pending.OptionId);
                    return false;
            }

            if (!ActGate.Allows(session.Mode, ActSettingOn(), actSwitch))
            {
                pending.Act = ActState.Blocked;
                pending.ActNote = "acting off";
                return false;
            }
            var command = ActCommands.Parse(pending.OptionId);
            limiter.Record(now);
            try
            {
                bool done = RunExecutor(command, pending.DecisionId, null);
                pending.Act = done ? ActState.Executed : ActState.Blocked;
                if (!done) pending.ActNote = "acting off";
                log.Info("act " + pending.DecisionId + " [" + pending.Signature + "]: " + (done ? "executed " : "skipped ") + pending.OptionId);
            }
            catch (Exception e)
            {
                actSwitch.Fault(e.GetType().Name + " in " + pending.OptionId + ": " + e.Message);
                pending.Act = ActState.Blocked;
                pending.ActNote = "error, acting off";
                log.Error("act " + pending.DecisionId + " failed, acting off for this session: " + e);
            }
            return true;
        }

        // The autopilot's counter move (docs/STREAM.md): the executor's own pick, attributed to the decision
        // the bridge logged for this counter state. Exceptions propagate to the autopilot, which stops.
        public bool ExecuteForAutopilot(ActCommand command, Func<bool> running)
        {
            if (command == null) return false;
            playerDismissing = command.Kind == ActKind.Dismiss;
            try
            {
                return RunExecutor(command, tracker.LastDecisionId, running);
            }
            finally
            {
                playerDismissing = false;
            }
        }

        // The only call of BarterExecutor, through a gate opened here: the bridge's act mode (P2a) or, with
        // `autopilotRunning`, the autopilot.
        bool RunExecutor(ActCommand command, string decisionId, Func<bool> autopilotRunning)
        {
            var gate = autopilotRunning != null ? ActGate.TryOpenAutopilot(autopilotRunning) : ActGate.TryOpen(() => session.Mode, ActSettingOn, actSwitch);
            if (gate == null) return false;
            attribution.BeginMod(decisionId, autopilotRunning != null ? Actors.Autopilot : Actors.Mod);
            recorder?.Filter.EnterExecutor();
            try
            {
                return executor.Execute(gate, command);
            }
            finally
            {
                attribution.EndMod();
                recorder?.Filter.ExitExecutor();
            }
        }

        // In act mode with acting on, the bridge's model plays the counter (P2a) while it is thinking or has a
        // move pending; the autopilot waits for it and plays its own pick only after a defer.
        public bool CounterLeftToBridge()
        {
            if (session.Mode != Modes.Act || !ActSettingOn() || actSwitch.Paused || actSwitch.Faulted) return false;
            return tracker.PendingAct != null || tracker.InFlight;
        }

        void TogglePause()
        {
            if (AutopilotStatus?.Invoke().pause != null && AutopilotToggle != null)
            {
                // One brake for both: a paused autopilot pauses the bridge's counter moves too.
                if (AutopilotToggle("overlay")) actSwitch.Resume();
                else actSwitch.Pause();
                return;
            }
            if (actSwitch.Faulted) return;
            if (actSwitch.Paused)
            {
                actSwitch.Resume();
                log.Info("acting resumed by the player");
            }
            else
            {
                actSwitch.Pause();
                var pending = tracker.PendingAct;
                if (pending != null)
                {
                    pending.Act = ActState.Blocked;
                    pending.ActNote = "paused";
                }
                log.Info("acting paused by the player");
            }
        }

        void AskAgain()
        {
            if (tracker.AskAgain())
            {
                ForgetStage();
                log.Info("asking again for " + tracker.CurrentSignature);
            }
            summaryPoller.Want();
        }

        ActDisplay ActNow()
        {
            if (session.Mode != Modes.Act) return ActDisplay.Advise;
            if (!ActSettingOn()) return ActDisplay.ConfigOff;
            if (actSwitch.Faulted) return ActDisplay.Faulted;
            return actSwitch.Paused ? ActDisplay.Paused : ActDisplay.Active;
        }

        // ---- overlay ----

        void ShowOverlay(long now)
        {
            if (!overlay.Enabled) return;
            var snap = lastSnap;
            bool hasClient = snap?.Client != null && ReferenceEquals(snap.Client.Key, tracker.ClientKey);
            (string line, bool alert, string pause) auto = (null, false, null);
            try
            {
                if (AutopilotStatus != null) auto = AutopilotStatus();
            }
            catch (Exception)
            {
            }
            var view = OverlayText.Build(new OverlayInput
            {
                Mode = session.Mode,
                Act = ActNow(),
                ActFault = actSwitch.FaultReason,
                Bridge = OverlayText.BridgeOf(session, now),
                Client = hasClient ? snap.Client : null,
                CoverageGap = hasClient ? snap.CoverageGap : null,
                Ready = hasClient && snap.Ready,
                View = hasClient ? tracker.View(canAsk && covered) : null,
                Today = today.Entries,
                Summary = summary,
                SummaryFailed = summaryPoller.LastFailed,
                Recording = RecordingStatus.Of(session, now),
                BeforeOpen = beforeOpen,
                Inspect = InspectLineNow(),
                Shortlist = hasClient ? stageTop : null,
                ShortlistBudgetLeft = stageBudget,
                Autopilot = auto.line,
                AutopilotAlert = auto.alert,
                AutopilotPause = auto.pause,
            });
            overlay.Update(view, now);
        }

        string InspectLineNow()
        {
            try
            {
                return InspectLine?.Invoke() ?? "";
            }
            catch (Exception)
            {
                return "";
            }
        }

        // A result line for today's log from another advisor (inspect).
        public void NoteOutcome(string text)
        {
            today.Note(text);
            summaryPoller.Want();
        }

        public void OnGameEvent(string kind)
        {
            if (kind == "waking_up" || kind == "new_game_started") beforeOpen = true;
            else if (kind == "shutter_opened" || kind == "shutter_closed") beforeOpen = false;
            else if (kind == "game_loaded") beforeOpen = GameReflect.StoreMorning();
            if (kind == "waking_up" || kind == "shutter_opened" || kind == "game_loaded")
            {
                summaryPoller.Want();
                try
                {
                    today.SetDay(game.Store().Day);
                }
                catch (Exception)
                {
                }
            }
        }

        void StartClient(ClientInfo info, bool arrived)
        {
            FlushTrades();
            tracker.NewClient(info?.Key);
            ForgetStage();
            limiter.NewClient();
            attribution.NewClient();
            summaryPoller.Want();
            if (info != null)
            {
                try
                {
                    today.SetDay(game.Store().Day);
                }
                catch (Exception)
                {
                }
                today.Arrive(info.Name);
            }
            game.ForgetLocalIds();
            walkedOut = false;
            playerDismissing = false;
            leftReported = false;
            if (info != null && observer.Arrived(tracker.ClientSeq, CustomerKinds.Of(info))) Observe("arrival", null);
            if (arrived && info != null)
            {
                emitEvent("client_arrived", new JObject
                {
                    ["name"] = info.Name,
                    ["faction"] = Factions.Visible(info),
                    ["intent"] = IntentName(info),
                });
            }
        }

        static string IntentName(ClientInfo c)
        {
            switch (c.Intent)
            {
                case ClientIntent.Buy: return "buy";
                case ClientIntent.Sell: return "sell";
                case ClientIntent.SellAndBuy: return "sell_and_buy";
                default: return (c.RawIntent ?? "other").ToLowerInvariant();
            }
        }

        void OnDecided(object key, string signature, DecideRequest request, DecisionOutcome outcome)
        {
            if (!ReferenceEquals(key, tracker.ClientKey)) return;
            var advice = new Advice
            {
                DecisionId = request.DecisionId,
                Kind = request.Kind,
                Signature = signature,
                Deferred = outcome.Choice == null || outcome.Choice.IsDefer,
                OptionId = outcome.Choice?.Option,
                Rationale = outcome.Response?.Rationale,
                ReceivedMs = Now(),
                Options = request.Options,
                Notes = outcome.Response?.OptionNotes,
            };
            advice.Label = request.Options.FirstOrDefault(o => o.Id == advice.OptionId)?.Label;
            if (outcome.Execute && !advice.Deferred && request.Kind == BarterContext.Kind)
            {
                if (!ActSettingOn()) Once("act-config", "bridge is in act mode but the act_enabled setting is off; advising only");
                else if (ActGate.Allows(session.Mode, true, actSwitch) && ActCommands.Parse(advice.OptionId) != null) advice.Act = ActState.Pending;
            }
            tracker.Answered(advice);
            if (advice.Deferred)
                log.Info("advice " + request.DecisionId + " [" + signature + "]: defer (" + outcome.Reason + ")");
            else
                log.Info("advice " + request.DecisionId + " [" + signature + "]: " + advice.OptionId + " — " + advice.Label
                         + (string.IsNullOrEmpty(advice.Rationale) ? "" : " | " + FeatureText.Truncate(advice.Rationale, 300)));
        }

        // ---- recording ----

        public AdviceInForce InForce()
        {
            bool present = false;
            try
            {
                present = CurrentIsTracked();
            }
            catch (Exception)
            {
            }
            if (!present) return new AdviceInForce();
            return new AdviceInForce { ClientSeq = tracker.ClientSeq, DecisionId = tracker.LastDecisionId, Advised = tracker.AnswerTo(tracker.LastDecisionId) };
        }

        // The game's own readiness predicate, for covered and uncovered customers alike.
        bool CounterReady(BarterSnapshot snap)
        {
            if (snap.CoverageGap == null) return snap.Ready;
            try
            {
                return BarterRules.DecisionReady(game.Ui(), snap.Client, game.CounterItems() ?? new List<ItemInfo>());
            }
            catch (Exception)
            {
                return false;
            }
        }

        // customer_observed (docs/PROTOCOL.md, Recording events): every customer once, covered or not.
        void Observe(string at, BarterSnapshot snap)
        {
            if (recorder == null || !hookUsable("barter.session_start")) return;
            recorder.Guard("customer_observed", () =>
            {
                var info = snap?.Client ?? game.Client();
                if (info == null) return;
                string gap = BarterRules.CoverageGap(info);
                JObject context = null, hidden = null;
                if (snap != null && snap.Ready && snap.Context != null)
                {
                    context = snap.Context;
                    hidden = snap.Hidden;
                }
                else
                {
                    try
                    {
                        var items = game.CounterItems() ?? new List<ItemInfo>();
                        context = BarterContext.Context(game.Store() ?? new StoreInfo(), game.Reputations(), info, items, game.Bargain() ?? new BargainFlags(), game.MarketEvents());
                        hidden = BarterContext.Hidden(info, items, game.DaysToInspection());
                    }
                    catch (Exception)
                    {
                    }
                }
                var (data, hide) = CustomerObservation.Build(tracker.ClientSeq, info, CustomerKinds.Of(info), at, gap, context,
                    snap != null && snap.Ready ? snap.Options : null, recorder.ClientExtras(info.Key), hidden);
                emitRecord(RecordKinds.CustomerObserved, data, hide);
            });
        }

        // ---- hooks ----

        public void OnClientArrived()
        {
            ClientInfo info = null;
            try
            {
                info = game.Client();
            }
            catch (Exception e)
            {
                Once("arrived", "cannot read the arriving customer: " + e.Message);
            }
            if (info != null && !ReferenceEquals(info.Key, tracker.ClientKey)) StartClient(info, true);
            EnsurePolling();
        }

        bool IsTrackedClient(object c) => c != null && ReferenceEquals(c, tracker.ClientKey);

        bool CurrentIsTracked() => IsTrackedClient(GameBarter.CurrentClient());

        public sealed class BargainProbe
        {
            public BargainFlags Flags;
            public ClientIntent Intent;
            public IList<FactionRep> Reps;
        }

        public BargainProbe BeforeBargain()
        {
            var c = GameBarter.CurrentClient();
            if (!IsTrackedClient(c)) return null;
            return new BargainProbe { Flags = game.Bargain(), Intent = GameBarter.MapIntent(c.clientIntent), Reps = game.Reputations() };
        }

        public void AfterBargain(BargainKind kind, int percent, BargainProbe before)
        {
            if (before == null) return;
            var c = GameBarter.CurrentClient();
            if (!IsTrackedClient(c)) return;
            var flags = game.Bargain();
            if (before.Flags.Offered(kind) || !flags.Offered(kind)) return;
            bool? applied = Applied(kind);
            var result = Outcomes.Bargain(kind, percent, before.Flags, flags, applied, before.Intent, GameBarter.MapIntent(c.clientIntent),
                Outcomes.RepDelta(before.Reps, game.Reputations()));
            if (result == null) return;
            if (result.Value<bool>("walked_out")) walkedOut = true;
            Send(result, attribution.Direct(tracker.LastDecisionId));
        }

        // The roll result lives only in BargainUIManager's private *Applied fields.
        bool? Applied(BargainKind kind)
        {
            try
            {
                switch (kind)
                {
                    case BargainKind.Markup: return GameReflect.MarkupApplied();
                    case BargainKind.BuyingDiscount: return GameReflect.BuyingDiscountApplied();
                    case BargainKind.Blackmail: return GameReflect.BlackmailApplied();
                    case BargainKind.Threaten: return GameReflect.ThreatenApplied();
                    default: return true;
                }
            }
            catch (Exception e)
            {
                Once("applied", "cannot read the bargain roll result; outcomes omit `accepted`: " + e.Message);
                return null;
            }
        }

        public void BeforeTrade(string side, object storeClient)
        {
            if (!IsTrackedClient(storeClient ?? GameBarter.CurrentClient())) return;
            if (!trades.NeedsSnapshot(side)) return;
            FlushTrades();
            var who = attribution.Direct(tracker.LastDecisionId);
            trades.Begin(side, game.Reputations(), who.decisionId, who.actor);
        }

        public void AfterTrade(string side, object storeClient, GameItem item, int amount)
        {
            Traded?.Invoke(side == TradeBatch.Sold, item, amount);
            if (!IsTrackedClient(storeClient ?? GameBarter.CurrentClient()) || trades.Side != side || trades.RepBefore == null) return;
            trades.Add(game.Uid(item), amount);
            lastTradeMs = Now();
        }

        void FlushTrades()
        {
            if (trades.IsEmpty) return;
            IList<FactionRep> after = null;
            try
            {
                after = game.Reputations();
            }
            catch (Exception)
            {
            }
            var who = (trades.DecisionId, trades.Actor);
            var result = trades.Flush(after);
            if (result != null) Send(result, who);
        }

        public void PlayerDismissing(bool on) => playerDismissing = on;

        public void BeforeDismiss()
        {
            if (!CurrentIsTracked() || leftReported) return;
            FlushTrades();
            leftReported = true;
            if (observer.Leaving(tracker.ClientSeq)) Observe("leaving", null);
            string cause = Outcomes.LeftCause(walkedOut, GameReflect.ShutterClosed(), playerDismissing);
            var who = cause == LeftCauses.Dismissed ? attribution.Direct(tracker.LastDecisionId)
                : cause == LeftCauses.Shutter ? (tracker.LastDecisionId, Actors.Human)
                : attribution.Consequence(tracker.LastDecisionId);
            Send(Outcomes.Left(cause), who);
        }

        void Send(JObject result, (string decisionId, string actor) who)
        {
            Outcomes.WithActor(result, who.actor);
            today.Note(OverlayText.Describe(result));
            summaryPoller.Want();
            string decisionId = who.decisionId;
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

        // ---- display ----

        // A minimal cue on counter items; the overlay carries the detail.
        public void AppendTooltip(RichTextBuilder builder, GameItem item)
        {
            if (!DisplayEnabled || !AdviseEnabled || item == null || tracker.ClientKey == null) return;
            if (!CurrentIsTracked() || !GameReflect.OnCounter(item)) return;
            var act = ActNow();
            if (act == ActDisplay.Paused || act == ActDisplay.Faulted) GameReflect.TooltipLine(builder, "fence: act paused", true);
            if (stageTop != null && stageTop.Count > 0)
            {
                GameReflect.TooltipLine(builder, "fence suggests offer: " + StageShortlist.Line(stageTop[0], stageBudget), false);
                return;
            }
            var view = tracker.View(canAsk && covered);
            switch (view.Status)
            {
                case AdviceStatus.Ready:
                    string verb = OverlayText.IsStageOffer(view.Advice) ? "fence suggests offer: " : TooltipVerb(view.Advice);
                    GameReflect.TooltipLine(builder, verb + OverlayText.Clean(view.Advice.Label ?? view.Advice.OptionId, 90), false);
                    break;
                case AdviceStatus.Thinking:
                    GameReflect.TooltipLine(builder, "fence: thinking…", true);
                    break;
                case AdviceStatus.Stale:
                    GameReflect.TooltipLine(builder, view.Advice.Act == ActState.Executed ? "fence did: " + OverlayText.Clean(view.Advice.Label, 90) : "fence: stale — state changed", true);
                    break;
            }
        }

        static string TooltipVerb(Advice a)
        {
            switch (a.Act)
            {
                case ActState.Pending: return "fence will: ";
                case ActState.Executed: return "fence did: ";
                case ActState.Stale: return "fence (stale, dropped): ";
                default: return "fence suggests: ";
            }
        }

        void Once(string key, string message)
        {
            if (loggedOnce.Add(key)) log.Warn(message);
        }
    }

    // Small game touches used by the advisor's hooks; NoInlining for the same reason as GameAccess.
    static class GameReflect
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool MarkupApplied() => (bool)ModReflect.Get(BargainUIManager.Instance, "markupApplied");

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool BuyingDiscountApplied() => (bool)ModReflect.Get(BargainUIManager.Instance, "buyingDiscountApplied");

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool BlackmailApplied() => (bool)ModReflect.Get(BargainUIManager.Instance, "blackmailApplied");

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool ThreatenApplied() => (bool)ModReflect.Get(BargainUIManager.Instance, "threatenApplied");

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool StoreMorning()
        {
            try
            {
                return PlayerStore.Instance != null && PlayerStore.Instance.storeState == PlayerStore.StoreState.MORNING;
            }
            catch (Exception)
            {
                return false;
            }
        }

        // EndDay closes the store before the shutter's delayed dismiss runs.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool ShutterClosed()
        {
            try
            {
                return PlayerStore.Instance != null && PlayerStore.Instance.storeState == PlayerStore.StoreState.CLOSED;
            }
            catch (Exception)
            {
                return false;
            }
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool OnCounter(GameItem item)
        {
            var entry = EmporiumEntry.Instance;
            if (entry == null) return false;
            var parent = item.parentInventory;
            return parent != null && (parent == entry.backInvinvElement || parent == entry.frontInvinvElement);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void TooltipLine(RichTextBuilder builder, string text, bool quiet)
        {
            builder.AddLine(text, true, quiet ? RenderHandler.ColorPalette.Gray : RenderHandler.ColorPalette.LightCyan, false, quiet);
        }
    }
}
