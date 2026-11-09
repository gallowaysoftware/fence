using System;
using System.Runtime.CompilerServices;
using Fence.Barter;

namespace Fence.Mod
{
    // P2: the only code that presses the game's buttons (Fence.Tests.ActSafetyTests). It runs one
    // command on the main thread and only with an open ActGate; the caller has already re-read the
    // state and re-applied the UI gates (ActPlanner over a fresh BarterRules option set). Exceptions
    // propagate: the caller turns acting off for the session.
    sealed class BarterExecutor
    {
        public bool Execute(ActGate gate, ActCommand command)
        {
            if (gate == null) throw new ArgumentNullException(nameof(gate));
            if (command == null || !gate.StillOpen) return false;
            switch (command.Kind)
            {
                case ActKind.SellAll:
                    // The tablet's Sell button opens a confirm panel when the total exceeds the budget;
                    // the player then clicks Yes and the sale is capped at the budget.
                    bool overBudget = !ClientHasBudget();
                    SellAll();
                    if (overBudget && gate.StillOpen) ConfirmOverBudget();
                    return true;
                case ActKind.BuyAll:
                    BuyAll();
                    return true;
                case ActKind.Wholesale:
                    Wholesale();
                    return true;
                case ActKind.Dismiss:
                    Dismiss();
                    return true;
                case ActKind.Bargain:
                    if (Array.IndexOf(BarterRules.PricePercents, command.Percent) < 0 && command.Percent != BarterRules.CoercionPercent) return false;
                    Bargain(command.Bargain, command.Percent);
                    return true;
                default:
                    return false;
            }
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool ClientHasBudget() => NegociationUIManager.Instance.IsClientHaveEnoughToBuy();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void SellAll() => NegociationUIManager.Instance.OnSellAllClick();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ConfirmOverBudget() => NegociationUIManager.Instance.OnConfirmPanelYes();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void BuyAll() => NegociationUIManager.Instance.OnBuyAllClick();

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Wholesale() => NegociationUIManager.Instance.OnWholesaleClick();

        // StoreDismissButton.OnClick passes false: the next customer is not called automatically.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Dismiss() => PlayerStore.Instance.TryPlayerDismissCurrentClient(false);

        // BargainUIManager.OnBargainButton: the method, then RefreshUI.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static void Bargain(BargainKind kind, int percent)
        {
            var b = BargainUIManager.Instance;
            switch (kind)
            {
                case BargainKind.Discount: b.OfferDiscount(percent); break;
                case BargainKind.Markup: b.OfferMarkup(percent); break;
                case BargainKind.BuyingMarkup: b.OfferBuyingMarkup(percent); break;
                case BargainKind.BuyingDiscount: b.DemandBuyingDiscount(percent); break;
                case BargainKind.Blackmail: b.Blackmail(); break;
                case BargainKind.Threaten: b.ThreatenAug(); break;
            }
            b.RefreshUI();
        }
    }
}
