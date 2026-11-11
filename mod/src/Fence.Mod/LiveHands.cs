using System;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using Fence.Barter;

namespace Fence.Mod
{
    // The autopilot's hands: the only code that presses non-counter buttons (Fence.Tests.ActSafetyTests);
    // counter moves go through BarterExecutor. Each press takes an ActGate and re-checks it first, then does
    // what the player's click does (docs/STREAM.md section 3: the handler, with the handler's own gates).
    // Exceptions propagate: the driver stops and shouts.
    sealed class LiveHands
    {
        public const string Stock = "stock";
        public const string Case = "case";
        public const string Back = "back";
        public const string Ground = "ground";

        // Item moves are the supported ModHelper.MoveItemTo* (quick-transfer's own call), gated on
        // GameItem.MayRemove as quick-transfer is.
        public bool Move(ActGate gate, GameItem item, string to)
        {
            if (gate == null || item == null || !gate.StillOpen || !MayRemove(item)) return false;
            switch (to)
            {
                case Case: ToCase(item); return true;
                case Back: ToBack(item); return true;
                case Stock: ToStock(item); return true;
                case Ground: ToGround(item); return true;
                default: return false;
            }
        }

        // The calendar the morning report opens (AdvCalendarUIManager.OpenUIFromNightlyReport).
        public bool ClosePanel(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            CloseCalendar();
            return true;
        }

        // Opens in the morning and closes in the evening; StoreShutterButton.OnClick applies its own gates
        // (locks, arrest, shootout, CanStartDay/CanEndDay) and the confirm panel when customers still wait.
        public bool Shutter(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            ShutterClick();
            return true;
        }

        public bool RingBell(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            return Bell();
        }

        public bool Choose(ActGate gate, int index)
        {
            if (gate == null || !gate.StillOpen) return false;
            Select(index);
            return true;
        }

        public bool Advance(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            AdvancePressed();
            return true;
        }

        public bool OpenTablet(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            return Bedroll();
        }

        // EndOfDayUIManager.OnToggled for each service the plan changes, then the tablet rebuilt so its
        // checkboxes show the selection. Mandatory and cooling-down services are not the player's to toggle.
        public bool SetServices(ActGate gate, ISet<string> on)
        {
            if (gate == null || on == null || !gate.StillOpen) return false;
            Toggle(on);
            return true;
        }

        // The tablet's End Day button; the button is disabled while tonight's bill exceeds the cash.
        public bool Sleep(ActGate gate)
        {
            if (gate == null || !gate.StillOpen || !BillCovered()) return false;
            EndDay();
            return true;
        }

        // ---- M2 ----

        // An engine's move into a machine, bay or grid, or an item into the dossier, the bin or the ground: the
        // same GraphUtils.TryAcceptAll the ModHelper.MoveItemTo* calls make. Returns the units not placed (an item
        // that may not move places none), or -1 when the gate is closed.
        public int Accept(ActGate gate, GameItem item, GraphNodeStorage to, int amount)
        {
            if (gate == null || item == null || to == null || !gate.StillOpen) return -1;
            if (!MayRemove(item)) return Math.Max(1, amount);
            return AcceptAll(to, item, amount);
        }

        // An engine's machine action (a pour, a dissolve, a fermentation start, a vending upgrade): the helper the
        // machine's own button or item-target handler calls, built by the shared engine.
        public bool Chore<T>(ActGate gate, Func<T> act, out T result)
        {
            result = default;
            if (gate == null || act == null || !gate.StillOpen) return false;
            result = act();
            return true;
        }

        // The phone on the counter (StorePhoneObject.OnClick), the phone book's quick dial (AutoCall) when the
        // number will answer, its dialogue's choices and lines, and hanging up.
        public bool OpenPhone(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            PhoneOpen();
            return true;
        }

        public bool Dial(ActGate gate, long number)
        {
            if (gate == null || !gate.StillOpen || !WillAnswer(number)) return false;
            AutoCall(number);
            return true;
        }

        public bool PhoneChoose(ActGate gate, int index)
        {
            if (gate == null || !gate.StillOpen) return false;
            PhoneSelect(index);
            return true;
        }

        public bool PhoneAdvance(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            PhoneAdvancePressed();
            return true;
        }

        public bool ClosePhone(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            PhoneClose();
            return true;
        }

        // Wild's Network: the slot's click, then the window's Use button (both private; ModReflect, the
        // supported reflection surface), and the window's close.
        public bool WildBuy(ActGate gate, string id)
        {
            if (gate == null || string.IsNullOrEmpty(id) || !gate.StillOpen) return false;
            WildSelectAndUse(id);
            return true;
        }

        public bool WildClose(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            ModHelper.CloseWildNetwork();
            return true;
        }

        // The night's door (StoreDoor.OnClick applies its own gates), the go-outside confirm (staged items on
        // the back counter), the bag choice, the map's travel and the dump's search button.
        public bool OpenDoor(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            return DoorClick();
        }

        public bool ConfirmGoOutside(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            GoOutsideConfirm();
            return true;
        }

        // The afterhours response's first choice: going out without a backpack, as the gym does.
        public bool ChooseNoBag(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            return FirstResponseChoice();
        }

        public bool Travel(ActGate gate, AfterhourLocation where)
        {
            if (gate == null || where == null || !gate.StillOpen) return false;
            TravelTo(where);
            return true;
        }

        public bool Scavenge(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            ScavengeClick();
            return true;
        }

        // MapUIManager.Return through the supported ModHelper (it asks about items left on the ground), and the
        // prompt's confirm button.
        public bool Return(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            ModHelper.ReturnFromAfterhour(true);
            return true;
        }

        public bool LeaveOnGround(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            ConfirmPrompt();
            return true;
        }

        // The magnifier on an appraisal item (its onThisTargetItemFunc opens the inspection view), the
        // appraisal panel's price buttons, and its submit.
        public bool Inspect(ActGate gate, GameItem item)
        {
            if (gate == null || item == null || !gate.StillOpen) return false;
            InspectItem(item);
            return true;
        }

        public bool NudgeEstimate(ActGate gate, int delta)
        {
            if (gate == null || delta == 0 || !gate.StillOpen) return false;
            ChangeEstimate(delta);
            return true;
        }

        public bool SubmitEstimate(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            Estimated();
            return true;
        }

        // ---- M3 ----

        // The shotgun's click (Shotgun.OnClick raises the gun; the hands only press it while Shotgun.CanPickup and
        // the gun is down, so the click never lowers it, which would dismiss the customer), and the shot: Arm's
        // left click, StoreUIManager.Shoot behind StoreUIManager.CanShoot, at the customer the verdict was about.
        public bool RaiseGun(ActGate gate)
        {
            if (gate == null || !gate.StillOpen) return false;
            return GunClick();
        }

        public bool Fire(ActGate gate, object target)
        {
            if (gate == null || target == null || !gate.StillOpen) return false;
            return Trigger(target);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool GunClick()
        {
            var gun = UnityEngine.Object.FindObjectOfType<Shotgun>();
            if (gun == null || StoreUIManager.Instance.isShootingMode || !gun.CanPickup()) return false;
            gun.OnClick();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool Trigger(object target)
        {
            var ui = StoreUIManager.Instance;
            var at = PlayerStore.Instance.currentClientInstance?.GetClientBlueprint();
            if (!ui.isShootingMode || !ui.CanShoot() || at == null || !ReferenceEquals(at, target)) return false;
            ui.Shoot();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool MayRemove(GameItem item) => item.MayRemove();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static int AcceptAll(GraphNodeStorage to, GameItem item, int amount) => GraphUtils.TryAcceptAll(to, item, amount);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void PhoneOpen() => PhoneUIManager.Instance.OpenUI();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool WillAnswer(long number) => PhoneUIManager.WillAnswerCall(number);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void AutoCall(long number) => PhoneUIManager.Instance.AutoCall(number);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void PhoneSelect(int index) => DialoguePhoneUIManager.Instance.SelectChoice(index);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void PhoneAdvancePressed() => DialoguePhoneUIManager.Instance.OnAdvancePressed();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void PhoneClose() => PhoneUIManager.Instance.CloseUI();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void WildSelectAndUse(string id)
        {
            ModReflect.CallStatic(typeof(WildNetworkUI), "OnSlotClicked", id);
            ModReflect.CallStatic(typeof(WildNetworkUI), "OnUseClicked");
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool DoorClick()
        {
            var door = UnityEngine.Object.FindObjectOfType<StoreDoor>();
            if (door == null) return false;
            door.OnClick();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void GoOutsideConfirm() => MapUIManager.Instance.OnGoOutsideConfirm();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool FirstResponseChoice()
        {
            var choices = EmporiumEntry.Instance.choices;
            if (choices == null || choices.Count == 0 || choices[0].Item2 == null) return false;
            choices[0].Item2();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void TravelTo(AfterhourLocation where) => MapUIManager.Instance.Travel(where);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ScavengeClick() => MapUIManager.Instance.ScavengeButtonClick();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ConfirmPrompt() => GenericConfirmationUIManager.Instance.ConfirmButtonPressed();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void InspectItem(GameItem item) => InspectionUIManager.Instance.OnInspectItem(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ChangeEstimate(int delta) => InspectionUIManager.Instance.ChangeAppraisalServicePrice(delta);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Estimated() => InspectionUIManager.Instance.OnAppraisalServiceEstimated();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ToCase(GameItem item) => ModHelper.MoveItemToDisplayCase(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ToBack(GameItem item) => ModHelper.MoveItemToBackCounter(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ToStock(GameItem item) => ModHelper.MoveItemToInventory(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ToGround(GameItem item) => ModHelper.MoveItemToAfterhours(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void CloseCalendar() => AdvCalendarUIManager.Instance.CloseUI();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ShutterClick() => StoreShutterButton.Instance.OnClick();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool Bell()
        {
            var bell = UnityEngine.Object.FindObjectOfType<StoreBell>();
            if (bell == null) return false;
            bell.RingBell();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Select(int index) => DialogUIManager.Instance.SelectChoice(index);

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void AdvancePressed() => DialogUIManager.Instance.OnAdvancePressed();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool Bedroll()
        {
            var bed = UnityEngine.Object.FindObjectOfType<StoreBedroll>();
            if (bed == null) return false;
            bed.OnClick();
            return true;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Toggle(ISet<string> on)
        {
            foreach (var s in PlayerStore.Instance.storeService)
                if (s.unlocked && !s.isMandatory && !s.IsOnCooldown()) s.isUsing = on.Contains(s.id);
            EndOfDayUIManager.Instance.InitUI();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool BillCovered()
        {
            var ps = PlayerStore.Instance;
            int bill = 0;
            foreach (var s in ps.storeService)
                if (s.unlocked && s.isUsing) bill += s.cost;
            return bill <= 0 || bill <= ps.playerCash;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void EndDay() => EndOfDayUIManager.Instance.OnEndDayButtonClick();
    }
}
