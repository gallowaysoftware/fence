using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Marketplace requests (StoreRequest, the store PC): a faction asks for an item by id and pays 1.2–2x its value
    // plus rep (Upper Levels +2 for a rare item, +5 for charity, before the difficulty factor). `requests: "fill"`
    // buys MARKETPLACE_REQUESTS, holds owned items a live request names, and fills it each morning when the reward
    // covers what it takes at estimate. Nothing is sourced for a request.
    internal static class Requests
    {
        public static int Filled, Credits;

        static bool On => Directives.Current.Raw.Value<string>("requests") == "fill";

        static IEnumerable<StoreRequest> Live() => PlayerStore.Instance.storeRequests?.Where(r => r != null && r.duration > 0) ?? Enumerable.Empty<StoreRequest>();

        public static IEnumerable<string> Upgrades() => On ? new[] { "MARKETPLACE_REQUESTS" } : new string[0];

        public static bool Reserved(GameItem i) => On && Live().Any(r => r.CanThisItemFillRequest(i));

        public static void Morning(IExecutorHost host)
        {
            if (!On) return;
            foreach (var r in Live().ToList())
            {
                if (!r.CanFillRequest()) continue;
                var items = EmporiumEntry.Instance.GetAllOwnedItems().Where(r.CanThisItemFillRequest).Take(r.requiredItemCount).ToList();
                if (r.creditReward < items.Sum(PlayerRole.Est)) continue;
                int cash0 = PlayerStore.Instance.playerCash;
                host.Human("fill_request", null, null, new JObject { ["request"] = r.id, ["item"] = r.requiredItem, ["reward"] = r.creditReward, ["faction"] = r.factionId }, "player:request");
                r.FillRequest();
                Filled++;
                Credits += PlayerStore.Instance.playerCash - cash0;
            }
        }

        public static JObject Status() => Filled == 0 ? null : new JObject { ["filled"] = Filled, ["credits"] = Credits };
    }
}
