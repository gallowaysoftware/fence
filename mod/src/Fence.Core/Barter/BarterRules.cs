using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Fence.Protocol;

namespace Fence.Barter
{
    // Which half of the tablet is showing (NegociationUIManager.ResolveMode). Only that half's trade
    // button exists, so it decides between sell_all and buy_all/accept_wholesale.
    public enum TabletMode
    {
        None,
        ClientBuys,
        ClientSells,
    }

    public static class OptionIds
    {
        public const string SellAll = "sell_all";
        public const string BuyAll = "buy_all";
        public const string AcceptWholesale = "accept_wholesale";
        public const string Blackmail = "blackmail";
        public const string Threaten = "threaten";
        public const string Dismiss = "dismiss";

        public static string Bargain(BargainKind kind, int percent)
        {
            switch (kind)
            {
                case BargainKind.Discount: return "offer_discount_" + percent;
                case BargainKind.Markup: return "add_markup_" + percent;
                case BargainKind.BuyingMarkup: return "offer_markup_" + percent;
                case BargainKind.BuyingDiscount: return "demand_discount_" + percent;
                case BargainKind.Blackmail: return Blackmail;
                default: return Threaten;
            }
        }
    }

    // Replicates the game's UI-only gates (docs/re/BARTER.md, "Buttons → methods"): the public
    // bargain and trade methods skip checks the buttons enforce, so the mod offers only what a player
    // could click right now.
    public static class BarterRules
    {
        public static readonly int[] PricePercents = { 10, 25, 50, 75 };
        public const int CoercionPercent = 20;

        public static bool IsBuying(ClientIntent i) => i == ClientIntent.Buy || i == ClientIntent.SellAndBuy;
        public static bool IsSelling(ClientIntent i) => i == ClientIntent.Sell || i == ClientIntent.SellAndBuy;

        // Null when the first integration covers this client; otherwise why it does not.
        public static string CoverageGap(ClientInfo c)
        {
            if (c == null) return "no client";
            if (c.Intent != ClientIntent.Buy && c.Intent != ClientIntent.Sell && c.Intent != ClientIntent.SellAndBuy)
                return "intent " + (c.RawIntent ?? c.Intent.ToString());
            if (!c.Dismissable) return "not dismissable";
            if (c.IsSecurity) return "security";
            // A poster's face is a special client; a stealth-wanted one has no poster, so the player serves him like
            // anyone else (docs/research/FAIR-PLAY.md).
            if (c.IsWanted && !c.IsStealthWanted) return "special client";
            return null;
        }

        // The predicate the game uses to enable the bargain buttons, plus intent-specific content
        // (docs/re/BARTER.md, "Decision and outcome points").
        public static bool DecisionReady(UiState ui, ClientInfo c, IList<ItemInfo> items)
        {
            if (ui == null || c == null) return false;
            if (!ui.ClientPresent || !ui.ClientArrived || ui.DialoguePlaying || ui.ClientMoving || ui.AboutToStartMainDialog || ui.TradeLocked) return false;
            if (!c.MainDialogueStarted || c.BusinessComplete) return false;
            if (c.Intent == ClientIntent.Sell) return items.Any(i => !i.Owned);
            return true;
        }

        public static TabletMode ResolveMode(ClientInfo c, IList<ItemInfo> items)
        {
            if (c == null || c.BusinessComplete) return TabletMode.None;
            switch (c.Intent)
            {
                case ClientIntent.Buy: return TabletMode.ClientBuys;
                case ClientIntent.Sell: return TabletMode.ClientSells;
                case ClientIntent.SellAndBuy:
                    bool wholesale = c.OffersWholesale && items.Any(i => !i.Owned);
                    var negotiated = items.FirstOrDefault(i => i.Negotiated && i.OnBack);
                    if (negotiated != null)
                    {
                        if (!negotiated.Owned) return TabletMode.ClientSells;
                        if (!wholesale || negotiated.ClientWants) return TabletMode.ClientBuys;
                    }
                    var ownedBack = items.Where(i => i.Owned && i.OnBack).ToList();
                    if (ownedBack.Count > 0 && (!wholesale || ownedBack.Any(i => i.ClientWants))) return TabletMode.ClientBuys;
                    if (items.Any(i => !i.Owned && i.OnBack)) return TabletMode.ClientSells;
                    return wholesale ? TabletMode.ClientSells : TabletMode.ClientBuys;
                default:
                    return TabletMode.None;
            }
        }

        // NegociationUIManager.RefreshBargainControl + BargainUIManager.RefreshUI.
        public static bool BargainPanelUsable(UiState ui, ClientInfo c) =>
            c != null && c.BargainAvailable && ui.BargainIntroduced && !ui.Outside && !ui.AwfulNegotiator
            && !ui.DialoguePlaying && !ui.ClientMoving && !ui.AboutToStartMainDialog && !ui.TradeLocked;

        // BargainUIManager.UpdateGroupVisibility + Can*: one bargain per side, no "nice" offer after
        // coercion, one blackmail, one threaten, never against security.
        public static bool CanBargain(BargainKind kind, ClientInfo c, BargainFlags f, UiState ui)
        {
            bool buying = IsBuying(c.Intent), selling = IsSelling(c.Intent);
            switch (kind)
            {
                case BargainKind.Discount: return buying && !f.SellSideUsed && !f.Coerced;
                case BargainKind.Markup: return buying && !f.SellSideUsed;
                case BargainKind.BuyingMarkup: return selling && !f.BuySideUsed && !f.Coerced;
                case BargainKind.BuyingDiscount: return selling && !f.BuySideUsed;
                case BargainKind.Blackmail: return !f.BlackmailOffered && !c.IsSecurity && (buying || selling);
                default: return ui.AugIntroduced && !f.ThreatenOffered && !c.IsSecurity && (buying || selling);
            }
        }

        public static List<DecisionOption> Options(StoreInfo store, ClientInfo c, IList<ItemInfo> items, BargainFlags flags, UiState ui, Func<BargainKind, int, Odds> odds)
        {
            var list = new List<DecisionOption>();
            if (c == null || ui == null) return list;
            var mode = ResolveMode(c, items);

            if (mode == TabletMode.ClientBuys && !ui.TradeLocked && !c.BusinessComplete)
            {
                var sellable = items.Where(i => i.Owned && i.OnBack && i.ClientWants).ToList();
                if (sellable.Count > 0)
                {
                    long sum = sellable.Sum(i => i.Price);
                    string label = "Sell " + Count(sellable.Count) + " for " + N(sum);
                    if (c.UsesBudget && sum > c.Budget) label += " (over their budget: they pay " + N(c.Budget) + ")";
                    list.Add(Option(OptionIds.SellAll, label));
                }
            }

            if (mode == TabletMode.ClientSells && !ui.TradeLocked && !c.BusinessComplete && store?.Cash != null)
            {
                int cash = store.Cash.Value;
                var buyable = items.Where(i => !i.Owned && i.OnBack).ToList();
                // With multi-buy disabled the button buys the row selected on the tablet, which is UI state we do not read.
                if (buyable.Count > 0 && !c.RegularBuyDisabled && !(c.MultiBuyDisabled && buyable.Count > 1))
                {
                    long sum = buyable.Sum(i => i.Price);
                    if (sum <= cash) list.Add(Option(OptionIds.BuyAll, "Buy " + Count(buyable.Count) + " for " + N(sum)));
                }
                int theirs = items.Count(i => !i.Owned);
                // OnWholesaleClick does not check cash; the button does.
                if (c.OffersWholesale && theirs > 0 && c.WholesalePrice.HasValue && cash >= c.WholesalePrice.Value)
                    list.Add(Option(OptionIds.AcceptWholesale,
                        "Accept wholesale: all " + Count(theirs) + " for " + N(c.WholesalePrice.Value) + " (" + c.WholesaleDiscount.ToString(CultureInfo.InvariantCulture) + "%)"));
            }

            if (BargainPanelUsable(ui, c))
            {
                foreach (var kind in new[] { BargainKind.Discount, BargainKind.Markup, BargainKind.BuyingMarkup, BargainKind.BuyingDiscount })
                {
                    if (!CanBargain(kind, c, flags, ui)) continue;
                    foreach (int pct in PricePercents)
                        list.Add(Option(OptionIds.Bargain(kind, pct), BargainLabel(kind, pct, odds(kind, pct), c)));
                }
                foreach (var kind in new[] { BargainKind.Blackmail, BargainKind.Threaten })
                {
                    if (CanBargain(kind, c, flags, ui))
                        list.Add(Option(OptionIds.Bargain(kind, CoercionPercent), BargainLabel(kind, CoercionPercent, odds(kind, CoercionPercent), c)));
                }
            }

            // StoreDismissButton.OnClick.
            if (ui.LockNextBell <= 0 && c.Dismissable) list.Add(Option(OptionIds.Dismiss, "Send the customer away"));
            return list;
        }

        public static string BargainLabel(BargainKind kind, int pct, Odds odds, ClientInfo c)
        {
            string p = pct.ToString(CultureInfo.InvariantCulture);
            switch (kind)
            {
                case BargainKind.Discount:
                    return "Offer " + p + "% discount (" + Accept(odds)
                        + (c.UsesBudget ? "; raises their budget " + (2 * pct).ToString(CultureInfo.InvariantCulture) + "%" : "") + ")";
                case BargainKind.Markup:
                    return "Add " + p + "% markup (" + Roll(odds) + ")";
                case BargainKind.BuyingMarkup:
                    return "Offer " + p + "% markup (" + Accept(odds) + ")";
                case BargainKind.BuyingDiscount:
                    return "Demand " + p + "% discount (" + Roll(odds) + ")";
                case BargainKind.Blackmail:
                    return "Blackmail for " + p + "% (accept " + N(odds.Success) + "% if they are a criminal, else -5 rep; "
                        + "if innocent: -7 rep, 50% they walk out -10 rep)";
                default:
                    return "Threaten for " + p + "% (accept " + N(odds.Success) + "% if they are an aug; "
                        + "if not: -7 rep, 50% they walk out -10 rep)";
            }
        }

        static string Accept(Odds o) => o.Success >= 100 ? "always accepted" : "accept " + N(o.Success) + "%";

        // The game rolls once: accept below Success, refuse-and-stay up to 100 - Leave, walk out above.
        // Stated as three exclusive shares like the game's own tooltip; models misread cumulative bands.
        static string Roll(Odds o)
        {
            if (o.Success >= 100) return "always accepted";
            if (o.Leave <= 0) return "accept " + N(o.Success) + "%, else refused and they stay";
            int refuse = System.Math.Max(0, 100 - o.Leave - o.Success);
            return "accept " + N(o.Success) + "%, refuse " + N(refuse) + "%, walk out " + N(o.Leave) + "% (-3 rep)";
        }

        static DecisionOption Option(string id, string label) => new DecisionOption { Id = id, Label = label };

        static string Count(int n) => N(n) + (n == 1 ? " item" : " items");

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
