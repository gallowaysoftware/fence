using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Autopilot;
using Fence.Barter;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class AutopilotPlannerTests
    {
        const long Quiet = 10000;

        static LiveView Store(StorePhase phase, Action<LiveView> with = null)
        {
            var v = new LiveView { InStore = true, Day = 12, Store = phase, CanEndDay = true };
            with?.Invoke(v);
            return v;
        }

        static readonly object Mara = new object();

        static LiveView Customer(Action<LiveView> with = null) => Store(StorePhase.Open, v =>
        {
            v.ClientPresent = v.ClientArrived = true;
            v.ClientKey = Mara;
            v.ClientName = "Mara";
            v.ClientKind = "trade";
            v.ClientDismissable = true;
            v.Intent = ClientIntent.Buy;
            v.CounterReady = true;
            v.CounterSignature = "sig-1";
            with?.Invoke(v);
        });

        static StepKind Next(LiveView v, AutopilotMemory m = null, long quiet = Quiet) => Planner.Next(v, m ?? new AutopilotMemory { Day = v.Day, ChoresDone = true, NightPlanned = true, EveningDone = true }, quiet).Kind;

        public static IEnumerable<object[]> Cases() => new[]
        {
            new object[] { "menu", new LiveView(), StepKind.Idle },
            new object[] { "loading", Store(StorePhase.Morning, v => v.Loading = true), StepKind.Idle },
            new object[] { "game over", Store(StorePhase.Open, v => v.GameOver = true), StepKind.Handover },
            new object[] { "outside with no outing planned", Store(StorePhase.Closed, v => { v.Outside = true; v.OutsideAt = "dumping_ground"; }), StepKind.Handover },
            new object[] { "blackscreen", Store(StorePhase.Open, v => v.BlockedBy = "blackscreen"), StepKind.Wait },
            new object[] { "a call the autopilot did not make", Store(StorePhase.Closed, v => v.PhoneDialogue = true), StepKind.Handover },
            new object[] { "a phone the autopilot did not open", Store(StorePhase.Morning, v => v.PhoneOpen = true), StepKind.Handover },
            new object[] { "Wild's window", Customer(v => v.WildOpen = true), StepKind.WildBuy },
            new object[] { "appraisal item", Customer(v => { v.AppraisalClient = true; v.AppraisalItemReady = true; v.CoverageGap = "intent APPRAISAL_SERVICE"; v.CounterReady = false; }), StepKind.Inspect },
            new object[] { "appraisal done", Customer(v => { v.AppraisalClient = true; v.CoverageGap = "intent APPRAISAL_SERVICE"; v.CounterReady = false; }), StepKind.Dismiss },
            new object[] { "permit official's last line", Customer(v => { v.DialogueActive = v.ChoicesShown = v.BribeDue = true; }), StepKind.PlanBribe },
            new object[] { "a choices window by day", Store(StorePhase.Open, v => v.ResponseOpen = true), StepKind.Handover },
            new object[] { "morning report", Store(StorePhase.Morning, v => v.MorningPanelOpen = true), StepKind.ClosePanel },
            new object[] { "window first", Store(StorePhase.Morning), StepKind.PlanWindow },
            new object[] { "walking in", Customer(v => v.ClientArrived = false), StepKind.Wait },
            new object[] { "bell locked", Customer(v => v.ClientMoving = true), StepKind.Wait },
            new object[] { "line playing", Customer(v => v.DialogueActive = true), StepKind.Wait },
            new object[] { "click through", Customer(v => { v.DialogueActive = true; v.WaitingForAdvance = true; }), StepKind.Advance },
            new object[] { "choices", Customer(v => { v.DialogueActive = true; v.ChoicesShown = true; }), StepKind.Choose },
            new object[] { "buyer", Customer(), StepKind.PlanStageBuyer },
            new object[] { "buyer with sell_all already", Customer(v => v.SellAllOffered = true), StepKind.Counter },
            new object[] { "seller", Customer(v => v.Intent = ClientIntent.Sell), StepKind.PlanStageSeller },
            new object[] { "seller before ready", Customer(v => { v.Intent = ClientIntent.Sell; v.CounterReady = false; v.TheirItemsOnCounter = true; }), StepKind.PlanStageSeller },
            new object[] { "wholesale swap: no staging", Customer(v => { v.Intent = ClientIntent.SellAndBuy; v.OffersWholesale = true; }), StepKind.PlanStageSeller },
            new object[] { "uncovered dismissable", Customer(v => { v.CoverageGap = "intent INFORMATION_DEALER"; v.ClientKind = "info_dealer"; }), StepKind.Dismiss },
            new object[] { "uncovered not dismissable", Customer(v => { v.CoverageGap = "not dismissable"; v.ClientKind = "dialogue"; v.ClientDismissable = false; }), StepKind.Handover },
            new object[] { "queue", Store(StorePhase.Open, v => v.Queue = 3), StepKind.RingBell },
            new object[] { "queue empty", Store(StorePhase.Open), StepKind.CloseShutter },
            new object[] { "queue held by a must-see customer", Store(StorePhase.Open, v => v.CanEndDay = false), StepKind.Wait },
            new object[] { "night", Store(StorePhase.Closed), StepKind.OpenTablet },
            new object[] { "a prompt nobody asked for", Store(StorePhase.Closed, v => v.LeavePromptOpen = true), StepKind.Handover },
            new object[] { "tablet", Store(StorePhase.Closed, v => v.TabletOpen = true), StepKind.SetServices },
            new object[] { "aug verdict before the counter", Customer(v => v.AugTells = "perfectly still"), StepKind.AugCheck },
            new object[] { "aug verdict waits for the line", Customer(v => { v.AugTells = "perfectly still"; v.DialogueActive = true; }), StepKind.Wait },
            new object[] { "a gun the autopilot did not raise", Customer(v => { v.ShootingMode = v.GunReady = true; v.BlockedBy = "shooting mode"; }), StepKind.Handover },
        };

        [Theory]
        [MemberData(nameof(Cases))]
        public void NextStep(string name, LiveView view, StepKind want)
        {
            Assert.True(want == Next(view), name + ": got " + Next(view));
        }

        [Fact]
        public void TheDayRunsInOrder()
        {
            var m = new AutopilotMemory();
            Assert.Equal(StepKind.Chores, Next(Store(StorePhase.Morning), m));
            m.ChoresDone = true;
            Assert.Equal(StepKind.PlanWindow, Next(Store(StorePhase.Morning), m));
            m.WindowDone = true;
            Assert.Equal(StepKind.Wait, Next(Store(StorePhase.Morning, v => v.ShutterLocked = true), m));
            Assert.Equal(StepKind.OpenShutter, Next(Store(StorePhase.Morning), m));

            Assert.Equal(StepKind.PlanStageBuyer, Next(Customer(), m));
            m.StagedBuyer = true;
            Assert.Equal(StepKind.PullExposable, Next(Customer(), m));
            m.ExposableChecked = true;
            Assert.Equal(StepKind.Counter, Next(Customer(), m));
            Assert.Equal(1, m.ClientSeq);

            // The customer leaves: their leftovers go back before the bell rings again.
            Assert.Equal(StepKind.PlanUnstage, Next(Store(StorePhase.Open, v => v.Queue = 2), m));
            m.UnstagePending = false;
            Assert.Equal(StepKind.RingBell, Next(Store(StorePhase.Open, v => v.Queue = 2), m));

            Assert.Equal(StepKind.PlanNight, Next(Store(StorePhase.Closed), m));
            m.NightPlanned = true;
            Assert.Equal(StepKind.EveningChores, Next(Store(StorePhase.Closed), m));
            m.EveningDone = true;
            Assert.Equal(StepKind.OpenTablet, Next(Store(StorePhase.Closed), m));
            m.ServicesSet = true;
            Assert.Equal(StepKind.Sleep, Next(Store(StorePhase.Closed, v => v.TabletOpen = true), m));
            m.SleepRequested = true;
            Assert.Equal(StepKind.Wait, Next(Store(StorePhase.Closed, v => v.TabletOpen = true), m));

            // The next morning starts fresh.
            Assert.Equal(StepKind.Chores, Next(Store(StorePhase.Morning, v => v.Day = 13), m));
            Assert.False(m.SleepRequested);
        }

        [Fact]
        public void ActsOnlyOnAQuietView()
        {
            Assert.Equal(StepKind.Wait, Next(Store(StorePhase.Morning), null, Planner.QuietMs - 1));
            Assert.Equal(StepKind.PlanWindow, Next(Store(StorePhase.Morning), null, Planner.QuietMs));
            // An uncovered customer gets time to finish its own flow first.
            var uncovered = Customer(v => v.CoverageGap = "intent BARTER");
            Assert.Equal(StepKind.Wait, Next(uncovered, null, Planner.UncoveredQuietMs - 1));
            Assert.Equal(StepKind.Dismiss, Next(uncovered, null, Planner.UncoveredQuietMs));
        }

        // The gun: raised once on the verdict, fired when the crosshair is up, Security's lines read through; never
        // a second raise for the same customer.
        [Fact]
        public void TheGunRaisesFiresAndReadsTheVerdict()
        {
            var m = new AutopilotMemory { Day = 12 };
            var step = Planner.Next(Customer(v => v.AugTells = "twitching"), m, Quiet);
            Assert.Equal(StepKind.AugCheck, step.Kind);
            Assert.Equal("twitching", step.Reason);
            m.GunDecided = m.GunRaised = true;
            Assert.Equal(StepKind.Wait, Next(Customer(v => { v.ShootingMode = true; v.BlockedBy = "blackscreen"; }), m));
            Assert.Equal(StepKind.Wait, Next(Customer(v => { v.ShootingMode = true; v.BlockedBy = "shooting mode"; }), m));
            Assert.Equal(StepKind.Wait, Next(Customer(v => { v.ShootingMode = v.GunReady = true; v.BlockedBy = "shooting mode"; }), m, Planner.QuietMs - 1));
            Assert.Equal(StepKind.Fire, Next(Customer(v => { v.ShootingMode = v.GunReady = true; v.BlockedBy = "shooting mode"; }), m));
            m.Fired = true;
            Assert.Equal(StepKind.Wait, Next(Customer(v => { v.ShootingMode = v.GunReady = true; v.BlockedBy = "shooting mode"; }), m));
            var verdict = Customer(v => { v.Shootout = true; v.BlockedBy = "an arrest or a shootout"; v.DialogueActive = v.WaitingForAdvance = true; });
            Assert.Equal(StepKind.Advance, Next(verdict, m));
            Assert.Equal(StepKind.PlanStageBuyer, Next(Customer(v => v.AugTells = "twitching"), m));
            Next(Store(StorePhase.Open), m);
            Assert.False(m.GunDecided || m.GunRaised || m.Fired);
        }

        [Fact]
        public void AShootoutTheAutopilotDidNotStartIsWaitedOut()
        {
            var v = Customer(x => { x.Shootout = true; x.BlockedBy = "an arrest or a shootout"; x.DialogueActive = x.WaitingForAdvance = true; });
            Assert.Equal(StepKind.Wait, Next(v, new AutopilotMemory { Day = 12 }));
        }

        [Fact]
        public void TheHandoverNamesTheCustomer()
        {
            var step = Planner.Next(Customer(v => { v.CoverageGap = "not dismissable"; v.ClientKind = "info_dealer"; v.ClientDismissable = false; v.ClientName = "Ivo"; }), new AutopilotMemory { Day = 12 }, Quiet);
            Assert.Equal("Ivo: info dealer customer, not dismissable", step.Reason);
        }

        [Fact]
        public void BatchesRunOneMoveAtATimeAndDieWithTheirCustomer()
        {
            var m = new AutopilotMemory { Day = 12, StagedBuyer = true };
            Next(Customer(), m);
            m.Moves.Enqueue(new PlannedMove { Kind = MoveKinds.Stage, Name = "a", ClientKey = Mara });
            m.Moves.Enqueue(new PlannedMove { Kind = MoveKinds.Stage, Name = "b", ClientKey = Mara });
            var step = Planner.Next(Customer(v => v.DialogueActive = true), m, Quiet);
            Assert.Equal(StepKind.Move, step.Kind);
            Assert.Equal("a", step.Move.Name);
            Assert.Equal(2, m.Moves.Count);

            var other = Customer(v => v.ClientKey = new object());
            Assert.Equal(StepKind.PlanStageBuyer, Next(other, m));
            Assert.Empty(m.Moves);
        }

        [Fact]
        public void TheBuyerIsCheckedForExposableItemsOnceStaged()
        {
            var m = new AutopilotMemory { Day = 12 };
            Next(Customer(), m);
            m.StagedBuyer = true;
            Assert.Equal(StepKind.PullExposable, Next(Customer(), m));
            m.ExposableChecked = true;
            Assert.Equal(StepKind.Counter, Next(Customer(), m));
        }

        // The morning's chores queue a supplier's order; the call runs before the window, the phone's own flow.
        [Fact]
        public void APhoneOrderRunsThroughThePhone()
        {
            var m = new AutopilotMemory { Day = 12, ChoresDone = true };
            m.Calls.Enqueue(new PhoneCall { Number = 56371, Choice = 1, Who = "Caulson" });
            Assert.Equal(StepKind.OpenPhone, Next(Store(StorePhase.Morning), m));
            Assert.NotNull(m.Call);
            Assert.Equal(StepKind.Dial, Next(Store(StorePhase.Morning, v => v.PhoneOpen = true), m));
            m.Call.Dialled = true;
            Assert.Equal(StepKind.PhoneAdvance, Next(Store(StorePhase.Morning, v => { v.PhoneOpen = v.PhoneDialogue = v.PhoneWaitingForAdvance = true; }), m));
            Assert.Equal(StepKind.PhoneChoose, Next(Store(StorePhase.Morning, v => { v.PhoneOpen = v.PhoneDialogue = v.PhoneChoicesShown = true; }), m));
            Assert.Equal(StepKind.ClosePhone, Next(Store(StorePhase.Morning, v => v.PhoneOpen = true), m));
            Assert.Equal(StepKind.PlanWindow, Next(Store(StorePhase.Morning), m));
            Assert.Null(m.Call);
        }

        // Calls wait while a customer is at the counter.
        [Fact]
        public void ACallWaitsForAnEmptyCounter()
        {
            var m = new AutopilotMemory { Day = 12, ChoresDone = true };
            m.Calls.Enqueue(new PhoneCall { Number = 8376, Choice = 0 });
            Assert.Equal(StepKind.PlanStageBuyer, Next(Customer(), m));
            Assert.Null(m.Call);
        }

        [Fact]
        public void TheNightGoesOutAndComesHome()
        {
            var m = new AutopilotMemory { Day = 12 };
            Assert.Equal(StepKind.PlanNight, Next(Store(StorePhase.Closed), m));
            m.NightPlanned = true;
            m.Outing = Locations.Dump;
            m.Scavenge = true;
            Assert.Equal(StepKind.OpenDoor, Next(Store(StorePhase.Closed), m));
            Assert.Equal(StepKind.ConfirmGoOutside, Next(Store(StorePhase.Closed, v => v.GoOutsideConfirmOpen = true), m));
            Assert.Equal(StepKind.ChooseBag, Next(Store(StorePhase.Closed, v => v.ResponseOpen = true), m));
            Assert.Equal(StepKind.Travel, Next(Store(StorePhase.Closed, v => v.MapOpen = true), m));
            LiveView Out(Action<LiveView> with = null) => Store(StorePhase.Closed, v => { v.Outside = true; v.OutsideAt = Locations.Dump; with?.Invoke(v); });
            Assert.Equal(StepKind.Scavenge, Next(Out(), m));
            m.ScavengeDone = true;
            Assert.Equal(StepKind.PlanLoot, Next(Out(), m));
            m.LootPlanned = true;
            m.Moves.Enqueue(new PlannedMove { Kind = MoveKinds.Loot, Name = "scrap" });
            Assert.Equal(StepKind.Move, Next(Out(), m));
            m.Moves.Clear();
            Assert.Equal(StepKind.Return, Next(Out(), m));
            m.Returning = true;
            Assert.Equal(StepKind.LeaveOnGround, Next(Out(v => v.LeavePromptOpen = true), m));
            // Home: the outing is not tried again; the evening goes on.
            Assert.Equal(StepKind.EveningChores, Next(Store(StorePhase.Closed), m));
        }

        [Fact]
        public void DrJacksonsShopStagesBuysAndCarriesHome()
        {
            var m = new AutopilotMemory { Day = 12, NightPlanned = true, Outing = Locations.Jackson };
            LiveView At() => Store(StorePhase.Closed, v => { v.Outside = true; v.OutsideAt = Locations.Jackson; v.ClientPresent = v.ClientArrived = true; v.ClientKey = new object(); });
            Assert.Equal(StepKind.PlanJackson, Next(At(), m));
            m.JacksonStaged = true;
            Assert.Equal(StepKind.JacksonBuy, Next(At(), m));
            m.JacksonBought = true;
            Assert.Equal(StepKind.PlanCarryHome, Next(At(), m));
            m.CarriedHome = true;
            Assert.Equal(StepKind.Return, Next(At(), m));
            // Dr. Jackson at his counter is not a customer of the shop.
            Assert.Null(m.ClientKey);
        }

        [Fact]
        public void OutsideSomewhereElseIsAHandover() =>
            Assert.Equal(StepKind.Handover, Next(Store(StorePhase.Closed, v => { v.Outside = true; v.OutsideAt = "commissary"; }), new AutopilotMemory { Day = 12, NightPlanned = true, Outing = Locations.Dump }));

        [Fact]
        public void AnEstimateIsSetThenSubmitted()
        {
            var m = new AutopilotMemory { Day = 12 };
            LiveView Appraising(Action<LiveView> with = null) => Customer(v => { v.AppraisalClient = true; v.CoverageGap = "intent APPRAISAL_SERVICE"; v.CounterReady = false; with?.Invoke(v); });
            Assert.Equal(StepKind.Inspect, Next(Appraising(v => v.AppraisalItemReady = true), m));
            m.Estimate = new PendingEstimate { Estimate = 420 };
            Assert.Equal(StepKind.SetEstimate, Next(Appraising(v => { v.InspectionOpen = true; v.EstimateField = 300; }), m));
            Assert.Equal(StepKind.SubmitEstimate, Next(Appraising(v => { v.InspectionOpen = true; v.EstimateField = 420; }), m));
            Assert.Equal(StepKind.Fault, Next(Appraising(), m));
        }

        [Fact]
        public void TheDonationIsStagedThenGivenAtMostTwice()
        {
            var m = new AutopilotMemory { Day = 12 };
            LiveView Official() => Customer(v => { v.DialogueActive = v.ChoicesShown = v.BribeDue = true; });
            Assert.Equal(StepKind.PlanBribe, Next(Official(), m));
            m.BribeTries = 1;
            m.DonationStaged = true;
            Assert.Equal(StepKind.Donate, Next(Official(), m));
            m.DonationStaged = false;
            m.BribeTries = 2;
            Assert.Equal(StepKind.Choose, Next(Official(), m));
        }

        // A gun order once the greeting is over: filled or refused once, then the client is served as whatever it
        // stays as (a melee buyer after a refusal or a delivery's follow-up) or sent away as before.
        [Fact]
        public void AGunOrderIsFilledOnceAfterTheGreeting()
        {
            LiveView Order(Action<LiveView> with = null) => Customer(v =>
            {
                v.Intent = ClientIntent.Other;
                v.CoverageGap = "intent GUNSMITH";
                v.CounterReady = false;
                v.ClientKind = "gunsmith";
                v.GunOrder = v.ArmsOrders = true;
                with?.Invoke(v);
            });
            var m = new AutopilotMemory { Day = 12 };
            Assert.Equal(StepKind.Advance, Next(Order(v => { v.DialogueActive = true; v.WaitingForAdvance = true; }), m));
            Assert.Equal(StepKind.GunOrder, Next(Order(), m));
            m.GunOrderDone = true;
            // Refused and dismissed, or delivered: never a second order step for this customer.
            Assert.Equal(StepKind.Wait, Next(Order(), m, Planner.UncoveredQuietMs - 1));
            Assert.Equal(StepKind.Dismiss, Next(Order(), m));
            // The client stays on as a melee buyer: the counter serves it.
            Assert.Equal(StepKind.PlanStageBuyer, Next(Customer(), m));

            // Orders off (no `arms`, or its members missing): the client is sent away as before.
            Assert.Equal(StepKind.Dismiss, Next(Order(v => v.ArmsOrders = false), new AutopilotMemory { Day = 12 }));
            // A service order waits for the client's gun, then plans anyway (the plan refuses without it).
            var s = new AutopilotMemory { Day = 12 };
            Assert.Equal(StepKind.Wait, Next(Order(v => v.ServiceGunPlaced = false), s, Planner.UncoveredQuietMs - 1));
            Assert.Equal(StepKind.GunOrder, Next(Order(v => v.ServiceGunPlaced = false), s));
            // The next customer gets their own order.
            var other = new object();
            Assert.Equal(StepKind.GunOrder, Next(Order(v => v.ClientKey = other), m));
            Assert.False(m.GunOrderDone);
        }

        [Fact]
        public void AnAugVerdictComesBeforeTheOrder() =>
            Assert.Equal(StepKind.AugCheck, Next(Customer(v => { v.GunOrder = v.ArmsOrders = true; v.CoverageGap = "intent GUNSMITH"; v.AugTells = "perfectly still"; })));

        [Fact]
        public void ACounterLoopIsAFault()
        {
            var m = new AutopilotMemory { Day = 12 };
            Next(Customer(), m);
            m.StagedBuyer = m.ExposableChecked = true;
            m.CounterMoves = Planner.MaxCounterMoves;
            Assert.Equal(StepKind.Fault, Next(Customer(), m));
        }
    }

    public class AutopilotControlTests
    {
        [Fact]
        public void StartsPausedAndOnlyTheHumanResumes()
        {
            var s = new AutopilotSwitch();
            Assert.Equal(AutopilotState.Paused, s.State);
            Assert.False(s.Fault("x"));
            Assert.True(s.Resume());
            Assert.True(s.Running);

            Assert.True(s.Handover("Ivo: appraisal customer"));
            Assert.True(s.Alarmed);
            Assert.False(s.Pause("late"));
            Assert.Equal("Ivo: appraisal customer", s.Reason);
            Assert.True(s.Resume());

            Assert.True(s.Fault("move did not take"));
            Assert.False(s.Handover("second stop"));
            Assert.Equal("move did not take", s.Reason);
            Assert.True(s.Resume());
            Assert.True(s.Pause("you moved an item"));
            Assert.False(s.Alarmed);
        }

        [Fact]
        public void TheSettingTurnsItOffAndBackToPaused()
        {
            var s = new AutopilotSwitch();
            s.Resume();
            s.SetEnabled(false);
            Assert.Equal(AutopilotState.Off, s.State);
            Assert.False(s.Resume());
            s.SetEnabled(true);
            Assert.Equal(AutopilotState.Paused, s.State);
            s.SetEnabled(true);
            Assert.Equal(AutopilotState.Paused, s.State);
        }

        [Theory]
        [InlineData(null, Pace.DefaultMs)]
        [InlineData("", Pace.DefaultMs)]
        [InlineData("fast", Pace.DefaultMs)]
        [InlineData("800", 800)]
        [InlineData(" 800 ", 800)]
        [InlineData("5", Pace.MinMs)]
        [InlineData("999999", Pace.MaxMs)]
        public void PaceParses(string setting, int want) => Assert.Equal(want, Pace.Parse(setting, Pace.DefaultMs));

        [Fact]
        public void PaceScalesByKind()
        {
            var p = new Pace { BaseMs = 800, HoldMs = 3000 };
            Assert.Equal(400, p.DelayMs(PaceKinds.Move));
            Assert.Equal(800, p.DelayMs(PaceKinds.Decision));
            Assert.Equal(800, p.DelayMs(PaceKinds.Dialogue, 20));
            Assert.Equal(1600, p.DelayMs(PaceKinds.Dialogue, 120));
            Assert.Equal(3200, p.DelayMs(PaceKinds.Dialogue, 100000));
            Assert.Equal(3000, p.DelayMs(PaceKinds.Hold));
            Assert.Equal(800, new Pace { BaseMs = 800, HoldMs = 0 }.DelayMs(PaceKinds.Hold));
            Assert.Equal(PaceKinds.Move, Pace.KindOf(StepKind.Move));
            Assert.Equal(PaceKinds.Dialogue, Pace.KindOf(StepKind.Choose));
            Assert.Equal(PaceKinds.Decision, Pace.KindOf(StepKind.Counter));
        }

        [Fact]
        public void TheWatchdogCountsStillness()
        {
            var w = new Watchdog();
            Assert.Equal(0, w.Quiet("a", 1000));
            Assert.Equal(5000, w.Quiet("a", 6000));
            Assert.Equal(0, w.Quiet("b", 7000));
            Assert.False(w.Stalled(w.Quiet("b", 7000 + Watchdog.LimitMs - 1)));
            Assert.True(w.Stalled(w.Quiet("b", 7000 + Watchdog.LimitMs)));
            w.Reset(100000);
            Assert.Equal(0, w.Quiet("b", 100000));
        }

        [Fact]
        public void ExpectationsHaveADeadline()
        {
            var e = Expectation.Within("the shutter opens", 1000, 5000, v => v.Store == StorePhase.Open);
            Assert.False(e.Met(new LiveView { Store = StorePhase.Morning }));
            Assert.False(e.Missed(6000));
            Assert.True(e.Missed(6001));
            Assert.True(e.Met(new LiveView { Store = StorePhase.Open }));
        }

        [Fact]
        public void TheSignatureTracksProgress()
        {
            var a = new LiveView { InStore = true, Store = StorePhase.Open, ClientPresent = true, CounterSignature = "x" };
            var b = new LiveView { InStore = true, Store = StorePhase.Open, ClientPresent = true, CounterSignature = "y" };
            Assert.NotEqual(a.Signature(), b.Signature());
            b.CounterSignature = "x";
            Assert.Equal(a.Signature(), b.Signature());
        }
    }

    // The autopilot's events are the fixtures' shapes (protocol/fixtures/event.autopilot*.json).
    public class AutopilotMachineTests
    {
        sealed class Node
        {
            public string Label;
            public bool Hidden;
            public readonly List<object> Kids = new List<object>();

            public Node(string label = null, params object[] kids)
            {
                Label = label;
                Kids.AddRange(kids);
            }
        }

        static Node Button(Node root, string label) =>
            UiTree.Find(root, o => ((Node)o).Kids, o => o is Node n && n.Label == label && !n.Hidden ? n : null);

        [Fact]
        public void TheButtonIsFoundByItsLabelAnywhereInTheWindow()
        {
            var start = new Node("Start");
            var output = new Node("Output");
            var window = new Node(null, new Node(null, new Node("Modules"), new Node(null, start)), new Node(null, new Node(null, new Node("Configure"), output)));
            Assert.Same(start, Button(window, "Start"));
            Assert.Same(output, Button(window, "Output"));
            Assert.Null(Button(window, "Blend"));
            output.Hidden = true;
            Assert.Null(Button(window, "Output"));
        }

        [Fact]
        public void ACycleOrADeepTreeEndsTheSearch()
        {
            var a = new Node();
            var b = new Node(null, a);
            a.Kids.Add(b);
            Assert.Null(Button(a, "Start"));
            var deep = new Node("Start");
            for (int k = 0; k <= UiTree.MaxDepth; k++) deep = new Node(null, deep);
            Assert.Null(Button(deep, "Start"));
            Assert.NotNull(Button((Node)deep.Kids[0], "Start"));
        }

        [Fact]
        public void TheThermostatTicksOnceASecondAndReportsInBatches()
        {
            var t = new Thermostat();
            Assert.True(t.Due(0));
            Assert.False(t.Due(250));
            Assert.False(t.Due(999));
            Assert.True(t.Due(1000));
            Assert.Equal(0, t.Switched(0, 1000));
            // The first switch is reported at once, the next ones summed until a report is due.
            Assert.Equal(1, t.Switched(1, 2000));
            Assert.Equal(0, t.Switched(1, 3000));
            Assert.Equal(0, t.Switched(1, 2000 + Thermostat.ReportMs - 1));
            Assert.Equal(3, t.Switched(1, 2000 + Thermostat.ReportMs));
            Assert.Equal(0, t.Switched(0, 2000 + 3 * Thermostat.ReportMs));
        }
    }

    public class AutopilotEventTests
    {
        static JObject FixtureData(string name) => (JObject)JObject.Parse(File.ReadAllText(Path.Combine(Paths.Fixtures, name)))["data"];

        [Fact]
        public void ActionMatchesTheFixture()
        {
            var built = AutopilotEvents.Build(new AutopilotAction
            {
                Day = 69, Phase = "customer", Action = "stage", TargetUid = 2231, TargetName = "Canned beans",
                Why = "what Mara wants, within the budget", Rule = "player:stage_for_buyer", PlanDay = 69, ClientSeq = 41, Client = "Mara",
                HiddenUsed = new[] { "water_purity", "item_exposable", "water_purity" },
            });
            Assert.True(JToken.DeepEquals(FixtureData("event.autopilot.json"), built), built.ToString());
        }

        [Fact]
        public void FaultMatchesTheFixture()
        {
            var built = AutopilotEvents.BuildFault(69, "customer", AutopilotState.Handover, "Ivo: appraisal customer, not dismissable", null, "Ivo", "appraisal");
            Assert.True(JToken.DeepEquals(FixtureData("event.autopilot_fault.json"), built), built.ToString());
        }

        [Fact]
        public void AnActionAlwaysSaysWhyAndWhatItRead()
        {
            var o = AutopilotEvents.Build(new AutopilotAction { Day = 3, Phase = "morning", Action = "open_shutter" });
            Assert.Equal("", (string)o["why"]);
            Assert.Empty((JArray)o["hidden_used"]);
            Assert.Null(o["target"]);
            Assert.Null(o["client_seq"]);
        }

        [Theory]
        [InlineData(StorePhase.Morning, false, "morning")]
        [InlineData(StorePhase.Open, false, "open")]
        [InlineData(StorePhase.Open, true, "customer")]
        [InlineData(StorePhase.Closed, false, "night")]
        public void Phases(StorePhase store, bool client, string want) =>
            Assert.Equal(want, AutopilotEvents.PhaseOf(new LiveView { InStore = true, Store = store, ClientPresent = client }));

        [Fact]
        public void TheAutopilotIsRecordedAtHighPriority()
        {
            Assert.Equal(Fence.Recording.EventPriority.High, Fence.Recording.RecordKinds.Priority(AutopilotEvents.Action));
            Assert.Equal(Fence.Recording.EventPriority.High, Fence.Recording.RecordKinds.Priority(AutopilotEvents.Fault));
        }
    }

    public class AutopilotGateTests
    {
        [Fact]
        public void TheAutopilotGateFollowsTheSwitch()
        {
            bool running = true;
            var gate = ActGate.TryOpenAutopilot(() => running);
            Assert.NotNull(gate);
            Assert.True(gate.StillOpen);
            running = false;
            Assert.False(gate.StillOpen);
            Assert.Null(ActGate.TryOpenAutopilot(() => running));
            Assert.Null(ActGate.TryOpenAutopilot(null));
        }

        [Fact]
        public void AutopilotMovesAreAttributedToTheAutopilot()
        {
            var a = new Attribution();
            a.BeginMod("d-1", Actors.Autopilot);
            Assert.Equal(("d-1", Actors.Autopilot), a.Direct("d-2"));
            a.EndMod();
            Assert.Equal(("d-1", Actors.Autopilot), a.Consequence("d-2"));
            a.BeginMod("d-3");
            Assert.Equal(("d-3", Actors.Mod), a.Direct("d-4"));
        }
    }

    public class AutopilotOverlayTests
    {
        [Fact]
        public void TheAutopilotTakesThePauseButtonAndTheBar()
        {
            var v = OverlayText.Build(new OverlayInput { Mode = "observe", Act = ActDisplay.Advise, Autopilot = "AUTOPILOT PAUSED — your turn", AutopilotAlert = true, AutopilotPause = "Resume autopilot" });
            Assert.Equal("Resume autopilot", v.PauseLabel);
            Assert.True(v.PauseInteractable);
            Assert.True(v.AutopilotAlert);
            Assert.StartsWith("AUTOPILOT PAUSED", v.Bar);

            var off = OverlayText.Build(new OverlayInput { Mode = "observe", Act = ActDisplay.Advise });
            Assert.Null(off.PauseLabel);
            Assert.Equal("", off.Autopilot);
            Assert.False(off.AutopilotAlert);
        }

        [Fact]
        public void BannersSayWhatToDo()
        {
            Assert.Contains("press Resume", AutopilotEvents.Banner(AutopilotState.Handover, "Ivo: appraisal customer, not dismissable"));
            Assert.Contains("did not take", AutopilotEvents.Banner(AutopilotState.Faulted, "did not take: the shutter opens"));
            Assert.Equal("", AutopilotEvents.Banner(AutopilotState.Off, null));
        }
    }
}
