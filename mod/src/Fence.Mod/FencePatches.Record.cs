using System;
using System.Collections.Generic;
using Fence.Barter;
using Fence.Recording;
using HarmonyLib;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // Phase D recording patches (compat/hooks.json, features record.*). Same rules as the barter
    // patches: applied one by one, observation only, every handler swallows through Recorder.Guard.
    // The tablet buttons, bargain and dismiss methods appear here only as patch targets.
    static partial class FencePatches
    {
        internal static Recorder Recorder;

        static readonly List<Spec> RecordSpecs = new List<Spec>
        {
            new Spec
            {
                HookId = "human.stage",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AddedItemToWeightedArea", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(StagePostfix)),
            },
            new Spec
            {
                HookId = "human.unstage",
                Target = () => AccessTools.Method(typeof(PlayerStore), "RemovedItemToWeightedArea", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(UnstagePostfix)),
            },
            new Spec
            {
                HookId = "human.front_add",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AddedItemToClientArea", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(FrontAddPostfix)),
            },
            new Spec
            {
                HookId = "human.front_remove",
                Target = () => AccessTools.Method(typeof(PlayerStore), "RemovedItemToClientArea", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(FrontRemovePostfix)),
            },
            new Spec
            {
                HookId = "human.showcase_add",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AddedItemToDisplayCase", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ShowcaseAddPostfix)),
            },
            new Spec
            {
                HookId = "human.showcase_remove",
                Target = () => AccessTools.Method(typeof(PlayerStore), "RemovedItemToDisplayCase", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ShowcaseRemovePostfix)),
            },
            new Spec
            {
                HookId = "record.scope.dismiss",
                Target = () => AccessTools.Method(typeof(PlayerStore), "DismissCurrentClient", new[] { typeof(bool), typeof(bool), typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.remove_not_owned",
                Target = () => AccessTools.Method(typeof(PlayerStore), "RemoveNotOwnedItemFromTable", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.buy_item",
                Target = () => AccessTools.Method(typeof(PlayerStore), "BuyItem", new[] { typeof(GameItem) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.sell_item",
                Target = () => AccessTools.Method(typeof(PlayerStore), "SellItem", new[] { typeof(GameItem) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.accept_barter",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AcceptBarter", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.add_weighted",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AddDirectToWeightedTable", new[] { typeof(GameItem), typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.move_deal",
                Target = () => AccessTools.Method(typeof(PlayerStore), "MoveDealItemToInventory", new[] { typeof(GameItem) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.add_selling",
                Target = () => AccessTools.Method(typeof(PlayerStore), "TryAddDirectSellingItemToTable", new[] { typeof(GameItem), typeof(bool), typeof(bool), typeof(bool), typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.add_middle",
                Target = () => AccessTools.Method(typeof(PlayerStore), "AddDirectSellingItemToTableMiddle", new[] { typeof(GameItem), typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.end_night",
                Target = () => AccessTools.Method(typeof(PlayerStore), "EndNight", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.begin_day",
                Target = () => AccessTools.Method(typeof(PlayerStore), "BeginDay", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.clear_back",
                Target = () => AccessTools.Method(typeof(EmporiumEntry), "ClearBackInv", new[] { typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "record.scope.place_inventor",
                Target = () => AccessTools.Method(typeof(StoreClientList), "PlaceInventorInventory", new[] { typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeEnter)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "human.client_click",
                Target = () => AccessTools.Method(typeof(StoreClientMono), "OnClick", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(OpenTabletPostfix)),
            },
            new Spec
            {
                HookId = "human.tablet_close",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "CloseUI", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(CloseTabletPrefix)),
            },
            new Spec
            {
                HookId = "human.feature_click",
                Target = () => AccessTools.Method(typeof(ItemFeatureUI), "OnFeatureUIClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(FeatureClickPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(FeatureClickPostfix)),
            },
            new Spec
            {
                HookId = "human.feature_bad_match",
                Target = () => AccessTools.Method(typeof(ItemFeatureUI), "OnExposeFeatureBadMatch", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(BadMatchPostfix)),
            },
            new Spec
            {
                HookId = "human.notebook_expose",
                Target = () => AccessTools.Method(typeof(NotebookElement), "ElementClicked", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(NotebookPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(NotebookPostfix)),
            },
            new Spec
            {
                HookId = "human.inspect_item",
                Target = () => AccessTools.Method(typeof(InspectionUIManager), "OnInspectItem", new[] { typeof(GameItem) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectItemPrefix)),
            },
            new Spec
            {
                HookId = "human.confront",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackConfront", new[] { typeof(string), typeof(string), typeof(string) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ConfrontPostfix)),
            },
            new Spec
            {
                HookId = "record.human.buy_all",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "OnBuyAllClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BuyAllClickPrefix)),
            },
            new Spec
            {
                HookId = "record.human.sell_all",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "OnSellAllClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(SellAllClickPrefix)),
            },
            new Spec
            {
                HookId = "record.human.wholesale",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "OnWholesaleClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(WholesaleClickPrefix)),
            },
            new Spec
            {
                HookId = "record.human.confirm_over_budget",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "OnConfirmPanelYes", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ConfirmClickPrefix)),
            },
            new Spec
            {
                HookId = "human.accept_offer",
                Target = () => AccessTools.Method(typeof(NegociationUIManager), "OnOfferAcceptClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(AcceptOfferPrefix)),
            },
            new Spec
            {
                HookId = "human.ring_bell",
                Target = () => AccessTools.Method(typeof(StoreBell), "RingBell", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(RingBellPrefix)),
            },
            new Spec
            {
                HookId = "human.dialogue_choice",
                Target = () => AccessTools.Method(typeof(DialogUIManager), "SelectChoice", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(DialogueChoicePrefix)),
            },
            new Spec
            {
                HookId = "night.phone_choice",
                Target = () => AccessTools.Method(typeof(DialoguePhoneUIManager), "SelectChoice", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(PhoneChoicePrefix)),
            },
            new Spec
            {
                HookId = "night.queue_client",
                Target = () => AccessTools.Method(typeof(PlayerStore), "QueueFuturClient", new[] { typeof(string), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(QueueClientPostfix)),
            },
            new Spec
            {
                HookId = "human.report",
                Target = () => AccessTools.Method(typeof(PlayerStore), "OnCustomerReported", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ReportPrefix)),
            },
            new Spec
            {
                HookId = "human.report_outcome",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackCustomerReported", new[] { typeof(StoreClient) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ReportedPostfix)),
            },
            new Spec
            {
                HookId = "human.wrongful_report",
                Target = () => AccessTools.Method(typeof(RunStats), "OnWrongfulReport", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(WrongfulReportPostfix)),
            },
            new Spec
            {
                HookId = "customer.wanted_not_reported",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackWantedNotReported", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(WantedNotReportedPostfix)),
            },
            new Spec
            {
                HookId = "human.shoot",
                Target = () => AccessTools.Method(typeof(PlayerStore), "OnWantedClientShot", new[] { typeof(StoreClient) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(AugResultPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ShotPostfix)),
            },
            new Spec
            {
                HookId = "human.aug_shot",
                Target = () => AccessTools.Method(typeof(RunStats), "OnAugShot", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(AugShotPostfix)),
            },
            new Spec
            {
                HookId = "human.appraisal_estimate",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "OnEstimate", new[] { typeof(GameItem), typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(EstimatePrefix)),
            },
            new Spec
            {
                HookId = "human.appraisal_sent",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackAppraisalSent", new[] { typeof(GameItem), typeof(int), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(AppraisalSentPostfix)),
            },
            new Spec
            {
                HookId = "night.poster_down",
                Target = () => AccessTools.Method(typeof(StorePoster), "PullDown", new[] { typeof(bool) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PosterPostfix)),
            },
            new Spec
            {
                HookId = "night.poster_up",
                Target = () => AccessTools.Method(typeof(StorePoster), "PullUp", new[] { typeof(bool) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PosterPostfix)),
            },
            new Spec
            {
                HookId = "night.sleep",
                Target = () => AccessTools.Method(typeof(EndOfDayUIManager), "OnEndDayButtonClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(SleepPrefix)),
            },
            new Spec
            {
                HookId = "night.upgrade_cash",
                Target = () => AccessTools.Method(typeof(RunStats), "OnWildUpgradeBought", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(UpgradeCashPostfix)),
            },
            new Spec
            {
                HookId = "night.upgrade_bought",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackWildUpgradePurchased", new[] { typeof(string) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(UpgradeBoughtPostfix)),
            },
            new Spec
            {
                HookId = "night.travel",
                Target = () => AccessTools.Method(typeof(MapUIManager), "Travel", new[] { typeof(AfterhourLocation) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(TravelPrefix)),
            },
            new Spec
            {
                HookId = "night.return",
                Target = () => AccessTools.Method(typeof(MapUIManager), "Return", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ReturnPrefix)),
            },
            new Spec
            {
                HookId = "night.scavenge",
                Target = () => AccessTools.Method(typeof(ScavHelper), "ScavengeDumpingGrounds", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ScavengePrefix)),
            },
            new Spec
            {
                HookId = "night.scavenge_attempt",
                Target = () => AccessTools.Method(typeof(RunStats), "OnScavengeAttempt", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ScavengeAttemptPostfix)),
            },
            new Spec
            {
                HookId = "night.scavenge_injury",
                Target = () => AccessTools.Method(typeof(RunStats), "OnScavengeInjury", new[] { typeof(bool) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ScavengeInjuryPostfix)),
            },
            new Spec
            {
                HookId = "night.scavenged",
                Target = () => AccessTools.Method(typeof(RunStats), "OnScavengedItems", new[] { typeof(List<GameItem>) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ScavengedPostfix)),
            },
            new Spec
            {
                HookId = "night.commissary_buy",
                Target = () => AccessTools.Method(typeof(CommissaryData), "BuyItem", new[] { typeof(string) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(CommissaryBuyPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(CommissaryBuyPostfix)),
            },
            new Spec
            {
                HookId = "night.commissary_redeem",
                Target = () => AccessTools.Method(typeof(CommissaryUIManager), "OnRedeemAllButtonClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(RedeemPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(RedeemPostfix)),
            },
            new Spec
            {
                HookId = "night.fill_request",
                Target = () => AccessTools.Method(typeof(StoreRequest), "FillRequest", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(FillRequestPrefix)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "run.continue",
                Target = () => AccessTools.Method(typeof(GameOverUIManager), "ContinuePlayClick", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ContinuePrefix)),
            },
            new Spec
            {
                HookId = "night.service_paid",
                Target = () => AccessTools.Method(typeof(RunStats), "OnServicePaid", new[] { typeof(string), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ServicePaidPostfix)),
            },
            new Spec
            {
                HookId = "night.rent_paid",
                Target = () => AccessTools.Method(typeof(RunStats), "OnRentPaid", new[] { typeof(int), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(RentPaidPostfix)),
            },
            new Spec
            {
                HookId = "night.rent_missed",
                Target = () => AccessTools.Method(typeof(RunStats), "OnRentMissed", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(RentMissedPostfix)),
            },
            new Spec
            {
                HookId = "night.store_bought",
                Target = () => AccessTools.Method(typeof(RunStats), "OnStoreBoughtWithCash", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(StoreBoughtPostfix)),
            },
            new Spec
            {
                HookId = "night.mortgage_taken",
                Target = () => AccessTools.Method(typeof(RunStats), "OnMortgageTaken", new[] { typeof(int), typeof(int), typeof(int), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(MortgageTakenPostfix)),
            },
            new Spec
            {
                HookId = "night.mortgage_payment",
                Target = () => AccessTools.Method(typeof(RunStats), "OnMortgagePayment", new[] { typeof(int), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(MortgagePaymentPostfix)),
            },
            new Spec
            {
                HookId = "night.mortgage_paid_off",
                Target = () => AccessTools.Method(typeof(RunStats), "OnMortgagePaidOff", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(MortgagePaidOffPostfix)),
            },
            new Spec
            {
                HookId = "night.georgio_loan",
                Target = () => AccessTools.Method(typeof(RunStats), "OnGeorgioLoan", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(GeorgioLoanPostfix)),
            },
            new Spec
            {
                HookId = "night.georgio_payback",
                Target = () => AccessTools.Method(typeof(RunStats), "OnGeorgioPayback", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(GeorgioPaybackPostfix)),
            },
            new Spec
            {
                HookId = "night.vending_sale",
                Target = () => AccessTools.Method(typeof(RunStats), "OnVendingSale", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(VendingSalePostfix)),
            },
            new Spec
            {
                HookId = "night.fountain_sale",
                Target = () => AccessTools.Method(typeof(RunStats), "OnFountainSale", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(FountainSalePostfix)),
            },
            new Spec
            {
                HookId = "night.burglary",
                Target = () => AccessTools.Method(typeof(RunStats), "OnBurglary", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(BurglaryPostfix)),
            },
            new Spec
            {
                HookId = "night.day_ended",
                Target = () => AccessTools.Method(typeof(RunStats), "OnDayEnded", new[] { typeof(int), typeof(int), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(DayEndedPostfix)),
            },
            new Spec
            {
                HookId = "customer.inspector_bribe_stats",
                Target = () => AccessTools.Method(typeof(RunStats), "OnInspectorBribe", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectorBribePostfix)),
            },
            new Spec
            {
                HookId = "money.permit_bought",
                Target = () => AccessTools.Method(typeof(RunStats), "OnPermitBought", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PermitBoughtPostfix)),
            },
            new Spec
            {
                HookId = "money.permit_bribe",
                Target = () => AccessTools.Method(typeof(RunStats), "OnPermitBribe", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PermitBribePostfix)),
            },
            new Spec
            {
                HookId = "money.item_sold",
                Target = () => AccessTools.Method(typeof(RunStats), "OnItemSold", new[] { typeof(GameItem), typeof(int), typeof(int), typeof(StoreClient) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ItemSoldPostfix)),
            },
            new Spec
            {
                HookId = "money.item_bought",
                Target = () => AccessTools.Method(typeof(RunStats), "OnItemBought", new[] { typeof(GameItem), typeof(int), typeof(StoreClient) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ItemBoughtPostfix)),
            },
            new Spec
            {
                HookId = "customer.inspector_arrived",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackInspectorArrived", new[] { typeof(int), typeof(bool) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectorArrivedPostfix)),
            },
            new Spec
            {
                HookId = "customer.inspection_bribe",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackInspectionBribe", new[] { typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectionBribePostfix)),
            },
            new Spec
            {
                HookId = "customer.inspection_start",
                Target = () => AccessTools.Method(typeof(ContrabandHelper), "StartInspection", new[] { typeof(int), typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectionStartPrefix)),
                Finalizer = () => AccessTools.Method(typeof(FencePatches), nameof(GameScopeExit)),
            },
            new Spec
            {
                HookId = "customer.inspected",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackOnInspected", new[] { typeof(string), typeof(int), typeof(int), typeof(bool) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectedPostfix)),
            },
            new Spec
            {
                HookId = "customer.inspection_stats",
                Target = () => AccessTools.Method(typeof(RunStats), "OnInspection", new[] { typeof(bool), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectionStatsPostfix)),
            },
            new Spec
            {
                HookId = "run.initial_save",
                Target = () => AccessTools.Method(typeof(PlayerStore), "InitialSave", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InitialSavePostfix)),
            },
            new Spec
            {
                HookId = "run.game_over",
                Target = () => AccessTools.Method(typeof(PlayerStore), "ExecuteGameOver", new[] { typeof(string) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(GameOverPrefix)),
            },
            new Spec
            {
                HookId = "record.prediction_capture",
                Target = () => AccessTools.Method(typeof(ShowcaseHelper), "GetDisplayCasePrediction", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PredictionPostfix)),
            },
        };

        static void Rec(string hook, Action<Recorder> a)
        {
            var r = Recorder;
            if (r == null) return;
            try
            {
                r.Guard(hook, () => a(r));
            }
            catch (Exception)
            {
            }
        }

        static void GameScopeEnter() => Rec("scope", r => r.Filter.EnterGame());

        static void GameScopeExit() => Rec("scope", r => r.Filter.ExitGame());

        static void StagePostfix(GameItem gameItem) => Rec("stage", r => r.CounterMove(HumanActions.Stage, gameItem, false));

        static void UnstagePostfix(GameItem gameItem) => Rec("unstage", r => r.CounterMove(HumanActions.Unstage, gameItem, false));

        static void FrontAddPostfix(GameItem gameItem) => Rec("front_add", r => r.CounterMove(HumanActions.FrontAdd, gameItem, true));

        static void FrontRemovePostfix(GameItem gameItem) => Rec("front_remove", r => r.CounterMove(HumanActions.FrontRemove, gameItem, true));

        static void ShowcaseAddPostfix(GameItem gameItem) => Rec("showcase", r => r.ShowcaseMove(HumanActions.Showcase, gameItem));

        static void ShowcaseRemovePostfix(GameItem gameItem) => Rec("unshowcase", r => r.ShowcaseMove(HumanActions.Unshowcase, gameItem));

        static void OpenTabletPostfix() => Rec("open_tablet", r => r.Tablet(HumanActions.OpenTablet));

        static void CloseTabletPrefix() => Rec("close_tablet", r => r.Tablet(HumanActions.CloseTablet));

        static void FeatureClickPrefix(ItemFeatureUI __instance, out object __state)
        {
            object attempt = null;
            Rec("expose", r => attempt = r.BeforeExpose(__instance.itemFeature, false));
            __state = attempt;
        }

        static void FeatureClickPostfix(ItemFeatureUI __instance, object __state) => Rec("expose", r => r.AfterExpose(__state, __instance.itemFeature));

        static void BadMatchPostfix() => Rec("expose", r => r.ExposeBadMatch());

        static void NotebookPrefix(NotebookElement __instance, out object __state)
        {
            object attempt = null;
            Rec("notebook", r => attempt = r.BeforeExpose(__instance.itemFeature, true));
            __state = attempt;
        }

        static void NotebookPostfix(NotebookElement __instance, object __state) => Rec("notebook", r => r.AfterExpose(__state, __instance.itemFeature));

        static void InspectItemPrefix(GameItem gameItem) => Rec("inspect_item", r => r.InspectItem(gameItem));

        static void ConfrontPostfix(string itemId, string confrontType, string outcome) => Rec("confront", r => r.Confront(itemId, confrontType, outcome));

        static void BuyAllClickPrefix() => Rec("buy_all", r => r.Click(OptionIds.BuyAll));

        static void SellAllClickPrefix() => Rec("sell_all", r => r.Click(OptionIds.SellAll));

        static void WholesaleClickPrefix() => Rec("wholesale", r => r.Click(OptionIds.AcceptWholesale));

        static void ConfirmClickPrefix() => Rec("confirm_over_budget", r => r.Click(HumanActions.ConfirmOverBudget));

        static void AcceptOfferPrefix() => Rec("accept_offer", r => r.Click(HumanActions.AcceptOffer, new JObject { ["kind"] = r.CurrentKind() }));

        static void RingBellPrefix() => Rec("ring_bell", r => r.Click(HumanActions.RingBell));

        static void DialogueChoicePrefix(DialogUIManager __instance, int index) => Rec("dialogue_choice", r => r.DialogueChoice(index, __instance.GetCurrentDialog(), false));

        static void PhoneChoicePrefix(DialoguePhoneUIManager __instance, int index) => Rec("phone_choice", r => r.DialogueChoice(index, __instance.GetCurrentDialog(), true));

        static void QueueClientPostfix(string storeClientId, int day) => Rec("phone_order", r => r.CourierQueued(storeClientId, day));

        static void ReportPrefix()
        {
            Rec("report", r => r.Report());
            Aug(a => a.Reported());
        }

        static void ReportedPostfix(StoreClient storeClient) => Rec("reported", r => r.RunStat("customer_reported", new JObject { ["name"] = GameRecord.ClientName(storeClient) }));

        static void WrongfulReportPostfix() => Rec("wrongful_report", r => r.RunStat("wrongful_report"));

        static void WantedNotReportedPostfix() => Rec("wanted_not_reported", r => r.RunStat("wanted_not_reported"));

        static void ShotPostfix(StoreClient storeClient) => Aug(a => a.WantedCapture(GameRecord.ClientName(storeClient)));

        static void AugShotPostfix() => Rec("aug_shot", r => r.RunStat("aug_shot"));

        static void EstimatePrefix(GameItem gameItem, int value) => Rec("appraisal_estimate", r => r.AppraisalEstimate(gameItem, value));

        static void AppraisalSentPostfix(GameItem gameItem, int sentValue, int actualValue) => Rec("appraisal_sent", r => r.AppraisalSent(gameItem, sentValue, actualValue));

        static void PosterPostfix(bool mute) => Rec("poster", r => r.Poster(mute));

        static void SleepPrefix() => Rec("sleep", r => r.Sleep());

        static void UpgradeCashPostfix(int cashSpent) => Rec("upgrade_cash", r => r.UpgradeCash(cashSpent));

        static void UpgradeBoughtPostfix(string upgradeId) => Rec("upgrade_buy", r => r.UpgradeBought(upgradeId));

        static void TravelPrefix(AfterhourLocation location) => Rec("go_out", r => r.Travel(location));

        static void ReturnPrefix() => Rec("return", r => r.Return());

        static void ScavengePrefix() => Rec("scavenge", r => r.Scavenge());

        static void ScavengeAttemptPostfix() => Rec("scavenge_attempt", r => r.RunStat("scavenge_attempt"));

        static void ScavengeInjuryPostfix(bool major) => Rec("scavenge_injury", r => r.RunStat("scavenge_injury", new JObject { ["major"] = major }));

        static void ScavengedPostfix(List<GameItem> items) => Rec("scavenged", r => r.Scavenged(items));

        static void CommissaryBuyPrefix(CommissaryData __instance) => Rec("commissary_buy", r => r.CommissaryBefore(__instance));

        static void CommissaryBuyPostfix(CommissaryData __instance, string id) => Rec("commissary_buy", r => r.CommissaryBought(id, __instance));

        static void RedeemPrefix() => Rec("commissary_redeem", r => r.CommissaryBefore(GameRecord.Commissary()));

        static void RedeemPostfix() => Rec("commissary_redeem", r => r.CommissaryRedeemed());

        static void FillRequestPrefix(StoreRequest __instance) => Rec("fill_request", r =>
        {
            r.Filter.EnterGame();
            r.FillRequest(__instance);
        });

        static void ContinuePrefix() => Rec("continue_play", r => r.Click(HumanActions.ContinuePlay));

        static void Money(string kind, JObject amounts) => Rec("money." + kind, r => r.Money(kind, amounts));

        static void ServicePaidPostfix(string serviceId, int cost) => Money(MoneyKinds.ServicePaid, new JObject { ["service"] = serviceId, ["cost"] = cost });

        static void RentPaidPostfix(int amount, int creditsLeft) => Money(MoneyKinds.RentPaid, new JObject { ["amount"] = amount, ["credits_left"] = creditsLeft });

        static void RentMissedPostfix() => Money(MoneyKinds.RentMissed, null);

        static void StoreBoughtPostfix(int price) => Money(MoneyKinds.StoreBought, new JObject { ["price"] = price });

        static void MortgageTakenPostfix(int price, int downPayment, int weeklyPayment, int weeks) =>
            Money(MoneyKinds.MortgageTaken, new JObject { ["price"] = price, ["down_payment"] = downPayment, ["weekly"] = weeklyPayment, ["weeks"] = weeks });

        static void MortgagePaymentPostfix(int amount, int creditsLeft) => Money(MoneyKinds.MortgagePayment, new JObject { ["amount"] = amount, ["credits_left"] = creditsLeft });

        static void MortgagePaidOffPostfix() => Money(MoneyKinds.MortgagePaidOff, null);

        static void GeorgioLoanPostfix(int amount) => Money(MoneyKinds.GeorgioLoan, new JObject { ["amount"] = amount });

        static void GeorgioPaybackPostfix(int amount) => Money(MoneyKinds.GeorgioPayback, new JObject { ["amount"] = amount });

        static void VendingSalePostfix(int value) => Money(MoneyKinds.VendingSale, new JObject { ["value"] = value });

        static void FountainSalePostfix(int value) => Money(MoneyKinds.FountainSale, new JObject { ["value"] = value });

        static void BurglaryPostfix(int value) => Money(MoneyKinds.Burglary, new JObject { ["value"] = value });

        static void DayEndedPostfix(int day, int netWorth, int credits) => Money(MoneyKinds.DayEnded, new JObject { ["day"] = day, ["net_worth"] = netWorth, ["credits"] = credits });

        static void InspectorBribePostfix(int amount) => Money(MoneyKinds.InspectorBribe, new JObject { ["amount"] = amount });

        static void PermitBoughtPostfix(int price) => Money(MoneyKinds.PermitBought, new JObject { ["price"] = price });

        static void PermitBribePostfix(int value) => Money(MoneyKinds.PermitBribe, new JObject { ["value"] = value });

        static void ItemSoldPostfix(GameItem item, int cost, int boughtFor, StoreClient client) => Rec("money.item_sold", r => r.ItemMoney(MoneyKinds.ItemSold, item, cost, boughtFor, client));

        static void ItemBoughtPostfix(GameItem item, int cost, StoreClient client) => Rec("money.item_bought", r => r.ItemMoney(MoneyKinds.ItemBought, item, cost, null, client));

        static void InspectorArrivedPostfix(int personality, bool fromShowcase) => Rec("inspector_arrived", r => r.InspectorArrived(personality, fromShowcase));

        static void InspectionBribePostfix(int bribeAmount) => Rec("inspection_bribe", r => r.InspectionBribe(bribeAmount));

        static void InspectionStartPrefix(int personality, bool fromShowcase) => Rec("inspection_search", r =>
        {
            r.Filter.EnterGame();
            r.InspectionSearch(personality, fromShowcase);
        });

        static void InspectedPostfix(string outcomeType, int lostCount, int lostValue, bool usedGrace) => Rec("inspected", r => r.Inspected(outcomeType, lostCount, lostValue, usedGrace));

        static void InspectionStatsPostfix(bool passed, int seizedValue) => Rec("inspection_stats", r => r.InspectionStats(passed, seizedValue));

        static void InitialSavePostfix() => Rec("run_start", r => r.RunStarted());

        static void GameOverPrefix(string ending) => Rec("run_end", r => r.GameOver(ending));

        static void PredictionPostfix(List<(string, int, string)> __result) => Rec("predictions", r => r.PredictionsComputed(__result));
    }
}
