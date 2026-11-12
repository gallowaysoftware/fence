using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using Fence.Autopilot;
using Fence.Barter;
using Fence.Decisions;
using Fence.Gym;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;
using UnityEngine;

namespace Fence.Mod
{
    // The live driver (docs/STREAM.md section 4): a coroutine that, while running and while the game holds
    // still, takes one action per tick by the gym executor's rules, re-reads the game to check it took, and
    // records it with its reason. Anything it cannot handle stops it and hands the game to the human.
    sealed partial class AutopilotDriver
    {
        const float TickSeconds = 0.25f;
        const int WindowCapacity = 35;
        const long ExpectMs = 5000;
        const long LeaveMs = 15000;
        const long SleepMs = 90000;
        // The hard incumbent (sim/lab/scripts/directives/hard_income_cash_sprint.json), shooting augs on visible tells.
        public const string DefaultDirectives = "{\"augs\":\"shoot\",\"bargain\":\"discount\",\"bargain_buy\":\"ev_positive\",\"bays\":2,\"buy\":{\"single_max_pct\":75,\"wholesale_max_pct\":88},\"conceal\":{\"policy\":\"always\"},\"engine\":\"wine+furnace\",\"jackson_every\":14,\"permit\":\"buy\",\"sell\":{\"liquidate_min_pct\":78,\"min_pct\":57},\"upgrades_cash_max\":937,\"window_mode\":\"concentrate\",\"mortgage_margin\":0.8,\"appraisal\":\"join\",\"buyout_by_day\":140,\"buyout_save\":[\"insurance\"],\"buyout_save_from\":56}";
        // What the live hands cannot do yet, over any plan: the poster stash (M3), and the labeler, which the gym
        // rewrites without its window.
        public const string LiveOnlyDirectives = "{\"conceal\":{\"use_poster\":false},\"labeler\":null,\"fakes\":null}";

        // Customers whose dialogue choices the executor's role decides by a rule of its own; anyone else's
        // choice falls to "first enabled", which the autopilot hands to the human instead.
        const string NoRule = "dialogue:first_enabled";
        static readonly HashSet<string> BigDecisions = new HashSet<string>(StringComparer.Ordinal)
        {
            "landlord", "landlordVending", "loan_shark2", "loan_shark_collect", "georgioLoan", "businessPermitOfficial",
        };

        readonly FenceLog log;
        readonly Func<IEnumerator, bool> startCoroutine;
        readonly Recorder.Emit emit;
        readonly Recorder recorder;
        readonly BarterAdvisor advisor;
        readonly Func<bool> setting;
        readonly Func<string> paceSetting;
        readonly Func<string> holdSetting;
        readonly Func<string> directivesSetting;
        readonly AutopilotSwitch sw = new AutopilotSwitch();
        readonly AutopilotMemory memory = new AutopilotMemory();
        readonly Pace pace = new Pace();
        readonly Watchdog watchdog = new Watchdog();
        readonly GameBarter game = new GameBarter();
        readonly LiveHands hands = new LiveHands();
        readonly Defaults defaults = new Defaults();
        readonly LiveHost host;
        readonly PlayerRole role;
        readonly string launchTag = Guid.NewGuid().ToString("N").Substring(0, 6);
        Expectation expecting;
        LiveView last;
        long nextActionMs;
        string heldFor;
        long heldSinceMs;
        int planDay = -1;
        int actions;
        string lastLine = "";
        string lastAction;
        string lastDefault;
        string unavailable;
        bool polling;
        bool stopped;
        bool executing;

        public AutopilotDriver(FenceLog log, Func<IEnumerator, bool> startCoroutine, Recorder.Emit emit, Recorder recorder, BarterAdvisor advisor,
            Func<bool> setting, Func<string> paceSetting, Func<string> holdSetting, Func<string> directivesSetting)
        {
            this.log = log;
            this.startCoroutine = startCoroutine;
            this.emit = emit;
            this.recorder = recorder;
            this.advisor = advisor;
            this.setting = setting;
            this.paceSetting = paceSetting;
            this.holdSetting = holdSetting;
            this.directivesSetting = directivesSetting;
            host = new LiveHost(game, emit, why => Stop(AutopilotState.Faulted, why), call => memory.Calls.Enqueue(call), MachineButton);
            Lab.Thermostat = true;
            defaults.Log = (key, detail) => lastDefault = key;
            role = new PlayerRole(defaults, new Knobs(""));
            Chore.Hands = new ChoreHands(this);
            advisor.Traded = Traded;
        }

        public AutopilotState State => sw.State;
        public bool Executing => executing;

        // A compat feature the autopilot needs is missing: it stays off and says why.
        public void Unavailable(string why) => unavailable = why;

        public void Stop()
        {
            stopped = true;
        }

        public void EnsurePolling()
        {
            if (polling || stopped) return;
            polling = startCoroutine(Poll());
        }

        IEnumerator Poll()
        {
            try
            {
                while (!stopped)
                {
                    try
                    {
                        Tick(BridgeClient.NowMs());
                    }
                    catch (Exception e)
                    {
                        log.Error("autopilot tick failed: " + e);
                        Stop(AutopilotState.Faulted, e.GetType().Name + ": " + e.Message);
                    }
                    yield return new WaitForSecondsRealtime(TickSeconds);
                }
            }
            finally
            {
                polling = false;
            }
        }

        // ---- the switch ----

        public bool Enabled
        {
            get
            {
                try
                {
                    return unavailable == null && setting();
                }
                catch (Exception)
                {
                    return false;
                }
            }
        }

        // The overlay's Pause/Resume button and the key bind.
        public bool Toggle(string by)
        {
            if (!Enabled) return false;
            if (sw.Running) Pause("paused by the player (" + by + ")");
            else Resume(by);
            return sw.Running;
        }

        void Resume(string by)
        {
            if (!sw.Resume()) return;
            expecting = null;
            heldFor = null;
            nextActionMs = 0;
            watchdog.Reset(BridgeClient.NowMs());
            Record("resume", null, "resumed by the player (" + by + ")", null);
            log.Info("autopilot resumed (" + by + ")");
        }

        void Pause(string why)
        {
            if (!sw.Pause(why)) return;
            Record("pause", null, why, null);
            log.Info("autopilot " + why);
        }

        // Recorder.ClaimHuman: the services recorded at sleep are the autopilot's (the game records them after
        // its press, in the blackscreen); any other move while it runs is the player's, and pauses it. Opening
        // or closing the counter tablet changes nothing the autopilot relies on (and the game closes it too).
        public string ClaimHuman(string action)
        {
            if (action == HumanActions.ServicesSet && memory.SleepRequested) return Actors.Autopilot;
            if (action == HumanActions.OpenTablet || action == HumanActions.CloseTablet) return null;
            if (sw.Running && !executing) Pause("you made a move (" + action + "): the autopilot stopped so it cannot fight you");
            return null;
        }

        void Stop(AutopilotState to, string why)
        {
            bool stoppedNow = to == AutopilotState.Handover ? sw.Handover(why) : sw.Fault(why);
            if (!stoppedNow) return;
            var v = last;
            emit(RecordKinds.AutopilotFault, AutopilotEvents.BuildFault(v?.Day ?? GameAccess.DayCounter(), AutopilotEvents.PhaseOf(v), to, why, lastAction, v?.ClientName, v?.ClientKind), null);
            if (to == AutopilotState.Faulted) log.Error("autopilot stopped: " + why);
            else log.Warn("autopilot handed over: " + why);
        }

        // The overlay's line, whether it is an alarm, and the pause button's label (null: the setting is off).
        public (string line, bool alert, string pause) Status()
        {
            if (unavailable != null) return ("Autopilot unavailable: " + unavailable, true, null);
            if (!Enabled) return ("", false, null);
            switch (sw.State)
            {
                case AutopilotState.Running: return ("Autopilot ▶ " + (lastLine.Length > 0 ? lastLine : "starting"), false, "Pause autopilot");
                case AutopilotState.Paused: return (AutopilotEvents.Banner(sw.State, sw.Reason) + " — press Resume autopilot", false, "Resume autopilot");
                default: return (AutopilotEvents.Banner(sw.State, sw.Reason), true, "Resume autopilot");
            }
        }

        // ---- the tick ----

        void Tick(long now)
        {
            if (!Enabled)
            {
                if (sw.State != AutopilotState.Off) sw.SetEnabled(false);
                return;
            }
            sw.SetEnabled(true);
            if (!sw.Running) return;
            pace.BaseMs = Pace.Parse(Safe(paceSetting), Pace.DefaultMs);
            pace.HoldMs = Pace.Parse(Safe(holdSetting), Pace.DefaultHoldMs);

            var (view, snap) = LiveGame.Read(game, memory.ClientSeq, recorder != null && recorder.Filter.Loading);
            if (!GunOrdersUsable) view.ArmsOrders = false;
            last = view;
            if (!(memory.GunDecided && ReferenceEquals(memory.ClientKey, view.ClientKey))) ReadAugVerdict(view, snap);
            CheckShot(view);
            if (!sw.Running) return;
            TickThermostat(view, now);
            if (!sw.Running) return;
            long quiet = watchdog.Quiet(view.Signature(), now);
            if (expecting != null)
            {
                if (expecting.Met(view)) expecting = null;
                else if (expecting.Missed(now)) Stop(AutopilotState.Faulted, "did not take: " + expecting.What);
                return;
            }
            var step = Planner.Next(view, memory, quiet);
            switch (step.Kind)
            {
                case StepKind.Idle:
                    return;
                case StepKind.Wait:
                    if (watchdog.Stalled(quiet)) Stop(AutopilotState.Faulted, "no progress for " + Watchdog.LimitMs / 1000 + " s (" + step.Reason + ")");
                    return;
                case StepKind.Handover:
                    Stop(AutopilotState.Handover, step.Reason);
                    return;
                case StepKind.Fault:
                    Stop(AutopilotState.Faulted, step.Reason);
                    return;
                case StepKind.Counter when advisor.CounterLeftToBridge():
                    if (watchdog.Stalled(quiet)) Stop(AutopilotState.Faulted, "the bridge's counter move never came");
                    return;
            }
            if (now < nextActionMs || !Held(step, view, now)) return;
            var gate = ActGate.TryOpenAutopilot(() => Enabled && sw.Running);
            if (gate == null) return;
            if (planDay != view.Day) Morning(view.Day);
            if (!sw.Running) return;
            executing = true;
            actGate = gate;
            recorder?.Filter.EnterExecutor();
            try
            {
                Execute(step, view, snap, gate, now);
            }
            finally
            {
                recorder?.Filter.ExitExecutor();
                actGate = null;
                executing = false;
            }
            actions++;
            watchdog.Reset(now);
            nextActionMs = now + pace.DelayMs(step.Kind == StepKind.Move ? PaceKinds.Move : PaceKinds.Decision);
        }

        // A hold before the step: the morning report and the big dialogue decisions stay on screen for the
        // panel and TTS, and a dialogue line or choice for as long as its text takes to read.
        bool Held(Step step, LiveView v, long now)
        {
            string kind = Pace.KindOf(step.Kind);
            if (step.Kind == StepKind.Choose && BigDecisions.Contains(v.ClientId ?? "")) kind = PaceKinds.Hold;
            int ms = kind == PaceKinds.Hold || kind == PaceKinds.Dialogue ? pace.DelayMs(kind, v.LineChars) : 0;
            if (ms == 0) return true;
            string key = step.Kind + "|" + v.Signature();
            if (heldFor != key)
            {
                heldFor = key;
                heldSinceMs = now;
            }
            return now - heldSinceMs >= ms;
        }

        void Execute(Step step, LiveView v, BarterSnapshot snap, ActGate gate, long now)
        {
            lastDefault = null;
            switch (step.Kind)
            {
                case StepKind.ClosePanel:
                    hands.ClosePanel(gate);
                    Record("close_morning_report", null, "the night's report read; the day starts" + PlanLine(), null);
                    expecting = Expectation.Within("the morning report closes", now, ExpectMs, x => !x.MorningPanelOpen);
                    return;
                case StepKind.PlanWindow:
                    PlanWindow();
                    return;
                case StepKind.Move:
                    MoveOne(gate, memory.Moves.Dequeue(), v);
                    return;
                case StepKind.OpenShutter:
                    hands.Shutter(gate);
                    Record("open_shutter", null, "the window is set; open for business", null);
                    expecting = Expectation.Within("the shutter opens", now, ExpectMs, x => x.Store == StorePhase.Open);
                    return;
                case StepKind.RingBell:
                    if (!hands.RingBell(gate))
                    {
                        Stop(AutopilotState.Faulted, "the bell is not in the scene");
                        return;
                    }
                    host.Human(HumanActions.RingBell, null, null, null, "autopilot:bell");
                    Record("ring_bell", null, v.Queue + " customer" + (v.Queue == 1 ? "" : "s") + " waiting", null);
                    expecting = Expectation.Within("the next customer comes in", now, ExpectMs, x => x.ClientPresent || x.Queue < v.Queue);
                    return;
                case StepKind.Advance:
                {
                    var line = LiveGame.CurrentLine();
                    hands.Advance(gate);
                    expecting = Expectation.Within("the dialogue line advances", now, ExpectMs, x => !x.WaitingForAdvance || !ReferenceEquals(LiveGame.CurrentLine(), line));
                    return;
                }
                case StepKind.Choose:
                    Choose(gate, v, now);
                    return;
                case StepKind.PlanStageBuyer:
                    PlanStageBuyer(v);
                    return;
                case StepKind.PlanStageSeller:
                    PlanStageSeller(v);
                    return;
                case StepKind.Counter:
                    Counter(gate, v, snap, now);
                    return;
                case StepKind.Dismiss:
                    Dismiss(v, step.Reason, "uncovered", now);
                    return;
                case StepKind.PlanUnstage:
                    PlanUnstage();
                    return;
                case StepKind.CloseShutter:
                    hands.Shutter(gate);
                    Record("close_shutter", null, "no one left in the queue", null);
                    expecting = Expectation.Within("the shutter closes", now, ExpectMs, x => x.Store == StorePhase.Closed);
                    return;
                case StepKind.OpenTablet:
                    if (!hands.OpenTablet(gate))
                    {
                        Stop(AutopilotState.Faulted, "the bedroll is not in the scene");
                        return;
                    }
                    Record("open_tablet", null, "the night's errands done; services, then sleep", null);
                    expecting = Expectation.Within("the end-of-day tablet opens", now, ExpectMs, x => x.TabletOpen);
                    return;
                case StepKind.SetServices:
                    SetServices(gate, v);
                    return;
                case StepKind.Sleep:
                    if (!hands.Sleep(gate))
                    {
                        Stop(AutopilotState.Handover, "tonight's services cost more than the cash");
                        return;
                    }
                    memory.SleepRequested = true;
                    host.Human(HumanActions.Sleep, null, null, null, "autopilot:sleep");
                    Record("sleep", null, "end of day " + v.Day, null);
                    int day = v.Day;
                    expecting = Expectation.Within("the next morning", now, SleepMs, x => x.InStore && x.Day != day && x.Store == StorePhase.Morning);
                    return;
                default:
                    ExecuteM2(step, v, gate, now);
                    return;
            }
        }

        // ---- the day's plan ----

        // The executor's plan for the day: directives (the setting's base, the strategist's day plan over it, what the
        // live hands cannot do yet over both) and the shared morning plan (ExecutorMorning.Plan) on the ledger the
        // game's run stats and this session's sales give.
        void Morning(int day)
        {
            planDay = day;
            Directives baseline;
            try
            {
                baseline = Directives.Parse(JObject.Parse(string.IsNullOrWhiteSpace(Safe(directivesSetting)) ? DefaultDirectives : Safe(directivesSetting)));
            }
            catch (Exception e)
            {
                Stop(AutopilotState.Faulted, "autopilot_directives is not valid JSON directives: " + e.Message);
                return;
            }
            Directives plan = null;
            var summary = advisor.LatestSummary;
            if (summary?.Directives != null && (summary.PlanDay ?? day) == day)
            {
                try
                {
                    plan = Directives.Parse(summary.Directives);
                }
                catch (Exception e)
                {
                    log.Warn("autopilot: the strategist's directives do not parse, playing the base: " + e.Message);
                }
            }
            Directives.Current = Directives.Merge(Directives.Merge(baseline, plan), Directives.Parse(JObject.Parse(LiveOnlyDirectives)));
            var d = Directives.Current;
            var days = LiveGame.Days(soldByDay);
            var p = ExecutorMorning.Plan(host, days);
            PlayerDay.Current = p;
            Record("plan_day", null, "rent " + p.RentDue + " in " + p.RentDaysLeft + " days; floor " + p.Floor + ", cash target " + p.CashTarget
                + (p.SavingForBuyout ? "; saving for the buyout" : "") + (p.NetGainWeek is int g ? "; net gain " + g + " a week" : "; " + days.Count + " days of history")
                + (plan != null ? "; the strategist's plan for today" : "; base directives"), null, detail: d.ToString());
        }

        string PlanLine()
        {
            var p = PlayerDay.Current;
            return p == null ? "" : " (floor " + p.Floor + ", cash target " + p.CashTarget + ")";
        }

        // ---- batches ----

        void PlanWindow()
        {
            memory.WindowDone = true;
            var owned = LiveGame.StockAndCase();
            var want = new HashSet<GameItem>(role.Window(owned, WindowCapacity));
            string rule = lastDefault;
            var hidden = new[] { "item_true_condition" };
            int outs = 0, ins = 0;
            foreach (var i in LiveGame.InCase().Where(i => !want.Contains(i)))
            {
                Queue(MoveKinds.Unshowcase, i, null, "not in today's window", rule, hidden);
                outs++;
            }
            foreach (var i in want.Where(i => !LiveGame.IsIn(i, LiveHands.Case)))
            {
                Queue(MoveKinds.Showcase, i, null, "today's window: the best of its type, nothing hot or hiding a flaw", rule, hidden);
                ins++;
            }
            Record("plan_window", null, ins + " into the case, " + outs + " out", rule, hidden);
        }

        void PlanStageBuyer(LiveView v)
        {
            memory.StagedBuyer = true;
            var client = LiveGame.Client();
            var picks = role.StageForBuyer(client, LiveGame.StockAndCase());
            string rule = lastDefault;
            // Keep() reads what a buyer could expose and a water container's true grade.
            var hidden = new[] { "item_exposable", "water_purity" };
            foreach (var i in picks) Queue(MoveKinds.Stage, i, v.ClientKey, "what " + (v.ClientName ?? "the buyer") + " wants, within the budget", rule, hidden);
            Record("plan_stage", null, picks.Count + " item" + (picks.Count == 1 ? "" : "s") + " to offer", rule, hidden, v);
        }

        void PlanStageSeller(LiveView v)
        {
            memory.StagedSeller = true;
            var theirs = LiveGame.TheirsOnCounter();
            var picks = role.StageFromSeller(LiveGame.Client(), theirs, LiveGame.Cash());
            string rule = lastDefault;
            foreach (var i in picks) Queue(MoveKinds.StageTheirs, i, v.ClientKey, "worth weighing a price for", rule, null);
            Record("plan_stage_seller", null, picks.Count + " of their " + theirs.Count + " items to weigh", rule, null, v);
        }

        void PlanUnstage()
        {
            memory.UnstagePending = false;
            RunChore("stow", Production.Stow);
            foreach (var k in Appraisal.KitOnCounter()) Queue(MoveKinds.StowKit, k, null, "the appraisal kit into the dossier, off the counter the night wipes", "player:appraisal_kit", null);
            var left = LiveGame.OwnedOnBack();
            // Gifts dropped on the front counter as already ours: the free ore courier and the miner's dividend the furnace uses, and the phone suppliers' free deliveries and dividends.
            if (Furnace.On || Suppliers.On) left.AddRange(LiveGame.OwnedOnFront().Where(i => (Furnace.On && Furnace.IsInput(i.identifier)) || Suppliers.IsGift(i)));
            foreach (var i in left) Queue(MoveKinds.Unstage, i, null, "unsold, back to stock", "unstage_leftovers", null);
            if (left.Count > 0) Record("plan_unstage", null, left.Count + " unsold item" + (left.Count == 1 ? "" : "s") + " back to stock", "unstage_leftovers", null);
        }

        void Queue(string kind, GameItem item, object clientKey, string why, string rule, IList<string> hidden) =>
            memory.Moves.Enqueue(new PlannedMove { Kind = kind, Item = item, Uid = game.Uid(item), Name = LiveGame.Name(item), Why = why, Rule = rule, HiddenUsed = hidden, ClientKey = clientKey });

        void MoveOne(ActGate gate, PlannedMove m, LiveView v)
        {
            var item = (GameItem)m.Item;
            if (MoveSpecial(gate, m, item, v)) return;
            string to = m.Kind == MoveKinds.Showcase ? LiveHands.Case
                : m.Kind == MoveKinds.Stage || m.Kind == MoveKinds.StageTheirs || m.Kind == MoveKinds.Donate || m.Kind == MoveKinds.JacksonStage ? LiveHands.Back
                : LiveHands.Stock;
            if (!hands.Move(gate, item, to))
            {
                Record(m.Kind, m, "skipped: the game does not let it move now", m.Rule, null, v);
                return;
            }
            if (!LiveGame.IsIn(item, to))
            {
                Stop(AutopilotState.Faulted, "move did not take: " + m.Name + " → " + to);
                return;
            }
            string action = m.Kind == MoveKinds.StageTheirs || m.Kind == MoveKinds.JacksonStage || m.Kind == MoveKinds.Donate ? HumanActions.Stage : m.Kind == MoveKinds.CarryHome ? HumanActions.Unstage : m.Kind;
            var advice = v.ClientPresent && !v.Outside ? advisor.InForce() : null;
            var extra = new JObject { ["owned"] = m.Kind != MoveKinds.StageTheirs && m.Kind != MoveKinds.JacksonStage };
            if (m.Kind == MoveKinds.Donate) extra["donation"] = true;
            host.Human(action, advice, m.Uid, extra, m.Rule);
            Record(m.Kind, m, m.Why, m.Rule, m.HiddenUsed, v);
        }

        // ---- the customer ----

        void Choose(ActGate gate, LiveView v, long now)
        {
            var view = LiveGame.Dialogue(out var infos);
            int pick = role.Dialogue(view);
            string rule = lastDefault;
            if (rule == NoRule)
            {
                Stop(AutopilotState.Handover, (v.ClientName ?? "a customer") + " asks something the executor has no rule for");
                return;
            }
            var choice = view.Choices.FirstOrDefault(c => c.Index == pick);
            if (choice == null || !choice.Enabled)
            {
                Stop(AutopilotState.Faulted, "the rule picked a choice that is not on offer (" + pick + ")");
                return;
            }
            var line = LiveGame.CurrentLine();
            hands.Choose(gate, pick);
            Record("dialogue_choice", null, FeatureText.Plain(choice.Text), rule, null, v, detail: (view.ClientId ?? "") + " [" + pick + "]");
            expecting = Expectation.Within("the dialogue choice takes", now, ExpectMs, x => !ReferenceEquals(LiveGame.CurrentLine(), line));
        }

        void Counter(ActGate gate, LiveView v, BarterSnapshot snap, long now)
        {
            if (snap == null || !snap.Ready)
            {
                expecting = null;
                return;
            }
            string id = "a-" + launchTag + "-" + (++memory.CounterMoves) + "-" + actions;
            var request = BarterContext.Request(snap, id, DecisionDeadlines.ForMode(Modes.Observe));
            string option = PlayerPolicy.Pick(request, out string why);
            string rule = "player:counter";
            if (option != null && !request.Options.Any(o => o.Id == option)) option = null;
            if (option == null)
            {
                option = role.CounterFallback(snap);
                rule = lastDefault ?? "counter_fallback";
            }
            if (option == null)
            {
                if (v.ClientDismissable) Dismiss(v, why ?? "nothing to weigh", rule, now);
                else Stop(AutopilotState.Handover, "no counter move for " + (v.ClientName ?? "this customer") + " and it cannot be dismissed");
                return;
            }
            var cmd = ActCommands.Parse(option);
            if (cmd == null)
            {
                Stop(AutopilotState.Handover, "the counter move " + option + " is not one the autopilot presses");
                return;
            }
            string label = request.Options.FirstOrDefault(o => o.Id == option)?.Label;
            string before = v.CounterSignature;
            object client = v.ClientKey;
            if (!advisor.ExecuteForAutopilot(cmd, () => Enabled && sw.Running))
            {
                Stop(AutopilotState.Faulted, "the counter move " + option + " was refused");
                return;
            }
            lastLine = OverlayText.Clean(label ?? option, 80);
            Record("counter", null, why ?? option, rule, null, v, detail: option);
            expecting = cmd.Kind == ActKind.Dismiss
                ? Expectation.Within("the customer leaves", now, LeaveMs, x => !x.ClientPresent || !ReferenceEquals(x.ClientKey, client))
                : Expectation.Within("the counter move " + option + " lands", now, ExpectMs,
                    x => !x.ClientPresent || !ReferenceEquals(x.ClientKey, client) || x.DialogueActive || !x.CounterReady || x.CounterSignature != before);
        }

        // A gun-order client sent away may stay on as a melee buyer (StoreClientListGun.TryStayForMeleeWeapon): its
        // order closes and the counter serves it like any buyer.
        void Dismiss(LiveView v, string why, string rule, long now)
        {
            var cmd = ActCommands.Parse(OptionIds.Dismiss);
            object client = v.ClientKey;
            bool order = v.GunOrder;
            if (!advisor.ExecuteForAutopilot(cmd, () => Enabled && sw.Running))
            {
                Stop(AutopilotState.Faulted, "the dismiss was refused");
                return;
            }
            if (order) memory.GunOrderDone = true;
            else memory.Dismissed = true;
            host.Human(OptionIds.Dismiss, advisor.InForce(), null, null, rule);
            Record("dismiss", null, why, rule, null, v);
            expecting = order
                ? Expectation.Within("the order client leaves or stays to buy", now, LeaveMs, x => !x.ClientPresent || !ReferenceEquals(x.ClientKey, client) || !x.GunOrder)
                : Expectation.Within("the customer leaves", now, LeaveMs, x => !x.ClientPresent || !ReferenceEquals(x.ClientKey, client));
        }

        // ---- the night ----

        // The executor's services, then the tablet's own rules as the player meets them (Episode.Sleep): the
        // dearest optional service goes while the bill exceeds the cash.
        void SetServices(ActGate gate, LiveView v)
        {
            memory.ServicesSet = true;
            var night = LiveGame.Night(v.Day);
            var want = role.Services(night) ?? new HashSet<string>(night.Services.Where(s => s.Using).Select(s => s.Id));
            string rule = lastDefault;
            var optional = night.Services.Where(s => s.Unlocked && !s.Mandatory && s.Cooldown <= 0).ToList();
            int bill = night.Services.Where(s => s.Unlocked && (s.Mandatory || (want.Contains(s.Id) && s.Cooldown <= 0))).Sum(s => s.Cost);
            var dropped = new List<string>();
            foreach (var s in optional.Where(s => want.Contains(s.Id)).OrderByDescending(s => s.Cost).ThenBy(s => s.Id, StringComparer.Ordinal))
            {
                if (bill <= night.Cash) break;
                want.Remove(s.Id);
                bill -= s.Cost;
                dropped.Add(s.Id);
            }
            hands.SetServices(gate, want);
            Record("services", null, "tonight: " + (want.Count == 0 ? "none" : string.Join(", ", want.OrderBy(x => x, StringComparer.Ordinal)))
                + (dropped.Count > 0 ? "; dropped for cash: " + string.Join(", ", dropped) : "") + "; bill " + bill, rule, null);
        }

        // ---- records ----

        void Record(string action, PlannedMove m, string why, string rule, IList<string> hidden = null, LiveView v = null, string detail = null)
        {
            v = v ?? last;
            var a = new AutopilotAction
            {
                Day = v?.Day ?? GameAccess.DayCounter(),
                Phase = AutopilotEvents.PhaseOf(v),
                Action = action,
                TargetUid = m?.Uid,
                TargetName = m?.Name,
                Detail = detail,
                Why = why,
                Rule = rule,
                PlanDay = planDay >= 0 ? planDay : (int?)null,
                ClientSeq = v != null && v.ClientPresent ? memory.ClientSeq : 0,
                Client = v != null && v.ClientPresent ? v.ClientName : null,
                HiddenUsed = hidden,
            };
            emit(RecordKinds.Autopilot, AutopilotEvents.Build(a), null);
            if (action == "pause" || action == "resume") return;
            lastAction = action;
            lastLine = action.Replace('_', ' ') + (m != null ? " " + m.Name : "") + (string.IsNullOrEmpty(why) ? "" : ": " + why);
        }

        static string Safe(Func<string> f)
        {
            try
            {
                return f?.Invoke();
            }
            catch (Exception)
            {
                return null;
            }
        }
    }
}
