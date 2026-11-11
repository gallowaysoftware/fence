using System;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Autopilot;
using Fence.Barter;
using Fence.Gym;
using Fence.Protocol;
using Fence.Recording;

namespace Fence.Mod
{
    // The autopilot's reads of the live game: the LiveView the planner decides from, and the live objects the
    // executor's rules take (owned stock, the customer, the counter). Reads only; LiveHands presses.
    static class LiveGame
    {
        public static (LiveView view, BarterSnapshot snap) Read(GameBarter game, int clientSeq, bool loading)
        {
            var v = new LiveView { Loading = loading };
            if (!StoreUp()) return (v, null);
            v.InStore = true;
            Scene(v);
            if (!v.ClientPresent) return (v, null);
            var snap = BarterContext.Read(game, clientSeq);
            var c = snap.Client;
            if (c == null) return (v, snap);
            v.ClientKey = c.Key;
            v.ClientId = c.Identifier;
            v.ClientName = c.Name;
            v.ClientKind = CustomerKinds.Of(c);
            v.CoverageGap = snap.CoverageGap;
            v.CounterReady = snap.Ready;
            v.Intent = c.Intent;
            v.OffersWholesale = c.OffersWholesale;
            v.SellAllOffered = snap.Options != null && snap.Options.Any(o => o.Id == OptionIds.SellAll);
            v.CounterSignature = snap.Ready ? snap.Signature : null;
            v.TheirItemsOnCounter = TheirsOnCounter().Count > 0;
            Counter(v);
            return (v, snap);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool StoreUp() => PlayerStore.Instance != null && EmporiumEntry.Instance != null && StoreUIManager.Instance != null && DialogUIManager.Instance != null && StoreStation.Instance != null;

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Scene(LiveView v)
        {
            var ps = PlayerStore.Instance;
            var ui = StoreUIManager.Instance;
            var dialog = DialogUIManager.Instance;
            var map = MapUIManager.Instance;
            v.Day = StoreStation.Instance.dayCounter;
            v.GameOver = GameOverUIManager.Instance != null && GameOverUIManager.Instance.IsShowing;
            v.Outside = map != null && map.isOutside;
            switch (ps.storeState)
            {
                case PlayerStore.StoreState.MORNING: v.Store = StorePhase.Morning; break;
                case PlayerStore.StoreState.OPEN: v.Store = StorePhase.Open; break;
                case PlayerStore.StoreState.CLOSED: v.Store = StorePhase.Closed; break;
            }
            if (ui.blackscreen != null && ui.blackscreen.gameObject.activeSelf) v.BlockedBy = "blackscreen";
            else if (ui.isShootingMode) v.BlockedBy = "shooting mode";
            else if (map != null && map.IsInTransit) v.BlockedBy = "travelling";
            else if (ui.closeShutterConfirmationPanel != null && ui.closeShutterConfirmationPanel.activeSelf) v.BlockedBy = "the close-shutter confirm panel";
            else if (ps.isClientBeingArrested || ps.isHandlingShootout) v.BlockedBy = "an arrest or a shootout";
            v.ShootingMode = ui.isShootingMode;
            v.GunReady = ui.isShootingMode && ui.shootingUI != null && ui.shootingUI.activeSelf && ui.CanShoot();
            v.Shootout = ps.isHandlingShootout;
            v.MorningPanelOpen = AdvCalendarUIManager.Instance != null && AdvCalendarUIManager.Instance.UIPanel != null && AdvCalendarUIManager.Instance.UIPanel.activeSelf;
            v.ShutterLocked = ps.lockShutterButtonFlag > 0 || ps.isClientBeingArrested || ps.isHandlingShootout;
            v.Queue = ps.storeClientManager != null && ps.storeClientManager.clientStack != null ? ps.storeClientManager.clientStack.Count : 0;
            v.CanEndDay = ps.CanEndDay();
            v.ClientPresent = ps.currentClientInstance != null;
            v.ClientArrived = ps.isClientArrived;
            v.ClientMoving = ps.lockNextBellFlag > 0 || (StoreClientMono.Instance != null && StoreClientMono.Instance.IsMoving());
            v.ClientDismissable = ps.IsClientDimissable() && ps.lockNextBellFlag <= 0;
            var line = dialog.GetCurrentDialog();
            v.DialogueActive = line != null;
            v.ChoicesShown = line != null && line.choices != null && line.choices.Count > 0 && dialog.dialogChoiceBubbles != null
                             && dialog.dialogChoiceBubbles.Count > 0 && dialog.dialogChoiceBubbles[0] != null && dialog.dialogChoiceBubbles[0].gameObject.activeSelf;
            v.WaitingForAdvance = line != null && dialog.isWaitingForManualAdvance;
            v.LineChars = line == null ? 0 : (line.plainText ?? "").Length;
            if (line?.choices != null)
                foreach (var c in line.choices) v.LineChars += (c.Item2?.plainText ?? "").Length;
            v.PhoneDialogue = DialoguePhoneUIManager.Instance != null && DialoguePhoneUIManager.Instance.GetCurrentDialog() != null;
            v.TabletOpen = EndOfDayUIManager.Instance != null && EndOfDayUIManager.Instance.IsOpen;

            v.OutsideAt = map?.Current?.Id;
            v.GoOutsideConfirmOpen = map != null && map.confirmationPanel != null && map.confirmationPanel.activeSelf;
            // Only EmporiumEntry.ShowResponseWindow pauses the interactables (the afterhours bag choice, a crate's scan).
            v.ResponseOpen = Interactable.isPaused;
            v.MapOpen = TransitUI.IsOpen;
            var confirm = GenericConfirmationUIManager.Instance;
            v.LeavePromptOpen = confirm != null && confirm.panel != null && confirm.panel.activeSelf;
            var phone = PhoneUIManager.Instance;
            v.PhoneOpen = phone != null && phone.phonePanel != null && phone.phonePanel.activeSelf;
            var pd = DialoguePhoneUIManager.Instance;
            var call = pd?.GetCurrentDialog();
            v.PhoneDialogue = call != null;
            v.PhoneChoicesShown = call != null && call.choices != null && call.choices.Count > 0 && pd.dialogChoiceBubbles != null
                                  && pd.dialogChoiceBubbles.Count > 0 && pd.dialogChoiceBubbles[0] != null && pd.dialogChoiceBubbles[0].gameObject.activeSelf;
            v.PhoneWaitingForAdvance = call != null && pd.isWaitingForManualAdvance;
            if (call != null && v.LineChars == 0) v.LineChars = (call.plainText ?? "").Length;
            v.WildOpen = WildNetworkUI.IsOpen;
            var ins = InspectionUIManager.Instance;
            v.InspectionOpen = ins != null && ins.panel != null && ins.panel.activeSelf && ins.currentInspectingItem != null;
            // The appraisal field shows this tag (UpdateAppraisalPanel copies it in every frame; the price buttons write it).
            if (v.InspectionOpen) v.EstimateField = ins.currentInspectingItem.GetTagReadonly("APPRAISAL_DEFAULT_VALUE").GetInt();
        }

        // The counter's M2 cases: an appraisal client, the permit official's donation.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Counter(LiveView v)
        {
            var c = GameBarter.CurrentClient();
            if (c == null) return;
            v.AppraisalClient = c.clientIntent == StoreClient.ClientIntent.APPRAISAL_SERVICE;
            if (v.AppraisalClient && !v.InspectionOpen) v.AppraisalItemReady = NextEstimate() != null;
            v.BribeDue = PlayerCounter.BribeDue(c, PlayerStore.Instance.playerCash);
        }

        // The appraisal client's next item to estimate (shared: Appraisal.Next), or null.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Appraisal.Answer NextEstimate() => Appraisal.Next(GameBarter.CurrentClient(), EmporiumEntry.Instance.frontInvinvElement.childItems);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Inspectable(GameItem item) => item != null && item.IsTag("INSPECTABLE_TAG") && Appraisal.Owns("magnifier");

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool OnFront(GameItem item) => ReferenceEquals(item.parentInventory, EmporiumEntry.Instance.frontInvinvElement);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static object CurrentPhoneLine() => DialoguePhoneUIManager.Instance?.GetCurrentDialog();

        // The phone call's menu, as the role reads a dialogue.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int PhoneChoices() => DialoguePhoneUIManager.Instance?.GetCurrentDialog()?.choices?.Count ?? 0;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Unlocked(string upgrade) => NetworkUpgrade.GetUpgradeById(upgrade)?.IsUnlocked() ?? false;

        // The first afterhours choice is going out without a backpack (ResponseList.CreateAfterhoursResponse).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool FirstChoiceIsNoBag()
        {
            var choices = EmporiumEntry.Instance.choices;
            return choices != null && choices.Count > 0 && choices[0].Item1 != null && choices[0].Item1.name == LocHelper.GetLocalizedMechanic("response_afterhours_no_backpack");
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static AfterhourLocation Location(string id) => AfterhourLocation.GetDiscovered().FirstOrDefault(l => l != null && l.Id == id && l.IsAvailable());

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<string> Locations() => AfterhourLocation.GetDiscovered().Where(l => l != null && l.IsAvailable()).Select(l => l.Id).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static (int left, double woundPct) ScavengeOdds() => (ScavHelper.GetScavTimeLeft(), ScavHelper.GetMinorWoundChance() + ScavHelper.GetMajorWoundChance());

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> Ground() => Owned(EmporiumEntry.Instance.afterhourInventory).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static GameInventory GroundInventory() => EmporiumEntry.Instance.afterhourInventory;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> Stock() => Owned(EmporiumEntry.Instance.invElement).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static GameInventory Bin() => EmporiumEntry.Instance.trashcan?.contentWindow?.child as GameInventory;

        // Under `to` (an inventory or a machine) at any depth: where a chore's move should have put the item.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Under(GameItem item, object to)
        {
            for (var inv = item?.parentInventory; inv != null;)
            {
                if (ReferenceEquals(inv, to)) return true;
                var owner = inv.GetParentItem();
                if (owner == null) return false;
                if (ReferenceEquals(owner, to)) return true;
                inv = owner.parentInventory;
            }
            return false;
        }

        // The ledger the morning plan reads: net worth by day from the game's own run stats (the calendar's
        // wealth chart), and what the autopilot itself saw sold on the days it played.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<Newtonsoft.Json.Linq.JObject> Days(IDictionary<int, int> soldByDay)
        {
            var days = new List<Newtonsoft.Json.Linq.JObject>();
            var rs = RunStats.Current;
            if (rs == null || rs.seriesStartDay < 0) return days;
            for (int k = 0; k < rs.netWorthByDay.Count; k++)
            {
                int day = rs.seriesStartDay + k;
                var o = new Newtonsoft.Json.Linq.JObject { ["day"] = day, ["net_worth_start"] = k > 0 ? rs.netWorthByDay[k - 1] : rs.netWorthByDay[k], ["net_worth_end"] = rs.netWorthByDay[k] };
                if (soldByDay != null && soldByDay.TryGetValue(day, out int sold)) o["sold_value"] = sold;
                days.Add(o);
            }
            return days;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Fence.Gym.UpgradeView Wild(string via)
        {
            var v = WildRead.View();
            v.Via = via;
            return v;
        }

        // ---- the executor's inputs ----

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static StoreClient Client() => GameBarter.CurrentClient();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int Cash() => PlayerStore.Instance.playerCash;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int Crime() => PlayerStore.Instance.secData?.GetTotalCrimeValue() ?? 0;

        // Owned stock and display case, as the gym's Episode hands them to the role.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> StockAndCase()
        {
            var e = EmporiumEntry.Instance;
            return Owned(e.invElement).Concat(Owned(e.showcaseElement)).ToList();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> InCase() => Owned(EmporiumEntry.Instance.showcaseElement).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> OwnedOnBack() => Owned(EmporiumEntry.Instance.backInvinvElement).ToList();

        // Items on the back counter that are (or are not) the shop's.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> OnBack(bool owned) => EmporiumEntry.Instance.backInvinvElement.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) == owned).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int BribeOwed() => PlayerStore.Instance.storeClientData.businessPermitBribeAmount;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> OwnedOnFront() => Owned(EmporiumEntry.Instance.frontInvinvElement).ToList();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<GameItem> TheirsOnCounter()
        {
            var front = EmporiumEntry.Instance.frontInvinvElement;
            return front.childItems.Where(i => i != null && !GeneralHelper.IsItemOwned(i)).ToList();
        }

        static IEnumerable<GameItem> Owned(GameInventory inv) => inv?.childItems?.Where(i => i != null && GeneralHelper.IsItemOwned(i)) ?? Enumerable.Empty<GameItem>();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string Name(GameItem i) => i.GetDisplayName(false);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool IsIn(GameItem item, string place)
        {
            var e = EmporiumEntry.Instance;
            GameInventory target = place == LiveHands.Case ? e.showcaseElement : place == LiveHands.Back ? (GameInventory)e.backInvinvElement : place == LiveHands.Ground ? e.afterhourInventory : e.invElement;
            return ReferenceEquals(item.parentInventory, target);
        }

        // The dialogue on screen as the executor's role reads it (Episode.Choose).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Fence.Gym.DialogueView Dialogue(out List<ChoiceInfo> infos)
        {
            infos = new List<ChoiceInfo>();
            var d = DialogUIManager.Instance.GetCurrentDialog();
            var c = GameBarter.CurrentClient();
            var ps = PlayerStore.Instance;
            var view = new Fence.Gym.DialogueView
            {
                ClientId = c?.identifier,
                ClientName = c?.displayName,
                Line = d?.plainText,
                Cash = ps.playerCash,
                RentDue = ps.GetRentValue(),
            };
            var owned = EmporiumEntry.Instance.GetAllOwnedItems(false, true);
            view.HoldsSeriousContraband = owned.Any(i => i != null && ContrabandHelper.IsSeriousContraband(i));
            view.HoldsStolen = owned.Any(i => i != null && StolenHelper.IsStolenItem(i));
            if (d?.choices == null) return view;
            for (int i = 0; i < d.choices.Count; i++)
            {
                var (token, reply, _) = d.choices[i];
                bool enabled = d.IsChoiceEnabledAt(i);
                view.Choices.Add(new Fence.Gym.ChoiceView { Index = i, Label = token?.name, Text = reply?.plainText, Enabled = enabled });
                infos.Add(new ChoiceInfo { Label = token?.name, Text = reply?.plainText, Enabled = enabled });
            }
            return view;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static object CurrentLine() => DialogUIManager.Instance.GetCurrentDialog();

        // The tablet's services, as the executor's role reads them (Episode.View).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Fence.Gym.NightView Night(int day)
        {
            var ps = PlayerStore.Instance;
            var view = new Fence.Gym.NightView { Day = day, Cash = ps.playerCash, RentDue = ps.GetRentValue() };
            foreach (var s in ps.storeService)
                view.Services.Add(new Fence.Gym.ServiceView { Id = s.id, Cost = s.cost, Using = s.isUsing, Mandatory = s.isMandatory, Unlocked = s.unlocked, Cooldown = s.currentCooldown });
            return view;
        }

        // Rent, services and the mortgage, for the day's plan.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static (int rent, int daysLeft, int services, bool mortgage) Rent()
        {
            var ps = PlayerStore.Instance;
            int svc = ps.storeService.Where(s => s.unlocked && s.isUsing).Sum(s => s.cost);
            return (ps.isPayingMortgage ? ps.mortgageWeeklyPayment : ps.GetRentValue(), ps.dayUntilRent, svc, ps.isPayingMortgage);
        }
    }
}
