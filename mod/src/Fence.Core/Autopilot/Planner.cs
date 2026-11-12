using System.Collections.Generic;
using Fence.Barter;

namespace Fence.Autopilot
{
    public enum StepKind
    {
        // Not a playable moment (menu, loading, game over): nothing to do, no watchdog.
        Idle,
        // Waiting for the game to settle; the watchdog counts.
        Wait,
        // Stop, shout, hand control to the human.
        Handover,
        Fault,
        ClosePanel,
        PlanWindow,
        Move,
        OpenShutter,
        RingBell,
        Advance,
        Choose,
        PlanStageBuyer,
        PlanStageSeller,
        Counter,
        Dismiss,
        PlanUnstage,
        CloseShutter,
        OpenTablet,
        SetServices,
        Sleep,
        // M2: the engines' chores, the phone, Wild's Network, the night's outing, the appraisal board, donations.
        Chores,
        EveningChores,
        OpenPhone,
        Dial,
        PhoneChoose,
        PhoneAdvance,
        ClosePhone,
        WildBuy,
        WildClose,
        PlanNight,
        OpenDoor,
        ConfirmGoOutside,
        ChooseBag,
        Travel,
        Scavenge,
        PlanLoot,
        PlanJackson,
        JacksonBuy,
        PlanCarryHome,
        Return,
        LeaveOnGround,
        Inspect,
        SetEstimate,
        SubmitEstimate,
        PlanBribe,
        Donate,
        PullExposable,
        // M3: the gun on an aug verdict (raise it, or hand over / serve as the setting says), the shot.
        AugCheck,
        Fire,
        // A gun-order client's order filled from stock and accepted, or the client sent away (Arms.Prepare).
        GunOrder,
    }

    public sealed class Step
    {
        public StepKind Kind;
        public string Reason;
        public PlannedMove Move;

        public static Step Of(StepKind kind, string reason = null) => new Step { Kind = kind, Reason = reason };

        public bool IsAction => Kind != StepKind.Idle && Kind != StepKind.Wait && Kind != StepKind.Handover && Kind != StepKind.Fault;

        public override string ToString() => Kind + (Reason != null ? " (" + Reason + ")" : "");
    }

    public static class MoveKinds
    {
        public const string Showcase = "showcase";
        public const string Unshowcase = "unshowcase";
        public const string Stage = "stage";
        public const string StageTheirs = "stage_theirs";
        public const string Unstage = "unstage";
        // Into the dossier (the appraisal kit) or the bin (appraisal slips).
        public const string StowKit = "stow_kit";
        public const string BinSlip = "bin_slip";
        public const string Donate = "stage_donation";
        public const string Loot = "loot_home";
        public const string JacksonStage = "stage_jackson";
        public const string CarryHome = "carry_home";
    }

    // A supplier or Wild's Network by phone: the number dialled and the menu choice to take (-1: none, the
    // call is an introduction or the choice is the only way on).
    public sealed class PhoneCall
    {
        public long Number;
        public int Choice = -1;
        public string Who;
        public string Why;
        public bool Dialled;
        public bool Chosen;
    }

    // An appraisal client's item in hand: the inspection view opens on it, the panel takes the estimate.
    public sealed class PendingEstimate
    {
        public object Item;
        public int Uid;
        public string Name;
        public int Estimate;
        public string Why;
        public bool Submitted;
    }

    // One item move of a batch (the window, a staging list, the leftovers), taken one per paced step.
    public sealed class PlannedMove
    {
        public string Kind;
        // The game item; opaque here.
        public object Item;
        public int Uid;
        public string Name;
        public string Why;
        public string Rule;
        public IList<string> HiddenUsed;
        // The customer the batch was planned for (null between customers): a new one drops the rest.
        public object ClientKey;
    }

    // What the driver remembers of the day and the customer at the counter.
    public sealed class AutopilotMemory
    {
        public int Day = -1;
        public bool WindowDone;
        public bool ServicesSet;
        public bool SleepRequested;
        public bool ChoresDone;
        public bool EveningDone;

        // The night's plan (PlayerRole.Night): an outing (null: none) and whether to search the dump; each part
        // is done or given up once.
        public bool NightPlanned;
        public string Outing;
        public bool Scavenge;
        public bool WildCalled;
        public bool OutingDone;
        public bool ScavengeDone;
        public bool LootPlanned;
        public bool JacksonStaged;
        public bool JacksonBought;
        public bool CarriedHome;
        public bool Returning;
        // Upgrades to buy while Wild's window is open (null: not yet planned for this opening).
        public Queue<string> WildBuys;

        public readonly Queue<PhoneCall> Calls = new Queue<PhoneCall>();
        public PhoneCall Call;

        public object ClientKey;
        public int ClientSeq;
        public bool StagedBuyer;
        public bool StagedSeller;
        public int CounterMoves;
        public bool Dismissed;
        public bool ExposableChecked;
        public int BribeTries;
        public bool DonationStaged;
        public PendingEstimate Estimate;
        // The gun for this customer: decided (shoot, serve or hand over: never asked again), raised, fired.
        public bool GunDecided;
        public bool GunRaised;
        public bool Fired;
        public int? CashBeforeShot;
        // This customer's gun order was filled or refused: never planned again.
        public bool GunOrderDone;
        // The last customer left and their leftovers are not yet planned back to stock.
        public bool UnstagePending;

        public readonly Queue<PlannedMove> Moves = new Queue<PlannedMove>();

        public void NewDay(int day)
        {
            Day = day;
            WindowDone = ServicesSet = SleepRequested = ChoresDone = EveningDone = false;
            NightPlanned = WildCalled = OutingDone = ScavengeDone = LootPlanned = JacksonStaged = JacksonBought = CarriedHome = Returning = false;
            Outing = null;
            Scavenge = false;
            WildBuys = null;
            Calls.Clear();
            Call = null;
            Moves.Clear();
            ForgetClient();
            UnstagePending = false;
        }

        public void NewClient(object key)
        {
            ForgetClient();
            ClientKey = key;
            ClientSeq++;
        }

        void ForgetClient()
        {
            ClientKey = null;
            StagedBuyer = StagedSeller = Dismissed = ExposableChecked = DonationStaged = false;
            GunDecided = GunRaised = Fired = GunOrderDone = false;
            CashBeforeShot = null;
            CounterMoves = BribeTries = 0;
            Estimate = null;
        }

        public void ClientLeft()
        {
            if (ClientKey == null) return;
            ForgetClient();
            UnstagePending = true;
        }
    }

    public static class Locations
    {
        public const string Dump = "dumping_ground";
        public const string Jackson = "dr_jackson";
    }

    // The live driver's brain, without the game: which kind of step comes next. The mod turns a step into
    // the concrete action (which items, which choice, which counter option) with the executor's rules.
    public static class Planner
    {
        // Every action waits for the view to hold still this long.
        public const long QuietMs = 400;
        // An uncovered customer (or a covered one whose counter never becomes ready) gets this long to finish
        // its own flow before the autopilot dismisses it or hands over.
        public const long UncoveredQuietMs = 3000;
        // More counter moves than this for one customer is a loop, not a negotiation.
        public const int MaxCounterMoves = 8;

        public static Step Next(LiveView v, AutopilotMemory m, long quietMs)
        {
            if (v == null || !v.InStore || v.Loading) return Step.Of(StepKind.Idle, "not in the shop");
            if (v.GameOver) return Step.Of(StepKind.Handover, "the run ended");
            if (v.BlockedBy != null) return Gun(v, m, quietMs) ?? Step.Of(StepKind.Wait, v.BlockedBy);
            if (m.Day != v.Day) m.NewDay(v.Day);

            object here = v.ClientPresent && !v.Outside ? v.ClientKey : null;
            if (here != null && !ReferenceEquals(here, m.ClientKey)) m.NewClient(here);
            else if (here == null && m.ClientKey != null) m.ClientLeft();
            while (m.Moves.Count > 0 && !ReferenceEquals(m.Moves.Peek().ClientKey, here)) m.Moves.Dequeue();

            if (quietMs < QuietMs) return Step.Of(StepKind.Wait, "settling");
            if (m.Moves.Count > 0) return new Step { Kind = StepKind.Move, Move = m.Moves.Peek() };

            var phone = Phone(v, m);
            if (phone != null) return phone;
            if (v.WildOpen) return m.WildBuys != null && m.WildBuys.Count == 0 ? Step.Of(StepKind.WildClose) : Step.Of(StepKind.WildBuy);
            m.WildBuys = null;
            if (v.Outside) return Outside(v, m);
            if (v.LeavePromptOpen) return Step.Of(StepKind.Handover, "a confirm panel the autopilot did not open");
            if (v.ResponseOpen && !(v.Store == StorePhase.Closed && m.Outing != null && !m.OutingDone)) return Step.Of(StepKind.Handover, "a choices window the autopilot did not open");

            switch (v.Store)
            {
                case StorePhase.Morning: return Morning(v, m);
                case StorePhase.Open: return v.ClientPresent ? Customer(v, m, quietMs) : BetweenCustomers(v, m);
                case StorePhase.Closed: return Night(v, m);
                default: return Step.Of(StepKind.Wait, "store state unknown");
            }
        }

        // A call in progress, or the next one queued. A phone the autopilot did not dial is the human's.
        static Step Phone(LiveView v, AutopilotMemory m)
        {
            if (m.Call == null && (v.PhoneDialogue || v.PhoneOpen)) return Step.Of(StepKind.Handover, "the phone is in use and the autopilot did not dial");
            if (m.Call == null)
            {
                // Calls wait for a moment with no customer at the counter and the shop not outside.
                if (m.Calls.Count == 0 || v.ClientPresent || v.Outside || v.WildOpen) return null;
                m.Call = m.Calls.Dequeue();
            }
            if (v.PhoneDialogue)
            {
                if (v.PhoneChoicesShown) return Step.Of(StepKind.PhoneChoose);
                if (v.PhoneWaitingForAdvance) return Step.Of(StepKind.PhoneAdvance);
                return Step.Of(StepKind.Wait, "on the phone");
            }
            if (!v.PhoneOpen)
            {
                // Hung up (or Wild's call closed the phone and opened his window): the call is over.
                if (m.Call.Dialled) { m.Call = null; return null; }
                return Step.Of(StepKind.OpenPhone);
            }
            if (!m.Call.Dialled) return Step.Of(StepKind.Dial);
            return Step.Of(StepKind.ClosePhone);
        }

        // While the view is blocked: the gun the autopilot raised fires once its crosshair is up, and Security's
        // verdict after its shot is read through. A gun raised by anyone else is the human's.
        static Step Gun(LiveView v, AutopilotMemory m, long quietMs)
        {
            if (v.ShootingMode)
            {
                if (!m.GunRaised) return Step.Of(StepKind.Handover, "the gun is raised and the autopilot did not raise it");
                if (!m.Fired && v.GunReady && quietMs >= QuietMs) return Step.Of(StepKind.Fire);
                return null;
            }
            if (v.Shootout && m.Fired && v.DialogueActive && v.WaitingForAdvance && !v.ChoicesShown && quietMs >= QuietMs)
                return Step.Of(StepKind.Advance);
            return null;
        }

        static Step Morning(LiveView v, AutopilotMemory m)
        {
            if (v.MorningPanelOpen) return Step.Of(StepKind.ClosePanel);
            if (!m.ChoresDone) return Step.Of(StepKind.Chores);
            if (m.Calls.Count > 0) return Step.Of(StepKind.Wait, "calls first");
            if (!m.WindowDone) return Step.Of(StepKind.PlanWindow);
            if (v.ShutterLocked) return Step.Of(StepKind.Wait, "shutter locked");
            return Step.Of(StepKind.OpenShutter);
        }

        static Step BetweenCustomers(LiveView v, AutopilotMemory m)
        {
            if (m.UnstagePending) return Step.Of(StepKind.PlanUnstage);
            if (v.ClientMoving) return Step.Of(StepKind.Wait, "a customer is walking");
            if (v.Queue > 0) return Step.Of(StepKind.RingBell);
            if (!v.CanEndDay) return Step.Of(StepKind.Wait, "a customer the day cannot end without");
            if (v.ShutterLocked) return Step.Of(StepKind.Wait, "shutter locked");
            return Step.Of(StepKind.CloseShutter);
        }

        static Step Customer(LiveView v, AutopilotMemory m, long quietMs)
        {
            if (!v.ClientArrived || v.ClientMoving) return Step.Of(StepKind.Wait, "the customer is walking");
            if (m.Dismissed) return Step.Of(StepKind.Wait, "the customer is leaving");
            // An aug verdict comes before the counter, once no line is still being typed out.
            if (v.AugTells != null && !m.GunDecided && (!v.DialogueActive || v.WaitingForAdvance || v.ChoicesShown))
                return Step.Of(StepKind.AugCheck, v.AugTells);
            if (v.DialogueActive)
            {
                // The permit official's last line: donate goods instead of answering (PlayerCounter).
                if (v.ChoicesShown && v.BribeDue && m.BribeTries < 2)
                    return m.DonationStaged ? Step.Of(StepKind.Donate) : Step.Of(StepKind.PlanBribe);
                if (v.ChoicesShown) return Step.Of(StepKind.Choose);
                if (v.WaitingForAdvance) return Step.Of(StepKind.Advance);
                return Step.Of(StepKind.Wait, "dialogue");
            }
            // A gun order once its greeting is over; a service order waits for the client's gun to reach the counter.
            if (v.GunOrder && v.ArmsOrders && !m.GunOrderDone)
            {
                if (!v.ServiceGunPlaced && quietMs < UncoveredQuietMs) return Step.Of(StepKind.Wait, "the order client's gun");
                return Step.Of(StepKind.GunOrder);
            }
            if (v.AppraisalClient)
            {
                if (m.Estimate != null && !m.Estimate.Submitted)
                {
                    if (!v.InspectionOpen) return Step.Of(StepKind.Fault, "the inspection view closed before the estimate was in");
                    return v.EstimateField == m.Estimate.Estimate ? Step.Of(StepKind.SubmitEstimate) : Step.Of(StepKind.SetEstimate);
                }
                if (v.InspectionOpen) return Step.Of(StepKind.Wait, "the inspection view");
                if (v.AppraisalItemReady) return Step.Of(StepKind.Inspect);
            }
            if (v.CoverageGap == null && v.CounterReady)
            {
                if (BarterRules.IsBuying(v.Intent) && !m.StagedBuyer && !v.SellAllOffered && (v.Intent == ClientIntent.Buy || !v.OffersWholesale))
                    return Step.Of(StepKind.PlanStageBuyer);
                if (m.StagedBuyer && !m.ExposableChecked) return Step.Of(StepKind.PullExposable);
                if (BarterRules.IsSelling(v.Intent) && !m.StagedSeller) return Step.Of(StepKind.PlanStageSeller);
                if (m.CounterMoves >= MaxCounterMoves) return Step.Of(StepKind.Fault, MaxCounterMoves + " counter moves for one customer");
                return Step.Of(StepKind.Counter);
            }
            if (v.CoverageGap == null && v.Intent == ClientIntent.Sell && !m.StagedSeller && v.TheirItemsOnCounter) return Step.Of(StepKind.PlanStageSeller);
            if (quietMs < UncoveredQuietMs) return Step.Of(StepKind.Wait, v.CoverageGap != null ? "letting the " + Kind(v) + " customer finish" : "counter not ready");
            if (v.ClientDismissable) return Step.Of(StepKind.Dismiss, v.CoverageGap ?? "counter never became ready");
            return Step.Of(StepKind.Handover, Who(v) + ": " + Kind(v) + " customer, not dismissable");
        }

        // The gym's night (Episode.Night, then Sleep): Wild's Network by phone, the outing, the vending machine
        // stocked at bedtime, the tablet's services, sleep.
        static Step Night(LiveView v, AutopilotMemory m)
        {
            if (m.SleepRequested) return Step.Of(StepKind.Wait, "going to sleep");
            if (m.UnstagePending) return Step.Of(StepKind.PlanUnstage);
            if (!m.NightPlanned) return Step.Of(StepKind.PlanNight);
            if (m.Calls.Count > 0) return Step.Of(StepKind.Wait, "calls first");
            if (m.Outing != null && !m.OutingDone)
            {
                if (v.GoOutsideConfirmOpen) return Step.Of(StepKind.ConfirmGoOutside);
                if (v.ResponseOpen) return Step.Of(StepKind.ChooseBag);
                if (v.MapOpen) return Step.Of(StepKind.Travel);
                return Step.Of(StepKind.OpenDoor);
            }
            if (v.MapOpen || v.ResponseOpen || v.GoOutsideConfirmOpen) return Step.Of(StepKind.Handover, "the door's menus are open and no outing is planned");
            if (!m.EveningDone) return Step.Of(StepKind.EveningChores);
            if (!v.TabletOpen) return Step.Of(StepKind.OpenTablet);
            if (!m.ServicesSet) return Step.Of(StepKind.SetServices);
            return Step.Of(StepKind.Sleep);
        }

        // At the location: search the dump, bring the loot home; or shop at Dr. Jackson's and carry it home. Then
        // back, answering the leave-on-the-ground prompt.
        static Step Outside(LiveView v, AutopilotMemory m)
        {
            if (m.Outing == null || v.OutsideAt != m.Outing) return Step.Of(StepKind.Handover, "outside at " + (v.OutsideAt ?? "?") + ", not where the night's plan went");
            m.OutingDone = true;
            if (v.LeavePromptOpen) return m.Returning ? Step.Of(StepKind.LeaveOnGround) : Step.Of(StepKind.Handover, "a confirm panel the autopilot did not open");
            if (m.Returning) return Step.Of(StepKind.Wait, "coming home");
            if (m.Outing == Locations.Dump)
            {
                if (m.Scavenge && !m.ScavengeDone) return Step.Of(StepKind.Scavenge);
                if (!m.LootPlanned) return Step.Of(StepKind.PlanLoot);
            }
            if (m.Outing == Locations.Jackson)
            {
                if (!m.JacksonStaged) return Step.Of(StepKind.PlanJackson);
                if (!m.JacksonBought) return Step.Of(StepKind.JacksonBuy);
                if (!m.CarriedHome) return Step.Of(StepKind.PlanCarryHome);
            }
            return Step.Of(StepKind.Return);
        }

        static string Kind(LiveView v) => string.IsNullOrEmpty(v.ClientKind) ? "special" : v.ClientKind.Replace('_', ' ');

        static string Who(LiveView v) => string.IsNullOrEmpty(v.ClientName) ? "the customer" : v.ClientName;
    }
}
