using Fence.Barter;

namespace Fence.Autopilot
{
    public enum StorePhase
    {
        Unknown,
        Morning,
        Open,
        Closed,
    }

    // One read of the live game, everything the autopilot's planner decides from (docs/STREAM.md section 4).
    // The mod fills it on the main thread each tick; the planner never touches the game.
    public sealed class LiveView
    {
        // The store scene is up and its managers exist.
        public bool InStore;
        // A save is loading or a new run is being set up.
        public bool Loading;
        public bool GameOver;
        // Not quiescent (blackscreen, shooting mode, a confirm panel, the map in transit...): the reason.
        public string BlockedBy;
        public bool Outside;
        public int Day;
        public StorePhase Store;

        public bool MorningPanelOpen;
        // lockShutterButtonFlag, an arrest or a shootout.
        public bool ShutterLocked;
        public int Queue;
        // PlayerStore.CanEndDay: every waiting and present customer is dismissable.
        public bool CanEndDay;

        public bool ClientPresent;
        public bool ClientArrived;
        // Walking in or out, or the bell is locked while one does.
        public bool ClientMoving;
        public object ClientKey;
        public string ClientId;
        public string ClientName;
        // CustomerKinds.Of.
        public string ClientKind;
        public bool ClientDismissable;
        // BarterRules.CoverageGap: null for a covered trade customer.
        public string CoverageGap;
        public bool CounterReady;
        public ClientIntent Intent;
        public bool OffersWholesale;
        public bool SellAllOffered;
        public bool TheirItemsOnCounter;
        // BarterSnapshot.Signature while the counter is ready: changes with every counter move.
        public string CounterSignature;

        public bool DialogueActive;
        // The choice bubbles are on screen.
        public bool ChoicesShown;
        public bool WaitingForAdvance;
        public bool PhoneDialogue;
        // Characters of the line on screen (or of the choices), for pacing.
        public int LineChars;

        public bool TabletOpen;

        // M2. The afterhours location the player is at (Outside), the door's menus, the return prompt.
        public string OutsideAt;
        public bool GoOutsideConfirmOpen;
        // The afterhours response window (backpack choice).
        public bool ResponseOpen;
        public bool MapOpen;
        // GenericConfirmationUIManager's panel (the "leave items on the ground?" prompt when outside).
        public bool LeavePromptOpen;
        public bool PhoneOpen;
        public bool PhoneChoicesShown;
        public bool PhoneWaitingForAdvance;
        public bool WildOpen;
        // An appraisal client is at the counter; one of their items has an estimate to give; the inspection
        // view is open; the number in its appraisal field.
        public bool AppraisalClient;
        public bool AppraisalItemReady;
        public bool InspectionOpen;
        public int EstimateField;
        // The permit official is owed a donation this visit (PlayerCounter.BribeDue).
        public bool BribeDue;

        // M3. The gun is raised; its crosshair is up and StoreUIManager.CanShoot; a shootout is running (Security on
        // the way or talking).
        public bool ShootingMode;
        public bool GunReady;
        public bool Shootout;
        // The executor's aug verdict on the customer at the counter (AugRule on the visible tells) when it says aug
        // and the gun can be raised: the tells, in words. Null otherwise.
        public string AugTells;

        // A gun-order client with the order still open (GUNSMITH, not yet served or sent away); whether the executor
        // fills orders (`arms`); false only while a service order's gun has not reached the counter.
        public bool GunOrder;
        public bool ArmsOrders;
        public bool ServiceGunPlaced = true;

        // What progress looks like: any change here resets the watchdog.
        public string Signature()
        {
            return string.Join("|", new[]
            {
                InStore ? "s" : "-", Loading ? "l" : "-", GameOver ? "g" : "-", BlockedBy ?? "", Outside ? "o" : "-", Day.ToString(), Store.ToString(),
                MorningPanelOpen ? "m" : "-", ShutterLocked ? "k" : "-", Queue.ToString(), ClientPresent ? "c" : "-", ClientArrived ? "a" : "-",
                ClientMoving ? "w" : "-", ClientId ?? "", CounterReady ? "r" : "-", CounterSignature ?? "", DialogueActive ? "d" : "-",
                ChoicesShown ? "h" : "-", WaitingForAdvance ? "v" : "-", LineChars.ToString(), TabletOpen ? "t" : "-",
                OutsideAt ?? "", GoOutsideConfirmOpen ? "G" : "-", ResponseOpen ? "R" : "-", MapOpen ? "M" : "-", LeavePromptOpen ? "L" : "-",
                PhoneOpen ? "P" : "-", PhoneDialogue ? "D" : "-", PhoneChoicesShown ? "H" : "-", PhoneWaitingForAdvance ? "V" : "-",
                WildOpen ? "W" : "-", InspectionOpen ? "I" : "-", EstimateField.ToString(),
                ShootingMode ? "S" : "-", GunReady ? "F" : "-", Shootout ? "O" : "-", AugTells != null ? "A" : "-",
                GunOrder ? "N" : "-", ServiceGunPlaced ? "g" : "-",
            });
        }
    }
}
