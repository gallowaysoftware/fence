using System.Linq;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `scratchers` (gym, default on): the player scratches their tickets before selling. A scratcher (20,
    // MiscItemDirectory) carries three SCRATCHER_STATE slots set at Init (SetRandomSymbols); the scratch UI the
    // player opens is the game's, and headless the executor runs the same path without it: MarkSymbolScratched on
    // each slot (UpdateValue computes the prize into unitValue and tags a winning card), then
    // ScratcherHelper.TryRedeemPrize, which pays the prize (PlayerStore.ModCash) and destroys the card when it is
    // fully scratched and winning. A dud (no winning tag) stays in stock to sell for its 20 base.
    internal static class Scratchers
    {
        public static int Scratched, Redeemed, Winning, Duds;
        public static long Prize;

        public static void Reset() { Scratched = Redeemed = Winning = Duds = 0; Prize = 0; }

        public static bool IsScratcher(GameItem i) => i != null && i.IsTag("SCRATCHER_TAG");

        static System.Collections.Generic.IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(IsScratcher);

        // Scratch every owned card and redeem the winners: free money a player takes before the shutter opens.
        public static void Morning(IExecutorHost ep)
        {
            int cash0 = PlayerStore.Instance.playerCash;
            var cards = Owned().OrderByDescending(PlayerRole.Est).ThenBy(i => i.uniqueId).ToList();
            if (cards.Count == 0) return;
            foreach (var i in cards)
            {
                int value = 0;
                Chore.Act("scratch", i, () =>
                {
                    for (int s = 1; s <= 3; s++) ScratcherHelper.MarkSymbolScratched(i, s);
                    if (!ScratcherHelper.IsFullyScratched(i)) return;
                    Scratched++;
                    if (ScratcherHelper.TryRedeemPrize(i))
                    {
                        Redeemed++;
                        value = (int)i.unitValue;
                        Prize += value;
                        if (value > 0) Winning++;
                    }
                    else Duds++;
                });
                ep.Human(HumanActions.Scratch, null, ep.Uid(i), new JObject
                {
                    ["identifier"] = i.identifier,
                    ["value"] = value,
                    ["dud"] = value == 0,
                }, "scratchers");
            }
            // The scratch summary rides the vending-style human record: one line, the tally and the cash delta.
            if (Scratched > 0)
                ep.Human("scratchers", null, null, new JObject
                {
                    ["scratched"] = Scratched,
                    ["redeemed"] = Redeemed,
                    ["winning"] = Winning,
                    ["duds"] = Duds,
                    ["prize"] = Prize,
                    ["cash_delta"] = PlayerStore.Instance.playerCash - cash0,
                }, "scratchers");
        }
    }
}
