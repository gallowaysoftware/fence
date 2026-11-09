using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using Fence.Barter;
using Fence.Mod;
using Fence.Protocol;

namespace Fence.Gym
{
    // Checks Fence.Core's replica of the tablet's UI gates (BarterRules) against the game's own code
    // on a live customer: the tablet mode (NegociationUIManager.ResolveMode), each bargain's Can*
    // guard (BargainUIManager), and the trade buttons' item sets. A mismatch means the live mod would
    // offer a move the game would not enable, or hide one it would.
    internal static class Parity
    {
        const BindingFlags I = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        const BindingFlags S = BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic;

        static readonly (BargainKind kind, string method)[] Guards =
        {
            (BargainKind.Discount, "CanOfferDiscount"), (BargainKind.Markup, "CanOfferMarkup"), (BargainKind.BuyingMarkup, "CanOfferBuyingMarkup"),
            (BargainKind.BuyingDiscount, "CanDemandBuyingDiscount"), (BargainKind.Blackmail, "CanBlackmail"), (BargainKind.Threaten, "CanThreatenAug"),
        };

        public static List<string> Check(BarterSnapshot snap, DecideRequest request, GameBarter reads)
        {
            var problems = new List<string>();
            var client = GameBarter.CurrentClient();
            var items = reads.CounterItems();
            var ui = reads.Ui();
            var nego = NegociationUIManager.Instance;

            var gameMode = typeof(NegociationUIManager).GetMethod("ResolveMode", I).Invoke(nego, new object[] { client }).ToString();
            var coreMode = BarterRules.ResolveMode(snap.Client, items);
            string want = gameMode == "Buying" ? "ClientBuys" : gameMode == "Selling" ? "ClientSells" : "None";
            if (want != coreMode.ToString()) problems.Add($"mode: game {gameMode}, Fence.Core {coreMode}");

            var ids = new HashSet<string>(request.Options.Select(o => o.Id), StringComparer.Ordinal);
            var bargain = BargainUIManager.Instance;
            if (BarterRules.BargainPanelUsable(ui, snap.Client))
                foreach (var (kind, method) in Guards)
                {
                    bool game = (bool)typeof(BargainUIManager).GetMethod(method, I).Invoke(bargain, null);
                    // Threaten's group is shown only once augs are introduced; Can* does not check that.
                    if (kind == BargainKind.Threaten) game &= ui.AugIntroduced;
                    bool core = kind == BargainKind.Blackmail || kind == BargainKind.Threaten
                        ? ids.Contains(OptionIds.Bargain(kind, BarterRules.CoercionPercent))
                        : BarterRules.PricePercents.Any(p => ids.Contains(OptionIds.Bargain(kind, p)));
                    if (game != core) problems.Add($"{kind}: game {method}={game}, offered={core}");
                }

            bool locked = (bool)typeof(NegociationUIManager).GetMethod("IsTradeLocked", S).Invoke(null, null);
            if (gameMode == "Buying" && !locked && !client.isBusinessComplete)
            {
                var sellable = (List<GameItem>)typeof(NegociationUIManager).GetMethod("GetSellableBackInvItems", S).Invoke(null, new object[] { client });
                if ((sellable.Count > 0) != ids.Contains(OptionIds.SellAll)) problems.Add($"sell_all: game has {sellable.Count} sellable, offered={ids.Contains(OptionIds.SellAll)}");
            }
            if (gameMode == "Selling" && !locked && !client.isBusinessComplete && !client.isRegularSellDisable && !client.isMultiBuyDisabled)
            {
                var buyable = (List<GameItem>)typeof(NegociationUIManager).GetMethod("GetNonOwnedBackInvItems", S).Invoke(null, null);
                long sum = buyable.Sum(i => i.GetNegociatedValue());
                bool game = buyable.Count > 0 && sum <= PlayerStore.Instance.playerCash;
                if (game != ids.Contains(OptionIds.BuyAll)) problems.Add($"buy_all: game {buyable.Count} items for {sum} with {PlayerStore.Instance.playerCash}, offered={ids.Contains(OptionIds.BuyAll)}");
            }
            return problems;
        }
    }
}
